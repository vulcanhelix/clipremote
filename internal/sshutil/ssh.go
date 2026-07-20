package sshutil

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/vulcanhelix/clipremote/internal/config"
	"github.com/vulcanhelix/clipremote/internal/push"
)

// WrapOptions configures clipremote ssh.
type WrapOptions struct {
	Cfg        config.Config
	Target     string
	ExtraArgs  []string
	RemoteForward bool
}

// RunSSH execs ssh with ControlMaster + optional RemoteForward for pull mode.
func RunSSH(opt WrapOptions) error {
	if opt.Target == "" {
		return fmt.Errorf("ssh target required")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	sshDir := filepath.Join(home, ".ssh")
	_ = os.MkdirAll(sshDir, 0o700)

	controlPath := expandTilde(opt.Cfg.ControlPathTemplate, home)
	// If template still has tokens, OpenSSH will expand them — good.
	// Use a fixed pattern OpenSSH understands:
	if controlPath == "" || !strings.Contains(controlPath, "%") {
		controlPath = filepath.Join(sshDir, "clipremote-%r@%h:%p")
	}

	args := []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + controlPath,
		"-o", "ControlPersist=4h",
	}

	if opt.RemoteForward {
		// Remote port → local daemon
		fwd := fmt.Sprintf("%d:127.0.0.1:%d", opt.Cfg.Port, opt.Cfg.Port)
		args = append(args, "-R", fwd)
		// Avoid failures if port already forwarded
		args = append(args, "-o", "ExitOnForwardFailure=no")
	}

	args = append(args, opt.ExtraArgs...)
	args = append(args, opt.Target)

	// Mark active for auto-push duration
	_ = push.MarkActive(opt.Target)
	defer func() { _ = push.UnmarkActive(opt.Target) }()

	cmd := exec.Command("ssh", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Forward signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		for range sigCh {
			if cmd.Process != nil {
				_ = cmd.Process.Signal(os.Interrupt)
			}
		}
	}()
	return cmd.Wait()
}

func expandTilde(p, home string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

// EnsureDaemonHint returns a message if local daemon seems down.
func EnsureDaemonHint(port int) string {
	// best-effort TCP check is done by caller
	return fmt.Sprintf("start the local daemon: clipremote daemon   (or: clipremote setup)")
}
