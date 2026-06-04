package shellinit

import (
	"strings"
	"testing"
)

func TestScriptReturnsPosixWrapperForZshAndBash(t *testing.T) {
	zshScript, err := Script("zsh")
	if err != nil {
		t.Fatalf("Script(zsh) error = %v", err)
	}
	bashScript, err := Script("bash")
	if err != nil {
		t.Fatalf("Script(bash) error = %v", err)
	}
	if zshScript != bashScript {
		t.Fatalf("zsh and bash scripts differ")
	}

	for _, want := range []string{
		"gwt() {",
		`command gwt --cd-file "$cd_file" "$@"`,
		`cd "$target" || return`,
		`return "$status"`,
	} {
		if !strings.Contains(zshScript, want) {
			t.Fatalf("posix script missing %q:\n%s", want, zshScript)
		}
	}
}

func TestScriptReturnsFishWrapper(t *testing.T) {
	script, err := Script("fish")
	if err != nil {
		t.Fatalf("Script(fish) error = %v", err)
	}

	for _, want := range []string{
		"function gwt",
		"command gwt --cd-file $cd_file $argv",
		"set gwt_status $status",
		`cd "$target"`,
		"return $gwt_status",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("fish script missing %q:\n%s", want, script)
		}
	}
}

func TestScriptRejectsUnsupportedShell(t *testing.T) {
	_, err := Script("powershell")
	if err == nil {
		t.Fatalf("Script(powershell) error = nil, want unsupported shell error")
	}
	if !strings.Contains(err.Error(), "unsupported shell") {
		t.Fatalf("Script(powershell) error = %q, want unsupported shell message", err.Error())
	}
}
