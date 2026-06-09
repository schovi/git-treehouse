package gitdata

import (
	"context"
	"strings"
)

const approvedHashConfigKey = "treehouse.approvedHash"

func ReadApprovedHash(ctx context.Context, repoRoot string, runner Runner) (string, error) {
	output, err := runner.Run(ctx, repoRoot, "git", "config", "--local", "--get", approvedHashConfigKey)
	if err != nil {
		if IsCommandFailure(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func WriteApprovedHash(ctx context.Context, repoRoot, hash string, runner Runner) error {
	_, err := runner.Run(ctx, repoRoot, "git", "config", "--local", approvedHashConfigKey, hash)
	return err
}
