//go:build windows

package main

import "testing"

// An in-place update must write the command line a fresh install would,
// spaces in the install and config paths included.
func TestServiceCommandLine(t *testing.T) {
	got := serviceCommandLine(`C:\Program Files\BRUV\bruv.exe`, []string{"--server", "--addr", "0.0.0.0:9870", "--config", `C:\ProgramData\BRUV`})
	want := `"C:\Program Files\BRUV\bruv.exe" --server --addr 0.0.0.0:9870 --config C:\ProgramData\BRUV`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}
