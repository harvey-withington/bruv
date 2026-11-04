//go:build !windows

package uncliengine

// serviceAccount is Windows-only; UNCLI is too.
func serviceAccount() string { return "" }
