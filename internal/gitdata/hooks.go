package gitdata

import "context"

func RunHook(ctx context.Context, dir, command string, env []string, runner Runner) error {
	_, err := runner.RunWithEnv(ctx, dir, env, "sh", "-c", command)
	return err
}
