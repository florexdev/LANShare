package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

type Config struct {
	DeviceID        string `json:"device_id"`
	DeviceName      string `json:"device_name"`
	OS              string `json:"os"`
	DownloadDir     string `json:"download_dir"`
	DiscoveryPort   int    `json:"discovery_port"`
	TransferPort    int    `json:"transfer_port"`
	UIPort          int    `json:"ui_port"`
	E2EEEnabled     bool   `json:"e2ee_enabled"`
	AutoAccept      bool   `json:"auto_accept"`
	SecretKey       string `json:"secret_key,omitempty"`
	configPath      string
	mu              sync.RWMutex
}

func DefaultConfig() (*Config, error) {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "LANShare-Device"
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}

	downloadDir := filepath.Join(homeDir, "Downloads", "LANShare")
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		downloadDir = filepath.Join(homeDir, "Downloads")
	}

	appConfigDir := filepath.Join(homeDir, ".lanshare")
	_ = os.MkdirAll(appConfigDir, 0755)
	configPath := filepath.Join(appConfigDir, "config.json")

	cfg := &Config{
		DeviceID:      fmt.Sprintf("%s-%d", hostname, os.Getpid()),
		DeviceName:    hostname,
		OS:            runtime.GOOS,
		DownloadDir:   downloadDir,
		DiscoveryPort: 52637,
		TransferPort:  52638,
		UIPort:        52639,
		E2EEEnabled:   true,
		AutoAccept:    false,
		configPath:    configPath,
	}

	// Load existing config if available
	if err := cfg.Load(); err == nil {
		// Loaded successfully
	} else {
		_ = cfg.Save()
	}

	return cfg, nil
}

func (c *Config) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile(c.configPath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, c)
}

func (c *Config) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.configPath, data, 0644)
}

func (c *Config) Update(name string, downloadDir string, e2ee bool, autoAccept bool) error {
	c.mu.Lock()
	if name != "" {
		c.DeviceName = name
	}
	if downloadDir != "" {
		c.DownloadDir = downloadDir
		_ = os.MkdirAll(downloadDir, 0755)
	}
	c.E2EEEnabled = e2ee
	c.AutoAccept = autoAccept
	c.mu.Unlock()

	return c.Save()
}
