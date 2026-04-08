package envx

import (
	"testing"
	"time"
)

func TestString(t *testing.T) {
	t.Setenv("ENVX_TEST_STRING", "  value  ")
	if got := String("ENVX_TEST_STRING", "fallback"); got != "value" {
		t.Fatalf("expected trimmed value, got %q", got)
	}
	if got := String("ENVX_TEST_MISSING", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}
}

func TestBool(t *testing.T) {
	t.Setenv("ENVX_TEST_BOOL", "yes")
	if !Bool("ENVX_TEST_BOOL", false) {
		t.Fatal("expected true")
	}
	t.Setenv("ENVX_TEST_BOOL", "off")
	if Bool("ENVX_TEST_BOOL", true) {
		t.Fatal("expected false")
	}
	t.Setenv("ENVX_TEST_BOOL", "invalid")
	if !Bool("ENVX_TEST_BOOL", true) {
		t.Fatal("expected fallback true")
	}
}

func TestIntAndDuration(t *testing.T) {
	t.Setenv("ENVX_TEST_INT", "42")
	if got := Int("ENVX_TEST_INT", 1); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
	t.Setenv("ENVX_TEST_DURATION", "15s")
	if got := Duration("ENVX_TEST_DURATION", time.Second); got != 15*time.Second {
		t.Fatalf("expected 15s, got %s", got)
	}
}
