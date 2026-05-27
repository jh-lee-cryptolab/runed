package main

import (
	"os"
	"testing"
	"time"
)

func TestParseIdleTimeout_Default(t *testing.T) {
	t.Setenv("RUNED_IDLE_TIMEOUT", "") // snapshot for restoration
	os.Unsetenv("RUNED_IDLE_TIMEOUT")  // empty != unset; force unset for the function-under-test
	got, err := parseIdleTimeout()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 10*time.Minute {
		t.Fatalf("default = %v; want 10m", got)
	}
}

func TestParseIdleTimeout_ValidOverride(t *testing.T) {
	t.Setenv("RUNED_IDLE_TIMEOUT", "30s")
	got, err := parseIdleTimeout()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 30*time.Second {
		t.Fatalf("got = %v; want 30s", got)
	}
}

func TestParseIdleTimeout_Zero(t *testing.T) {
	t.Setenv("RUNED_IDLE_TIMEOUT", "0")
	got, err := parseIdleTimeout()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Fatalf("got = %v; want 0", got)
	}
}

func TestParseIdleTimeout_Invalid(t *testing.T) {
	t.Setenv("RUNED_IDLE_TIMEOUT", "not-a-duration")
	if _, err := parseIdleTimeout(); err == nil {
		t.Fatal("expected error for invalid duration, got nil")
	}
}
