package clipboard

// SetOptions controls how an image is written to the system clipboard.
type SetOptions struct {
	Display string // X11 DISPLAY override
}

// Image payload from the local OS clipboard.
type Image struct {
	PNG []byte
}
