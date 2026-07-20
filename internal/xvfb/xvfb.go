package xvfb

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const defaultDisplay = ":99"

// Ensure starts Xvfb if needed and returns DISPLAY value.
func Ensure(display string) (string, error) {
	if display == "" {
		display = defaultDisplay
	}
	if !strings.HasPrefix(display, ":") {
		display = ":" + display
	}

	// Already have a working display?
	if os.Getenv("DISPLAY") != "" && os.Getenv("CLIPREMOTE_FORCE_XVFB") == "" {
		// trust existing
		return os.Getenv("DISPLAY"), nil
	}

	if _, err := exec.LookPath("Xvfb"); err != nil {
		return "", fmt.Errorf("Xvfb not installed (apt install xvfb)")
	}

	// Check if display already up
	if xDisplayLive(display) {
		return display, nil
	}

	cmd := exec.Command("Xvfb", display, "-screen", "0", "1280x720x24", "-ac")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start Xvfb: %w", err)
	}
	// give it a moment
	time.Sleep(200 * time.Millisecond)
	if !xDisplayLive(display) {
		return "", fmt.Errorf("Xvfb started but display %s not ready", display)
	}
	return display, nil
}

func xDisplayLive(display string) bool {
	cmd := exec.Command("xdpyinfo", "-display", display)
	cmd.Env = append(os.Environ(), "DISPLAY="+display)
	return cmd.Run() == nil
}

// ShellSnippet returns env lines users can source.
func ShellSnippet(display string) string {
	if display == "" {
		display = defaultDisplay
	}
	return fmt.Sprintf("export DISPLAY=%s\n# clipremote: use this DISPLAY so Grok/xclip share the virtual clipboard\n", display)
}

// DisplayNum parses :N
func DisplayNum(display string) int {
	display = strings.TrimPrefix(display, ":")
	n, _ := strconv.Atoi(display)
	return n
}
