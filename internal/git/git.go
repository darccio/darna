// Package git provides utilities for interacting with git repositories.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// GetStagedFiles returns the list of staged files in the specified directory.
// Only includes files that are added, copied, modified, or renamed (not deleted).
func GetStagedFiles(ctx context.Context, dir string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "diff", "--cached", "--name-only", "--diff-filter=ACMR")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("getting staged files: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return []string{}, nil
	}

	return lines, nil
}

// GetUnstagedModified returns the list of files with unstaged modifications in the specified directory.
func GetUnstagedModified(ctx context.Context, dir string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "diff", "--name-only")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("getting unstaged files: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return []string{}, nil
	}

	return lines, nil
}

// FileStatus represents the git status of a file.
type FileStatus struct {
	Staging  byte // Index status.
	Worktree byte // Working tree status.
}

// GetAllFileStatus returns the status of all files in the specified directory using git status --porcelain.
// The status uses two-character codes: first is staging area, second is working tree.
func GetAllFileStatus(ctx context.Context, dir string) (map[string]FileStatus, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain", "-z", "--untracked-files=all")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("getting file status: %w", err)
	}

	status := make(map[string]FileStatus)

	entries := bytes.SplitSeq(output, []byte{0})
	for entry := range entries {
		if len(entry) >= 4 { //nolint:mnd // Git porcelain format: 2 status chars + space + filename.
			status[string(entry[3:])] = FileStatus{
				Staging:  entry[0],
				Worktree: entry[1],
			}
		}
	}

	return status, nil
}

// GetStagedContent reads the staged content of a file from the git index in the specified directory.
// This is important for files with partial staging.
func GetStagedContent(ctx context.Context, dir, path string) ([]byte, error) {
	//nolint:gosec // Path comes from git status output.
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "show", ":"+path)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("getting staged content for %s: %w", path, err)
	}

	return output, nil
}

// FilterGoFiles filters a list of files to only include .go files.
func FilterGoFiles(files []string) []string {
	var goFiles []string

	for _, f := range files {
		if strings.HasSuffix(f, ".go") {
			goFiles = append(goFiles, f)
		}
	}

	return goFiles
}
