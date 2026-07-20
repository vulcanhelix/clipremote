package daemon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/vulcanhelix/clipremote/internal/clipboard"
	"github.com/vulcanhelix/clipremote/internal/config"
	"github.com/vulcanhelix/clipremote/internal/push"
)

// Server is the local laptop daemon: HTTP pull endpoint + clipboard watcher.
type Server struct {
	Cfg config.Config

	mu      sync.RWMutex
	lastPNG []byte
	lastAt  time.Time
	lastErr string
}

type imageResponse struct {
	OK    bool   `json:"ok"`
	Image string `json:"image,omitempty"` // base64 PNG
	Error string `json:"error,omitempty"`
	Bytes int    `json:"bytes,omitempty"`
	At    string `json:"at,omitempty"`
}

// Run starts HTTP server and clipboard watch loop until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
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

	go s.watchLoop(ctx)

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
	})
}

func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	png := append([]byte(nil), s.lastPNG...)
	at := s.lastAt
	lastErr := s.lastErr
	s.mu.RUnlock()

	// Refresh from clipboard if empty
	if len(png) == 0 {
		img, err := clipboard.ReadImage()
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(imageResponse{OK: false, Error: err.Error()})
			return
		}
		png = img.PNG
		at = time.Now()
		s.mu.Lock()
		s.lastPNG = png
		s.lastAt = at
		s.mu.Unlock()
	}

	if len(png) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(imageResponse{OK: false, Error: lastErr})
		return
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
		img, err := clipboard.ReadImage()
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		png = img.PNG
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(png)
}

func (s *Server) watchLoop(ctx context.Context) {
	ticker := time.NewTicker(300 * time.Millisecond)
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
				// still try ReadImage periodically when change count fails
				count = -1
			}
			if initialized && count == lastCount && count != -1 {
				continue
			}
			// On first tick or change, try read
			img, err := clipboard.ReadImage()
			if err != nil {
				if !initialized {
					initialized = true
					lastCount = count
				}
				// only update lastCount if we got a valid count
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
				go s.autoPush(img.PNG)
			}
		}
	}
}

func (s *Server) autoPush(png []byte) {
	hosts, err := push.ActiveTargets(s.Cfg)
	if err != nil {
		log.Printf("auto-push: list targets: %v", err)
		return
	}
	if len(hosts) == 0 {
		log.Printf("auto-push: no active hosts (open a session with: clipremote ssh <host>)")
		return
	}
	for _, h := range hosts {
		if err := push.ToHost(h, png); err != nil {
			log.Printf("auto-push to %s: %v", h, err)
		} else {
			log.Printf("auto-push to %s: ok", h)
		}
	}
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
