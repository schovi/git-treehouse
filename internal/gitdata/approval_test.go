package gitdata

import (
	"context"
	"errors"
	"testing"
)

func TestReadApprovedHashUsesLocalGitConfig(t *testing.T) {
	runner := fakeRunner{
		"/repo|git config --local --get treehouse.approvedHash": {output: "abc123\n"},
	}

	hash, err := ReadApprovedHash(context.Background(), "/repo", runner)
	if err != nil {
		t.Fatalf("ReadApprovedHash() error = %v", err)
	}
	if hash != "abc123" {
		t.Fatalf("ReadApprovedHash() = %q, want abc123", hash)
	}
}

func TestReadApprovedHashTreatsMissingKeyAsEmpty(t *testing.T) {
	commandError := CommandError{
		Name: "git",
		Args: []string{"config", "--local", "--get", approvedHashConfigKey},
		Err:  errors.New("exit status 1"),
	}
	runner := fakeRunner{
		"/repo|git config --local --get treehouse.approvedHash": {err: commandError},
	}

	hash, err := ReadApprovedHash(context.Background(), "/repo", runner)
	if err != nil {
		t.Fatalf("ReadApprovedHash() error = %v", err)
	}
	if hash != "" {
		t.Fatalf("ReadApprovedHash() = %q, want empty hash", hash)
	}
}

func TestWriteApprovedHashUsesLocalGitConfig(t *testing.T) {
	runner := fakeRunner{
		"/repo|git config --local treehouse.approvedHash abc123": {},
	}

	if err := WriteApprovedHash(context.Background(), "/repo", "abc123", runner); err != nil {
		t.Fatalf("WriteApprovedHash() error = %v", err)
	}
}
