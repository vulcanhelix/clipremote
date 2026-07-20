package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/vulcanhelix/clipremote/internal/clipboard"
	"github.com/vulcanhelix/clipremote/internal/config"
	"github.com/vulcanhelix/clipremote/internal/daemon"
	"github.com/vulcanhelix/clipremote/internal/doctor"
	"github.com/vulcanhelix/clipremote/internal/ingest"
	"github.com/vulcanhelix/clipremote/internal/paths"
	"github.com/vulcanhelix/clipremote/internal/push"
	"github.com/vulcanhelix/clipremote/internal/shots"
	"github.com/vulcanhelix/clipremote/internal/sshutil"
	"github.com/vulcanhelix/clipremote/internal/xvfb"
)

var version = "0.1.6"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "help", "-h", "--help":
		usage()
	case "version", "-v", "--version":
		fmt.Printf("clipremote %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
	case "ingest":
		err = cmdIngest(args)
	case "latest":
		err = cmdLatest(args)
	case "paste":
		err = cmdPaste(args)
	case "push":
		err = cmdPush(args)
	case "daemon":
		err = cmdDaemon(args)
	case "ssh":
		err = cmdSSH(args)
	case "host":
		err = cmdHost(args)
	case "setup":
		err = cmdSetup(args)
	case "install-service", "service-install":
		err = cmdInstallService(args)
	case "doctor":
		err = cmdDoctor(args)
	case "xvfb":
		err = cmdXvfb(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "clipremote: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `clipremote — auto-sync laptop screenshots to a remote host for AI agents

Usage:
  clipremote <command> [flags]

Laptop (macOS recommended):
  setup                 Write config + LaunchAgent plist
  install-service       Install + start login daemon (survives reboot)
  daemon                Run folder watcher (foreground)
  push [host]           Upload recent screenshots (folder by default)
  host add|list|rm      Manage remote targets
  ssh <target> [args]   SSH with ControlMaster + reverse tunnel

Remote (Linux):
  setup --remote        Cache dirs + history defaults
  ingest [--clipboard]  Stdin image → ~/.cache/clipremote/latest.png
  paste                 Pull via reverse-tunneled laptop daemon
  latest                Print path to latest.png
  xvfb                  Optional virtual display for clipboard tools

Both:
  doctor                Diagnose setup
  version               Print version

Typical setup:
  # remote
  clipremote setup --remote

  # laptop
  clipremote setup
  clipremote host add myserver you@host
  clipremote install-service

  # daily: take a screenshot → on remote agent:
  #   @~/.cache/clipremote/latest.png

Docs: https://github.com/vulcanhelix/clipremote
`)
}

func loadCfg() config.Config {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "clipremote: warning: config: %v\n", err)
		return config.Default()
	}
	return cfg
}

func cmdIngest(args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	setClip := fs.Bool("clipboard", false, "also set remote system clipboard")
	display := fs.String("display", "", "DISPLAY for xclip (optional)")
	history := fs.Int("history", 0, "history entries to keep (0 = config default)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg := loadCfg()
	hist := *history
	if hist == 0 {
		hist = cfg.History
	}
	disp := *display
	if disp == "" {
		disp = os.Getenv("DISPLAY")
	}
	// Auto-use clipremote Xvfb display if set
	if disp == "" {
		if d := os.Getenv("CLIPREMOTE_DISPLAY"); d != "" {
			disp = d
		}
	}

	res, err := ingest.FromReader(os.Stdin, ingest.Options{
		HistoryN:     hist,
		SetClipboard: *setClip,
		Display:      disp,
	})
	if err != nil {
		return err
	}
	fmt.Println(res.Path)
	if res.Clipboard != "" && res.Clipboard != "skipped" {
		fmt.Fprintf(os.Stderr, "clipboard: %s\n", res.Clipboard)
	}
	return nil
}

func cmdLatest(args []string) error {
	p, err := paths.LatestPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err != nil {
		return fmt.Errorf("no latest image yet at %s (copy a screenshot on your laptop with clipremote daemon running)", p)
	}
	fmt.Println(p)
	return nil
}

func cmdPaste(args []string) error {
	fs := flag.NewFlagSet("paste", flag.ContinueOnError)
	setClip := fs.Bool("clipboard", true, "set remote system clipboard")
	port := fs.Int("port", 0, "local daemon port via reverse tunnel")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg := loadCfg()
	p := *port
	if p == 0 {
		p = cfg.Port
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/image", p)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("pull from local daemon at %s: %w\n  connect with: clipremote ssh <host>", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("daemon returned %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var body struct {
		OK    bool   `json:"ok"`
		Image string `json:"image"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return err
	}
	if !body.OK || body.Image == "" {
		return fmt.Errorf("no image on local clipboard: %s", body.Error)
	}
	raw, err := base64.StdEncoding.DecodeString(body.Image)
	if err != nil {
		return err
	}
	res, err := ingest.FromReader(bytes.NewReader(raw), ingest.Options{
		HistoryN:     cfg.History,
		SetClipboard: *setClip,
		Display:      os.Getenv("DISPLAY"),
	})
	if err != nil {
		return err
	}
	fmt.Println(res.Path)
	return nil
}

