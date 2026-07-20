//go:build !linux && !darwin

package clipboard

import "fmt"

func SetImage(data []byte, opt SetOptions) error {
	return fmt.Errorf("clipboard write not supported on this OS")
}

func ReadImage() (Image, error) {
	return Image{}, fmt.Errorf("clipboard read not supported on this OS")
}

func Available() (string, bool) {
	return "unsupported", false
}
