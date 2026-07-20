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
	"github.com/vulcanhelix/clipremote/internal/sshutil"
	"github.com/vulcanhelix/clipremote/internal/xvfb"
)

var version = "0.1.0"

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
	fmt.Fprintf(os.Stderr, `clipremote — paste local clipboard images into remote agent TUIs (Grok, Claude, Codex, …)

Usage:
  clipremote <command> [flags]

Local (Mac laptop):
  setup                 Install config + launchd plist hints
  daemon                Run clipboard watcher + pull HTTP server
  push [host]           Push current clipboard image to host(s)
  host add|list|rm      Manage configured hosts
  ssh <target> [args]   SSH with ControlMaster + reverse tunnel + auto-push

Remote (Linux host):
  setup --remote        Create cache dirs; print PATH/Xvfb tips
  ingest [--clipboard]  Read image from stdin → ~/.cache/clipremote/latest.png
  paste                 Pull image from reverse-tunneled local daemon
  latest                Print path to latest.png
  xvfb                  Start optional virtual display for true Ctrl+V

Both:
  doctor                Diagnose clipboard, daemon, tunnel, last ingest
  version               Print version

Typical flow:
  # Mac
  clipremote setup
  clipremote daemon &          # or launchd
  clipremote host add box user@server
  clipremote ssh box

  # Copy a screenshot on Mac — it auto-pushes.
  # In remote Grok: Ctrl+V  (or @~/.cache/clipremote/latest.png)

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
	cfg := loadCfg()
	img, err := clipboard.ReadImage()
	if err != nil {
		return fmt.Errorf("read local clipboard: %w", err)
	}

	var targets []string
	if len(args) > 0 {
		for _, a := range args {
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
			// fall back to all configured hosts
			for _, h := range cfg.Hosts {
				targets = append(targets, h.SSH)
			}
		}
	}
	if len(targets) == 0 {
		return fmt.Errorf("no targets: pass a host, or: clipremote host add NAME user@host, then clipremote ssh NAME")
	}

	var errs []string
	for _, t := range targets {
		if err := push.ToHost(t, img.PNG); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", t, err))
			fmt.Fprintf(os.Stderr, "push %s: %v\n", t, err)
		} else {
			fmt.Printf("pushed %d bytes → %s\n", len(img.PNG), t)
		}
	}
	if len(errs) == len(targets) {
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
	withXvfb := fs.Bool("with-xvfb", false, "print/install Xvfb tips on remote")
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

	if *remote {
		fmt.Println("clipremote remote setup")
		fmt.Printf("  binary:  %s\n", exe)
		fmt.Printf("  config:  %s\n", cfgPath)
		fmt.Printf("  cache:   %s\n", cache)
		fmt.Printf("  latest:  %s\n", filepath.Join(cache, paths.LatestFileName))
		fmt.Println()
		fmt.Println("Put clipremote on PATH (example):")
		fmt.Printf("  mkdir -p ~/.local/bin && cp %s ~/.local/bin/clipremote\n", exe)
		fmt.Println()
		fmt.Println("Clipboard (for true Ctrl+V in GUI sessions):")
		fmt.Println("  # Debian/Ubuntu")
		fmt.Println("  sudo apt install -y xclip wl-clipboard")
		if *withXvfb {
			fmt.Println()
			fmt.Println("Headless true paste (optional Xvfb):")
			fmt.Println("  sudo apt install -y xvfb xclip")
			fmt.Println("  clipremote xvfb")
			fmt.Println("  export DISPLAY=:99   # in the shell that runs grok")
			fmt.Print(xvfb.ShellSnippet(":99"))
		}
		fmt.Println()
		fmt.Println("Fallback (always works): attach the stable path in Grok")
		fmt.Printf("  @%s\n", filepath.Join(cache, paths.LatestFileName))
		return nil
	}

	// Local setup
	fmt.Println("clipremote local setup")
	fmt.Printf("  binary:  %s\n", exe)
	fmt.Printf("  config:  %s\n", cfgPath)
	fmt.Printf("  port:    %d\n", cfg.Port)
	fmt.Println()

	if runtime.GOOS == "darwin" {
		plistDir, err := writeLaunchdPlist(exe, cfg.Port)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  launchd: %v\n", err)
		} else {
			fmt.Printf("  launchd plist written: %s\n", plistDir)
			fmt.Println("  load with:")
			fmt.Printf("    launchctl load %s\n", plistDir)
		}
		fmt.Println()
		fmt.Println("Optional: brew install pngpaste")
	} else {
		fmt.Println("Start the daemon in the background:")
		fmt.Println("  clipremote daemon &")
		fmt.Println("Or a user systemd unit (example in scripts/).")
	}

	fmt.Println()
	fmt.Println("Next:")
	fmt.Println("  clipremote host add mybox user@hostname")
	fmt.Println("  clipremote ssh mybox")
	fmt.Println("  # copy a screenshot, then Ctrl+V in remote Grok")
	fmt.Println("  # or: @~/.cache/clipremote/latest.png")
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
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, exe, port, logPath, logPath)
	if err := os.WriteFile(plistPath, []byte(content), 0o644); err != nil {
		return "", err
	}
	return plistPath, nil
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
