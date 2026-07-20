package daemon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/vulcanhelix/clipremote/internal/clipboard"
	"github.com/vulcanhelix/clipremote/internal/config"
	"github.com/vulcanhelix/clipremote/internal/push"
	"github.com/vulcanhelix/clipremote/internal/shots"
)

// Server is the local laptop daemon: HTTP pull endpoint + folder/clipboard watcher.
type Server struct {
	Cfg config.Config

	mu      sync.RWMutex
	lastPNG []byte
	lastAt  time.Time
	lastErr string

	seen map[string]bool // fingerprints already pushed
}

type imageResponse struct {
	OK    bool   `json:"ok"`
	Image string `json:"image,omitempty"` // base64 PNG
	Error string `json:"error,omitempty"`
	Bytes int    `json:"bytes,omitempty"`
	At    string `json:"at,omitempty"`
}

// Run starts HTTP server and watch loops until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	addr := fmt.Sprintf("127.0.0.1:%d", s.Cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	log.Printf("clipremote daemon listening on http://%s", addr)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/image", s.handleImage)
	mux.HandleFunc("/image.png", s.handleImagePNG)

	httpServer := &http.Server{Handler: mux}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	// Keep SSH mux warm so auto-push works without an open terminal session.
	if s.Cfg.AutoPush && len(s.Cfg.Hosts) > 0 {
		go func() {
			push.EnsureAllMux(s.Cfg)
			log.Printf("ssh mux ready for %d configured host(s)", len(s.Cfg.Hosts))
		}()
	}

	src := s.Cfg.Source
	if src == "" {
		src = "folder"
	}
	if src == "folder" || src == "auto" {
		go s.folderWatchLoop(ctx)
	}
	if src == "clipboard" || src == "auto" {
		go s.clipboardWatchLoop(ctx)
	}

	err = httpServer.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	s.mu.RLock()
	defer s.mu.RUnlock()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":         true,
		"has_image":  len(s.lastPNG) > 0,
		"bytes":      len(s.lastPNG),
		"at":         s.lastAt.Format(time.RFC3339),
		"last_error": s.lastErr,
		"source":     s.Cfg.Source,
	})
}

func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	png := append([]byte(nil), s.lastPNG...)
	at := s.lastAt
	lastErr := s.lastErr
	s.mu.RUnlock()

	if len(png) == 0 {
		// try latest folder file
		if data, err := s.latestFolderBytes(); err == nil {
			png = data
			at = time.Now()
		} else if img, err := clipboard.ReadImage(); err == nil {
			png = img.PNG
			at = time.Now()
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(imageResponse{OK: false, Error: lastErr})
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(imageResponse{
		OK:    true,
		Image: base64.StdEncoding.EncodeToString(png),
		Bytes: len(png),
		At:    at.Format(time.RFC3339),
	})
}

func (s *Server) handleImagePNG(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	png := append([]byte(nil), s.lastPNG...)
	s.mu.RUnlock()
	if len(png) == 0 {
		if data, err := s.latestFolderBytes(); err == nil {
			png = data
		} else if img, err := clipboard.ReadImage(); err == nil {
			png = img.PNG
		} else {
			http.Error(w, "no image available", http.StatusNotFound)
			return
		}
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(png)
}

func (s *Server) latestFolderBytes() ([]byte, error) {
	dir, err := shots.ResolveDir(s.Cfg.ScreenshotsDir)
	if err != nil {
		return nil, err
	}
	n := s.Cfg.ScreenshotsN
	if n <= 0 {
		n = 20
	}
	files, err := shots.Recent(dir, 1)
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("no images in %s", dir)
	}
	return os.ReadFile(files[0].Path)
}

func (s *Server) folderWatchLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// First pass: mark existing files as seen so we don't flood push on startup
	// unless none were seen before — still seed lastPNG from newest.
	dir, err := shots.ResolveDir(s.Cfg.ScreenshotsDir)
	if err != nil {
		log.Printf("folder watch: %v (set screenshots_dir in config)", err)
	} else {
		n := s.Cfg.ScreenshotsN
		if n <= 0 {
			n = 20
		}
		log.Printf("watching screenshots folder: %s (last %d)", dir, n)
		s.seedSeen(dir)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pollFolder()
		}
	}
}

