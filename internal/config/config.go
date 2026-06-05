package config

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Editor                      string `toml:"editor"`
	PathTemplate                string `toml:"path_template"`
	MainBranch                  string `toml:"main_branch"`
	SkipShellIntegrationWelcome bool   `toml:"skip_shell_integration_welcome"`
}

func Default() Config {
	return Config{
		Editor:       defaultEditor(),
		PathTemplate: "{repo_parent}/{branch}",
	}
}

func LoadDefault() (Config, error) {
	config := Default()
	path, err := Path()
	if err != nil {
		return config, nil
	}
	if _, err := os.Stat(path); err != nil {
		return config, nil
	}
	return Load(path)
}

func Path() (string, error) {
	if configHome := os.Getenv("XDG_CONFIG_HOME"); configHome != "" {
		return filepath.Join(configHome, "gwt", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "gwt", "config.toml"), nil
}

func SaveDefault(config Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	var buffer bytes.Buffer
	if err := toml.NewEncoder(&buffer).Encode(config); err != nil {
		return err
	}
	return os.WriteFile(path, buffer.Bytes(), 0600)
}

func PatchDefault(update func(*Config)) error {
	config, err := LoadDefault()
	if err != nil {
		return err
	}
	update(&config)
	return SaveDefault(config)
}

func Load(path string) (Config, error) {
	config := Default()
	if _, err := toml.DecodeFile(path, &config); err != nil {
		return config, err
	}
	if config.Editor == "" {
		config.Editor = defaultEditor()
	}
	if config.PathTemplate == "" {
		config.PathTemplate = "{repo_parent}/{branch}"
	}
	return config, nil
}

func defaultEditor() string {
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	return "code"
}
