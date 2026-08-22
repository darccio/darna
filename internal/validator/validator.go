// Package validator provides atomic commit validation logic.
package validator

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

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
		if !errors.Is(err, analyzer.ErrPackagesContainErrors) {
			return nil, fmt.Errorf("loading packages: %w", err)
		}

		// Package errors exist. Only fail if any error is in a staged file —
		// errors confined to unstaged or untracked files can be ignored.
		if hasErrorsInStagedFiles(pkgs, stagedSet) {
			analyzer.PrintErrors(pkgs)

			return nil, fmt.Errorf("loading packages: %w", err)
		}
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
		if !strings.HasSuffix(file, ".go") {
			continue
		}

		absPath, err := filepath.Abs(filepath.Join(absWorkDir, file))
		if err != nil {
			continue
		}

		// Overlay files with working-tree changes (both " M" and "MM") with
		// their staged/index content so the loader sees only what will be
		// committed, not unrelated in-progress work.
		if status.Worktree == ' ' || status.Staging == '?' {
			continue
		}

		content, err := git.GetStagedContent(ctx, absWorkDir, file)
		if err != nil {
			continue // Fall back to working tree.
		}

		overlay[absPath] = content
	}

	return overlay
}

// hasErrorsInStagedFiles reports whether any package error originates from a staged file.
// Errors confined to unstaged or untracked files can be safely ignored.
func hasErrorsInStagedFiles(pkgs []*packages.Package, stagedSet map[string]bool) bool {
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			file := fileFromErrorPos(e.Pos)
			if file != "" && stagedSet[file] {
				return true
			}
		}
	}

	return false
}

