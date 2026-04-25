package notify

import (
	"testing"
)

// TestServiceSmokes exercises every method through the stateless
// service. internal/config is the source of truth for persistence;
// this test confirms the service wires up without panicking.
func TestServiceSmokes(t *testing.T) {
	// Config package reads from os.UserConfigDir which may or may not
	// exist in a test environment — accept either a successful read or
	// a filesystem-layer error, but never a panic.
	svc := New()

	if _, err := svc.GetConfig(); err != nil {
		t.Logf("GetConfig returned error (acceptable in CI): %v", err)
	}
	if _, err := svc.List(); err != nil {
		t.Logf("List returned error (acceptable in CI): %v", err)
	}
}
