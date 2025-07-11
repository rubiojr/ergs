package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

//go:embed config.toml.sample
var configTemplate string

type Config struct {
	StorageDir    string                    `toml:"storage_dir"`
	FetchInterval Duration                  `toml:"fetch_interval"`
	Datasources   map[string]DatasourceInfo `toml:"datasources"`
}

type Duration struct {
	time.Duration
}

func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

func (d *Duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

type DatasourceInfo struct {
	Type string `toml:"type"`
	// Interval specifies how often this datasource should be fetched.
	// If not specified, defaults to 30 minutes.
	Interval *Duration   `toml:"interval,omitempty"`
	Config   interface{} `toml:"config"`
}

func GetDefaultConfig() *Config {
	return &Config{
		StorageDir:    GetDefaultStorageDir(),
		FetchInterval: Duration{30 * time.Minute},
		Datasources:   make(map[string]DatasourceInfo),
	}
}

func LoadConfig(configPath string) (*Config, error) {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return GetDefaultConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var config Config
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if config.StorageDir == "" {
		config.StorageDir = GetDefaultStorageDir()
	}

	if config.FetchInterval.Duration == 0 {
		config.FetchInterval = Duration{30 * time.Minute}
	}

	if config.Datasources == nil {
		config.Datasources = make(map[string]DatasourceInfo)
	}

	return &config, nil
}

func (c *Config) SaveConfig(configPath string) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	return os.WriteFile(configPath, data, 0644)
}

func (c *Config) SaveTemplateConfig(configPath string) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	template := c.generateConfigTemplate()
	return os.WriteFile(configPath, []byte(template), 0644)
}

func (c *Config) generateConfigTemplate() string {
	storageDir := c.StorageDir
	if storageDir == "" {
		storageDir = GetDefaultStorageDir()
	}

	// Replace the placeholder storage_dir with the actual path
	template := strings.Replace(configTemplate, "/home/user/.local/share/ergs", storageDir, 1)
	return template
}

func (c *Config) AddDatasource(name, dsType string, dsConfig interface{}) error {
	info := DatasourceInfo{
		Type:   dsType,
		Config: dsConfig,
	}

	c.Datasources[name] = info
	return nil
}

func (c *Config) AddDatasourceWithInterval(name, dsType string, dsConfig interface{}, interval *Duration) error {
	info := DatasourceInfo{
		Type:     dsType,
		Interval: interval,
		Config:   dsConfig,
	}

	c.Datasources[name] = info
	return nil
}

func (c *Config) GetDatasourceConfig(name string) (string, interface{}, error) {
	info, exists := c.Datasources[name]
	if !exists {
		return "", nil, fmt.Errorf("datasource %s not found", name)
	}

	return info.Type, info.Config, nil
}

func (c *Config) GetDatasourceInterval(name string) time.Duration {
	info, exists := c.Datasources[name]
	if !exists || info.Interval == nil {
		return 30 * time.Minute // Default to 30 minutes
	}
	return info.Interval.Duration
}

func (c *Config) ListDatasources() []string {
	names := make([]string, 0, len(c.Datasources))
	for name := range c.Datasources {
		names = append(names, name)
	}
	return names
}

func (c *Config) RemoveDatasource(name string) {
	delete(c.Datasources, name)
}

// GetDefaultStorageDir returns the default storage directory for databases
func GetDefaultStorageDir() string {
	// Use XDG_DATA_HOME if set, otherwise use ~/.local/share
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			// Fallback to current directory if we can't get home dir
			return "./data"
		}
		dataDir = filepath.Join(homeDir, ".local", "share")
	}

	ergsDir := filepath.Join(dataDir, "ergs")

	// Create the directory if it doesn't exist
	if err := os.MkdirAll(ergsDir, 0755); err != nil {
		// Fallback to current directory if we can't create the data dir
		return "./data"
	}

	return ergsDir
}

// GetDefaultDBPath returns the default database path in the user's data directory
func GetDefaultDBPath() string {
	return filepath.Join(GetDefaultStorageDir(), "ergs.db")
}

// GetConfigDir returns the configuration directory for ergs
func GetConfigDir() string {
	// Use XDG_CONFIG_HOME if set, otherwise use ~/.config
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "."
		}
		configDir = filepath.Join(homeDir, ".config")
	}

	ergsConfigDir := filepath.Join(configDir, "ergs")

	// Create the directory if it doesn't exist
	if err := os.MkdirAll(ergsConfigDir, 0755); err != nil {
		return "."
	}

	return ergsConfigDir
}

// GetDefaultConfigPath returns the default configuration file path
func GetDefaultConfigPath() string {
	return filepath.Join(GetConfigDir(), "config.toml")
}
