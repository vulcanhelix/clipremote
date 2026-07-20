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

// ToHost streams image bytes to `clipremote ingest` on the remote host via SSH.
func ToHost(sshTarget string, png []byte) error {
	return ToHostOpts(sshTarget, png, true)
}

// ToHostOpts streams image bytes; setClipboard asks remote to update its clipboard too.
func ToHostOpts(sshTarget string, png []byte, setClipboard bool) error {
	if sshTarget == "" {
		return fmt.Errorf("empty ssh target")
	}
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=8",
	}
	if cp := controlPathFor(sshTarget); cp != "" {
		args = append(args, "-o", "ControlPath="+cp, "-o", "ControlMaster=auto", "-o", "ControlPersist=yes")
	}
	ingestArgs := "ingest"
	if setClipboard {
		ingestArgs = "ingest --clipboard"
	}
	// Prefer real file binaries — never exec a directory named clipremote (e.g. the git repo).
	remoteCmd := fmt.Sprintf(`set -e
for c in "$HOME/.local/bin/clipremote" /usr/local/bin/clipremote /usr/bin/clipremote; do
  if [ -f "$c" ] && [ -x "$c" ]; then
    exec "$c" %s
  fi
done
export PATH="$HOME/.local/bin:/usr/local/bin:$PATH"
c=$(command -v clipremote 2>/dev/null || true)
if [ -n "$c" ] && [ -f "$c" ] && [ -x "$c" ]; then
  exec "$c" %s
fi
echo "clipremote binary not found on remote — install to ~/.local/bin/clipremote" >&2
exit 127
`, ingestArgs, ingestArgs)
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

// FileToHost uploads a local image file to the remote host.
func FileToHost(sshTarget, localPath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	return ToHostOpts(sshTarget, data, true)
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

// ActiveTargets returns hosts marked by an open clipremote ssh session.
func ActiveTargets(cfg config.Config) ([]string, error) {
	return collectTargets(cfg, false)
}

// PushTargets returns every host that should receive auto-push:
// all configured hosts, plus any active session markers.
// Configured hosts are enough — you do not need clipremote ssh open.
func PushTargets(cfg config.Config) ([]string, error) {
	return collectTargets(cfg, true)
}

func collectTargets(cfg config.Config, includeConfigured bool) ([]string, error) {
	seen := map[string]bool{}
	var out []string

	add := func(t string) {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			return
		}
		seen[t] = true
		out = append(out, t)
	}

	if includeConfigured {
		for _, h := range cfg.Hosts {
			if h.SSH != "" {
				add(h.SSH)
			} else {
				add(h.Name)
			}
		}
	}

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
			add(target)
		}
	}

	// Hosts with a live control socket
	for _, h := range cfg.Hosts {
		t := h.SSH
		if t == "" {
			t = h.Name
		}
		if cp := controlPathFor(t); cp != "" {
			if _, err := os.Stat(cp); err == nil {
				add(t)
			}
		}
	}
	return out, nil
}

// EnsureControlMaster opens a background multiplexed SSH connection so later
// pushes are fast and don't need an interactive session.
func EnsureControlMaster(sshTarget string) error {
	if sshTarget == "" {
		return fmt.Errorf("empty ssh target")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	sshDir := filepath.Join(home, ".ssh")
	_ = os.MkdirAll(sshDir, 0o700)
	controlPath := filepath.Join(sshDir, "clipremote-%r@%h:%p")

	// Already up?
	check := exec.Command("ssh",
		"-o", "ControlPath="+controlPath,
		"-o", "ControlMaster=auto",
		"-O", "check",
		sshTarget,
	)
	if err := check.Run(); err == nil {
		return nil
	}

	cmd := exec.Command("ssh",
		"-o", "BatchMode=yes",
		"-o", "ControlMaster=auto",
		"-o", "ControlPath="+controlPath,
		"-o", "ControlPersist=yes",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"-Nf",
		sshTarget,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ssh -Nf %s: %w (%s)", sshTarget, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// EnsureAllMux opens ControlMaster for every configured host.
func EnsureAllMux(cfg config.Config) {
	for _, h := range cfg.Hosts {
		t := h.SSH
		if t == "" {
			t = h.Name
		}
		if t == "" {
			continue
		}
		if err := EnsureControlMaster(t); err != nil {
			fmt.Fprintf(os.Stderr, "clipremote: mux %s: %v\n", t, err)
		}
	}
}

// ListActive returns active host markers for doctor.
func ListActive() ([]string, error) {
	return ActiveTargets(config.Config{})
}
