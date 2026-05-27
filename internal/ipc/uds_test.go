//go:build !windows

package ipc

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shortTempDir returns a per-test temp dir under /tmp. macOS's $TMPDIR and
// Go's t.TempDir() produce paths that can exceed the 104-byte sockaddr_un
// limit, causing bind EINVAL for unrelated reasons. /tmp keeps paths short.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "runed-ipc-")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestListen_CreatesSocketWith0700(t *testing.T) {
	dir := shortTempDir(t)
	sockPath := filepath.Join(dir, "embedding.sock")

	lis, err := Listen(sockPath)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer lis.Close()

	info, err := os.Stat(sockPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode()&0o777 != 0o700 {
		t.Fatalf("want mode 0700, got %o", info.Mode()&0o777)
	}
}

func TestListen_CleansUpStaleSocket(t *testing.T) {
	dir := shortTempDir(t)
	sockPath := filepath.Join(dir, "embedding.sock")

	// Create a stale file manually (no listener).
	f, _ := os.Create(sockPath)
	f.Close()

	// Listen should remove and rebind.
	lis, err := Listen(sockPath)
	if err != nil {
		t.Fatalf("Listen over stale: %v", err)
	}
	defer lis.Close()

	_, ok := lis.(*net.UnixListener)
	if !ok {
		t.Fatalf("expected UnixListener")
	}
}

func TestListen_RejectsLiveSocket(t *testing.T) {
	dir := shortTempDir(t)
	sockPath := filepath.Join(dir, "embedding.sock")

	l1, err := Listen(sockPath)
	if err != nil {
		t.Fatalf("first Listen: %v", err)
	}
	defer l1.Close()

	_, err = Listen(sockPath)
	if err == nil {
		t.Fatal("expected error on second Listen, got nil")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("want 'already in use' error, got: %v", err)
	}
}
