//go:build !windows

package client

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestIntegration_AutoSpawn_IdleExit_Respawn runs the full Plan B flow:
// cold start (no daemon, no socket) → Connect auto-spawns → Embed →
// wait past idle_timeout + ticker cadence → daemon exits on its own →
// Connect again → auto-spawn fires a second time → Embed succeeds.
//
// Requires:
//
//	RUNED_TEST_LLAMA_SERVER  path to llama-server binary
//	RUNED_TEST_GGUF          path to GGUF model
//	RUNED_TEST_RUNED         path to a runed binary built with the
//	                         changes in this branch
//
// Skips otherwise so plain `make test` keeps passing without a model on disk.
func TestIntegration_AutoSpawn_IdleExit_Respawn(t *testing.T) {
	llamaBin := os.Getenv("RUNED_TEST_LLAMA_SERVER")
	model := os.Getenv("RUNED_TEST_GGUF")
	runedBin := os.Getenv("RUNED_TEST_RUNED")
	if llamaBin == "" || model == "" || runedBin == "" {
		t.Skip("set RUNED_TEST_LLAMA_SERVER, RUNED_TEST_GGUF, RUNED_TEST_RUNED to run")
	}

	// shortTempDir keeps the socket path under /tmp so we stay below the
	// 104-byte sockaddr_un limit on macOS, where t.TempDir() returns paths
	// under /var/folders/... that routinely overflow and surface as a
	// confusing `bind: invalid argument` in the daemon log.
	home := shortTempDir(t)
	cfgPath := filepath.Join(home, "config.json")
	cfg := fmt.Sprintf(`{
        "version": 1,
        "llama_server": %q,
        "model": %q,
        "runed_binary": %q,
        "idle_timeout": "3s"
    }`, llamaBin, model, runedBin)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("RUNED_CONFIG", cfgPath)
	t.Setenv("RUNED_HOME", home)

	sock := filepath.Join(home, "embedding.sock")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Phase 1: cold start — no daemon, no socket. Connect must auto-spawn.
	c, err := Connect(ctx, WithSocketPath(sock))
	if err != nil {
		t.Fatalf("phase 1 Connect (cold): %v", err)
	}
	vec, err := c.Embed(ctx, "hello plan B")
	if err != nil {
		t.Fatalf("phase 1 Embed: %v", err)
	}
	if len(vec) != 1024 {
		t.Fatalf("phase 1 vector dim = %d; want 1024", len(vec))
	}
	// Intentionally do NOT close c here — Phase 3a re-uses it with a
	// now-stale gRPC connection to exercise the T9 retry-on-Unavailable path.

	// Phase 2: wait past idle_timeout + 30s ticker cadence. The daemon's
	// idle ticker fires at most every 30s, so even with a 3s timeout the
	// earliest exit is ~30s after the last RPC. 35s gives a small buffer.
	t.Logf("waiting 35s for daemon to idle-exit...")
	time.Sleep(35 * time.Second)
	if probeUDS(sock) {
		t.Fatal("daemon still reachable after idle timeout; idle exit did not fire")
	}

	// Phase 3a: T9 retry-on-Unavailable. Re-use the Phase 1 client whose
	// gRPC connection is now stale (daemon idle-exited). The Embed call
	// should hit Unavailable, auto-trigger spawn.EnsureDaemon, reconnect,
	// and retry — surfacing success to the caller.
	vec2, err := c.Embed(ctx, "hello again")
	if err != nil {
		t.Fatalf("phase 3a Embed (retry path): %v", err)
	}
	if len(vec2) != 1024 {
		t.Fatalf("phase 3a vector dim = %d; want 1024", len(vec2))
	}
	c.Close()

	// Phase 3b: T8 fresh-Connect auto-spawn. After waiting for the just-
	// respawned daemon to idle out again would be too slow, so we just
	// re-validate that a fresh Connect against the (live) daemon also
	// returns a working Client.
	c3, err := Connect(ctx, WithSocketPath(sock))
	if err != nil {
		t.Fatalf("phase 3b Connect: %v", err)
	}
	vec3, err := c3.Embed(ctx, "hello once more")
	if err != nil {
		t.Fatalf("phase 3b Embed: %v", err)
	}
	if len(vec3) != 1024 {
		t.Fatalf("phase 3b vector dim = %d; want 1024", len(vec3))
	}
	c3.Close()
}

// probeUDS reports whether a unix socket at path accepts connections.
// Pure-Go net.DialTimeout instead of shelling out to `nc -zU`: same
// signal, no dependency on netcat being present, no portability surprises
// between BSD nc (macOS) and GNU nc (Linux CI).
func probeUDS(path string) bool {
	conn, err := net.DialTimeout("unix", path, 100*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
