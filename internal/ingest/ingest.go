package ingest

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/vulcanhelix/clipremote/internal/clipboard"
	"github.com/vulcanhelix/clipremote/internal/paths"
)

// Result is returned after a successful ingest.
type Result struct {
	Path      string    `json:"path"`
	Bytes     int       `json:"bytes"`
	Clipboard string    `json:"clipboard,omitempty"` // "ok", "skipped", "error: ..."
	At        time.Time `json:"at"`
}

// Status is persisted for doctor.
type Status struct {
	Last Result `json:"last"`
}

// Options controls ingest behavior.
type Options struct {
	HistoryN      int
	SetClipboard  bool
	Display       string // optional DISPLAY for xclip
}

// FromReader writes PNG bytes from r to cache, updates history, optional clipboard.
func FromReader(r io.Reader, opt Options) (Result, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Result{}, fmt.Errorf("read image: %w", err)
	}
	if len(data) == 0 {
		return Result{}, fmt.Errorf("empty image data")
	}
	if !looksLikeImage(data) {
		return Result{}, fmt.Errorf("input does not look like PNG/JPEG/GIF/WebP (got %d bytes)", len(data))
	}

	// Normalize: if JPEG/etc, still store as-is but name .png for simplicity only if PNG.
	// Prefer keeping original bytes; use .png extension when PNG magic, else detect.
	ext := detectExt(data)
	latestName := "latest" + ext

	cache, err := paths.CacheDir()
	if err != nil {
		return Result{}, err
	}
	if err := paths.EnsureDir(cache); err != nil {
		return Result{}, err
	}

	// Always also maintain latest.png path for stable @ attachment when PNG;
	// for other formats write latest.<ext> and symlink/copy to a well-known path.
	latest := filepath.Join(cache, latestName)
	stablePNG := filepath.Join(cache, paths.LatestFileName)

	if err := os.WriteFile(latest, data, 0o644); err != nil {
		return Result{}, fmt.Errorf("write latest: %w", err)
	}
	// Keep stable latest.png for tools that hardcode it.
	if latest != stablePNG {
		_ = os.WriteFile(stablePNG, data, 0o644)
	} else {
		stablePNG = latest
	}

	// History
	histN := opt.HistoryN
	if histN <= 0 {
		histN = paths.DefaultHistory
	}
	histDir, err := paths.HistoryDir()
	if err != nil {
		return Result{}, err
	}
	if err := paths.EnsureDir(histDir); err != nil {
		return Result{}, err
	}
	stamp := time.Now().UTC()
	histPath := filepath.Join(histDir, fmt.Sprintf("%d%s", stamp.UnixNano(), ext))
	if err := os.WriteFile(histPath, data, 0o644); err != nil {
		return Result{}, fmt.Errorf("write history: %w", err)
	}
	_ = pruneHistory(histDir, histN)

	res := Result{
		Path:  stablePNG,
		Bytes: len(data),
		At:    stamp,
	}

	if opt.SetClipboard {
		if err := clipboard.SetImage(data, clipboard.SetOptions{Display: opt.Display}); err != nil {
			res.Clipboard = "error: " + err.Error()
		} else {
			res.Clipboard = "ok"
		}
	} else {
		res.Clipboard = "skipped"
	}

	if err := writeStatus(res); err != nil {
		// non-fatal
		fmt.Fprintf(os.Stderr, "clipremote: warning: could not write status: %v\n", err)
	}
	return res, nil
}

func writeStatus(res Result) error {
	p, err := paths.StatusPath()
	if err != nil {
		return err
	}
	st := Status{Last: res}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// ReadStatus loads last ingest status if present.
func ReadStatus() (Status, error) {
	p, err := paths.StatusPath()
	if err != nil {
		return Status{}, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return Status{}, err
	}
	var st Status
	if err := json.Unmarshal(b, &st); err != nil {
		return Status{}, err
	}
	return st, nil
}

func pruneHistory(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	type fi struct {
		name string
		mod  time.Time
	}
	var files []fi
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fi{name: e.Name(), mod: info.ModTime()})
	}
	if len(files) <= keep {
		return nil
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].mod.After(files[j].mod)
	})
	for _, f := range files[keep:] {
		_ = os.Remove(filepath.Join(dir, f.name))
	}
	return nil
}

func looksLikeImage(b []byte) bool {
	if len(b) < 8 {
		return false
	}
	// PNG
	if b[0] == 0x89 && b[1] == 'P' && b[2] == 'N' && b[3] == 'G' {
		return true
	}
	// JPEG
	if b[0] == 0xff && b[1] == 0xd8 {
		return true
	}
	// GIF
	if string(b[:6]) == "GIF87a" || string(b[:6]) == "GIF89a" {
		return true
	}
	// WebP
	if len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP" {
		return true
	}
	return false
}

func detectExt(b []byte) string {
	if len(b) >= 8 && b[0] == 0x89 && b[1] == 'P' {
		return ".png"
	}
	if len(b) >= 2 && b[0] == 0xff && b[1] == 0xd8 {
		return ".jpg"
	}
	if len(b) >= 6 && (string(b[:6]) == "GIF87a" || string(b[:6]) == "GIF89a") {
		return ".gif"
	}
	if len(b) >= 12 && string(b[0:4]) == "RIFF" {
		return ".webp"
	}
	return ".bin"
}
