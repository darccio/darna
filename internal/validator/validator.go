// Package validator provides atomic commit validation logic.
package validator

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/darccio/darna/internal/analyzer"
	"github.com/darccio/darna/internal/git"
	"github.com/darccio/darna/internal/graph"
)

// Violation represents a violation of the atomic commit rule.
type Violation struct {
	StagedFile    string // File being committed.
	StagedSymbol  string // Symbol defined in staged file.
	MissingFile   string // File with unstaged changes that's needed.
	MissingSymbol string // Symbol from missing file that's used.
}

// ValidateAtomicCommit validates that staged files form an atomic commit.
// Returns violations if staged code depends on unstaged changes.
func ValidateAtomicCommit(ctx context.Context, workDir string) ([]Violation, error) {
	// Convert workDir to absolute path for proper relative path calculations.
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return nil, fmt.Errorf("resolving work dir: %w", err)
	}

	// 1. Get file statuses from git.
	statuses, err := git.GetAllFileStatus(ctx, absWorkDir)
	if err != nil {
		return nil, fmt.Errorf("getting file status: %w", err)
	}

	// Categorize files and convert to absolute paths.
	staged, stagedSet, notStagedSet := categorizeFiles(absWorkDir, statuses)

	// Filter to .go files.
	stagedGo := git.FilterGoFiles(staged)
	if len(stagedGo) == 0 {
		return nil, nil // Nothing to validate.
	}

	// Build overlay for partially-staged files (MM status) so the package
	// loader sees the staged content instead of the working tree version.
	overlay := buildOverlay(ctx, absWorkDir, statuses)

	// 2. Load all packages in the repo.
	pkgs, err := analyzer.LoadPackages(absWorkDir, overlay, "./...")
	if err != nil {
		return nil, fmt.Errorf("loading packages: %w", err)
	}

	// 3. Build dependency graph.
	dg := graph.NewDependencyGraph()
	for _, pkg := range pkgs {
		dg.AnalyzePackage(pkg)
	}

	// 4. For each staged file, check dependencies.
	return findViolations(dg, stagedGo, stagedSet, notStagedSet, absWorkDir), nil
}

//nolint:nonamedreturns // Named returns clarify same-type values.
func categorizeFiles(
	absWorkDir string, statuses map[string]git.FileStatus,
) (staged []string, stagedSet, notStagedSet map[string]bool) {
	stagedSet = make(map[string]bool)
	notStagedSet = make(map[string]bool) // Includes modified unstaged AND untracked.

	for file, status := range statuses {
		absPath, err := filepath.Abs(filepath.Join(absWorkDir, file))
		if err != nil {
			continue
		}

		// Check if file is staged (has any index changes).
		if status.Staging != ' ' && status.Staging != '?' {
			staged = append(staged, absPath)
			stagedSet[absPath] = true
		}

		// Check if file has unstaged changes or is untracked.
		if status.Worktree != ' ' || status.Staging == '?' {
			notStagedSet[absPath] = true
		}
	}

	return staged, stagedSet, notStagedSet
}

func buildOverlay(ctx context.Context, absWorkDir string, statuses map[string]git.FileStatus) map[string][]byte {
	overlay := make(map[string][]byte)

	for file, status := range statuses {
		if status.Staging == ' ' || status.Staging == '?' || status.Worktree == ' ' {
			continue
		}

		absPath, err := filepath.Abs(filepath.Join(absWorkDir, file))
		if err != nil {
			continue
		}

		if !strings.HasSuffix(absPath, ".go") {
			continue
		}

		content, err := git.GetStagedContent(ctx, absWorkDir, file)
		if err != nil {
			continue // Fall back to working tree (current behavior).
		}

		overlay[absPath] = content
	}

	return overlay
}

func findViolations(
	dg *graph.DependencyGraph,
	stagedGo []string,
	stagedSet, notStagedSet map[string]bool,
	absWorkDir string,
) []Violation {
	var violations []Violation

	for _, file := range stagedGo {
		symbols := dg.FileSyms[file]
		for _, symID := range symbols {
			deps := dg.TransitiveDeps(symID)
			for _, depID := range deps {
				depSym := dg.Symbols[depID]
				if depSym == nil {
					continue // External dependency, skip.
				}

				depFile := depSym.File

				// Check if dependency is not staged (either unstaged or untracked).
				if !stagedSet[depFile] && isNotStaged(depFile, notStagedSet) {
					violations = append(violations, newViolation(file, symID, depFile, depID, absWorkDir))
				}
			}
		}
	}

	return violations
}

func newViolation(file, symID, depFile, depID, absWorkDir string) Violation {
	// Convert to relative path for better display.
	relFile, err := filepath.Rel(absWorkDir, file)
	if err != nil {
		relFile = file
	}

	relDepFile, err := filepath.Rel(absWorkDir, depFile)
	if err != nil {
		relDepFile = depFile
	}

	return Violation{
		StagedFile:    relFile,
		StagedSymbol:  symID,
		MissingFile:   relDepFile,
		MissingSymbol: depID,
	}
}

// isNotStaged checks if a file is not staged, handling directory prefixes
// for untracked directories (git reports "dir/" instead of "dir/file.go").
func isNotStaged(file string, notStagedSet map[string]bool) bool {
	// Direct check.
	if notStagedSet[file] {
		return true
	}

	// Check if any parent directory is in the set
	// e.g., if "internal" is untracked, then "internal/foo/bar.go" is also untracked.
	for dir := range notStagedSet {
		if strings.HasPrefix(file, ensureTrailingSlash(dir)) {
			return true
		}
	}

	return false
}

