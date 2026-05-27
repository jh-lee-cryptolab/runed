// Command runed is the embedding daemon.
//
// Required environment:
//
//	RUNED_LLAMA_SERVER   Path to llama-server binary.
//	RUNED_MODEL          Path to GGUF model file.
//
// Optional environment:
//
//	RUNED_HOME           Data directory (default: $HOME/.runed).
//	RUNED_CTX_SIZE       Max input length in tokens; lower = less KV-cache memory (default: 2048).
//
// The daemon listens on $RUNED_HOME/embedding.sock (UDS) and terminates
// gracefully on SIGINT, SIGTERM, or a Shutdown RPC. Graceful termination
// drains in-flight gRPC calls (10s ceiling) and then stops the llama-server
// child process; the UDS file is auto-unlinked when the listener closes.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	runedv1 "github.com/CryptoLabInc/runed/gen/runed/v1"
	"github.com/CryptoLabInc/runed/internal/backend"
	"github.com/CryptoLabInc/runed/internal/ipc"
	"github.com/CryptoLabInc/runed/internal/server"
	"google.golang.org/grpc"
)

// daemonVersion is set at build time via -ldflags "-X main.daemonVersion=..."
// The default is a development marker so `go run` / un-flagged builds still
// produce a sensible Info.daemon_version.
var daemonVersion = "v0.1.0-alpha"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	llamaBin := os.Getenv("RUNED_LLAMA_SERVER")
	model := os.Getenv("RUNED_MODEL")
	var missing []string
	if llamaBin == "" {
		missing = append(missing, "RUNED_LLAMA_SERVER")
	}
	if model == "" {
		missing = append(missing, "RUNED_MODEL")
	}
	if len(missing) > 0 {
		return fmt.Errorf("required env var(s) not set: %s", strings.Join(missing, ", "))
	}

	home := os.Getenv("RUNED_HOME")
	if home == "" {
		u, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("home dir: %w", err)
		}
		home = filepath.Join(u, ".runed")
	}
	sockPath := filepath.Join(home, "embedding.sock")
	logDir := filepath.Join(home, "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return fmt.Errorf("logs dir: %w", err)
	}

	// ctx-size: RUNED_CTX_SIZE if valid, else backend default (2048).
	ctxSize := 0
	if v := os.Getenv("RUNED_CTX_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			ctxSize = n
		} else {
			logger.Warn("invalid RUNED_CTX_SIZE, using default", "value", v)
		}
	}

	modelID, err := sha256File(model)
	if err != nil {
		return fmt.Errorf("model hash: %w", err)
	}
	logger.Info("model identity", "sha256", modelID, "path", model)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger.Info("starting llama-server", "binary", llamaBin, "model", model)
	b := backend.NewLlamaBackend(backend.Config{
		BinaryPath: llamaBin,
		ModelPath:  model,
		LogPath:    filepath.Join(logDir, "llama-server.log"),
		CtxSize:    ctxSize,
	})
	// NOTE: backend uses exec.CommandContext(ctx, ...) internally, which means
	// the child llama-server dies when this ctx is Done. We therefore pass the
	// long-lived daemon ctx rather than a short-lived timeout wrapper —
	// backend.Start already bounds start-up on its own (~15s health poll +
	// early-exit detection via the stderr scanner).
	if err := b.Start(ctx); err != nil {
		return fmt.Errorf("backend start: %w", err)
	}
	logger.Info("llama-server ready", "port", b.Port())

	lis, err := ipc.Listen(sockPath)
	if err != nil {
		// Best-effort backend cleanup — we have no logger-side context to thread here.
		_ = b.Stop(context.Background())
		return fmt.Errorf("ipc listen: %w", err)
	}
	logger.Info("listening", "socket", sockPath)

	srv := server.New(b, daemonVersion, modelID)
	gs := grpc.NewServer(grpc.UnaryInterceptor(srv.UnaryActivityInterceptor()))
	runedv1.RegisterRunedServiceServer(gs, srv)

	// Serve in a goroutine so main can block on signals/Shutdown/serve error.
	// gs.Serve returns nil on graceful stop; any non-nil err is a real fault.
	serveErr := make(chan error, 1)
	go func() {
		if err := gs.Serve(lis); err != nil {
			serveErr <- err
		}
		close(serveErr)
	}()

	idleTimeout, err := parseIdleTimeout()
	if err != nil {
		_ = b.Stop(context.Background())
		return fmt.Errorf("RUNED_IDLE_TIMEOUT: %w", err)
	}
	if idleTimeout > 0 {
		logger.Info("idle exit enabled", "timeout", idleTimeout.String())
		go func() {
			// Tick at 30s; idle-exit latency is therefore RUNED_IDLE_TIMEOUT + up to 30s.
			// Cadence is chosen to keep wake-up overhead negligible on long-idle daemons;
			// values < 30s of RUNED_IDLE_TIMEOUT will still see ~30s of post-idle slack.
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					elapsed := time.Since(srv.LastActivity())
					if elapsed > idleTimeout {
						logger.Info("idle timeout reached, triggering shutdown",
							"elapsed", elapsed.String())
						srv.TriggerShutdown()
						return
					}
				}
			}
		}()
	} else {
		logger.Info("idle exit disabled (RUNED_IDLE_TIMEOUT=0)")
	}

	sigCh := make(chan os.Signal, 1)
	// SIGHUP is included so closing the controlling terminal triggers the
	// graceful shutdown path; otherwise Go's default SIGHUP handler kills
	// runed without running b.Stop(), orphaning the llama-server child.
	// SIGKILL cannot be intercepted from user space.
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	select {
	case s := <-sigCh:
		logger.Info("received signal", "signal", s.String())
	case <-srv.ShutdownCh():
		logger.Info("received Shutdown RPC")
	case err := <-serveErr:
		if err != nil {
			logger.Error("gRPC serve error", "err", err)
			// Still fall through to backend cleanup below.
			_ = b.Stop(context.Background())
			return fmt.Errorf("serve: %w", err)
		}
	}

	// Phase 1: drain in-flight RPCs. GracefulStop blocks until all active
	// handlers return; 10s is a safety net for a wedged client.
	logger.Info("draining in-flight requests")
	graceDone := make(chan struct{})
	go func() {
		gs.GracefulStop()
		close(graceDone)
	}()
	select {
	case <-graceDone:
		logger.Info("graceful stop complete")
	case <-time.After(10 * time.Second):
		logger.Warn("graceful stop timed out, forcing")
		gs.Stop()
		<-graceDone
	}

	// Phase 2: stop backend. Deferred until after GracefulStop because Embed
	// handlers may still be waiting on backend HTTP calls during drain.
	if err := b.Stop(context.Background()); err != nil {
		logger.Warn("backend stop returned error", "err", err)
	}
	logger.Info("shutdown complete")
	return nil
}

// sha256File returns a short, prefixed SHA-256 identifier of the file at path.
// The 16-char hex truncation keeps Info.model_identity compact while retaining
// enough entropy to distinguish GGUF revisions in practice (Plan A ships a
// single model; this mostly guards against silent file swaps).
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))[:16], nil
}

// parseIdleTimeout reads RUNED_IDLE_TIMEOUT and returns the parsed duration.
// Empty or unset env returns the default 10 minutes. A value of "0" disables
// idle exit entirely (preserves Plan A semantics). Invalid values return an
// error so the daemon refuses to start with a misconfigured timeout.
func parseIdleTimeout() (time.Duration, error) {
	raw := os.Getenv("RUNED_IDLE_TIMEOUT")
	if raw == "" {
		return 10 * time.Minute, nil
	}
	return time.ParseDuration(raw)
}
