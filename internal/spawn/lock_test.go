package spawn

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcquireLock_SequentialOK(t *testing.T) {
	p := filepath.Join(t.TempDir(), "lock")
	l1, err := acquireLock(p, time.Second)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	l1.Release()
	l2, err := acquireLock(p, time.Second)
	if err != nil {
		t.Fatalf("second acquire after release: %v", err)
	}
	l2.Release()
}

func TestAcquireLock_ConcurrentSerializes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "lock")
	var holding atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l, err := acquireLock(p, 5*time.Second)
			if err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			// Only one goroutine may be inside the critical section.
			n := holding.Add(1)
			if n != 1 {
				t.Errorf("two holders simultaneously: %d", n)
			}
			time.Sleep(20 * time.Millisecond)
			holding.Add(-1)
			l.Release()
		}()
	}
	wg.Wait()
}

func TestAcquireLock_Timeout(t *testing.T) {
	p := filepath.Join(t.TempDir(), "lock")
	held, err := acquireLock(p, time.Second)
	if err != nil {
		t.Fatalf("initial: %v", err)
	}
	defer held.Release()
	// Second acquire must time out.
	start := time.Now()
	if _, err := acquireLock(p, 300*time.Millisecond); err == nil {
		t.Fatal("expected timeout, got nil")
	}
	elapsed := time.Since(start)
	if elapsed < 250*time.Millisecond {
		t.Errorf("returned too fast (%v); expected ≥300ms", elapsed)
	}
}
