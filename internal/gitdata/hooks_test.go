package gitdata

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type hookRecordingRunner struct {
	runCalled bool
	dir       string
	env       []string
	name      string
	args      []string
	err       error
}

func (runner *hookRecordingRunner) Run(_ context.Context, _ string, _ string, _ ...string) ([]byte, error) {
	runner.runCalled = true
	return nil, errors.New("Run called")
}

func (runner *hookRecordingRunner) RunWithEnv(_ context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	runner.dir = dir
	runner.env = append([]string(nil), env...)
	runner.name = name
	runner.args = append([]string(nil), args...)
	return nil, runner.err
}

func TestRunHookUsesShellCommandWithEnv(t *testing.T) {
	runner := &hookRecordingRunner{}
	env := []string{"GTH_SOURCE=/repo/main", "GTH_DEST=/repo/feature"}

	err := RunHook(context.Background(), "/repo/feature", "npm install", env, runner)
	if err != nil {
		t.Fatalf("RunHook() error = %v", err)
	}
	if runner.runCalled {
		t.Fatal("RunHook() called Run(), want RunWithEnv()")
	}
	if runner.dir != "/repo/feature" {
		t.Fatalf("RunHook() dir = %q, want /repo/feature", runner.dir)
	}
	if !reflect.DeepEqual(runner.env, env) {
		t.Fatalf("RunHook() env = %#v, want %#v", runner.env, env)
	}
	if runner.name != "sh" {
		t.Fatalf("RunHook() name = %q, want sh", runner.name)
	}
	wantArgs := []string{"-c", "npm install"}
	if !reflect.DeepEqual(runner.args, wantArgs) {
		t.Fatalf("RunHook() args = %#v, want %#v", runner.args, wantArgs)
	}
}

func TestRunHookReturnsRunnerError(t *testing.T) {
	wantErr := errors.New("hook failed")
	runner := &hookRecordingRunner{err: wantErr}

	err := RunHook(context.Background(), "/repo/feature", "npm install", nil, runner)
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunHook() error = %v, want %v", err, wantErr)
	}
}
