package correlation

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrEmbeddedModeUnavailable signals that history/correlation cannot run in an
// embedded (in-process Dolt) project: bd export carries no git-tracked JSONL to
// diff for commit correlation, and bt opens no Dolt SQL connection in embedded
// mode (bt-ij71a / bt-qrt2u). Consumers surface this as a clear "not available
// yet" state rather than a silent-empty result. Full embedded correlation is a
// separate, decide-first follow-up: bt-5uaxh. Correlation is unchanged for
// JSONL-in-git and shared-server projects.
var ErrEmbeddedModeUnavailable = errors.New("history/correlation not available in embedded (in-process Dolt) mode yet - tracked as bt-5uaxh (works for JSONL-in-git and shared-server projects)")

var defaultBeadsFiles = []string{
	".beads/issues.jsonl",
	".beads/beads.jsonl",
	".beads/beads.base.jsonl",
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func pickBeadsFiles(repoPath string, candidates []string) []string {
	if len(candidates) == 0 {
		return nil
	}

	primary := ""
	for _, rel := range candidates {
		if rel == "" {
			continue
		}
		if fileExists(filepath.Join(repoPath, rel)) {
			primary = rel
			break
		}
	}
	if primary == "" {
		return candidates
	}

	out := make([]string, 0, len(candidates))
	out = append(out, primary)
	for _, rel := range candidates {
		if rel == primary {
			continue
		}
		out = append(out, rel)
	}
	return out
}

// HasJSONLOnDisk reports whether any of the standard beads JSONL files
// (.beads/issues.jsonl, .beads/beads.jsonl, .beads/beads.base.jsonl) exists
// on disk at repoPath. Mirrors the existence test that ValidateRepository
// already uses to decide whether the JSONL+git-diff witness path can run.
//
// Existence says nothing about whether the file is current, git-tracked, or
// the project's data source: Dolt-canonical repos often keep a stale,
// untracked or gitignored export. So this is only the fallback gate for when
// no Dolt source is available; it must never outrank a configured Dolt
// source when choosing the history path (bt-zxdl9). Returning false means
// the JSONL extractor cannot produce data on this repo at all.
//
// Returns false when repoPath is empty. Errors from os.Stat are silently
// treated as absent.
func HasJSONLOnDisk(repoPath string) bool {
	if repoPath == "" {
		return false
	}
	for _, rel := range defaultBeadsFiles {
		if fileExists(filepath.Join(repoPath, rel)) {
			return true
		}
	}
	return false
}

func prependBeadsFile(primary string, candidates []string) []string {
	if primary == "" {
		return candidates
	}
	out := []string{primary}
	for _, rel := range candidates {
		if rel == primary {
			continue
		}
		out = append(out, rel)
	}
	return out
}
