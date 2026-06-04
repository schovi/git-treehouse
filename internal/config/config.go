package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Editor       string `toml:"editor"`
	PathTemplate string `toml:"path_template"`
	MainBranch   string `toml:"main_branch"`
}

func Default() Config {
	return Config{
		Editor:       defaultEditor(),
		PathTemplate: "{repo_parent}/{branch}",
	}
}

func LoadDefault() (Config, error) {
	config := Default()
	home, err := os.UserHomeDir()
	if err != nil {
		return config, nil
	}
	path := filepath.Join(home, ".config", "gwt", "config.toml")
	if _, err := os.Stat(path); err != nil {
		return config, nil
	}
	return Load(path)
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
