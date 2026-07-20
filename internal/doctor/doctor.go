package doctor

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/vulcanhelix/clipremote/internal/clipboard"
	"github.com/vulcanhelix/clipremote/internal/config"
	"github.com/vulcanhelix/clipremote/internal/ingest"
	"github.com/vulcanhelix/clipremote/internal/paths"
	"github.com/vulcanhelix/clipremote/internal/push"
)

// Run prints diagnostics to w.
func Run(w io.Writer, cfg config.Config, remoteMode bool) int {
	fmt.Fprintf(w, "clipremote doctor\n")
	fmt.Fprintf(w, "  os:      %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(w, "  role:    %s\n", map[bool]string{true: "remote", false: "local"}[remoteMode])
	fmt.Fprintf(w, "  port:    %d\n", cfg.Port)

	code := 0

	// Paths
	cache, _ := paths.CacheDir()
	cfgPath, _ := paths.ConfigFile()
	latest, _ := paths.LatestPath()
	fmt.Fprintf(w, "  config:  %s\n", cfgPath)
	fmt.Fprintf(w, "  cache:   %s\n", cache)
	fmt.Fprintf(w, "  latest:  %s\n", latest)

	// Clipboard
	tool, ok := clipboard.Available()
	if ok {
		fmt.Fprintf(w, "  clipboard: %s ✓\n", tool)
	} else {
		fmt.Fprintf(w, "  clipboard: %s ✗\n", tool)
		if remoteMode {
			fmt.Fprintf(w, "    tip: install xclip or wl-clipboard; for headless use Xvfb or @latest.png\n")
		} else if runtime.GOOS == "darwin" {
			fmt.Fprintf(w, "    tip: brew install pngpaste  (optional but recommended)\n")
		}
		// not fatal for remote file path
	}

	// Latest file
	if st, err := os.Stat(latest); err == nil {
		fmt.Fprintf(w, "  latest.png: %d bytes, mtime %s ✓\n", st.Size(), st.ModTime().Format(time.RFC3339))
	} else {
		fmt.Fprintf(w, "  latest.png: missing (no ingest yet)\n")
	}

	// Status
	if st, err := ingest.ReadStatus(); err == nil {
		fmt.Fprintf(w, "  last ingest: %s (%d bytes, clipboard=%s)\n",
			st.Last.At.Format(time.RFC3339), st.Last.Bytes, st.Last.Clipboard)
	}

	if remoteMode {
		// pull endpoint via reverse tunnel
		url := fmt.Sprintf("http://127.0.0.1:%d/health", cfg.Port)
		client := &http.Client{Timeout: 800 * time.Millisecond}
		resp, err := client.Get(url)
		if err != nil {
			fmt.Fprintf(w, "  pull tunnel: unreachable (%v)\n", err)
			fmt.Fprintf(w, "    tip: connect with: clipremote ssh <host>  (adds RemoteForward)\n")
		} else {
			defer resp.Body.Close()
			fmt.Fprintf(w, "  pull tunnel: http://127.0.0.1:%d ✓\n", cfg.Port)
		}
		// clipremote on PATH
		if _, err := exec.LookPath("clipremote"); err != nil {
			fmt.Fprintf(w, "  PATH: clipremote not found ✗\n")
			code = 1
		} else {
			fmt.Fprintf(w, "  PATH: clipremote ✓\n")
		}
		return code
	}

	// Local daemon
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.Port), 400*time.Millisecond)
	if err != nil {
		fmt.Fprintf(w, "  daemon: not listening on 127.0.0.1:%d ✗\n", cfg.Port)
		fmt.Fprintf(w, "    tip: clipremote daemon   or   clipremote setup\n")
		code = 1
	} else {
		_ = conn.Close()
		fmt.Fprintf(w, "  daemon: listening on 127.0.0.1:%d ✓\n", cfg.Port)
		// health
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/health", cfg.Port))
		if err == nil {
			defer resp.Body.Close()
			var m map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&m)
			fmt.Fprintf(w, "  daemon health: has_image=%v bytes=%v\n", m["has_image"], m["bytes"])
		}
	}

	// Active hosts
	hosts, _ := push.ActiveTargets(cfg)
	if len(hosts) == 0 {
		fmt.Fprintf(w, "  active hosts: none (auto-push idle)\n")
		fmt.Fprintf(w, "    tip: clipremote ssh user@host\n")
	} else {
		fmt.Fprintf(w, "  active hosts:\n")
		for _, h := range hosts {
			fmt.Fprintf(w, "    - %s\n", h)
		}
	}

	// Configured hosts
	if len(cfg.Hosts) > 0 {
		fmt.Fprintf(w, "  configured hosts:\n")
		for _, h := range cfg.Hosts {
			fmt.Fprintf(w, "    - %s → %s\n", h.Name, h.SSH)
		}
	}

	// ssh binary
	if _, err := exec.LookPath("ssh"); err != nil {
		fmt.Fprintf(w, "  ssh: not found ✗\n")
		code = 1
	} else {
		fmt.Fprintf(w, "  ssh: ✓\n")
	}

	return code
}
