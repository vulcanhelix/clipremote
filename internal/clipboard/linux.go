//go:build linux

package clipboard

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// SetImage puts PNG (or other image) bytes on the Linux clipboard.
func SetImage(data []byte, opt SetOptions) error {
	display := opt.Display
	if display == "" {
		display = os.Getenv("DISPLAY")
	}
	wayland := os.Getenv("WAYLAND_DISPLAY")

	// Prefer wl-copy on Wayland
	if wayland != "" {
		if path, err := exec.LookPath("wl-copy"); err == nil {
			cmd := exec.Command(path, "--type", "image/png")
			cmd.Stdin = bytes.NewReader(data)
			cmd.Env = os.Environ()
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("wl-copy: %w (%s)", err, strings.TrimSpace(string(out)))
			}
			return nil
		}
	}

	// xclip
	if path, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.Command(path, "-selection", "clipboard", "-t", "image/png", "-i")
		cmd.Stdin = bytes.NewReader(data)
		env := os.Environ()
		if display != "" {
			env = append(env, "DISPLAY="+display)
		}
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("xclip: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	// xsel does not handle image types well; skip
	return fmt.Errorf("no clipboard tool found (install wl-clipboard or xclip; set DISPLAY or WAYLAND_DISPLAY)")
}

// ReadImage tries to read an image from the Linux clipboard (local desktop use).
func ReadImage() (Image, error) {
	wayland := os.Getenv("WAYLAND_DISPLAY")
	if wayland != "" {
		if path, err := exec.LookPath("wl-paste"); err == nil {
			cmd := exec.Command(path, "--type", "image/png")
			out, err := cmd.Output()
			if err == nil && len(out) > 0 {
				return Image{PNG: out}, nil
			}
		}
	}
	if path, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.Command(path, "-selection", "clipboard", "-t", "image/png", "-o")
		if d := os.Getenv("DISPLAY"); d != "" {
			cmd.Env = append(os.Environ(), "DISPLAY="+d)
		}
		out, err := cmd.Output()
		if err == nil && len(out) > 0 {
			return Image{PNG: out}, nil
		}
	}
	return Image{}, fmt.Errorf("no image on clipboard (or no wl-paste/xclip)")
}

// Available reports whether a clipboard write path exists.
func Available() (string, bool) {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if _, err := exec.LookPath("wl-copy"); err == nil {
			return "wl-copy", true
		}
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		if os.Getenv("DISPLAY") != "" {
			return "xclip", true
		}
		return "xclip (no DISPLAY)", false
	}
	return "none", false
}
