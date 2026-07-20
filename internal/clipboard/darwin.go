//go:build darwin

package clipboard

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// SetImage on macOS (local laptop writing clipboard — rarely needed for our push model).
func SetImage(data []byte, opt SetOptions) error {
	// Use Swift/osascript via a temp file + pbcopy doesn't support images well.
	// pngpaste is read-only; use osascript with temporary file.
	tmp, err := os.CreateTemp("", "clipremote-*.png")
	if err != nil {
		return err
	}
	path := tmp.Name()
	defer os.Remove(path)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	script := fmt.Sprintf(`set the clipboard to (read (POSIX file "%s") as «class PNGf»)`, path)
	cmd := exec.Command("osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("osascript: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ReadImage reads PNG from the macOS pasteboard.
func ReadImage() (Image, error) {
	// Prefer pngpaste when available (fast, reliable).
	if path, err := exec.LookPath("pngpaste"); err == nil {
		cmd := exec.Command(path, "-")
		out, err := cmd.Output()
		if err == nil && len(out) > 0 {
			return Image{PNG: out}, nil
		}
	}

	// Fallback: osascript → temp file
	tmp, err := os.CreateTemp("", "clipremote-read-*.png")
	if err != nil {
		return Image{}, err
	}
	path := tmp.Name()
	tmp.Close()
	defer os.Remove(path)

	script := fmt.Sprintf(`
try
  set png_data to the clipboard as «class PNGf»
  set f to open for access POSIX file "%s" with write permission
  write png_data to f
  close access f
on error errMsg
  try
    close access POSIX file "%s"
  end try
  error errMsg
end try
`, path, path)
	cmd := exec.Command("osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return Image{}, fmt.Errorf("no image on clipboard: %s", strings.TrimSpace(string(out)))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Image{}, err
	}
	if len(data) == 0 {
		return Image{}, fmt.Errorf("empty clipboard image")
	}
	return Image{PNG: data}, nil
}

// Available reports clipboard read capability on macOS.
func Available() (string, bool) {
	if _, err := exec.LookPath("pngpaste"); err == nil {
		return "pngpaste", true
	}
	if _, err := exec.LookPath("osascript"); err == nil {
		return "osascript", true
	}
	return "none", false
}

// changeCount returns a pasteboard change indicator for polling.
func changeCount() (int, error) {
	cmd := exec.Command("osascript", "-e", `get (the clipboard as string)`)
	// That forces string; better use Cocoa via python or swift.
	// Use `pbpaste` change is hard — use osascript:
	cmd = exec.Command("osascript", "-e", `
use framework "AppKit"
set pb to current application's NSPasteboard's generalPasteboard()
return pb's changeCount() as integer
`)
	out, err := cmd.Output()
	if err != nil {
		// fallback: hash of image/text
		return 0, err
	}
	var n int
	_, err = fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &n)
	return n, err
}

// WatchChangeCount is used by the daemon poller.
func WatchChangeCount() (int, error) {
	return changeCount()
}

// Silence unused import on some paths
var _ = bytes.NewReader
