//go:build darwin

package clipboard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SetImage on macOS (local laptop writing clipboard — rarely needed for our push model).
func SetImage(data []byte, opt SetOptions) error {
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

// ReadImage reads an image from the macOS pasteboard (PNG/TIFF/JPEG/file URL).
func ReadImage() (Image, error) {
	var errs []string

	// 1) pngpaste — check PATH and common Homebrew locations
	for _, bin := range pngpasteBins() {
		cmd := exec.Command(bin, "-")
		out, err := cmd.CombinedOutput()
		if err == nil && looksLikeImageBytes(out) {
			return Image{PNG: out}, nil
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("pngpaste(%s): %s", bin, strings.TrimSpace(string(out))))
		}
	}

	// 2) AppKit: PNG / TIFF / JPEG pasteboard types → write temp file
	if data, err := readViaAppKit(); err == nil && looksLikeImageBytes(data) {
		return Image{PNG: normalizeToPNG(data)}, nil
	} else if err != nil {
		errs = append(errs, "appkit: "+err.Error())
	}

	// 3) File URL / path on clipboard (CleanShot "copy file", Finder copy)
	if data, err := readViaFileURL(); err == nil && looksLikeImageBytes(data) {
		return Image{PNG: normalizeToPNG(data)}, nil
	} else if err != nil {
		errs = append(errs, "file-url: "+err.Error())
	}

	// 4) Legacy PNGf class (often fails for screenshots — last resort)
	if data, err := readViaPNGfClass(); err == nil && looksLikeImageBytes(data) {
		return Image{PNG: data}, nil
	} else if err != nil {
		errs = append(errs, "pngf: "+err.Error())
	}

	return Image{}, fmt.Errorf("no image on clipboard (%s)", strings.Join(errs, "; "))
}

func pngpasteBins() []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		if _, err := os.Stat(p); err != nil {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	if p, err := exec.LookPath("pngpaste"); err == nil {
		add(p)
	}
	add("/usr/local/bin/pngpaste")
	add("/opt/homebrew/bin/pngpaste")
	return out
}

func readViaAppKit() ([]byte, error) {
	tmp, err := os.CreateTemp("", "clipremote-pb-*.bin")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	tmp.Close()
	defer os.Remove(path)

	// Write raw pasteboard image bytes (PNG preferred, else TIFF).
	script := fmt.Sprintf(`
use framework "AppKit"
use framework "Foundation"
use scripting additions
set pb to current application's NSPasteboard's generalPasteboard()
set types to {current application's NSPasteboardTypePNG, current application's NSPasteboardTypeTIFF, "public.jpeg", "public.png", "public.tiff"}
set imgData to missing value
repeat with t in types
  set imgData to pb's dataForType:t
  if imgData is not missing value then exit repeat
end repeat
if imgData is missing value then error "no image types on pasteboard"
set ok to imgData's writeToFile:"%s" atomically:true
if ok is false then error "failed writing pasteboard data"
`, path)
	cmd := exec.Command("osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return os.ReadFile(path)
}

func readViaFileURL() ([]byte, error) {
	// Prefer AppKit file URL, then plain string path.
	script := `
use framework "AppKit"
use framework "Foundation"
use scripting additions
set pb to current application's NSPasteboard's generalPasteboard()
set urls to pb's readObjectsForClasses:{current application's NSURL} options:(current application's NSDictionary's dictionary())
if urls is missing value or (count of urls) is 0 then
  try
    return POSIX path of (the clipboard as «class furl»)
  on error
    error "no file on clipboard"
  end try
else
  set u to item 1 of urls
  return (u's |path|() as text)
end if
`
	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return nil, fmt.Errorf("empty path")
	}
	// Only accept common image extensions
	ext := strings.ToLower(filepath.Ext(p))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".tif", ".tiff", ".bmp", ".heic":
	default:
		return nil, fmt.Errorf("clipboard file is not an image: %s", p)
	}
	return os.ReadFile(p)
}

func readViaPNGfClass() ([]byte, error) {
	tmp, err := os.CreateTemp("", "clipremote-read-*.png")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	tmp.Close()
	defer os.Remove(path)

	script := fmt.Sprintf(`
try
  set png_data to the clipboard as «class PNGf»
  set f to open for access POSIX file "%s" with write permission
  set eof f to 0
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
		return nil, fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return os.ReadFile(path)
}

// normalizeToPNG converts TIFF/JPEG/etc to PNG via sips when needed.
func normalizeToPNG(data []byte) []byte {
	if len(data) >= 8 && data[0] == 0x89 && data[1] == 'P' {
		return data
	}
	in, err := os.CreateTemp("", "clipremote-in-*")
	if err != nil {
		return data
	}
	inPath := in.Name()
	defer os.Remove(inPath)
	if _, err := in.Write(data); err != nil {
		in.Close()
		return data
	}
	in.Close()

	outPath := inPath + ".png"
	defer os.Remove(outPath)
	cmd := exec.Command("sips", "-s", "format", "png", inPath, "--out", outPath)
	if err := cmd.Run(); err != nil {
		return data
	}
	out, err := os.ReadFile(outPath)
	if err != nil || len(out) == 0 {
		return data
	}
	return out
}

func looksLikeImageBytes(b []byte) bool {
	if len(b) < 8 {
		return false
	}
	if b[0] == 0x89 && b[1] == 'P' && b[2] == 'N' && b[3] == 'G' {
		return true
	}
	if b[0] == 0xff && b[1] == 0xd8 {
		return true
	}
	if string(b[:6]) == "GIF87a" || string(b[:6]) == "GIF89a" {
		return true
	}
	if len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP" {
		return true
	}
	// TIFF
	if (b[0] == 'I' && b[1] == 'I') || (b[0] == 'M' && b[1] == 'M') {
		return true
	}
	return false
}

// Available reports clipboard read capability on macOS.
func Available() (string, bool) {
	if bins := pngpasteBins(); len(bins) > 0 {
		return "pngpaste", true
	}
	if _, err := exec.LookPath("osascript"); err == nil {
		return "osascript", true
	}
	return "none", false
}

func changeCount() (int, error) {
	cmd := exec.Command("osascript", "-e", `
use framework "AppKit"
set pb to current application's NSPasteboard's generalPasteboard()
return pb's changeCount() as integer
`)
	out, err := cmd.Output()
	if err != nil {
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
