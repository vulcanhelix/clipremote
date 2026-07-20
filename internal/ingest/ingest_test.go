package ingest

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// minimal 1x1 PNG
var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xde, 0x00, 0x00, 0x00,
	0x0c, 0x49, 0x44, 0x41, 0x54, 0x08, 0xd7, 0x63, 0xf8, 0xff, 0xff, 0x3f,
	0x00, 0x05, 0xfe, 0x02, 0xfe, 0xdc, 0xcc, 0x59, 0xe7, 0x00, 0x00, 0x00,
	0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func TestFromReader(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	res, err := FromReader(bytes.NewReader(tinyPNG), Options{
		HistoryN:     5,
		SetClipboard: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Bytes != len(tinyPNG) {
		t.Fatalf("bytes: got %d want %d", res.Bytes, len(tinyPNG))
	}
	data, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, tinyPNG) {
		t.Fatal("latest content mismatch")
	}

	// history should exist
	hist := filepath.Join(dir, "clipremote", "history")
	entries, err := os.ReadDir(hist)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("history entries: %d", len(entries))
	}
}

func TestRejectsGarbage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	_, err := FromReader(bytes.NewReader([]byte("not an image")), Options{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLooksLikeImage(t *testing.T) {
	if !looksLikeImage(tinyPNG) {
		t.Fatal("png should match")
	}
	if looksLikeImage([]byte("hello")) {
		t.Fatal("text should not match")
	}
}
