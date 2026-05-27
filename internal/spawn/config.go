// Package spawn handles auto-spawning of the runed daemon from the client.
// It reads ~/.runed/config.json for the paths to llama-server, the GGUF
// model file, and (optionally) the runed binary itself, then forks a
// detached daemon and waits for it to come up.
//
// The package is unix-only for Plan B. Windows support is deferred.
package spawn

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// ConfigVersion is the only supported value of Config.Version.
const ConfigVersion = 1

// Config is the parsed shape of ~/.runed/config.json. All file paths must
// be absolute or resolvable when the daemon is spawned.
type Config struct {
	Version     int    `json:"version"`
	LlamaServer string `json:"llama_server"`
	Model       string `json:"model"`
	RunedBinary string `json:"runed_binary,omitempty"`
	IdleTimeout string `json:"idle_timeout,omitempty"`
}

// LoadConfig reads, parses, and validates the config file. The path comes
// from $RUNED_CONFIG if set, otherwise from $RUNED_HOME/config.json,
// otherwise from ~/.runed/config.json.
func LoadConfig() (*Config, error) {
	path, err := resolveConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config not found at %s: run installation first", path)
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("config parse: %w", err)
	}
	if cfg.Version != ConfigVersion {
		return nil, fmt.Errorf("config version %d not supported (need %d)",
			cfg.Version, ConfigVersion)
	}
	if cfg.RunedBinary == "" {
		if self, err := os.Executable(); err == nil {
			cfg.RunedBinary = self
		} else if p, err := exec.LookPath("runed"); err == nil {
			cfg.RunedBinary = p
		} else {
			return nil, fmt.Errorf("runed_binary not set in config and cannot resolve self or PATH: %w", err)
		}
	}
	if err := validateExecutable(cfg.LlamaServer, "llama_server"); err != nil {
		return nil, err
	}
	if err := validateExecutable(cfg.RunedBinary, "runed_binary"); err != nil {
		return nil, err
	}
	if err := validateGGUF(cfg.Model); err != nil {
		return nil, err
	}
	// Canonicalize to absolute paths so the detached daemon child (T7) sees the
	// same files regardless of the caller's working directory.
	if cfg.LlamaServer, err = filepath.Abs(cfg.LlamaServer); err != nil {
		return nil, fmt.Errorf("abs llama_server: %w", err)
	}
	if cfg.Model, err = filepath.Abs(cfg.Model); err != nil {
		return nil, fmt.Errorf("abs model: %w", err)
	}
	if cfg.RunedBinary, err = filepath.Abs(cfg.RunedBinary); err != nil {
		return nil, fmt.Errorf("abs runed_binary: %w", err)
	}
	return &cfg, nil
}

func resolveConfigPath() (string, error) {
	if p := os.Getenv("RUNED_CONFIG"); p != "" {
		return p, nil
	}
	if h := os.Getenv("RUNED_HOME"); h != "" {
		return filepath.Join(h, "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home: %w", err)
	}
	return filepath.Join(home, ".runed", "config.json"), nil
}

func validateExecutable(path, label string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s %q: %w", label, path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s %q: is a directory", label, path)
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("%s %q: not executable", label, path)
	}
	return nil
}

func validateGGUF(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("model %q: %w", path, err)
	}
	defer f.Close()
	var magic [4]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return fmt.Errorf("model %q: read magic: %w", path, err)
	}
	if string(magic[:]) != "GGUF" {
		return fmt.Errorf("model %q: not a GGUF file (magic=%x)", path, magic)
	}
	return nil
}
