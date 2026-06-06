#!/usr/bin/env bash
set -euo pipefail

terminal_width="${COLUMNS:-}"
if [[ -z "$terminal_width" ]]; then
	terminal_width="$(tput cols 2>/dev/null || printf '132')"
fi
if ! [[ "$terminal_width" =~ ^[0-9]+$ ]]; then
	terminal_width=132
fi

outer_width="$terminal_width"
if ((outer_width > 150)); then
	outer_width=150
fi
if ((outer_width < 80)); then
	outer_width=80
fi

box_width=$((outer_width - 4))
content_width=$((outer_width - 6))
gap=2
if ((outer_width < 112)); then
	gap=1
fi

marker_width=1
marker_gap=1
status_width=8
remote_width=7
main_width=8
age_width=5
pr_width=7
size_width=5
fixed_width=$((marker_width + marker_gap + status_width + remote_width + main_width + age_width + pr_width + size_width + (gap * 7)))
flex_width=$((content_width - fixed_width))
branch_width=$((flex_width * 45 / 100))
if ((branch_width > 36)); then
	branch_width=36
fi
if ((branch_width < 18)); then
	branch_width=18
fi
commit_width=$((flex_width - branch_width))
if ((commit_width < 12)); then
	branch_width=$((branch_width - (12 - commit_width)))
	commit_width=12
fi

if [[ "${NO_COLOR:-}" ]]; then
	blue=""
	cyan=""
	green=""
	yellow=""
	red=""
	magenta=""
	dim=""
	bold=""
	reset=""
	selected=""
else
	blue=$'\033[38;5;75m'
	cyan=$'\033[38;5;45m'
	green=$'\033[38;5;42m'
	yellow=$'\033[38;5;214m'
	red=$'\033[38;5;203m'
	magenta=$'\033[38;5;141m'
	dim=$'\033[38;5;245m'
	bold=$'\033[1m'
	reset=$'\033[0m'
	selected=$'\033[48;5;62m'
fi

row_selected=""
row_background=""
after_style="$reset"

repeat_char() {
	local char="$1"
	local count="$2"

	while ((count > 0)); do
		printf '%s' "$char"
		count=$((count - 1))
	done
}

