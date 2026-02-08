// Package validator provides atomic commit validation logic.
package validator

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"dario.cat/darna/internal/analyzer"
	"dario.cat/darna/internal/git"
	"dario.cat/darna/internal/graph"
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

// FindCommittableSet identifies unstaged files that can be committed as a set.
// Returns the first independent file (sorted lexicographically).
// If includeDependants is true, also returns direct dependants that only depend on
// the base file and committed code.
func FindCommittableSet(ctx context.Context, workDir string, includeDependants bool) ([]string, error) {
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

	// 6. Find first independent file and optionally its dependants.
	return findCommittableSet(dg, candidatesGo, statuses, absWorkDir, includeDependants), nil
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

// findCommittableSet finds the first independent file from candidates.
// If includeDependants is true, also includes direct dependants.
// Files are sorted lexicographically by path, and the first independent file is selected.
// Returns relative paths, or nil if none found.
//
//nolint:revive // Internal helper for FindCommittableSet public API.
func findCommittableSet(
	dg *graph.DependencyGraph,
	candidates []string,
	statuses map[string]git.FileStatus,
	absWorkDir string,
	includeDependants bool,
) []string {
	sortedCandidates := sortFilesCopy(candidates)
	changesetFiles := buildChangesetMap(absWorkDir, statuses)

	// Find first independent file.
	for _, file := range sortedCandidates {
		if isIndependent(dg, file, changesetFiles) {
			result := buildCommittableSet(dg, file, changesetFiles, includeDependants)

			return convertToRelativePaths(result, absWorkDir)
		}
	}

	return nil
}

// sortFilesCopy creates a sorted copy of files lexicographically.
func sortFilesCopy(files []string) []string {
	sorted := make([]string, len(files))
	copy(sorted, files)

	// Use simple bubble sort for deterministic output.
	for i := range sorted {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

// buildCommittableSet builds the set of committable files starting from baseFile.
//
//nolint:revive // Flag parameter acceptable for internal helper.
func buildCommittableSet(
	dg *graph.DependencyGraph,
	baseFile string,
	changesetFiles map[string]bool,
	includeDependants bool,
) []string {
	result := []string{baseFile}

	if includeDependants {
		dependants := findDirectDependants(dg, baseFile, changesetFiles)
		result = append(result, dependants...)
	}

	return result
}

// convertToRelativePaths converts absolute paths to relative paths.
func convertToRelativePaths(absPaths []string, absWorkDir string) []string {
	result := make([]string, len(absPaths))

	for i, absPath := range absPaths {
		relPath, err := filepath.Rel(absWorkDir, absPath)
		if err != nil {
			relPath = absPath
		}

		result[i] = relPath
	}

	return result
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

// canCommitWithBase checks if a file can be committed together with baseFile.
// Returns true if the file ONLY depends on:
// - baseFile itself
// - Already committed files (not in changeset).
func canCommitWithBase(
	dg *graph.DependencyGraph,
	file string,
	baseFile string,
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

			// Allow dependencies on the base file.
			if depFile == baseFile {
				continue
			}

			// If dependency is in changeset (excluding baseFile and self), can't commit.
			if changesetFiles[depFile] {
				return false
			}
		}
	}

	return true
}

// findDirectDependants finds files that:
// 1. Depend on baseFile
// 2. Are in the changeset
// 3. Have NO dependencies on OTHER changeset files (only baseFile + committed).
func findDirectDependants(
	dg *graph.DependencyGraph,
	baseFile string,
	changesetFiles map[string]bool,
) []string {
	dependantFiles := collectDependantFiles(dg, baseFile, changesetFiles)
	validDependants := filterCommittableWithBase(dg, dependantFiles, baseFile, changesetFiles)

	return sortFilesCopy(validDependants)
}

// collectDependantFiles finds all files that depend on baseFile and are in the changeset.
func collectDependantFiles(
	dg *graph.DependencyGraph,
	baseFile string,
	changesetFiles map[string]bool,
) map[string]bool {
	dependantFiles := make(map[string]bool)
	baseSymbols := dg.FileSyms[baseFile]

	for _, baseSymID := range baseSymbols {
		collectSymbolDependants(dg, baseSymID, baseFile, changesetFiles, dependantFiles)
	}

	return dependantFiles
}

// collectSymbolDependants collects files that depend on a specific symbol.
func collectSymbolDependants(
	dg *graph.DependencyGraph,
	baseSymID string,
	baseFile string,
	changesetFiles map[string]bool,
	dependantFiles map[string]bool,
) {
	for dependentSymID := range dg.InEdges[baseSymID] {
		dependentSym := dg.Symbols[dependentSymID]
		if dependentSym == nil {
			continue
		}

		dependentFile := dependentSym.File
		if dependentFile == baseFile {
			continue
		}

		if changesetFiles[dependentFile] {
			dependantFiles[dependentFile] = true
		}
	}
}

// filterCommittableWithBase filters files to only those committable with baseFile.
func filterCommittableWithBase(
	dg *graph.DependencyGraph,
	dependantFiles map[string]bool,
	baseFile string,
	changesetFiles map[string]bool,
) []string {
	var result []string

	for file := range dependantFiles {
		if canCommitWithBase(dg, file, baseFile, changesetFiles) {
			result = append(result, file)
		}
	}

	return result
}
