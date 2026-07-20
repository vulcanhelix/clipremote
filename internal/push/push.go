package push

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vulcanhelix/clipremote/internal/config"
	"github.com/vulcanhelix/clipremote/internal/paths"
)

// ToHost streams PNG bytes to `clipremote ingest` on the remote host via SSH.
func ToHost(sshTarget string, png []byte) error {
	if sshTarget == "" {
		return fmt.Errorf("empty ssh target")
	}
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=8",
	}
	// Prefer ControlMaster if present
	if cp := controlPathFor(sshTarget); cp != "" {
		args = append(args, "-o", "ControlPath="+cp, "-o", "ControlMaster=auto", "-o", "ControlPersist=yes")
	}
	// Non-interactive SSH often has a minimal PATH. Prefer login shell + common install dirs.
	remoteCmd := `export PATH="$HOME/.local/bin:/usr/local/bin:$PATH"; exec clipremote ingest --clipboard`
	args = append(args, sshTarget, "bash", "-lc", remoteCmd)

	cmd := exec.Command("ssh", args...)
	cmd.Stdin = bytes.NewReader(png)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ssh %s: %w: %s", sshTarget, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func controlPathFor(target string) string {
	// Expand common ControlPath from env or default clipremote path
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	// Try to discover an open control socket matching target
	// OpenSSH expands %r@%h:%p — we scan ~/.ssh for clipremote-* sockets
	sshDir := filepath.Join(home, ".ssh")
	entries, err := os.ReadDir(sshDir)
	if err != nil {
		return ""
	}
	// crude: if only one clipremote-* socket, use it; else match name contains host
	host := target
	if i := strings.LastIndex(target, "@"); i >= 0 {
		host = target[i+1:]
	}
	var match string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "clipremote-") && !strings.HasPrefix(name, "cm-") {
			continue
		}
		path := filepath.Join(sshDir, name)
		// socket must exist and be connectable-ish
		fi, err := os.Stat(path)
		if err != nil {
			continue
		}
		// skip stale (mode check only)
		_ = fi
		if strings.Contains(name, host) {
			return path
		}
		match = path
	}
	return match
}

// MarkActive records that a host session is open for auto-push.
func MarkActive(sshTarget string) error {
	dir, err := paths.ActiveHostsDir()
	if err != nil {
		return err
	}
	if err := paths.EnsureDir(dir); err != nil {
		return err
	}
	// filename is url-safe-ish
	name := strings.ReplaceAll(sshTarget, "/", "_")
	path := filepath.Join(dir, name)
	return os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"+sshTarget+"\n"), 0o644)
}

// UnmarkActive removes an active host marker.
func UnmarkActive(sshTarget string) error {
	dir, err := paths.ActiveHostsDir()
	if err != nil {
		return err
	}
	name := strings.ReplaceAll(sshTarget, "/", "_")
	return os.Remove(filepath.Join(dir, name))
}

// ActiveTargets returns hosts that should receive auto-push.
func ActiveTargets(cfg config.Config) ([]string, error) {
	seen := map[string]bool{}
	var out []string

	dir, err := paths.ActiveHostsDir()
	if err == nil {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			lines := strings.Split(strings.TrimSpace(string(b)), "\n")
			target := e.Name()
			if len(lines) >= 2 {
				target = strings.TrimSpace(lines[1])
			} else if len(lines) == 1 && strings.Contains(lines[0], "@") {
				target = strings.TrimSpace(lines[0])
			}
			// Also accept filename as target if content empty
			if target == "" {
				continue
			}
			if !seen[target] {
				seen[target] = true
				out = append(out, target)
			}
		}
	}

	// Also include configured hosts that have a live control socket
	for _, h := range cfg.Hosts {
		t := h.SSH
		if t == "" {
			t = h.Name
		}
		if seen[t] {
			continue
		}
		if cp := controlPathFor(t); cp != "" {
			// check socket exists
			if _, err := os.Stat(cp); err == nil {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	return out, nil
}

// ListActive returns active host markers for doctor.
func ListActive() ([]string, error) {
	return ActiveTargets(config.Config{})
}
