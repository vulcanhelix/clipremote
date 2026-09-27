//go:build linux

package clipboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeTool installs an executable shell script named name at the front of PATH.
func fakeTool(t *testing.T, name, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// forkingTool mimics xclip -i / wl-copy: consume stdin, leave a background child
// holding the inherited stdout/stderr (to serve the selection), and exit 0.
const forkingTool = "cat >/dev/null\n(sleep 30) &\nexit 0\n"

func setImageWithin(t *testing.T, d time.Duration) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- SetImage([]byte("png"), SetOptions{Display: ":99"}) }()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		t.Fatalf("SetImage did not return within %s (waiting on forked clipboard owner)", d)
		return nil
	}
}

func TestSetImageXclipForkDoesNotBlock(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	fakeTool(t, "xclip", forkingTool)
	if err := setImageWithin(t, 5*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestSetImageWlCopyForkDoesNotBlock(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	fakeTool(t, "wl-copy", forkingTool)
	if err := setImageWithin(t, 5*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestSetImageReportsToolError(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	fakeTool(t, "xclip", "echo \"Error: Can't open display: (null)\" >&2\nexit 1\n")
	err := setImageWithin(t, 5*time.Second)
	if err == nil || !strings.Contains(err.Error(), "Can't open display") {
		t.Fatalf("want xclip stderr in error, got %v", err)
	}
}
