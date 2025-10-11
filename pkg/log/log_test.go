package log

import (
	"bytes"
	"strings"
	"testing"
)

// helper resets output and returns buffer and logger
func newTestLogger(t *testing.T, name string) (*Logger, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	SetOutput(buf)
	return ForService(name), buf
}

func TestPrefixInfo(t *testing.T) {
	SetGlobalDebug(false)

	const name = "prefix_service_test"
	l, buf := newTestLogger(t, name)

	l.Infof("hello world")
	out := buf.String()

	if !strings.Contains(out, "["+name+">]") {
		t.Fatalf("expected prefix [%s>] in output, got: %q", name, out)
	}
	if !strings.Contains(out, "hello world") {
		t.Fatalf("expected message in output, got: %q", out)
	}
}

func TestDebugPerService(t *testing.T) {
	SetGlobalDebug(false)

	const name = "debug_service_specific"
	DisableDebugFor(name) // ensure clean state
	l, buf := newTestLogger(t, name)

	l.Debugf("should not appear")
	if strings.Contains(buf.String(), "should not appear") {
		t.Fatalf("debug message appeared while debug disabled (per service & global)")
	}

	EnableDebugFor(name)
	l.Debugf("visible now")
	if !strings.Contains(buf.String(), "visible now") {
		t.Fatalf("expected debug message after enabling per-service debug; got: %q", buf.String())
	}
}

func TestDebugGlobal(t *testing.T) {
	SetGlobalDebug(false)

	const name = "debug_service_global"
	DisableDebugFor(name)
	l, buf := newTestLogger(t, name)

	l.Debugf("hidden")
	if strings.Contains(buf.String(), "hidden") {
		t.Fatalf("debug message appeared while global debug disabled")
	}

	SetGlobalDebug(true)
	defer SetGlobalDebug(false) // cleanup for other tests

	l.Debugf("global visible")
	if !strings.Contains(buf.String(), "global visible") {
		t.Fatalf("expected debug message after enabling global debug; got: %q", buf.String())
	}
}

func TestWarnIncludesPrefix(t *testing.T) {
	SetGlobalDebug(false)

	const name = "warn_service_test"
	l, buf := newTestLogger(t, name)

	l.Warnf("attention needed")
	out := buf.String()

	// Warn emits a one-time "warnings active" line first; we only ensure prefix & message appear
	if !strings.Contains(out, "["+name+">]") {
		t.Fatalf("expected prefix [%s>] in warn output, got: %q", name, out)
	}
	if !strings.Contains(out, "attention needed") {
		t.Fatalf("expected warn message in output, got: %q", out)
	}
}