// fileFromErrorPos extracts the file path from a packages.Error.Pos string.
// The format is "file:line" or "file:line:col" or "" or "-".
func fileFromErrorPos(pos string) string {
	if pos == "" || pos == "-" {
		return ""
	}

	idx := strings.Index(pos, ":")
	if idx <= 0 {
		return ""
	}

	return pos[:idx]
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
	if err != nil && !errors.Is(err, analyzer.ErrPackagesContainErrors) {
		return nil, fmt.Errorf("loading packages: %w", err)
	}
	// Package errors in unstaged files are tolerated: analysis continues with
	// the packages that compiled successfully.

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

// canCommitWithBase checks if a file can be committed together with baseFile
// and the given set of already-accepted dependants. Returns true if the file
// ONLY depends on:
// - baseFile itself
// - Other files in the accepted set (fellow direct dependants of baseFile)
// - Already committed files (not in changeset).
func canCommitWithBase(
	dg *graph.DependencyGraph,
	file string,
	baseFile string,
	accepted map[string]bool,
	changesetFiles map[string]bool,
) bool {
	for _, symID := range dg.FileSyms[file] {
		for _, depID := range dg.TransitiveDeps(symID) {
			if !depAllowedWithBase(dg, depID, file, baseFile, accepted, changesetFiles) {
				return false
			}
		}
	}

	return true
}

// depAllowedWithBase reports whether a single dependency (depID, used by a
// symbol defined in file) is compatible with committing file alongside
// baseFile: it must resolve to an external (non-changeset) symbol, the file
// itself, baseFile, or another accepted dependant of baseFile.
func depAllowedWithBase(
	dg *graph.DependencyGraph,
	depID string,
	file, baseFile string,
	accepted, changesetFiles map[string]bool,
) bool {
	depSym := dg.Symbols[depID]
	if depSym == nil {
		return true // External dependency, allowed.
	}

	depFile := depSym.File

	switch {
	case depFile == file: // Self-reference.
		return true
	case depFile == baseFile:
		return true
	case accepted[depFile]:
		// Fellow accepted dependant of baseFile, e.g. a shared test helper
		// that also directly calls a symbol in baseFile and will be
		// committed alongside it.
		return true
	case changesetFiles[depFile]:
		return false
	default:
		return true
	}
}

// findDirectDependants finds files that:
// 1. Depend directly on a symbol defined in baseFile.
// 2. Are in the changeset.
// 3. Have no dependencies on OTHER changeset files, except baseFile and
//    fellow direct dependants that also survive this filter.
//
// Direct dependants of baseFile commonly share helpers with each other
// (e.g. sibling _test.go files calling a common setup function), so a
// dependant is only excluded when it depends on a changeset file that is
// NOT itself a direct dependant being committed in the same set.
func findDirectDependants(
	dg *graph.DependencyGraph,
	baseFile string,
	changesetFiles map[string]bool,
) []string {
	candidates := collectDependantFiles(dg, baseFile, changesetFiles)
	accepted := pruneToFixedPoint(dg, candidates, baseFile, changesetFiles)

	result := make([]string, 0, len(accepted))
	for file := range accepted {
		result = append(result, file)
	}

	return sortFilesCopy(result)
}

// pruneToFixedPoint repeatedly removes candidates that depend (among
// changeset files) on something outside baseFile and the remaining
// candidates, until no more removals happen. This lets direct dependants
// that depend on each other (but still resolve within the committable set)
// stay in, while still excluding dependants whose dependencies fall outside
// the set entirely.
func pruneToFixedPoint(
	dg *graph.DependencyGraph,
	candidates map[string]bool,
	baseFile string,
	changesetFiles map[string]bool,
) map[string]bool {
	accepted := make(map[string]bool, len(candidates))
	for file := range candidates {
		accepted[file] = true
	}

	for {
		removedAny := false

		for file := range accepted {
			if canCommitWithBase(dg, file, baseFile, accepted, changesetFiles) {
				continue
			}

			delete(accepted, file)

			removedAny = true
		}

		if !removedAny {
			break
		}
	}

	return accepted
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

// DetectChangesets partitions staged .go files into independent changesets.
// A changeset is a maximal set of staged files connected by direct symbol
// dependencies in either direction. Returns one slice of relative file paths
// per changeset, sorted lexicographically within each changeset and by first
// element across changesets. Returns nil when nothing is staged.
func DetectChangesets(ctx context.Context, workDir string) ([][]string, error) {
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return nil, fmt.Errorf("resolving work dir: %w", err)
	}

	statuses, err := git.GetAllFileStatus(ctx, absWorkDir)
	if err != nil {
		return nil, fmt.Errorf("getting file status: %w", err)
	}

	staged, stagedSet, _ := categorizeFiles(absWorkDir, statuses)

	stagedGo := git.FilterGoFiles(staged)
	if len(stagedGo) == 0 {
		return nil, nil
	}

	overlay := buildOverlay(ctx, absWorkDir, statuses)

	pkgs, err := analyzer.LoadPackages(absWorkDir, overlay, "./...")
	if err != nil {
		if !errors.Is(err, analyzer.ErrPackagesContainErrors) {
			return nil, fmt.Errorf("loading packages: %w", err)
		}

		if hasErrorsInStagedFiles(pkgs, stagedSet) {
			analyzer.PrintErrors(pkgs)

			return nil, fmt.Errorf("loading packages: %w", err)
		}
	}

	dg := graph.NewDependencyGraph()
	for _, pkg := range pkgs {
		dg.AnalyzePackage(pkg)
	}

	return findChangesets(dg, stagedGo, stagedSet, absWorkDir), nil
}

// findChangesets groups staged files into connected components by direct
// symbol dependencies between staged files, then returns each component as
// a sorted slice of relative paths.
func findChangesets(
	dg *graph.DependencyGraph,
	stagedGo []string,
	stagedSet map[string]bool,
	absWorkDir string,
) [][]string {
	adj := buildStagedFileAdjacency(dg, stagedGo, stagedSet)
	components := connectedComponents(stagedGo, adj)

	return sortChangesets(components, absWorkDir)
}

// buildStagedFileAdjacency builds an undirected adjacency map between staged
// files. Every staged file is present as a node, even if isolated. An edge
// A—B exists if any symbol in A directly depends on a symbol whose file is B
// (and B != A).
func buildStagedFileAdjacency(
	dg *graph.DependencyGraph,
	stagedGo []string,
	stagedSet map[string]bool,
) map[string]map[string]struct{} {
	adj := make(map[string]map[string]struct{}, len(stagedGo))
	for _, file := range stagedGo {
		adj[file] = make(map[string]struct{})
	}

	for _, file := range stagedGo {
		addFileEdges(dg, file, stagedSet, adj)
	}

	return adj
}

// addFileEdges records undirected edges between file and every other staged
// file that any symbol defined in file directly depends on.
func addFileEdges(
	dg *graph.DependencyGraph,
	file string,
	stagedSet map[string]bool,
	adj map[string]map[string]struct{},
) {
	for _, symID := range dg.FileSyms[file] {
		for depID := range dg.OutEdges[symID] {
			depSym := dg.Symbols[depID]
			if depSym == nil {
				continue
			}

			depFile := depSym.File
			if depFile == file || !stagedSet[depFile] {
				continue
			}

			adj[file][depFile] = struct{}{}
			adj[depFile][file] = struct{}{}
		}
	}
}

// connectedComponents finds connected components via BFS. Start nodes are
// iterated in stagedGo order so component discovery is deterministic.
func connectedComponents(
	stagedGo []string,
	adj map[string]map[string]struct{},
) [][]string {
	visited := make(map[string]bool, len(stagedGo))

	var components [][]string

	for _, start := range stagedGo {
		if visited[start] {
			continue
		}

		components = append(components, bfsComponent(start, adj, visited))
	}

	return components
}

// bfsComponent returns all nodes reachable from start via undirected edges,
// marking each as visited.
func bfsComponent(
	start string,
	adj map[string]map[string]struct{},
	visited map[string]bool,
) []string {
	queue := []string{start}
	visited[start] = true
	component := []string{}

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		component = append(component, node)

		for neighbor := range adj[node] {
			if visited[neighbor] {
				continue
			}

			visited[neighbor] = true

			queue = append(queue, neighbor)
		}
	}

	return component
}

// sortChangesets sorts each component's files lexicographically (as relative
// paths), then sorts components by their first element for deterministic output.
func sortChangesets(components [][]string, absWorkDir string) [][]string {
	result := make([][]string, 0, len(components))

	for _, comp := range components {
		relPaths := convertToRelativePaths(comp, absWorkDir)
		sort.Strings(relPaths)
		result = append(result, relPaths)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i][0] < result[j][0]
	})

	return result
}
