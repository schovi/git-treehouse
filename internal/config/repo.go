package config

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type RepoConfig struct {
	PathTemplate  string   `toml:"path_template"`
	CopyUntracked []string `toml:"copy_untracked"`
	PostCreate    string   `toml:"post_create"`
	BeforeDelete  string   `toml:"before_delete"`
}

func LoadRepoConfig(repoRoot string) (RepoConfig, error) {
	var config RepoConfig
	_, err := toml.DecodeFile(filepath.Join(repoRoot, ".worktree"), &config)
	if err != nil {
		if os.IsNotExist(err) {
			return RepoConfig{}, nil
		}
		return config, err
	}
	return config, nil
}

func (config RepoConfig) HasHooks() bool {
	return config.PostCreate != "" || config.BeforeDelete != ""
}

func HookHash(config RepoConfig) string {
	hash := sha256.Sum256([]byte("post_create=" + config.PostCreate + "\x00before_delete=" + config.BeforeDelete))
	return hex.EncodeToString(hash[:])
}

func EffectivePathTemplate(global Config, repo RepoConfig) string {
	if repo.PathTemplate != "" {
		return repo.PathTemplate
	}
	return global.PathTemplate
}
