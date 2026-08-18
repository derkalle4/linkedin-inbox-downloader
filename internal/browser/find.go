package browser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Found is a discovered Chromium-based browser.
type Found struct {
	Name string
	Path string
}

// Find locates Edge / Chrome / Chromium. On Windows, Edge is preferred.
func Find() (*Found, error) {
	candidates := candidatesForOS()
	for _, c := range candidates {
		if path, ok := lookFor(c); ok {
			return &Found{Name: c.name, Path: path}, nil
		}
	}
	return nil, fmt.Errorf("%s", missingMessage())
}

type candidate struct {
	name  string
	paths []string // absolute paths and/or PATH names
}

func candidatesForOS() []candidate {
	switch runtime.GOOS {
	case "windows":
		local := os.Getenv("LOCALAPPDATA")
		prog := os.Getenv("PROGRAMFILES")
		prog86 := os.Getenv("PROGRAMFILES(X86)")
		return []candidate{
			{name: "Microsoft Edge", paths: []string{
				filepath.Join(prog, "Microsoft", "Edge", "Application", "msedge.exe"),
				filepath.Join(prog86, "Microsoft", "Edge", "Application", "msedge.exe"),
				"msedge",
			}},
			{name: "Google Chrome", paths: []string{
				filepath.Join(prog, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(prog86, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(local, "Google", "Chrome", "Application", "chrome.exe"),
				"chrome",
			}},
		}
	default: // linux and others — prefer Chrome so the UA matches a typical install
		return []candidate{
			{name: "Google Chrome", paths: []string{"google-chrome", "google-chrome-stable", "google-chrome-beta"}},
			{name: "Chromium", paths: []string{"chromium", "chromium-browser"}},
			{name: "Microsoft Edge", paths: []string{"microsoft-edge", "microsoft-edge-stable", "microsoft-edge-beta"}},
		}
	}
}

func lookFor(c candidate) (string, bool) {
	for _, p := range c.paths {
		if p == "" {
			continue
		}
		if filepath.IsAbs(p) {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p, true
			}
			continue
		}
		if path, err := exec.LookPath(p); err == nil {
			return path, true
		}
	}
	return "", false
}

func missingMessage() string {
	if runtime.GOOS == "windows" {
		return "No supported browser found. This app uses Microsoft Edge (already on most Windows PCs) or Google Chrome. Please install Edge or Chrome and try again."
	}
	return "No supported browser found. Please install Microsoft Edge, Google Chrome, or Chromium, then try again."
}

// DisplayName returns a short label for UI messages.
func (f *Found) DisplayName() string {
	if f == nil {
		return "browser"
	}
	return f.Name
}

// IsEdge reports whether the found browser is Edge.
func (f *Found) IsEdge() bool {
	return f != nil && strings.Contains(strings.ToLower(f.Name), "edge")
}
