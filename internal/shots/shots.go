package shots

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var imageExt = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
	".tif":  true,
	".tiff": true,
	".heic": true,
	".bmp":  true,
}

// DefaultDirs returns common macOS screenshot / CleanShot locations.
func DefaultDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, "Desktop"),
		filepath.Join(home, "Pictures", "Screenshots"),
		filepath.Join(home, "Screenshots"),
		filepath.Join(home, "Pictures"),
	}
}

// ResolveDir picks an explicit dir, or the first default dir that exists and has images.
func ResolveDir(explicit string) (string, error) {
	if explicit != "" {
		p := expandHome(explicit)
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			return "", fmt.Errorf("screenshots dir not found: %s", p)
		}
		return p, nil
	}
	var tried []string
	for _, d := range DefaultDirs() {
		tried = append(tried, d)
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			continue
		}
		// Prefer a dir that already has images
		if files, err := Recent(d, 1); err == nil && len(files) > 0 {
			return d, nil
		}
	}
	// Fall back to Desktop even if empty (CleanShot default for many)
	home, _ := os.UserHomeDir()
	desktop := filepath.Join(home, "Desktop")
	if st, err := os.Stat(desktop); err == nil && st.IsDir() {
		return desktop, nil
	}
	return "", fmt.Errorf("no screenshots folder found (tried: %s); set screenshots_dir in config", strings.Join(tried, ", "))
}

// File is a local image with metadata.
type File struct {
	Path    string
	ModTime time.Time
	Size    int64
}

// Recent returns up to n newest image files in dir (non-recursive).
func Recent(dir string, n int) ([]File, error) {
	if n <= 0 {
		n = 20
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []File
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if !imageExt[ext] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		// skip tiny/empty
		if info.Size() < 32 {
			continue
		}
		files = append(files, File{
			Path:    filepath.Join(dir, name),
			ModTime: info.ModTime(),
			Size:    info.Size(),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime.After(files[j].ModTime)
	})
	if len(files) > n {
		files = files[:n]
	}
	return files, nil
}

// Fingerprint for change detection.
func (f File) Fingerprint() string {
	return fmt.Sprintf("%s|%d|%d", f.Path, f.ModTime.UnixNano(), f.Size)
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	return p
}