func cmdPush(args []string) error {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	nFlag := fs.Int("n", 0, "number of recent screenshots to upload (default from config, usually 10)")
	dirFlag := fs.String("dir", "", "screenshots folder (default: auto-detect Desktop/Screenshots)")
	clipOnly := fs.Bool("clipboard", false, "push clipboard image only (ignore folder)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg := loadCfg()
	rest := fs.Args()

	var targets []string
	var err error
	if len(rest) > 0 {
		for _, a := range rest {
			if h, ok := cfg.FindHost(a); ok {
				targets = append(targets, h.SSH)
			} else {
				targets = append(targets, a)
			}
		}
	} else {
		targets, err = push.ActiveTargets(cfg)
		if err != nil {
			return err
		}
		if len(targets) == 0 {
			for _, h := range cfg.Hosts {
				targets = append(targets, h.SSH)
			}
		}
	}
	if len(targets) == 0 {
		return fmt.Errorf("no targets: pass a host, or: clipremote host add NAME user@host, then clipremote ssh NAME")
	}

	// Collect local images to upload (oldest → newest so latest.png is newest)
	type item struct {
		label string
		data  []byte
	}
	var items []item

	src := cfg.Source
	if src == "" {
		src = "folder"
	}
	if *clipOnly {
		src = "clipboard"
	}

	if src == "folder" || src == "auto" {
		dir := *dirFlag
		if dir == "" {
			dir = cfg.ScreenshotsDir
		}
		resolved, rerr := shots.ResolveDir(dir)
		if rerr != nil {
			if src == "folder" {
				return rerr
			}
			fmt.Fprintf(os.Stderr, "folder: %v\n", rerr)
		} else {
			n := *nFlag
			if n <= 0 {
				n = cfg.ScreenshotsN
			}
			if n <= 0 {
				n = 20
			}
			files, ferr := shots.Recent(resolved, n)
			if ferr != nil {
				return ferr
			}
			if len(files) == 0 {
				if src == "folder" {
					return fmt.Errorf("no images in %s", resolved)
				}
			} else {
				fmt.Fprintf(os.Stderr, "uploading %d image(s) from %s\n", len(files), resolved)
				// files are newest-first; reverse for upload order
				for i := len(files) - 1; i >= 0; i-- {
					f := files[i]
					data, err := os.ReadFile(f.Path)
					if err != nil {
						return err
					}
					items = append(items, item{label: f.Path, data: data})
				}
			}
		}
	}

	if len(items) == 0 && (src == "clipboard" || src == "auto") {
		img, err := clipboard.ReadImage()
		if err != nil {
			return fmt.Errorf("no folder images and clipboard empty: %w", err)
		}
		items = append(items, item{label: "(clipboard)", data: img.PNG})
	}
	if len(items) == 0 {
		return fmt.Errorf("nothing to push")
	}

	var errs []string
	for _, t := range targets {
		for _, it := range items {
			if err := push.ToHost(t, it.data); err != nil {
				errs = append(errs, fmt.Sprintf("%s→%s: %v", it.label, t, err))
				fmt.Fprintf(os.Stderr, "push %s → %s: %v\n", it.label, t, err)
			} else {
				fmt.Printf("pushed %s (%d bytes) → %s\n", it.label, len(it.data), t)
			}
		}
	}
	if len(errs) > 0 && len(errs) == len(targets)*len(items) {
		return fmt.Errorf("all pushes failed")
	}
	return nil
}

func cmdDaemon(args []string) error {
	cfg := loadCfg()
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	port := fs.Int("port", 0, "listen port")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *port > 0 {
		cfg.Port = *port
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	s := &daemon.Server{Cfg: cfg}
	return s.Run(ctx)
}

func cmdSSH(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: clipremote ssh <target> [ssh-args...]")
	}
	cfg := loadCfg()
	target := args[0]
	if h, ok := cfg.FindHost(target); ok {
		target = h.SSH
	}
	extra := args[1:]

	// Best-effort: remind if daemon down (do not hard-fail; push needs daemon only for auto)
	return sshutil.RunSSH(sshutil.WrapOptions{
		Cfg:           cfg,
		Target:        target,
		ExtraArgs:     extra,
		RemoteForward: true,
	})
}

func cmdHost(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: clipremote host add|list|rm ...")
	}
	cfg := loadCfg()
	switch args[0] {
	case "list":
		if len(cfg.Hosts) == 0 {
			fmt.Println("(no hosts configured)")
			return nil
		}
		for _, h := range cfg.Hosts {
			fmt.Printf("%s\t%s\n", h.Name, h.SSH)
		}
		return nil
	case "add":
		if len(args) < 3 {
			return fmt.Errorf("usage: clipremote host add <name> <user@host>")
		}
		name, ssh := args[1], args[2]
		// replace if exists
		found := false
		for i, h := range cfg.Hosts {
			if h.Name == name {
				cfg.Hosts[i].SSH = ssh
				found = true
				break
			}
		}
		if !found {
			cfg.Hosts = append(cfg.Hosts, config.Host{Name: name, SSH: ssh})
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Printf("added host %s → %s\n", name, ssh)
		return nil
	case "rm", "remove":
		if len(args) < 2 {
			return fmt.Errorf("usage: clipremote host rm <name>")
		}
		name := args[1]
		var next []config.Host
		for _, h := range cfg.Hosts {
			if h.Name != name && h.SSH != name {
				next = append(next, h)
			}
		}
		cfg.Hosts = next
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Printf("removed %s\n", name)
		return nil
	default:
		return fmt.Errorf("usage: clipremote host add|list|rm")
	}
}

func cmdSetup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	remote := fs.Bool("remote", false, "configure this machine as the remote side")
	_ = fs.Bool("with-xvfb", false, "deprecated (ignored)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg := loadCfg()

	// Always ensure dirs + default config
	if _, err := config.Load(); err != nil {
		return err
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	cache, _ := paths.CacheDir()
	_ = paths.EnsureDir(cache)
	hist, _ := paths.HistoryDir()
	_ = paths.EnsureDir(hist)
	cfgPath, _ := paths.ConfigFile()

	exe, _ := os.Executable()

	// Sensible auto-sync defaults
	if cfg.ScreenshotsN <= 0 {
		cfg.ScreenshotsN = 20
	}
	cfg.History = 20 // VPS keeps last 20 screenshots
	if cfg.Source == "" {
		cfg.Source = "folder"
	}
	if cfg.ScreenshotsDir == "" && runtime.GOOS == "darwin" {
		if home, err := os.UserHomeDir(); err == nil {
			cfg.ScreenshotsDir = filepath.Join(home, "Desktop")
		}
	}
	cfg.AutoPush = true
	_ = config.Save(cfg)

	if *remote {
		// Cap remote history at 10
		cfg.History = 10
		_ = config.Save(cfg)
		fmt.Println("clipremote remote setup")
		fmt.Printf("  binary:  %s\n", exe)
		fmt.Printf("  config:  %s\n", cfgPath)
		fmt.Printf("  cache:   %s (keeps last %d images)\n", cache, cfg.History)
		fmt.Printf("  latest:  %s\n", filepath.Join(cache, paths.LatestFileName))
		fmt.Println()
		fmt.Println("Put clipremote on PATH (example):")
		fmt.Printf("  mkdir -p ~/.local/bin && cp %s ~/.local/bin/clipremote\n", exe)
		fmt.Println()
		fmt.Println("In Grok always attach:")
		fmt.Printf("  @%s\n", filepath.Join(cache, paths.LatestFileName))
		return nil
	}

	// Local setup
	fmt.Println("clipremote local setup (auto-sync)")
	fmt.Printf("  binary:  %s\n", exe)
	fmt.Printf("  config:  %s\n", cfgPath)
	fmt.Printf("  watch:   %s (last %d → remote, remote keeps %d)\n", cfg.ScreenshotsDir, cfg.ScreenshotsN, cfg.History)
	fmt.Printf("  port:    %d\n", cfg.Port)
	fmt.Println()

	if runtime.GOOS == "darwin" {
		plistDir, err := writeLaunchdPlist(exe, cfg.Port)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  launchd: %v\n", err)
		} else {
			fmt.Printf("  launchd plist written: %s\n", plistDir)
			fmt.Println("  enable with (macOS):")
			fmt.Printf("    launchctl bootout gui/$(id -u) %s 2>/dev/null\n", plistDir)
			fmt.Printf("    launchctl bootstrap gui/$(id -u) %s\n", plistDir)
			fmt.Printf("    launchctl kickstart -k gui/$(id -u)/com.clipremote.daemon\n")
			fmt.Println("  or simply:  clipremote daemon &")
		}
	} else {
		fmt.Println("Start the daemon in the background:")
		fmt.Println("  clipremote daemon &")
	}

	fmt.Println()
	fmt.Println("One-time:")
	fmt.Println("  clipremote host add box user@hostname")
	fmt.Println("  # daemon watches screenshots and auto-pushes new ones")
	fmt.Println("  # in remote Grok: @~/.cache/clipremote/latest.png")
	return nil
}

func writeLaunchdPlist(exe string, port int) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	plistPath := filepath.Join(dir, "com.clipremote.daemon.plist")
	logPath := filepath.Join(home, "Library", "Logs", "clipremote.log")
	_ = os.MkdirAll(filepath.Dir(logPath), 0o755)

	// Absolute binary path; include Homebrew + local bin for ssh/pngpaste
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.clipremote.daemon</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>daemon</string>
    <string>--port</string>
    <string>%d</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ThrottleInterval</key>
  <integer>5</integer>
  <key>ProcessType</key>
  <string>Background</string>
  <key>WorkingDirectory</key>
  <string>%s</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    <key>HOME</key>
    <string>%s</string>
  </dict>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, exe, port, home, home, logPath, logPath)
	if err := os.WriteFile(plistPath, []byte(content), 0o644); err != nil {
		return "", err
	}
	return plistPath, nil
}

