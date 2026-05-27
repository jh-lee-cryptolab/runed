package spawn

import (
	"net"
	"os"
	"testing"
)

func startFakeListener(t *testing.T, path string) func() {
	t.Helper()
	lis, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("bind unix %s: %v", path, err)
	}
	go func() {
		for {
			c, err := lis.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	return func() { lis.Close() }
}

// shortTempDir returns a per-test temp dir under /tmp. macOS's $TMPDIR and
// Go's t.TempDir() produce paths that can exceed the 104-byte sockaddr_un
// limit, causing bind EINVAL for unrelated reasons. /tmp keeps paths short.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "runed-spawn-")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}
