package mcp

import (
	"context"
	"testing"
	"time"
)

// testContext returns a context with a short timeout that expires
// well within the test runner's overall deadline. Used by tests that
// involve LoadAndStart or other context-aware operations.
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
