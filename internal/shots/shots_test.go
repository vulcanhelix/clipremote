package shots

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecent(t *testing.T) {
	dir := t.TempDir()
	// older
	p1 := filepath.Join(dir, "a.png")
	p2 := filepath.Join(dir, "b.png")
	if err := os.WriteFile(p1, []byte("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(p2, []byte("yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy"), 0o644); err != nil {
		t.Fatal(err)
	}
	// non-image ignored
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hi"), 0o644)

	files, err := Recent(dir, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files", len(files))
	}
	if files[0].Path != p2 {
		t.Fatalf("newest should be b.png, got %s", files[0].Path)
	}
}