func ensureTrailingSlash(dir string) string {
	if len(dir) > 0 && dir[len(dir)-1] != '/' {
		return dir + "/"
	}

	return dir
}

// FindCommittableFiles identifies unstaged files that can be committed independently.
// Returns relative file paths that have no dependencies on other changeset files.
func FindCommittableFiles(ctx context.Context, workDir string) ([]string, error) {
	// Convert workDir to absolute path for proper relative path calculations.
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return nil, fmt.Errorf("resolving work dir: %w", err)
	}

	// 1. Get file statuses from git.
	statuses, err := git.GetAllFileStatus(ctx, absWorkDir)
	if err != nil {
		return nil, fmt.Errorf("getting file status: %w", err)
	}

	// 2. Extract candidates (unstaged/untracked files only).
	candidates := getCandidates(absWorkDir, statuses)

	// Filter to .go files.
	candidatesGo := git.FilterGoFiles(candidates)
	if len(candidatesGo) == 0 {
		return nil, nil // No candidates.
	}

	// 3. Build overlay for partially-staged files (MM status).
	overlay := buildOverlay(ctx, absWorkDir, statuses)

	// 4. Load all packages in the repo.
	pkgs, err := analyzer.LoadPackages(absWorkDir, overlay, "./...")
	if err != nil {
		return nil, fmt.Errorf("loading packages: %w", err)
	}

	// 5. Build dependency graph.
	dg := graph.NewDependencyGraph()
	for _, pkg := range pkgs {
		dg.AnalyzePackage(pkg)
	}

	// 6. Find first independent file.
	return findCommittableFile(dg, candidatesGo, statuses, absWorkDir), nil
}

// getCandidates extracts files that are candidates for committable selection.
// Returns absolute paths of unstaged and untracked files, excluding already-staged files.
func getCandidates(absWorkDir string, statuses map[string]git.FileStatus) []string {
	var candidates []string

	for file, status := range statuses {
		absPath, err := filepath.Abs(filepath.Join(absWorkDir, file))
		if err != nil {
			continue
		}

		// Include files that are:
		// - Modified unstaged (worktree != ' ')
		// - Untracked (staging == '?')
		// Exclude files that are already staged (staging != ' ' and staging != '?').
		isStaged := status.Staging != ' ' && status.Staging != '?'
		isModifiedOrUntracked := status.Worktree != ' ' || status.Staging == '?'

		if !isStaged && isModifiedOrUntracked {
			candidates = append(candidates, absPath)
		}
	}

	return candidates
}

// findCommittableFile finds the first independent file from candidates.
// Returns a single-element slice with the relative path, or nil if none found.
func findCommittableFile(
	dg *graph.DependencyGraph,
	candidates []string,
	statuses map[string]git.FileStatus,
	absWorkDir string,
) []string {
	// Sort candidates lexicographically for deterministic output.
	candidatesCopy := make([]string, len(candidates))
	copy(candidatesCopy, candidates)
	candidates = candidatesCopy

	// Use filepath for sorting to ensure consistent path handling.
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[i] > candidates[j] {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// Build changeset map (all files with modifications).
	changesetFiles := buildChangesetMap(absWorkDir, statuses)

	// Find first independent file.
	for _, file := range candidates {
		if isIndependent(dg, file, changesetFiles) {
			// Convert to relative path for output.
			relPath, err := filepath.Rel(absWorkDir, file)
			if err != nil {
				relPath = file
			}

			return []string{relPath}
		}
	}

	return nil
}

// buildChangesetMap tracks which files are in the changeset (modified or untracked).
// Returns a map of absolute paths for O(1) lookup.
// For committable mode, only unstaged/untracked files are considered part of the changeset.
func buildChangesetMap(absWorkDir string, statuses map[string]git.FileStatus) map[string]bool {
	changesetFiles := make(map[string]bool)

	for file, status := range statuses {
		absPath, err := filepath.Abs(filepath.Join(absWorkDir, file))
		if err != nil {
			continue
		}

		// Include files that are:
		// - Modified unstaged (worktree != ' ')
		// - Untracked (staging == '?')
		// Exclude files that are only staged (no unstaged changes).
		if status.Worktree != ' ' || status.Staging == '?' {
			changesetFiles[absPath] = true
		}
	}

	return changesetFiles
}

// isIndependent checks if a file is independent (has no dependencies on changeset files).
func isIndependent(
	dg *graph.DependencyGraph,
	file string,
	changesetFiles map[string]bool,
) bool {
	// Get all symbols defined in the file.
	symbols := dg.FileSyms[file]

	// Check each symbol's transitive dependencies.
	for _, symID := range symbols {
		deps := dg.TransitiveDeps(symID)
		for _, depID := range deps {
			depSym := dg.Symbols[depID]
			if depSym == nil {
				continue // External dependency, skip.
			}

			depFile := depSym.File

			// Skip if dependency is the file itself (self-reference).
			if depFile == file {
				continue
			}

			// Check if dependency file is in changeset.
			// If the dependency is in the changeset, this file is not independent.
			if changesetFiles[depFile] {
				return false
			}
		}
	}

	return true
}
