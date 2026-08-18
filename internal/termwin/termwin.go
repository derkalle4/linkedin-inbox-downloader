//go:build !windows

package termwin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/term"
)

const envMarker = "LINKEDIN_INBOX_IN_TERM"

// Setup opens a new terminal window when the binary was double-clicked
// (no TTY). If already running inside a terminal, this is a no-op.
func Setup() {
	if os.Getenv(envMarker) == "1" {
		return
	}
	if hasTTY() {
		return
	}
	if err := openInTerminal(); err != nil {
		notifyLaunchFailure(err)
		fmt.Fprintf(os.Stderr, "LinkedIn Inbox Downloader needs a terminal.\n%v\n", err)
		fmt.Fprintf(os.Stderr, "Open a terminal and run:\n  %s\n", os.Args[0])
		os.Exit(1)
	}
	os.Exit(0)
}

func hasTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) || term.IsTerminal(int(os.Stdout.Fd()))
}

func openInTerminal() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}

	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return fmt.Errorf("no graphical display (DISPLAY/WAYLAND_DISPLAY not set)")
	}

	env := append(os.Environ(), envMarker+"=1")
	args := os.Args[1:]
	dir := filepath.Dir(exe)

	type launcher struct {
		bin  string
		argv []string
	}

	// Prefer common desktop terminals; geometry ≈ Windows console size.
	candidates := []launcher{
		{"gnome-terminal", append([]string{"--geometry=110x36", "--working-directory=" + dir, "--", exe}, args...)},
		{"ptyxis", append([]string{"--geometry=110x36", "-x", exe}, args...)},
		{"kgx", append([]string{"--", exe}, args...)},
		{"konsole", append([]string{"--workdir", dir, "-e", exe}, args...)},
		{"xfce4-terminal", append([]string{fmt.Sprintf("--working-directory=%s", dir), "--geometry=110x36", "-e", exe}, args...)},
		{"mate-terminal", append([]string{fmt.Sprintf("--working-directory=%s", dir), "--geometry=110x36", "-e", exe}, args...)},
		{"tilix", append([]string{fmt.Sprintf("--working-directory=%s", dir), "-e", exe}, args...)},
		{"lxterminal", append([]string{fmt.Sprintf("--working-directory=%s", dir), "-e", exe}, args...)},
		{"x-terminal-emulator", append([]string{"-e", exe}, args...)},
		{"xterm", append([]string{"-geometry", "110x36", "-T", "LinkedIn Inbox Downloader", "-e", exe}, args...)},
	}

	var tried []string
	for _, c := range candidates {
		path, err := exec.LookPath(c.bin)
		if err != nil {
			continue
		}
		tried = append(tried, c.bin)
		cmd := exec.Command(path, c.argv...)
		cmd.Env = env
		cmd.Dir = dir
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			continue
		}
		_ = cmd.Process.Release()
		return nil
	}

	if len(tried) == 0 {
		return fmt.Errorf("no terminal emulator found (install gnome-terminal, konsole, xfce4-terminal, or xterm)")
	}
	return fmt.Errorf("could not start a terminal (tried: %s)", strings.Join(tried, ", "))
}

func notifyLaunchFailure(err error) {
	msg := fmt.Sprintf("Could not open a terminal window.\n%v", err)
	if path, lookErr := exec.LookPath("notify-send"); lookErr == nil {
		_ = exec.Command(path, "--urgency=critical", "LinkedIn Inbox Downloader", msg).Start()
	}
}
