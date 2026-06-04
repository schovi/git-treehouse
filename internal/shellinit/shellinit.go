package shellinit

import (
	"fmt"
)

func Script(shell string) (string, error) {
	switch shell {
	case "fish":
		return fishScript(), nil
	case "zsh", "bash":
		return posixScript(), nil
	default:
		return "", fmt.Errorf("unsupported shell %q, expected fish, zsh, or bash", shell)
	}
}

func posixScript() string {
	return `gwt() {
  local cd_file
  cd_file="$(mktemp -t gwt.XXXXXX)" || return
  command gwt --cd-file "$cd_file" "$@"
  local status=$?
  if [ -s "$cd_file" ]; then
    local target
    target="$(cat "$cd_file")"
    rm -f "$cd_file"
    if [ -n "$target" ]; then
      cd "$target" || return
    fi
  else
    rm -f "$cd_file"
  fi
  return "$status"
}
`
}

func fishScript() string {
	return `function gwt
  set cd_file (mktemp -t gwt.XXXXXX)
  command gwt --cd-file $cd_file $argv
  set gwt_status $status
  if test -s $cd_file
    set target (cat $cd_file)
    rm -f $cd_file
    if test -n "$target"
      cd "$target"
    end
  else
    rm -f $cd_file
  end
  return $gwt_status
end
`
}