// cmdInstallService installs and starts the login LaunchAgent (macOS) so the
// daemon survives reboots. On Linux, prints a systemd user unit hint.
func cmdInstallService(args []string) error {
	cfg := loadCfg()
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// Resolve symlinks so launchd gets a stable path
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	if runtime.GOOS == "darwin" {
		plistPath, err := writeLaunchdPlist(exe, cfg.Port)
		if err != nil {
			return err
		}
		uid := os.Getuid()
		domain := fmt.Sprintf("gui/%d", uid)
		label := "com.clipremote.daemon"

		// bootout old (ignore errors), bootstrap, kickstart
		_ = exec.Command("launchctl", "bootout", domain, plistPath).Run()
		if out, err := exec.Command("launchctl", "bootstrap", domain, plistPath).CombinedOutput(); err != nil {
			// fallback to legacy load
			if out2, err2 := exec.Command("launchctl", "load", "-w", plistPath).CombinedOutput(); err2 != nil {
				return fmt.Errorf("launchctl bootstrap failed: %v (%s); load failed: %v (%s)",
					err, strings.TrimSpace(string(out)), err2, strings.TrimSpace(string(out2)))
			}
		}
		_ = exec.Command("launchctl", "enable", domain+"/"+label).Run()
		_ = exec.Command("launchctl", "kickstart", "-k", domain+"/"+label).Run()

		fmt.Println("clipremote daemon installed for login (survives reboot)")
		fmt.Printf("  plist:  %s\n", plistPath)
		fmt.Printf("  logs:   ~/Library/Logs/clipremote.log\n")
		fmt.Println("  check:  launchctl print gui/$(id -u)/com.clipremote.daemon | head")
		fmt.Println("  hosts:  clipremote host list   # must list your VPS")
		return nil
	}

	// Linux user systemd unit
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		return err
	}
	unitPath := filepath.Join(unitDir, "clipremote-daemon.service")
	unit := fmt.Sprintf(`[Unit]
Description=clipremote screenshot auto-sync daemon
After=network-online.target

[Service]
ExecStart=%s daemon --port %d
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`, exe, cfg.Port)
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return err
	}
	fmt.Println("wrote", unitPath)
	fmt.Println("enable with:")
	fmt.Println("  systemctl --user daemon-reload")
	fmt.Println("  systemctl --user enable --now clipremote-daemon.service")
	fmt.Println("  loginctl enable-linger $USER   # optional: run without login")
	return nil
}

func cmdDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	remote := fs.Bool("remote", false, "force remote checks")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg := loadCfg()
	// Heuristic: remote if Linux and no daemon expected — user flag preferred
	isRemote := *remote || runtime.GOOS == "linux" && os.Getenv("CLIPREMOTE_ROLE") == "remote"
	// Better: if CLIPREMOTE_ROLE not set, run both-ish based on GOOS
	if !*remote {
		// On linux default to remote-oriented doctor (this is the common server case)
		if runtime.GOOS == "linux" {
			isRemote = true
		}
	}
	code := doctor.Run(os.Stdout, cfg, isRemote)
	if code != 0 {
		os.Exit(code)
	}
	return nil
}

func cmdXvfb(args []string) error {
	fs := flag.NewFlagSet("xvfb", flag.ContinueOnError)
	display := fs.String("display", ":99", "X display")
	if err := fs.Parse(args); err != nil {
		return err
	}
	d, err := xvfb.Ensure(*display)
	if err != nil {
		return err
	}
	fmt.Println(d)
	fmt.Fprint(os.Stderr, xvfb.ShellSnippet(d))
	return nil
}
