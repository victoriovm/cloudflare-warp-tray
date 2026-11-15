package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type connectionState int

const (
	unknown connectionState = iota
	disconnected
	connecting
	connected
)

func parseStatus(output string) connectionState {
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok || !strings.EqualFold(key, "Status update") {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "connected":
			return connected
		case "disconnected":
			return disconnected
		case "connecting", "reconnecting", "disconnecting":
			return connecting
		}
	}
	return unknown
}

func findCLI() (string, error) {
	if path, err := exec.LookPath("warp-cli.exe"); err == nil {
		return path, nil
	}
	for _, variable := range []string{"ProgramW6432", "ProgramFiles", "ProgramFiles(x86)"} {
		if base := os.Getenv(variable); base != "" {
			path := filepath.Join(base, "Cloudflare", "Cloudflare WARP", "warp-cli.exe")
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path, nil
			}
		}
	}
	return "", fmt.Errorf("warp-cli.exe not found; install Cloudflare WARP")
}

func runCLI(path, command string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--no-ansi", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return string(output), fmt.Errorf("warp-cli %s: %w", command, ctx.Err())
	}
	if err != nil {
		return string(output), fmt.Errorf("warp-cli %s: %w: %s", command, err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}
