package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const AppName = "localcode"

type ModelProfile struct {
	Name      string `yaml:"name"`
	Context   int    `yaml:"context"`
	KeepAlive string `yaml:"keep_alive"`
}

type ProviderConfig struct {
	Type    string `yaml:"type"`
	BaseURL string `yaml:"base_url"`
}

type ModelsConfig struct {
	Fast  ModelProfile `yaml:"fast"`
	Smart ModelProfile `yaml:"smart"`
	Code  ModelProfile `yaml:"code"`
}

type RoutingConfig struct {
	Default              string `yaml:"default"`
	OneModelPerRequest   bool   `yaml:"one_model_per_request"`
}

type SafetyConfig struct {
	RequirePatchApproval             bool `yaml:"require_patch_approval"`
	RequireDangerousCommandApproval  bool `yaml:"require_dangerous_command_approval"`
	RestrictToRepo                   bool `yaml:"restrict_to_repo"`
}

type Config struct {
	Provider ProviderConfig `yaml:"provider"`
	Models   ModelsConfig   `yaml:"models"`
	Routing  RoutingConfig  `yaml:"routing"`
	Safety   SafetyConfig   `yaml:"safety"`
}

func Default() *Config {
	return &Config{
		Provider: ProviderConfig{
			Type:    "ollama",
			BaseURL: "http://127.0.0.1:11434",
		},
		Models: ModelsConfig{
			Fast: ModelProfile{
				Name:      "qwen3.5:9b",
				Context:   16384,
				KeepAlive: "2m",
			},
			Smart: ModelProfile{
				Name:      "qwen3.8:27b",
				Context:   8192,
				KeepAlive: "2m",
			},
			Code: ModelProfile{
				// Installed tag on this machine; coding-specific tag can replace later.
				Name:      "qwen3.6:27b",
				Context:   8192,
				KeepAlive: "2m",
			},
		},
		Routing: RoutingConfig{
			Default:            "auto",
			OneModelPerRequest: true,
		},
		Safety: SafetyConfig{
			RequirePatchApproval:            true,
			RequireDangerousCommandApproval: true,
			RestrictToRepo:                  true,
		},
	}
}

func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, AppName), nil
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

func DataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	// Prefer XDG data on Linux when available.
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, AppName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(base, AppName, "data"), nil
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "share")); err == nil {
		return filepath.Join(home, ".local", "share", AppName), nil
	}
	return filepath.Join(base, AppName, "data"), nil
}

func Load() (*Config, error) {
	cfg := Default()
	path, err := Path()
	if err != nil {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.applyDefaults()
	return cfg, nil
}

func (c *Config) applyDefaults() {
	d := Default()
	if c.Provider.Type == "" {
		c.Provider.Type = d.Provider.Type
	}
	if c.Provider.BaseURL == "" {
		c.Provider.BaseURL = d.Provider.BaseURL
	}
	if c.Models.Fast.Name == "" {
		c.Models.Fast = d.Models.Fast
	}
	if c.Models.Smart.Name == "" {
		c.Models.Smart = d.Models.Smart
	}
	if c.Models.Code.Name == "" {
		c.Models.Code = d.Models.Code
	}
	if c.Routing.Default == "" {
		c.Routing.Default = d.Routing.Default
	}
}

func WriteDefault() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "config.yaml")
	data, err := yaml.Marshal(Default())
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return "", err
	}
	dataDir, err := DataDir()
	if err == nil {
		_ = os.MkdirAll(filepath.Join(dataDir, "sessions"), 0o750)
	}
	return path, nil
}

func (c *Config) Profile(mode string) (ModelProfile, error) {
	switch mode {
	case "fast":
		return c.Models.Fast, nil
	case "smart":
		return c.Models.Smart, nil
	case "code":
		return c.Models.Code, nil
	default:
		return ModelProfile{}, fmt.Errorf("unknown mode %q (use fast|smart|code)", mode)
	}
}
