//go:build !linux && !darwin

package clipboard

func WatchChangeCount() (int, error) {
	return 0, nil
}
