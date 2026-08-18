package config

import (
	"errors"
	"os"

	"github.com/derkalle4/linkedin-inbox-downloader/internal/paths"
	"gopkg.in/yaml.v3"
)

const DefaultDownloadPath = "archive"

// Config is stored as config.yaml next to the executable.
type Config struct {
	DisclaimerAccepted bool   `yaml:"disclaimer_accepted"`
	DownloadPath       string `yaml:"download_path"`
}

// Load reads config.yaml. Returns (nil, nil) if the file does not exist.
func Load() (*Config, error) {
	path, err := paths.ConfigFile()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.DownloadPath == "" {
		cfg.DownloadPath = DefaultDownloadPath
	}
	return &cfg, nil
}

// Save writes config.yaml next to the executable.
func Save(cfg *Config) error {
	path, err := paths.ConfigFile()
	if err != nil {
		return err
	}
	if cfg.DownloadPath == "" {
		cfg.DownloadPath = DefaultDownloadPath
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Accepted returns true when a saved config has disclaimer_accepted: true.
func Accepted() (bool, *Config, error) {
	cfg, err := Load()
	if err != nil {
		return false, nil, err
	}
	if cfg == nil || !cfg.DisclaimerAccepted {
		return false, cfg, nil
	}
	return true, cfg, nil
}

// AcceptAndSave writes a fresh accepted config with the default download path.
func AcceptAndSave() (*Config, error) {
	cfg := &Config{
		DisclaimerAccepted: true,
		DownloadPath:       DefaultDownloadPath,
	}
	if err := Save(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// WipeSession deletes config.yaml and linkedin_cookies.json (used on disclaimer decline).
func WipeSession() error {
	cfgPath, err := paths.ConfigFile()
	if err != nil {
		return err
	}
	cookiePath, err := paths.CookiesFile()
	if err != nil {
		return err
	}
	_ = os.Remove(cfgPath)
	_ = os.Remove(cookiePath)
	return nil
}

// EnsureDownloadDir creates the download directory if needed and returns its absolute path.
func EnsureDownloadDir(cfg *Config) (string, error) {
	dir, err := paths.ResolveDownloadPath(cfg.DownloadPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}
