package paths

import (
	"os"
	"path/filepath"
)

const (
	DefaultPort     = 18765
	DefaultHistory  = 10
	AppName         = "clipremote"
	LatestFileName  = "latest.png"
	HistoryDirName  = "history"
	StatusFileName  = "status.json"
	ActiveDirName   = "active-hosts"
)

// ConfigDir returns ~/.config/clipremote
func ConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, AppName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", AppName), nil
}

// CacheDir returns ~/.cache/clipremote
func CacheDir() (string, error) {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, AppName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", AppName), nil
}

// StateDir returns ~/.local/state/clipremote (or cache fallback)
func StateDir() (string, error) {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, AppName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", AppName), nil
}

func ConfigFile() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.toml"), nil
}

func LatestPath() (string, error) {
	d, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, LatestFileName), nil
}

func HistoryDir() (string, error) {
	d, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, HistoryDirName), nil
}

func StatusPath() (string, error) {
	d, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, StatusFileName), nil
}

func ActiveHostsDir() (string, error) {
	d, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, ActiveDirName), nil
}

func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}
