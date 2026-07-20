//go:build linux

package clipboard

// WatchChangeCount is a best-effort poll token on Linux (hash of clipboard image if any).
func WatchChangeCount() (int, error) {
	img, err := ReadImage()
	if err != nil {
		return 0, nil // no image — count 0
	}
	// simple checksum
	h := 0
	for _, b := range img.PNG {
		h = h*31 + int(b)
	}
	return h, nil
}
