package gitdata

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type Runner interface {
	Run(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

type CommandError struct {
	Name   string
	Args   []string
	Stderr string
	Err    error
}

func (error CommandError) Error() string {
	if strings.TrimSpace(error.Stderr) != "" {
		return strings.TrimSpace(error.Stderr)
	}
	return fmt.Sprintf("%s %s failed: %v", error.Name, strings.Join(error.Args, " "), error.Err)
}

func (error CommandError) Unwrap() error {
	return error.Err
}

func (runner ExecRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return output, CommandError{Name: name, Args: args, Stderr: stderr.String(), Err: err}
	}
	return output, nil
}

func IsCommandFailure(err error) bool {
	var commandError CommandError
	return errors.As(err, &commandError)
}