func (s *Server) seedSeen(dir string) {
	n := s.Cfg.ScreenshotsN
	if n <= 0 {
		n = 20
	}
	files, err := shots.Recent(dir, n)
	if err != nil || len(files) == 0 {
		return
	}
	// newest first from Recent
	if data, err := os.ReadFile(files[0].Path); err == nil {
		s.mu.Lock()
		s.lastPNG = data
		s.lastAt = files[0].ModTime
		s.mu.Unlock()
	}
	for _, f := range files {
		s.seen[f.Fingerprint()] = true
	}
	log.Printf("seeded %d existing screenshots (won't re-push until new ones appear)", len(files))
}

func (s *Server) pollFolder() {
	dir, err := shots.ResolveDir(s.Cfg.ScreenshotsDir)
	if err != nil {
		return
	}
	n := s.Cfg.ScreenshotsN
	if n <= 0 {
		n = 20
	}
	files, err := shots.Recent(dir, n)
	if err != nil || len(files) == 0 {
		return
	}

	// Find new files (not seen). Recent is newest-first; push oldest-new first so latest.png = newest.
	var newOnes []shots.File
	for i := len(files) - 1; i >= 0; i-- {
		f := files[i]
		fp := f.Fingerprint()
		if s.seen[fp] {
			continue
		}
		newOnes = append(newOnes, f)
	}
	if len(newOnes) == 0 {
		return
	}

	for _, f := range newOnes {
		data, err := os.ReadFile(f.Path)
		if err != nil {
			log.Printf("read %s: %v", f.Path, err)
			continue
		}
		s.mu.Lock()
		s.lastPNG = data
		s.lastAt = f.ModTime
		s.lastErr = ""
		s.mu.Unlock()
		s.seen[f.Fingerprint()] = true
		log.Printf("new screenshot: %s (%d bytes)", f.Path, len(data))

		if s.Cfg.AutoPush {
			// push file path for logging
			go s.autoPushFile(f.Path, data)
		}
	}
}

func (s *Server) clipboardWatchLoop(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastCount int
	var initialized bool

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, err := clipboard.WatchChangeCount()
			if err != nil {
				count = -1
			}
			if initialized && count == lastCount && count != -1 {
				continue
			}
			img, err := clipboard.ReadImage()
			if err != nil {
				if !initialized {
					initialized = true
					lastCount = count
				}
				if count != -1 {
					lastCount = count
				}
				continue
			}
			if count != -1 {
				lastCount = count
			}
			initialized = true

			s.mu.RLock()
			same := len(s.lastPNG) == len(img.PNG) && bytesEqual(s.lastPNG, img.PNG)
			s.mu.RUnlock()
			if same {
				continue
			}

			s.mu.Lock()
			s.lastPNG = img.PNG
			s.lastAt = time.Now()
			s.lastErr = ""
			s.mu.Unlock()
			log.Printf("clipboard image captured (%d bytes)", len(img.PNG))

			if s.Cfg.AutoPush {
				go s.autoPushBytes(img.PNG)
			}
		}
	}
}

func (s *Server) autoPushFile(path string, data []byte) {
	hosts, err := push.PushTargets(s.Cfg)
	if err != nil {
		log.Printf("auto-push: %v", err)
		return
	}
	if len(hosts) == 0 {
		log.Printf("auto-push: no hosts configured — run: clipremote host add box user@host")
		return
	}
	for _, h := range hosts {
		// refresh mux if needed (cheap no-op when already up)
		_ = push.EnsureControlMaster(h)
		if err := push.ToHost(h, data); err != nil {
			log.Printf("auto-push %s → %s: %v", path, h, err)
		} else {
			log.Printf("auto-push %s → %s: ok", path, h)
		}
	}
}

func (s *Server) autoPushBytes(png []byte) {
	s.autoPushFile("(clipboard)", png)
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

