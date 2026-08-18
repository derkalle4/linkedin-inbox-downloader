package paths

import (
	"os"
	"path/filepath"
)

// Dir returns the directory containing the running executable.
func Dir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// Join joins path elements to the executable directory.
func Join(elem ...string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	parts := append([]string{dir}, elem...)
	return filepath.Join(parts...), nil
}

// ConfigFile returns the path to config.yaml next to the executable.
func ConfigFile() (string, error) {
	return Join("config.yaml")
}

// CookiesFile returns the path to linkedin_cookies.json next to the executable.
func CookiesFile() (string, error) {
	return Join("linkedin_cookies.json")
}

// ResolveDownloadPath resolves a configured download path relative to the executable dir.
func ResolveDownloadPath(configured string) (string, error) {
	if configured == "" {
		configured = "archive"
	}
	if filepath.IsAbs(configured) {
		return configured, nil
	}
	return Join(configured)
}
