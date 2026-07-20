package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg := Default()
	cfg.Port = 12345
	cfg.AutoPush = false
	cfg.Hosts = []Host{{Name: "box", SSH: "user@host"}}
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "clipremote", "config.toml")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Port != 12345 {
		t.Fatalf("port %d", got.Port)
	}
	if got.AutoPush {
		t.Fatal("auto_push should be false")
	}
	if len(got.Hosts) != 1 || got.Hosts[0].Name != "box" || got.Hosts[0].SSH != "user@host" {
		t.Fatalf("hosts: %+v", got.Hosts)
	}
}
