package gitdata

import (
	"strconv"
	"strings"
	"time"
)

func ParseWorktreeList(output string) []Worktree {
	blocks := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n\n")
	worktrees := make([]Worktree, 0, len(blocks))
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		var worktree Worktree
		for _, line := range strings.Split(block, "\n") {
			key, value, hasValue := strings.Cut(line, " ")
			switch key {
			case "worktree":
				if hasValue {
					worktree.Path = value
				}
			case "gitdir":
				if hasValue {
					worktree.GitDir = value
				}
			case "HEAD":
				if hasValue {
					worktree.Head = value
				}
			case "branch":
				if hasValue {
					worktree.Branch = strings.TrimPrefix(value, "refs/heads/")
				}
			case "bare":
				worktree.Bare = true
			case "detached":
				worktree.Detached = true
			case "locked":
				worktree.Locked = true
				if hasValue {
					worktree.LockReason = value
				}
			case "prunable":
				worktree.Prunable = true
				if hasValue {
					worktree.PruneReason = value
				}
			}
		}
		worktrees = append(worktrees, worktree)
	}
	return worktrees
}

type ParsedStatus struct {
	Counts       StatusCounts
	UpstreamGone bool
	Upstream     string
}

func ParseStatusPorcelain(output string) ParsedStatus {
	var status ParsedStatus
	for lineIndex, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		if line == "" {
			continue
		}
		if lineIndex == 0 && strings.HasPrefix(line, "## ") {
			parseBranchStatusLine(line, &status)
			continue
		}
		if strings.HasPrefix(line, "??") {
			status.Counts.Untracked++
			continue
		}
		if len(line) < 2 {
			continue
		}
		indexStatus := line[0]
		worktreeStatus := line[1]
		if indexStatus != ' ' && indexStatus != '?' {
			status.Counts.Staged++
		}
		if worktreeStatus != ' ' && worktreeStatus != '?' {
			status.Counts.Modified++
		}
	}
	return status
}

func parseBranchStatusLine(line string, status *ParsedStatus) {
	body := strings.TrimPrefix(line, "## ")
	if strings.Contains(body, "[gone]") {
		status.UpstreamGone = true
	}
	if !strings.Contains(body, "...") {
		return
	}
	_, rest, _ := strings.Cut(body, "...")
	upstream := rest
	if bracket := strings.Index(upstream, " ["); bracket >= 0 {
		upstream = upstream[:bracket]
	}
	status.Upstream = strings.TrimSpace(upstream)
}

func ParseAheadBehind(output string) (ahead int, behind int, ok bool) {
	parts := strings.Fields(output)
	if len(parts) < 2 {
		return 0, 0, false
	}
	left, leftErr := strconv.Atoi(parts[0])
	right, rightErr := strconv.Atoi(parts[1])
	if leftErr != nil || rightErr != nil {
		return 0, 0, false
	}
	return left, right, true
}

type refMetadata struct {
	Branch       string
	ObjectName   string
	ObjectShort  string
	CommitTime   time.Time
	Subject      string
	Upstream     string
	UpstreamGone bool
	HeadSync     SyncState
	MainSync     SyncState
}

func ParseRefMetadata(output string) map[string]refMetadata {
	metadata := map[string]refMetadata{}
	output = strings.ReplaceAll(output, "\r\n", "\n")
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\x00")
		if len(fields) < 8 || fields[0] == "" {
			continue
		}
		item := refMetadata{
			Branch:      fields[0],
			ObjectName:  fields[1],
			ObjectShort: fields[2],
			Subject:     fields[4],
			Upstream:    fields[5],
		}
		if unixSeconds, err := strconv.ParseInt(fields[3], 10, 64); err == nil && unixSeconds > 0 {
			item.CommitTime = time.Unix(unixSeconds, 0)
		}
		item.UpstreamGone, item.HeadSync = ParseUpstreamTrack(fields[6])
		if item.Upstream == "" {
			item.HeadSync = SyncState{NoUpstream: true}
		}
		if ahead, behind, ok := ParseAheadBehind(fields[7]); ok {
			item.MainSync = SyncState{Available: true, Ahead: ahead, Behind: behind}
		}
		metadata[item.Branch] = item
	}
	return metadata
}

func ParseUpstreamTrack(track string) (gone bool, sync SyncState) {
	track = strings.TrimSpace(track)
	if track == "" {
		return false, SyncState{Available: true}
	}
	if track == "gone" {
		return true, SyncState{}
	}
	sync.Available = true
	track = strings.ReplaceAll(track, ",", "")
	parts := strings.Fields(track)
	for index := 0; index+1 < len(parts); index += 2 {
		value, err := strconv.Atoi(parts[index+1])
		if err != nil {
			continue
		}
		switch parts[index] {
		case "ahead":
			sync.Ahead = value
		case "behind":
			sync.Behind = value
		}
	}
	return false, sync
}
