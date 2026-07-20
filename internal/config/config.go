package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/clipremote/clipremote/internal/paths"
)

// Config is the user configuration for clipremote.
type Config struct {
	Port      int
	AutoPush  bool
	History   int
	Hosts     []Host
	ControlPathTemplate string
}

// Host is a configured remote target.
type Host struct {
	Name string // short name or alias
	SSH  string // user@hostname or Host alias
}

func Default() Config {
	return Config{
		Port:                paths.DefaultPort,
		AutoPush:            true,
		History:             paths.DefaultHistory,
		ControlPathTemplate: "~/.ssh/clipremote-%r@%h:%p",
	}
}

// Load reads config from disk, or returns defaults if missing.
func Load() (Config, error) {
	cfg := Default()
	path, err := paths.ConfigFile()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	return parseTOMLLite(string(data), cfg)
}

// Save writes config to disk (simple TOML subset).
func Save(cfg Config) error {
	dir, err := paths.ConfigDir()
	if err != nil {
		return err
	}
	if err := paths.EnsureDir(dir); err != nil {
		return err
	}
	path, err := paths.ConfigFile()
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# clipremote config\n")
	b.WriteString(fmt.Sprintf("port = %d\n", cfg.Port))
	b.WriteString(fmt.Sprintf("auto_push = %v\n", cfg.AutoPush))
	b.WriteString(fmt.Sprintf("history = %d\n", cfg.History))
	b.WriteString(fmt.Sprintf("control_path = %q\n", cfg.ControlPathTemplate))
	b.WriteString("\n")
	for _, h := range cfg.Hosts {
		b.WriteString("[[hosts]]\n")
		b.WriteString(fmt.Sprintf("name = %q\n", h.Name))
		b.WriteString(fmt.Sprintf("ssh = %q\n", h.SSH))
		b.WriteString("\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// FindHost returns a host by name or ssh target.
func (c Config) FindHost(name string) (Host, bool) {
	for _, h := range c.Hosts {
		if h.Name == name || h.SSH == name {
			return h, true
		}
	}
	return Host{}, false
}

// parseTOMLLite parses our minimal config format (no external TOML dep).
func parseTOMLLite(src string, cfg Config) (Config, error) {
	var current *Host
	flush := func() {
		if current != nil && (current.Name != "" || current.SSH != "") {
			if current.Name == "" {
				current.Name = current.SSH
			}
			if current.SSH == "" {
				current.SSH = current.Name
			}
			cfg.Hosts = append(cfg.Hosts, *current)
			current = nil
		}
	}

	lines := strings.Split(src, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "[[hosts]]" {
			flush()
			current = &Host{}
			continue
		}
		key, val, ok := splitKV(line)
		if !ok {
			continue
		}
		if current != nil {
			switch key {
			case "name":
				current.Name = unquote(val)
			case "ssh":
				current.SSH = unquote(val)
			}
			continue
		}
		switch key {
		case "port":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.Port = n
			}
		case "auto_push":
			cfg.AutoPush = val == "true" || val == "1"
		case "history":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.History = n
			}
		case "control_path":
			cfg.ControlPathTemplate = unquote(val)
		}
	}
	flush()
	// reset hosts if we re-parsed into existing - actually we append to Default which has empty hosts
	return cfg, nil
}

func splitKV(line string) (string, string, bool) {
	i := strings.IndexByte(line, '=')
	if i < 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:i])
	val := strings.TrimSpace(line[i+1:])
	return key, val, true
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