truncate_text() {
	local text="$1"
	local width="$2"

	if ((width <= 0)); then
		return
	fi
	if ((${#text} <= width)); then
		printf '%s' "$text"
		return
	fi
	if ((width == 1)); then
		printf '…'
		return
	fi

	printf '%s…' "${text:0:width - 1}"
}

style_for_selected_row() {
	local value="$1"

	if [[ -n "$selected" && "$row_selected" == "yes" ]]; then
		printf '%s' "${value//$reset/$reset$selected}"
	else
		printf '%s' "$value"
	fi
}

begin_table_content() {
	row_selected="$1"

	if [[ "$row_selected" == "yes" ]]; then
		row_background="$selected"
		after_style="$reset$selected"
	else
		row_background=""
		after_style="$reset"
	fi

	printf '%s│ │%s' "$green" "$row_background"
}

end_table_content() {
	if [[ "$row_selected" == "yes" ]]; then
		printf '%s%s│ %s│%s\n' "$reset" "$green" "$green" "$reset"
	else
		printf '%s│ %s│%s\n' "$green" "$green" "$reset"
	fi
}

cell() {
	local width="$1"
	local plain="$2"
	local colored="${3:-$2}"
	local fallback_style="${4:-}"
	local visible
	local rendered
	local padding

	visible="$(truncate_text "$plain" "$width")"
	if [[ "$visible" != "$plain" ]]; then
		rendered="$fallback_style$visible"
	else
		rendered="$colored"
	fi
	rendered="$(style_for_selected_row "$rendered")"
	padding=$((width - ${#visible}))

	printf '%s%s' "$rendered" "$after_style"
	if ((padding > 0)); then
		printf '%*s' "$padding" ''
	fi
}

space_gap() {
	printf '%*s' "$gap" ''
}

space_marker_gap() {
	printf '%*s' "$marker_gap" ''
}

frame_title() {
	local title_plain="$1"
	local title_colored="$2"
	local filler=$((box_width - 5 - ${#title_plain}))

	printf '%s│ ╭─%s %s ' "$green" "$reset" "$title_colored"
	repeat_char '─' "$filler"
	printf '%s╮ %s│%s\n' "$green" "$green" "$reset"
}

frame_bottom() {
	printf '%s│ ╰' "$green"
	repeat_char '─' "$((box_width - 2))"
	printf '╯ │%s\n' "$reset"
}

worktrees_frame_bottom() {
	local footer_plain='h root · a active · Tab filter: all · s search'
	local label_plain=" $footer_plain "
	local label_colored=" ${blue}h${reset} root ${dim}·${reset} ${blue}a${reset} active ${dim}·${reset} ${blue}Tab${reset} filter: all ${dim}·${reset} ${blue}s${reset} search "
	local max_label_width=$((box_width - 3))
	local filler

	if ((${#label_plain} > max_label_width)); then
		footer_plain="$(truncate_text "$footer_plain" "$((max_label_width - 2))")"
		label_plain=" $footer_plain "
		label_colored="${blue}${label_plain}${reset}"
	fi

	filler=$((box_width - 3 - ${#label_plain}))
	if ((filler < 0)); then
		filler=0
	fi

	printf '%s│ ╰─%s%s%s' "$green" "$reset" "$label_colored" "$green"
	repeat_char '─' "$filler"
	printf '╯ │%s\n' "$reset"
}

top_rule() {
	local left_plain=' treehouse  git-treehouse  8 worktrees  root: codex/list-rendering-polish '
	local left_colored=" ${blue}treehouse${reset}  git-treehouse  ${dim}8 worktrees${reset}  ${dim}root:${reset} ${bold}codex/list-rendering-polish${reset} "
	local right_plain=' n new · r refresh · ? help · q quit '
	local right_colored="${dim}${right_plain}${reset}"
	local available_left=$((outer_width - 5 - ${#right_plain}))
	local filler

	if ((${#left_plain} > available_left)); then
		left_plain="$(truncate_text "$left_plain" "$available_left")"
		left_colored="$left_plain"
	fi

	filler=$((outer_width - 4 - ${#left_plain} - ${#right_plain}))
	if ((filler < 1)); then
		filler=1
	fi

	printf '%s╭─%s%s%s' "$green" "$reset" "$left_colored" "$green"
	repeat_char '─' "$filler"
	printf '%s%s─╮%s\n' "$right_colored" "$green" "$reset"
}

bottom_rule() {
	local left_plain=' Esc close/clear '
	local right_plain=' ⌂ root · ! locked · × prunable · remote ✓/-/gone · + staged · ~ modified · ? untracked '
	local left_colored="${blue}${left_plain}${reset}"
	local right_colored="${dim}${right_plain}${reset}"
	local min_left=8
	local max_right=$((outer_width - 5 - min_left))
	local available_left=$((outer_width - 5 - ${#right_plain}))
	local filler

	if ((${#right_plain} > max_right)); then
		right_plain="$(truncate_text "$right_plain" "$max_right")"
		right_colored="${dim}${right_plain}${reset}"
	fi

	available_left=$((outer_width - 5 - ${#right_plain}))
	if ((${#left_plain} > available_left)); then
		left_plain="$(truncate_text "$left_plain" "$available_left")"
		left_colored="${blue}${left_plain}${reset}"
	fi

	filler=$((outer_width - 4 - ${#left_plain} - ${#right_plain}))
	if ((filler < 1)); then
		filler=1
	fi

	printf '%s╰─%s%s%s' "$green" "$reset" "$left_colored" "$green"
	repeat_char '─' "$filler"
	printf '%s%s─╯%s\n' "$right_colored" "$green" "$reset"
}

table_header() {
	begin_table_content no
	cell "$marker_width" '' ''
	space_marker_gap
	cell "$branch_width" 'branch' "${bold}branch${reset}"
	space_gap
	cell "$status_width" 'status' "${bold}status${reset}"
	space_gap
	cell "$remote_width" 'remote' "${bold}remote${reset}"
	space_gap
	cell "$main_width" 'main±' "${bold}main±${reset}"
	space_gap
	cell "$commit_width" 'commit' "${bold}commit${reset}"
	space_gap
	cell "$age_width" 'age' "${bold}age${reset}"
	space_gap
	cell "$pr_width" 'PR' "${bold}PR${reset}"
	space_gap
	cell "$size_width" 'size' "${bold}size${reset}"
	end_table_content
}

table_row() {
	local selected_row="$1"
	local marker_plain="$2"
	local marker_colored="$3"
	local branch_plain="$4"
	local branch_colored="$5"
	local status_plain="$6"
	local status_colored="$7"
	local remote_plain="$8"
	local remote_colored="$9"
	shift 9
	local main_plain="$1"
	local main_colored="$2"
	local commit_plain="$3"
	local commit_colored="$4"
	local age_plain="$5"
	local age_colored="$6"
	local pr_plain="$7"
	local pr_colored="$8"
	local size_plain="$9"
	local size_colored="${10}"

	begin_table_content "$selected_row"
	cell "$marker_width" "$marker_plain" "$marker_colored" "$dim"
	space_marker_gap
	cell "$branch_width" "$branch_plain" "$branch_colored"
	space_gap
	cell "$status_width" "$status_plain" "$status_colored"
	space_gap
	cell "$remote_width" "$remote_plain" "$remote_colored" "$dim"
	space_gap
	cell "$main_width" "$main_plain" "$main_colored" "$dim"
	space_gap
	cell "$commit_width" "$commit_plain" "$commit_colored" "$dim"
	space_gap
	cell "$age_width" "$age_plain" "$age_colored" "$dim"
	space_gap
	cell "$pr_width" "$pr_plain" "$pr_colored" "$blue"
	space_gap
	cell "$size_width" "$size_plain" "$size_colored" "$green"
	end_table_content
}

detail_row() {
	local label="$1"
	local value_plain="$2"
	local value_colored="$3"
	local action_plain="$4"
	local action_colored="$5"
	local left_width=$(((content_width - 3) / 2))
	local right_width=$((content_width - 3 - left_width))
	local left_value_width=$((left_width - 10))

	begin_table_content no
	cell 9 "$label" "${blue}${label}${reset}"
	cell "$left_value_width" "$value_plain" "$value_colored"
	printf '%s │ ' "$green"
	cell "$right_width" "$action_plain" "$action_colored"
	end_table_content
}

top_rule
frame_title 'Worktrees' "${blue}${bold}Worktrees${reset}"
table_header
table_row yes '⌂' "${blue}⌂${reset}" \
	'codex/list-rendering-polish' "${bold}codex/list-rendering-polish${reset}" \
	'✓' "${green}✓${reset}" \
	'-' "${dim}-${reset}" \
	'↑1 ↓14' "${yellow}↑1${reset} ${red}↓14${reset}" \
	'eeaf5e5 fix: Improve list PR rendering and table layout' "${blue}eeaf5e5${reset} fix: Improve list PR rendering and table layout" \
	'7m' "${bold}7m${reset}" \
	'' '' \
	'68M' "${bold}68M${reset}"
table_row no '' '' \
	'docs/troubleshooting' 'docs/troubleshooting' \
	'✓' "${green}✓${reset}" \
	'✓' "${green}✓${reset}" \
	'↑1 ↓14' "${yellow}↑1${reset} ${red}↓14${reset}" \
	'd3c7756 docs: add troubleshooting guide' "${blue}d3c7756${reset} docs: add troubleshooting guide" \
	'30h' "${dim}30h${reset}" \
	'#1 ○' "${blue}#1 ○${reset}" \
	'163K' "${green}163K${reset}"
table_row no '' '' \
	'hotfix/panic-on-empty-list' 'hotfix/panic-on-empty-list' \
	'+1 ~1' "${green}+1${reset} ${yellow}~1${reset}" \
	'-' "${dim}-${reset}" \
	'↑2 ↓15' "${yellow}↑2${reset} ${red}↓15${reset}" \
	'761aa04 docs: note empty-list panic' "${blue}761aa04${reset} docs: note empty-list panic" \
	'33h' "${dim}33h${reset}" \
	'' '' \
	'161K' "${green}161K${reset}"
table_row no '' '' \
	'some-feature' 'some-feature' \
	'✓' "${green}✓${reset}" \
	'-' "${dim}-${reset}" \
	'↑3 ↓15' "${yellow}↑3${reset} ${red}↓15${reset}" \
	'52d1d68 spec: link some-feature notes' "${blue}52d1d68${reset} spec: link some-feature notes" \
	'33h' "${dim}33h${reset}" \
	'' '' \
	'161K' "${green}161K${reset}"
table_row no '' '' \
	'feature/login' 'feature/login' \
	'~1 ?1' "${yellow}~1${reset} ${cyan}?1${reset}" \
	'-' "${dim}-${reset}" \
	'↑1 ↓15' "${yellow}↑1${reset} ${red}↓15${reset}" \
	'1c8f928 snapshot' "${blue}1c8f928${reset} snapshot" \
	'33h' "${dim}33h${reset}" \
	'' '' \
	'161K' "${green}161K${reset}"
table_row no '!' "${magenta}!${reset}" \
	'experiment/locked locked' "experiment/locked ${magenta}locked${reset}" \
	'✓' "${green}✓${reset}" \
	'-' "${dim}-${reset}" \
	'↑1 ↓15' "${yellow}↑1${reset} ${red}↓15${reset}" \
	'1c8f928 snapshot' "${blue}1c8f928${reset} snapshot" \
	'33h' "${dim}33h${reset}" \
	'' '' \
	'160K' "${green}160K${reset}"
table_row no '' '' \
	'cd5e190 detached' "${blue}cd5e190${reset} ${cyan}detached${reset}" \
	'✓' "${green}✓${reset}" \
	'-' "${dim}-${reset}" \
	'↓15' "${red}↓15${reset}" \
	'cd5e190 feat: Polish table UI and shell integration hints' "${blue}cd5e190${reset} feat: Polish table UI and shell integration hints" \
	'33h' "${dim}33h${reset}" \
	'' '' \
	'142K' "${green}142K${reset}"
table_row no '×' "${red}×${reset}" \
	'stale/abandoned prunable' "${dim}stale/abandoned${reset} ${red}prunable${reset}" \
	'' '' \
	'gone' "${red}gone${reset}" \
	'' '' \
	'' '' \
	'' '' \
	'' '' \
	'…' "${dim}…${reset}"
worktrees_frame_bottom

frame_title 'Details' "${blue}${bold}Details${reset}"
detail_row 'Branch' 'codex/list-rendering-polish' "${bold}${blue}codex/list-rendering-polish${reset}" 'Current root repository' "${blue}${bold}Current${reset} root repository"
detail_row 'HEAD' 'eeaf5e5 on codex/list-rendering-polish' "${blue}eeaf5e5${reset} on codex/list-rendering-polish" '↵ go' "${blue}↵ go${reset}"
detail_row 'Path' '.' "${blue}.${reset}" 'o editor' 'o editor'
detail_row 'Status' 'clean' "${green}clean${reset}" 'd delete' "${blue}d delete${reset}"
detail_row 'Dirty' 'none' "${blue}none${reset}" 'y abs path' "${blue}y abs path${reset}"
detail_row 'Remote' 'no upstream' "${blue}no upstream${reset}" 'p PR' "${blue}p PR${reset}"
detail_row 'Main' '↑1 ↓14 vs local main' "${yellow}↑1${reset} ${red}↓14${reset} vs local main" '' ''
detail_row 'Commit' 'eeaf5e5 fix: Improve list PR rendering, 7m' "${blue}eeaf5e5${reset} fix: Improve list PR rendering, 7m" '' ''
detail_row 'Delete' 'blocked, active root repository' "${yellow}blocked, active root repository${reset}" '' ''
frame_bottom
bottom_rule
