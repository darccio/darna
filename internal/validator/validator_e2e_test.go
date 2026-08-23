package validator_test

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"dario.cat/darna/internal/validator"
)

const (
	testComment        = "\n// Comment\n"
	fileMainGo         = "main.go"
	fileUtilsGo        = "utils.go"
	fileConsumerGo     = "consumer.go"
	fileProcessorGo    = "processor.go"
	fileTypesGo        = "types.go"
	fileHelperFmtGo    = "helper/formatter.go"
	fileHelperValidGo  = "helper/validator.go"
	fileModelsResponse = "models/response.go"
)

// setupTestRepo creates a temporary git repository with the test project.
func setupTestRepo(t *testing.T) string {
	t.Helper()

	// Create temp directory.
	tmpDir := t.TempDir()

	// Initialize git repo.
	runGit(t, tmpDir, "init")
	runGit(t, tmpDir, "config", "user.email", "test@example.com")
	runGit(t, tmpDir, "config", "user.name", "Test User")
	runGit(t, tmpDir, "config", "commit.gpgsign", "false")

	// Copy testdata files.
	testdataDir := filepath.Join("testdata", "project")

	files := []string{
		"go.mod", "main.go", "service.go", "utils.go", "types.go",
		"circular_a.go", "circular_b.go",
		"alpha.go", "beta.go", "gamma.go",
		"constants.go", "variables.go", "consumer.go",
		"calculator.go", "calculator_user.go",
		"processor.go",
	}
	for _, file := range files {
		src := filepath.Join(testdataDir, file)
		dst := filepath.Join(tmpDir, file)
		copyFile(t, src, dst)
	}

	// Copy subpackage files.
	subpackages := map[string][]string{
		"helper": {"formatter.go", "validator.go"},
		"models": {"request.go", "response.go"},
	}
	for pkg, pkgFiles := range subpackages {
		pkgDir := filepath.Join(tmpDir, pkg)

		err := os.MkdirAll(pkgDir, 0o750)
		if err != nil {
			t.Fatalf("Failed to create package dir %s: %v", pkg, err)
		}

		for _, file := range pkgFiles {
			src := filepath.Join(testdataDir, pkg, file)
			dst := filepath.Join(pkgDir, file)
			copyFile(t, src, dst)
		}
	}

	// Initial commit.
	runGit(t, tmpDir, "add", ".")
	runGit(t, tmpDir, "commit", "-m", "Initial commit")

	return tmpDir
}

// runGit runs a git command in the specified directory.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...) //nolint:gosec // Test helper for git commands.
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\nOutput: %s", args, err, output)
	}
}

// copyFile copies a file from src to dst.
func copyFile(t *testing.T, src, dst string) {
	t.Helper()

	data, err := os.ReadFile(src) //nolint:gosec // Test helper reads testdata files.
	if err != nil {
		t.Fatalf("Failed to read %s: %v", src, err)
	}

	err = os.WriteFile(dst, data, 0o600) //nolint:gosec // Test helper writes to temp dir.
	if err != nil {
		t.Fatalf("Failed to write %s: %v", dst, err)
	}
}

// modifyFile appends content to a file to trigger git changes.
func modifyFile(t *testing.T, path, content string) {
	t.Helper()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // Test helper opens temp files.
	if err != nil {
		t.Fatalf("Failed to open %s: %v", path, err)
	}

	defer func() {
		closeErr := f.Close()
		if closeErr != nil {
			t.Fatalf("Failed to close %s: %v", path, closeErr)
		}
	}()

	_, err = f.WriteString(content)
	if err != nil {
		t.Fatalf("Failed to write to %s: %v", path, err)
	}
}

// stageFiles stages the specified files.
func stageFiles(t *testing.T, repoDir string, files ...string) {
	t.Helper()

	args := append([]string{"add"}, files...)
	runGit(t, repoDir, args...)
}

// createUntrackedFile creates a new untracked file in the repo.
func createUntrackedFile(t *testing.T, repoDir, filename, content string) {
	t.Helper()

	path := filepath.Join(repoDir, filename)

	err := os.WriteFile(path, []byte(content), 0o600)
	if err != nil {
		t.Fatalf("Failed to create %s: %v", path, err)
	}
}

// createUntrackedSubpackage creates a new untracked subpackage directory in the repo.
// Returns the absolute path to the created directory.
func createUntrackedSubpackage(t *testing.T, repoDir, subpkg string) string {
	t.Helper()

	dir := filepath.Join(repoDir, subpkg)

	err := os.MkdirAll(dir, 0o750)
	if err != nil {
		t.Fatalf("Failed to create %s dir: %v", subpkg, err)
	}

	return dir
}

// logTestPattern logs a structured description of the test pattern.
func logTestPattern(t *testing.T, pattern, graphDesc, setup, expect string) {
	t.Helper()
	t.Logf("\n"+
		"===================================================================\n"+
		"PATTERN: %s\n"+
		"GRAPH:   %s\n"+
		"SETUP:   %s\n"+
		"EXPECT:  %s\n"+
		"===================================================================",
		pattern, graphDesc, setup, expect)
}

func TestValidateAtomicCommit_NoViolations(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Complete Atomic Commit (No Violations)",
		"main.go -> service.go -> utils.go, service.go -> types.go",
		"Modified [main.go, service.go, utils.go, types.go] | Staged [ALL] | Unstaged [NONE]",
		"No violations - all dependencies are staged together")

	repoDir := setupTestRepo(t)

	// Modify and stage all files together.
	modifyFile(t, filepath.Join(repoDir, "main.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "service.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "utils.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "types.go"), testComment)
	stageFiles(t, repoDir, "main.go", "service.go", "utils.go", "types.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) != 0 {
		t.Errorf("Expected no violations, got %d: %+v", len(violations), violations)
	}
}

func TestValidateAtomicCommit_DirectDependency(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Direct Dependency Violation",
		"main.go (main func) -> utils.go (Helper func)",
		"Modified [main.go, utils.go] | Staged [main.go] | Unstaged [utils.go]",
		"Violation detected - main.go depends on unstaged utils.go")

	repoDir := setupTestRepo(t)

	// Modify main.go (depends on Helper from utils.go) and utils.go.
	modifyFile(t, filepath.Join(repoDir, "main.go"), "\n// Comment in main\n")
	modifyFile(t, filepath.Join(repoDir, "utils.go"), "\n// Comment in utils\n")

	// Only stage main.go.
	stageFiles(t, repoDir, "main.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violations, got none")
	}

	// Verify we have violations involving main.go depending on utils.go.
	found := false

	for _, v := range violations {
		if v.StagedFile == fileMainGo && v.MissingFile == fileUtilsGo {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Expected violation from main.go to utils.go, violations: %+v", violations)
	}
}

func TestValidateAtomicCommit_SignatureDependency(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Function Signature Dependency Violation",
		"runner.go (Run) -> interfaces.go (Handler) — dependency via parameter type only",
		"Untracked [interfaces.go, runner.go] | Staged [runner.go] | Unstaged [interfaces.go]",
		"Violation detected - staged file depends on untracked file through function signature")

	repoDir := setupTestRepo(t)

	// Create a type definition in an untracked file.
	createUntrackedFile(t, repoDir, "interfaces.go", `package main

// Handler handles requests.
type Handler interface {
	Handle() error
}
`)

	// Create a function that uses Handler only in its signature (not in body).
	createUntrackedFile(t, repoDir, "runner.go", `package main

// Run runs a handler.
func Run(h Handler) error {
	return h.Handle()
}
`)

	// Stage only the consumer file; the type definition remains untracked.
	stageFiles(t, repoDir, "runner.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violation: staged runner.go depends on untracked interfaces.go, got none")
	}

	found := false

	for _, v := range violations {
		if v.StagedFile == "runner.go" && v.MissingFile == "interfaces.go" {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Expected violation from runner.go to interfaces.go, violations: %+v", violations)
	}
}

func TestValidateAtomicCommit_TestFileDependsOnSourceFile(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Test File Depends on Unstaged Source File",
		"widget_test.go (TestNewWidget) -> widget.go (NewWidget)",
		"Untracked [widget.go, widget_test.go] | Staged [widget_test.go] | Unstaged [widget.go]",
		"Violation detected - staged test file depends on unstaged source file")

	repoDir := setupTestRepo(t)

	// Create a new source file with an exported function.
	createUntrackedFile(t, repoDir, "widget.go", `package main

// NewWidget creates a new widget.
func NewWidget() string {
	return "widget"
}
`)

	// Create a test file in the same package that uses NewWidget.
	createUntrackedFile(t, repoDir, "widget_test.go", `package main

import "testing"

func TestNewWidget(t *testing.T) {
	t.Parallel()

	got := NewWidget()
	if got == "" {
		t.Error("NewWidget returned empty string")
	}
}
`)

	// Stage only the test file, leaving widget.go untracked.
	stageFiles(t, repoDir, "widget_test.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violation: staged widget_test.go depends on untracked widget.go, got none")
	}

	found := false

	for _, v := range violations {
		if v.StagedFile == "widget_test.go" && v.MissingFile == "widget.go" {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Expected violation from widget_test.go to widget.go, violations: %+v", violations)
	}
}

// transitiveViolationTest is a helper to avoid code duplication between
// TestValidateAtomicCommit_TransitiveDependency and
// TestValidateAtomicCommit_SpecificSymbol_TransitiveChain.
func transitiveViolationTest(
	t *testing.T,
	pattern, graphDesc, setup, expect string,
	fileA, fileB, fileC string,
	stageA, stageB string,
) {
	t.Helper()

	logTestPattern(t, pattern, graphDesc, setup, expect)

	repoDir := setupTestRepo(t)

	modifyFile(t, filepath.Join(repoDir, fileA), testComment)
	modifyFile(t, filepath.Join(repoDir, fileB), testComment)
	modifyFile(t, filepath.Join(repoDir, fileC), testComment)

	stageFiles(t, repoDir, stageA, stageB)

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violations, got none")
	}

	hasAToC := false
	hasBToC := false

	for _, v := range violations {
		if v.StagedFile == stageA && v.MissingFile == fileC {
			hasAToC = true
		}

		if v.StagedFile == stageB && v.MissingFile == fileC {
			hasBToC = true
		}
	}

	if !hasAToC {
		t.Errorf("Expected violation from %s to %s", stageA, fileC)
	}

	if !hasBToC {
		t.Errorf("Expected violation from %s to %s", stageB, fileC)
	}
}

func TestValidateAtomicCommit_TransitiveDependency(t *testing.T) {
	t.Parallel()

	transitiveViolationTest(t,
		"Transitive Dependency Violation",
		"main.go -> service.go -> utils.go (transitive chain)",
		"Modified [main.go, service.go, utils.go] | Staged [main.go, service.go] | Unstaged [utils.go]",
		"Violations detected - both main.go and service.go depend on unstaged utils.go (direct and transitive)",
		"main.go", "service.go", "utils.go",
		"main.go", "service.go",
	)
}

func TestValidateAtomicCommit_CircularDependency(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Circular Dependency Violation",
		"circular_a.go (FuncA) <-> circular_b.go (FuncB) - circular dependency",
		"Modified [circular_a.go, circular_b.go] | Staged [circular_a.go] | Unstaged [circular_b.go]",
		"Violation detected - circular_a.go depends on unstaged circular_b.go")

	repoDir := setupTestRepo(t)

	// Modify both circular files.
	modifyFile(t, filepath.Join(repoDir, "circular_a.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "circular_b.go"), testComment)

	// Stage only circular_a.go.
	stageFiles(t, repoDir, "circular_a.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violations for circular dependency, got none")
	}

	// Should have violation from circular_a.go to circular_b.go.
	found := false

	for _, v := range violations {
		if v.StagedFile == "circular_a.go" && v.MissingFile == "circular_b.go" {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Expected violation from circular_a.go to circular_b.go, violations: %+v", violations)
	}
}

func TestValidateAtomicCommit_TypeDependency(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Type Dependency Violation",
		"service.go (UseDefaultConfig func) -> types.go (DefaultConfig func, Config/User types)",
		"Modified [service.go, types.go] | Staged [service.go] | Unstaged [types.go]",
		"Violation detected - service.go depends on unstaged types.go")

	repoDir := setupTestRepo(t)

	// Modify service.go (uses Config and User types) and types.go.
	modifyFile(t, filepath.Join(repoDir, "service.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "types.go"), testComment)

	// Stage only service.go.
	stageFiles(t, repoDir, "service.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violations for type dependency, got none")
	}

	// Should have violation from service.go to types.go.
	found := false

	for _, v := range violations {
		if v.StagedFile == "service.go" && v.MissingFile == fileTypesGo {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Expected violation from service.go to types.go, violations: %+v", violations)
	}
}

func TestValidateAtomicCommit_MultipleViolations(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Multiple Independent Violations",
		"main.go -> utils.go, service.go -> types.go (independent chains)",
		"Modified [main.go, service.go, utils.go, types.go] | Staged [main.go, service.go] | Unstaged [utils.go, types.go]",
		"Multiple violations detected - main.go -> utils.go AND service.go -> types.go")

	repoDir := setupTestRepo(t)

	// Modify multiple files.
	modifyFile(t, filepath.Join(repoDir, "main.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "service.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "utils.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "types.go"), testComment)

	// Stage only main.go and service.go (both depend on unstaged files).
	stageFiles(t, repoDir, "main.go", "service.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected multiple violations, got none")
	}

	// Should have multiple violations.
	minExpectedViolations := 2
	if len(violations) < minExpectedViolations {
		t.Errorf("Expected at least %d violations, got %d: %+v", minExpectedViolations, len(violations), violations)
	}

	// Verify at least some expected violations exist.
	hasMainToUtils := false
	hasServiceToTypes := false

	for _, v := range violations {
		if v.StagedFile == fileMainGo && v.MissingFile == fileUtilsGo {
			hasMainToUtils = true
		}

		if v.StagedFile == "service.go" && v.MissingFile == fileTypesGo {
			hasServiceToTypes = true
		}
	}

	if !hasMainToUtils {
		t.Error("Expected violation from main.go to utils.go")
	}

	if !hasServiceToTypes {
		t.Error("Expected violation from service.go to types.go")
	}
}

func TestValidateAtomicCommit_UntrackedFile(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Untracked File Dependency Violation",
		"main.go (UseNewHelper func) -> newutil.go (NewHelper func - UNTRACKED)",
		"Modified [main.go] | Staged [main.go] | Untracked [newutil.go]",
		"Violation detected - main.go depends on untracked newutil.go")

	repoDir := setupTestRepo(t)

	// Create a new untracked file with a function.
	createUntrackedFile(t, repoDir, "newutil.go", `package main

// NewHelper is a new helper function.
func NewHelper() string {
	return "new helper"
}
`)

	// Modify main.go to use the new function and stage it.
	modifyFile(t, filepath.Join(repoDir, "main.go"), `
func UseNewHelper() {
	_ = NewHelper()
}
`)
	stageFiles(t, repoDir, "main.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violation for untracked file dependency, got none")
	}

	// Should have violation from main.go to newutil.go.
	found := false

	for _, v := range violations {
		if v.StagedFile == fileMainGo && v.MissingFile == "newutil.go" {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Expected violation from main.go to newutil.go, violations: %+v", violations)
	}
}

func TestValidateAtomicCommit_NoStagedFiles(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"No Staged Files (Empty Commit Check)",
		"N/A - no dependencies to check",
		"Modified [main.go] | Staged [NONE] | Unstaged [main.go]",
		"No violations - nothing staged means nothing to validate")

	repoDir := setupTestRepo(t)

	// Modify a file but don't stage it.
	modifyFile(t, filepath.Join(repoDir, "main.go"), testComment)

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	// Should have no violations when nothing is staged.
	if len(violations) != 0 {
		t.Errorf("Expected no violations when nothing staged, got %d: %+v", len(violations), violations)
	}
}

func TestValidateAtomicCommit_SpecificSymbol_Function(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Specific Function Symbol Tracking",
		"beta.go (BetaFunc) -> alpha.go (AlphaFunc)",
		"Modified [beta.go, alpha.go] | Staged [beta.go] | Unstaged [alpha.go]",
		"Violation with exact symbol names: BetaFunc -> AlphaFunc")

	repoDir := setupTestRepo(t)

	// Modify beta.go (depends on AlphaFunc from alpha.go) and alpha.go.
	modifyFile(t, filepath.Join(repoDir, "beta.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "alpha.go"), testComment)

	// Stage only beta.go.
	stageFiles(t, repoDir, "beta.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violations, got none")
	}

	// Verify specific symbol names are reported.
	found := false

	for _, v := range violations {
		if v.StagedFile == "beta.go" && v.MissingFile == "alpha.go" && //nolint:goconst // Test data.

			v.StagedSymbol == "example.com/testproject.BetaFunc" &&
			v.MissingSymbol == "example.com/testproject.AlphaFunc" {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Expected violation with specific symbols BetaFunc->AlphaFunc, violations: %+v", violations)
	}
}

func TestValidateAtomicCommit_SpecificSymbol_TransitiveChain(t *testing.T) {
	t.Parallel()

	transitiveViolationTest(t,
		"Transitive Symbol Chain (3 levels)",
		"gamma.go (GammaFunc) -> beta.go (BetaFunc) -> alpha.go (AlphaFunc)",
		"Modified [gamma.go, beta.go, alpha.go] | Staged [gamma.go, beta.go] | Unstaged [alpha.go]",
		"Violations: GammaFunc -> AlphaFunc (transitive) AND BetaFunc -> AlphaFunc (direct)",
		"gamma.go", "beta.go", "alpha.go",
		"gamma.go", "beta.go",
	)
}

func TestValidateAtomicCommit_SpecificSymbol_Constant(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Constant Symbol Dependency",
		"consumer.go (ConsumeConstant func) -> constants.go (MaxRetries const)",
		"Modified [consumer.go, constants.go] | Staged [consumer.go] | Unstaged [constants.go]",
		"Violation tracking constant: ConsumeConstant -> MaxRetries")

	repoDir := setupTestRepo(t)

	// Modify consumer.go (uses MaxRetries from constants.go) and constants.go.
	modifyFile(t, filepath.Join(repoDir, "consumer.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "constants.go"), testComment)

	// Stage only consumer.go.
	stageFiles(t, repoDir, "consumer.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violations for constant dependency, got none")
	}

	// Verify constant symbol is tracked.
	found := false

	for _, v := range violations {
		if v.StagedFile == fileConsumerGo && v.MissingFile == "constants.go" &&
			v.MissingSymbol == "example.com/testproject.MaxRetries" {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Expected violation tracking MaxRetries constant, violations: %+v", violations)
	}
}

func TestValidateAtomicCommit_SpecificSymbol_Variable(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Variable Symbol Dependency",
		"consumer.go (ConsumeVariable func) -> variables.go (DefaultTimeout var)",
		"Modified [consumer.go, variables.go] | Staged [consumer.go] | Unstaged [variables.go]",
		"Violation tracking variable: ConsumeVariable -> DefaultTimeout")

	repoDir := setupTestRepo(t)

	// Modify consumer.go (uses DefaultTimeout from variables.go) and variables.go.
	modifyFile(t, filepath.Join(repoDir, "consumer.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "variables.go"), testComment)

	// Stage only consumer.go.
	stageFiles(t, repoDir, "consumer.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violations for variable dependency, got none")
	}

	// Verify variable symbol is tracked.
	found := false

	for _, v := range violations {
		if v.StagedFile == fileConsumerGo && v.MissingFile == "variables.go" &&
			v.MissingSymbol == "example.com/testproject.DefaultTimeout" {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Expected violation tracking DefaultTimeout variable, violations: %+v", violations)
	}
}

func TestValidateAtomicCommit_SpecificSymbol_Method(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Method Symbol Dependency",
		"calculator_user.go (UseCalculator func) -> calculator.go (Calculator type + Add method)",
		"Modified [calculator_user.go, calculator.go] | Staged [calculator_user.go] | Unstaged [calculator.go]",
		"Violation tracking method: UseCalculator -> Calculator.Add")

	repoDir := setupTestRepo(t)

	// Modify calculator_user.go (uses Calculator.Add from calculator.go) and calculator.go.
	modifyFile(t, filepath.Join(repoDir, "calculator_user.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "calculator.go"), testComment)

	// Stage only calculator_user.go.
	stageFiles(t, repoDir, "calculator_user.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violations for method dependency, got none")
	}

	// The method symbol must be tracked precisely as Calculator.Add, not just
	// via the Calculator type composite literal.
	found := false

	for _, v := range violations {
		if v.StagedFile == "calculator_user.go" && v.MissingFile == "calculator.go" &&
			v.MissingSymbol == "example.com/testproject.Calculator.Add" {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Expected violation tracking Calculator.Add method, violations: %+v", violations)
	}
}

func TestValidateAtomicCommit_MultipleSymbols_PartialViolation(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Partial Violation (Multiple Symbols in One File)",
		"consumer.go has 2 funcs: ConsumeConstant -> MaxRetries (OK), ConsumeVariable -> DefaultTimeout (VIOLATION)",
		"Modified [consumer.go, constants.go, variables.go] | Staged [consumer.go, constants.go] | Unstaged [variables.go]",
		"Only ConsumeVariable violates (depends on unstaged DefaultTimeout), ConsumeConstant is fine")

	repoDir := setupTestRepo(t)

	// Consumer.go has two functions: ConsumeConstant and ConsumeVariable
	// Modify consumer.go, constants.go, but not variables.go.
	modifyFile(t, filepath.Join(repoDir, "consumer.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "constants.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "variables.go"), testComment)

	// Stage consumer.go and constants.go (ConsumeConstant satisfied, ConsumeVariable not).
	stageFiles(t, repoDir, "consumer.go", "constants.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violations, got none")
	}

	// Should have violation for ConsumeVariable -> DefaultTimeout.
	foundVariable := false
	foundConstant := false

	for _, v := range violations {
		if v.StagedFile == fileConsumerGo {
			if v.MissingSymbol == "example.com/testproject.DefaultTimeout" {
				foundVariable = true
			}

			if v.MissingSymbol == "example.com/testproject.MaxRetries" {
				foundConstant = true
			}
		}
	}

	if !foundVariable {
		t.Error("Expected violation for ConsumeVariable depending on DefaultTimeout")
	}

	if foundConstant {
		t.Error("Should NOT have violation for ConsumeConstant since constants.go is staged")
	}
}

func TestValidateAtomicCommit_DependencyOnCommittedFile(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Dependency on Already-Committed File (No Violation)",
		"main.go (main func) -> utils.go (Helper func - ALREADY COMMITTED)",
		"Modified [main.go] | Staged [main.go] | Unmodified [utils.go - already committed]",
		"No violation - utils.go is already in the repository")

	repoDir := setupTestRepo(t)

	// Modify and stage only main.go (which depends on utils.go)
	// utils.go remains unmodified from the initial commit.
	modifyFile(t, filepath.Join(repoDir, "main.go"), "\n// Comment in main\n")
	stageFiles(t, repoDir, "main.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	// Should have no violations - utils.go is already committed.
	if len(violations) != 0 {
		t.Errorf("Expected no violations when depending on committed file, got %d: %+v", len(violations), violations)
	}
}

func TestValidateAtomicCommit_Subpackage_MainToHelper(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Main Package -> Helper Subpackage Violation",
		"processor.go (main) -> helper/validator.go (ValidatePositive, ValidateLength)",
		"Modified [processor.go, helper/validator.go] | Staged [processor.go] | Unstaged [helper/validator.go]",
		"Violation detected - main package file depends on unstaged helper subpackage")

	repoDir := setupTestRepo(t)

	// Modify processor.go and helper/validator.go.
	modifyFile(t, filepath.Join(repoDir, "processor.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "helper", "validator.go"), testComment)

	// Stage only processor.go.
	stageFiles(t, repoDir, "processor.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violations for main->helper dependency, got none")
	}

	// Should have violation from processor.go to helper/validator.go.
	found := false

	for _, v := range violations {
		if v.StagedFile == fileProcessorGo && v.MissingFile == fileHelperValidGo {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Expected violation from processor.go to helper/validator.go, violations: %+v", violations)
	}
}

func TestValidateAtomicCommit_Subpackage_MainToModels(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Main Package -> Models Subpackage Violation",
		"processor.go (main) -> models/request.go (Request type)",
		"Modified [processor.go, models/request.go] | Staged [processor.go] | Unstaged [models/request.go]",
		"Violation detected - main package file depends on unstaged models subpackage")

	repoDir := setupTestRepo(t)

	// Modify processor.go and models/request.go.
	modifyFile(t, filepath.Join(repoDir, "processor.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "models", "request.go"), testComment)

	// Stage only processor.go.
	stageFiles(t, repoDir, "processor.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violations for main->models dependency, got none")
	}

	// Should have violation from processor.go to models/request.go.
	found := false

	for _, v := range violations {
		if v.StagedFile == fileProcessorGo && v.MissingFile == "models/request.go" {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Expected violation from processor.go to models/request.go, violations: %+v", violations)
	}
}

func TestValidateAtomicCommit_Subpackage_CrossPackage(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Cross-Subpackage Dependency Violation",
		"models/response.go -> helper/formatter.go (FormatMessage func)",
		"Modified [models/response.go, helper/formatter.go] | Staged [models/response.go] | Unstaged [helper/formatter.go]",
		"Violation detected - models subpackage depends on unstaged helper subpackage")

	repoDir := setupTestRepo(t)

	// Modify models/response.go and helper/formatter.go.
	modifyFile(t, filepath.Join(repoDir, "models", "response.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "helper", "formatter.go"), testComment)

	// Stage only models/response.go.
	stageFiles(t, repoDir, "models/response.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violations for models->helper dependency, got none")
	}

	// Should have violation from models/response.go to helper/formatter.go.
	found := false

	for _, v := range violations {
		if v.StagedFile == fileModelsResponse && v.MissingFile == fileHelperFmtGo {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Expected violation from models/response.go to helper/formatter.go, violations: %+v", violations)
	}
}

func TestValidateAtomicCommit_Subpackage_TransitiveAcrossPackages(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Transitive Cross-Package Dependency Violation",
		"processor.go (main) -> models/response.go -> helper/formatter.go (transitive chain)",
		"Modified [processor.go, models/response.go, helper/formatter.go] | "+
			"Staged [processor.go, models/response.go] | Unstaged [helper/formatter.go]",
		"Violations detected - both direct and transitive dependencies on unstaged helper/formatter.go")

	repoDir := setupTestRepo(t)

	// Modify processor.go, models/response.go, and helper/formatter.go.
	modifyFile(t, filepath.Join(repoDir, "processor.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "models", "response.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "helper", "formatter.go"), testComment)

	// Stage processor.go and models/response.go, but not helper/formatter.go.
	stageFiles(t, repoDir, "processor.go", "models/response.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violations for transitive cross-package dependency, got none")
	}

	// Should have violations involving helper/formatter.go.
	hasProcessorToFormatter := false
	hasResponseToFormatter := false

	for _, v := range violations {
		if v.StagedFile == fileProcessorGo && v.MissingFile == fileHelperFmtGo {
			hasProcessorToFormatter = true
		}

		if v.StagedFile == fileModelsResponse && v.MissingFile == fileHelperFmtGo {
			hasResponseToFormatter = true
		}
	}

	if !hasResponseToFormatter {
		t.Error("Expected direct violation from models/response.go to helper/formatter.go")
	}

	if !hasProcessorToFormatter {
		t.Error("Expected transitive violation from processor.go to helper/formatter.go")
	}
}

func TestValidateAtomicCommit_Subpackage_CompleteAtomicCommit(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Complete Cross-Package Atomic Commit (No Violations)",
		"processor.go -> models/response.go -> helper/formatter.go (complete chain)",
		"Modified [processor.go, models/response.go, helper/formatter.go, "+
			"models/request.go, helper/validator.go] | Staged [ALL]",
		"No violations - all cross-package dependencies are staged together")

	repoDir := setupTestRepo(t)

	// Modify all related files.
	modifyFile(t, filepath.Join(repoDir, "processor.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "models", "response.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "models", "request.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "helper", "formatter.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "helper", "validator.go"), testComment)

	// Stage all files together.
	stageFiles(t, repoDir, "processor.go", "models/response.go", "models/request.go",
		"helper/formatter.go", "helper/validator.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) != 0 {
		t.Errorf("Expected no violations when all cross-package dependencies are staged, got %d: %+v",
			len(violations), violations)
	}
}

func TestValidateAtomicCommit_Subpackage_MultipleMainToSubpackages(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Main -> Multiple Subpackages Violation",
		"processor.go -> helper/validator.go AND processor.go -> models/request.go (multiple dependencies)",
		"Modified [processor.go, helper/validator.go, models/request.go] | "+
			"Staged [processor.go] | Unstaged [helper/validator.go, models/request.go]",
		"Multiple violations - main file depends on multiple unstaged subpackage files")

	repoDir := setupTestRepo(t)

	// Modify processor.go and both subpackage files.
	modifyFile(t, filepath.Join(repoDir, "processor.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "helper", "validator.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "models", "request.go"), testComment)

	// Stage only processor.go.
	stageFiles(t, repoDir, "processor.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected multiple violations, got none")
	}

	// Should have violations to both subpackage files.
	hasHelperViolation := false
	hasModelsViolation := false

	for _, v := range violations {
		if v.StagedFile == fileProcessorGo && v.MissingFile == fileHelperValidGo {
			hasHelperViolation = true
		}

		if v.StagedFile == fileProcessorGo && v.MissingFile == "models/request.go" {
			hasModelsViolation = true
		}
	}

	if !hasHelperViolation {
		t.Error("Expected violation from processor.go to helper/validator.go")
	}

	if !hasModelsViolation {
		t.Error("Expected violation from processor.go to models/request.go")
	}
}

// writeFileContent overwrites a file with the given content.
func writeFileContent(t *testing.T, path, content string) {
	t.Helper()

	err := os.WriteFile(path, []byte(content), 0o600) //nolint:gosec // Test helper writes to temp dir.
	if err != nil {
		t.Fatalf("Failed to write %s: %v", path, err)
	}
}

func TestValidateAtomicCommit_PartialStaging_NoFalsePositive(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Partial Staging - No False Positive (MM file)",
		"main.go (UsePartialFunc) -> service.go (PartialFunc - staged has it, working tree removes it)",
		"Modified [main.go (staged), service.go (MM)] | Staged version of service.go has PartialFunc",
		"No violation - overlay ensures staged version of service.go is analyzed")

	repoDir := setupTestRepo(t)

	// Step 1: Add PartialFunc to service.go and commit it.
	modifyFile(t, filepath.Join(repoDir, "service.go"), "\nfunc PartialFunc() string { return \"partial\" }\n")
	stageFiles(t, repoDir, "service.go")
	runGit(t, repoDir, "commit", "-m", "Add PartialFunc")

	// Step 2: Modify main.go to call PartialFunc and stage it.
	modifyFile(t, filepath.Join(repoDir, "main.go"), "\nfunc UsePartialFunc() { _ = PartialFunc() }\n")
	stageFiles(t, repoDir, "main.go")

	// Step 3: Also modify service.go (stage the change that keeps PartialFunc).
	modifyFile(t, filepath.Join(repoDir, "service.go"), "\n// Another comment\n")
	stageFiles(t, repoDir, "service.go")

	// Step 4: Now remove PartialFunc in the working tree (creating MM state).
	// Read current content and rewrite without PartialFunc.
	data, err := os.ReadFile(filepath.Join(repoDir, "service.go")) //nolint:gosec // Test reads temp file.
	if err != nil {
		t.Fatalf("Failed to read service.go: %v", err)
	}

	// Remove the PartialFunc line from working tree.
	modified := strings.ReplaceAll(string(data), "func PartialFunc() string { return \"partial\" }\n", "")
	writeFileContent(t, filepath.Join(repoDir, "service.go"), modified)

	// Now service.go is MM: staged version has PartialFunc, working tree does not.

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	// Should have NO violations because the staged version of service.go still has PartialFunc.
	for _, v := range violations {
		if v.MissingSymbol == "example.com/testproject.PartialFunc" {
			t.Errorf("Got false positive violation for PartialFunc (should use staged version): %+v", v)
		}
	}
}

func TestValidateAtomicCommit_PartialStaging_StagedSymbolPresent(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Partial Staging - Symbol Present in Both Versions",
		"main.go (UseHelper) -> utils.go (Helper - present in both staged and working tree)",
		"Modified [main.go (staged), utils.go (MM)] | Both versions of utils.go have Helper",
		"No violation - symbol exists in staged version")

	repoDir := setupTestRepo(t)

	// Modify main.go and stage it.
	modifyFile(t, filepath.Join(repoDir, "main.go"), "\n// Comment for staging\n")
	stageFiles(t, repoDir, "main.go")

	// Modify utils.go and stage it (creates staged change).
	modifyFile(t, filepath.Join(repoDir, "utils.go"), "\n// Staged comment\n")
	stageFiles(t, repoDir, "utils.go")

	// Modify utils.go again without staging (creates MM state).
	modifyFile(t, filepath.Join(repoDir, "utils.go"), "\n// Unstaged comment\n")

	// Now utils.go is MM but Helper is present in both versions.

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	// Should have no violations - utils.go is staged and Helper exists.
	for _, v := range violations {
		if v.StagedFile == fileMainGo && v.MissingFile == fileUtilsGo {
			t.Errorf("Unexpected violation for utils.go which is staged: %+v", v)
		}
	}
}

func TestValidateAtomicCommit_UnstagedOnly_StillFlagsViolation(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Unstaged Only - Still Flags Violation",
		"main.go (staged) -> utils.go (unstaged only, space-M status)",
		"Modified [main.go (staged), utils.go (unstaged only)] | utils.go has only working tree changes",
		"Violation detected - overlay logic does not suppress legitimate violations")

	repoDir := setupTestRepo(t)

	// Modify main.go and stage it.
	modifyFile(t, filepath.Join(repoDir, "main.go"), testComment)
	stageFiles(t, repoDir, "main.go")

	// Modify utils.go but do NOT stage it (creates ` M` status, not `MM`).
	modifyFile(t, filepath.Join(repoDir, "utils.go"), "\n// Unstaged change\n")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	// Should have violations - utils.go has unstaged changes and is not staged.
	found := false

	for _, v := range violations {
		if v.StagedFile == fileMainGo && v.MissingFile == fileUtilsGo {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Expected violation from main.go to utils.go for unstaged-only file, violations: %+v", violations)
	}
}

func TestValidateAtomicCommit_Subpackage_PartialStaging(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Partial Subpackage Staging (Within Same Package)",
		"models/response.go -> helper/formatter.go (cross-package dep)",
		"Modified [models/response.go, models/request.go, helper/formatter.go] | "+
			"Staged [models/response.go, models/request.go]",
		"Violation detected - models package files staged but missing helper dependency")

	repoDir := setupTestRepo(t)

	// Modify both models files and helper/formatter.go.
	modifyFile(t, filepath.Join(repoDir, "models", "response.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "models", "request.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "helper", "formatter.go"), testComment)

	// Stage both models files but not helper.
	stageFiles(t, repoDir, "models/response.go", "models/request.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violations, got none")
	}

	// Should have violation from models/response.go to helper/formatter.go.
	found := false

	for _, v := range violations {
		if v.StagedFile == fileModelsResponse && v.MissingFile == fileHelperFmtGo {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Expected violation from models/response.go to helper/formatter.go, violations: %+v", violations)
	}
}

// ===========================.
// FindCommittableFiles Tests.
// ===========================.

func TestFindCommittableFiles_IndependentFiles(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"FindCommittableFiles - Independent Files",
		"alpha.go and beta.go (both independent, no dependencies)",
		"Modified [alpha.go, beta.go] | Unstaged [alpha.go, beta.go]",
		"Should return alpha.go (first lexicographically)")

	repoDir := setupTestRepo(t)

	// Modify both alpha.go and beta.go (both are independent).
	modifyFile(t, filepath.Join(repoDir, "alpha.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "beta.go"), testComment)

	files, err := validator.FindCommittableSet(t.Context(), repoDir, false)
	if err != nil {
		t.Fatalf("FindCommittableFiles failed: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("Expected 1 file, got %d: %v", len(files), files)
	}

	if files[0] != "alpha.go" {
		t.Errorf("Expected alpha.go, got %s", files[0])
	}
}

func TestFindCommittableFiles_WithDependency(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"FindCommittableFiles - Dependency Chain",
		"main.go -> utils.go (main depends on utils)",
		"Modified [main.go, utils.go] | Unstaged [main.go, utils.go]",
		"Should return utils.go (independent), main.go is not independent")

	repoDir := setupTestRepo(t)

	// Modify main.go (depends on utils.go) and utils.go.
	modifyFile(t, filepath.Join(repoDir, "main.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "utils.go"), testComment)

	files, err := validator.FindCommittableSet(t.Context(), repoDir, false)
	if err != nil {
		t.Fatalf("FindCommittableFiles failed: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("Expected 1 file, got %d: %v", len(files), files)
	}

	if files[0] != "utils.go" {
		t.Errorf("Expected utils.go (independent), got %s", files[0])
	}

	// Commit utils.go and check again - now main.go should be committable.
	stageFiles(t, repoDir, "utils.go")
	runGit(t, repoDir, "commit", "-m", "update utils")

	files, err = validator.FindCommittableSet(t.Context(), repoDir, false)
	if err != nil {
		t.Fatalf("FindCommittableFiles failed after committing utils: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("Expected 1 file after utils committed, got %d: %v", len(files), files)
	}

	if files[0] != "main.go" {
		t.Errorf("Expected main.go after utils committed, got %s", files[0])
	}
}

func TestFindCommittableFiles_CircularDependency(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"FindCommittableFiles - Circular Dependency",
		"circular_a.go <-> circular_b.go (circular dependency)",
		"Modified [circular_a.go, circular_b.go] | Unstaged [circular_a.go, circular_b.go]",
		"Should return empty (neither file is independent)")

	repoDir := setupTestRepo(t)

	// Modify both circular files.
	modifyFile(t, filepath.Join(repoDir, "circular_a.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "circular_b.go"), testComment)

	files, err := validator.FindCommittableSet(t.Context(), repoDir, false)
	if err != nil {
		t.Fatalf("FindCommittableFiles failed: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("Expected no files for circular dependency, got %d: %v", len(files), files)
	}
}

func TestFindCommittableFiles_UntrackedFile(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"FindCommittableFiles - Untracked File",
		"newfile.go (untracked, no dependencies)",
		"Untracked [newfile.go]",
		"Should return newfile.go (independent)")

	repoDir := setupTestRepo(t)

	// Create a new untracked file with no dependencies.
	createUntrackedFile(t, repoDir, "newfile.go", `package main

// IndependentFunc is a new independent function.
func IndependentFunc() string {
	return "independent"
}
`)

	files, err := validator.FindCommittableSet(t.Context(), repoDir, false)
	if err != nil {
		t.Fatalf("FindCommittableFiles failed: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("Expected 1 file, got %d: %v", len(files), files)
	}

	if files[0] != "newfile.go" {
		t.Errorf("Expected newfile.go, got %s", files[0])
	}
}

func TestFindCommittableFiles_NoModifications(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"FindCommittableFiles - No Modifications",
		"N/A - clean working directory",
		"Modified [] | Unstaged []",
		"Should return empty (no files to commit)")

	repoDir := setupTestRepo(t)

	files, err := validator.FindCommittableSet(t.Context(), repoDir, false)
	if err != nil {
		t.Fatalf("FindCommittableFiles failed: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("Expected no files for clean repo, got %d: %v", len(files), files)
	}
}

func TestFindCommittableFiles_ExcludesStagedFiles(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"FindCommittableFiles - Excludes Staged Files",
		"alpha.go (staged), beta.go (unstaged)",
		"Modified [alpha.go, beta.go] | Staged [alpha.go] | Unstaged [beta.go]",
		"Should return beta.go only (exclude already-staged files)")

	repoDir := setupTestRepo(t)

	// Modify and stage alpha.go.
	modifyFile(t, filepath.Join(repoDir, "alpha.go"), testComment)
	stageFiles(t, repoDir, "alpha.go")

	// Modify beta.go but don't stage it.
	modifyFile(t, filepath.Join(repoDir, "beta.go"), testComment)

	files, err := validator.FindCommittableSet(t.Context(), repoDir, false)
	if err != nil {
		t.Fatalf("FindCommittableFiles failed: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("Expected 1 file, got %d: %v", len(files), files)
	}

	if files[0] != "beta.go" {
		t.Errorf("Expected beta.go (unstaged), got %s", files[0])
	}
}

func TestFindCommittableFiles_ExcludesPartiallyStaged(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"FindCommittableFiles - Excludes Partially Staged (MM)",
		"main.go (MM status)",
		"Modified [main.go] | Staged [main.go (partial)] | Unstaged [main.go (partial)]",
		"Should NOT return main.go (let standard mode handle MM files)")

	repoDir := setupTestRepo(t)

	// Modify and stage main.go.
	modifyFile(t, filepath.Join(repoDir, "main.go"), "\n// Staged comment\n")
	stageFiles(t, repoDir, "main.go")

	// Modify main.go again without staging (creates MM state).
	modifyFile(t, filepath.Join(repoDir, "main.go"), "\n// Unstaged comment\n")

	files, err := validator.FindCommittableSet(t.Context(), repoDir, false)
	if err != nil {
		t.Fatalf("FindCommittableFiles failed: %v", err)
	}

	// Should return empty - MM files are excluded from candidates.
	if len(files) != 0 {
		t.Errorf("Expected no files for MM status, got %d: %v", len(files), files)
	}
}

func TestFindCommittableFiles_ProgressiveCommit(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"FindCommittableFiles - Progressive Commit Workflow",
		"alpha.go, beta.go, gamma.go (all independent)",
		"Modified [alpha.go, beta.go, gamma.go] | Unstaged [all]",
		"Progressive: alpha -> beta -> gamma (lexicographic order)")

	repoDir := setupTestRepo(t)

	// Modify all three independent files.
	modifyFile(t, filepath.Join(repoDir, "alpha.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "beta.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "gamma.go"), testComment)

	// First call should return alpha.go.
	files, err := validator.FindCommittableSet(t.Context(), repoDir, false)
	if err != nil {
		t.Fatalf("FindCommittableFiles failed (1st call): %v", err)
	}

	if len(files) != 1 || files[0] != "alpha.go" {
		t.Errorf("Expected alpha.go on first call, got %v", files)
	}

	// Commit alpha.go.
	stageFiles(t, repoDir, "alpha.go")
	runGit(t, repoDir, "commit", "-m", "update alpha")

	// Second call should return beta.go.
	files, err = validator.FindCommittableSet(t.Context(), repoDir, false)
	if err != nil {
		t.Fatalf("FindCommittableFiles failed (2nd call): %v", err)
	}

	if len(files) != 1 || files[0] != "beta.go" {
		t.Errorf("Expected beta.go on second call, got %v", files)
	}

	// Commit beta.go.
	stageFiles(t, repoDir, "beta.go")
	runGit(t, repoDir, "commit", "-m", "update beta")

	// Third call should return gamma.go.
	files, err = validator.FindCommittableSet(t.Context(), repoDir, false)
	if err != nil {
		t.Fatalf("FindCommittableFiles failed (3rd call): %v", err)
	}

	if len(files) != 1 || files[0] != "gamma.go" {
		t.Errorf("Expected gamma.go on third call, got %v", files)
	}

	// Commit gamma.go.
	stageFiles(t, repoDir, "gamma.go")
	runGit(t, repoDir, "commit", "-m", "update gamma")

	// Fourth call should return empty (all committed).
	files, err = validator.FindCommittableSet(t.Context(), repoDir, false)
	if err != nil {
		t.Fatalf("FindCommittableFiles failed (4th call): %v", err)
	}

	if len(files) != 0 {
		t.Errorf("Expected no files after all committed, got %v", files)
	}
}

// TestFindCommittableSet_WithDependants_SingleDependant tests the --dependants flag
// with a base file and one dependant.
func TestFindCommittableSet_WithDependants_SingleDependant(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"FindCommittableSet - With Dependants - Single Dependant",
		"types.go (independent) <- service.go (depends on types.go)",
		"Modified [types.go, service.go] | Unstaged [types.go, service.go]",
		"Should return [types.go, service.go] when --dependants is used")

	repoDir := setupTestRepo(t)

	// Modify types.go (independent) and service.go (depends on types.go).
	modifyFile(t, filepath.Join(repoDir, "types.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "service.go"), testComment)

	// Without dependants flag - should return only types.go.
	files, err := validator.FindCommittableSet(t.Context(), repoDir, false)
	if err != nil {
		t.Fatalf("FindCommittableSet failed: %v", err)
	}

	if len(files) != 1 || files[0] != "types.go" {
		t.Fatalf("Without dependants: expected [types.go], got %v", files)
	}

	// With dependants flag - should return types.go + service.go.
	files, err = validator.FindCommittableSet(t.Context(), repoDir, true)
	if err != nil {
		t.Fatalf("FindCommittableSet with dependants failed: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("With dependants: expected 2 files, got %d: %v", len(files), files)
	}

	// Result should include both types.go (base) and service.go (dependant).
	// The base file comes first, then dependants in sorted order.
	expectedFiles := map[string]bool{"types.go": true, "service.go": true}
	for _, f := range files {
		if !expectedFiles[f] {
			t.Errorf("Unexpected file in result: %s", f)
		}
	}
}

// TestFindCommittableSet_WithDependants_MultipleDependants tests the --dependants flag
// with a base file and multiple dependants.
func TestFindCommittableSet_WithDependants_MultipleDependants(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"FindCommittableSet - With Dependants - Multiple Dependants",
		"utils.go (independent) <- service.go (depends on utils.go) and main.go (depends on utils.go)",
		"Modified [utils.go, service.go, main.go] | Unstaged [utils.go, service.go, main.go]",
		"Should return [main.go, service.go, utils.go] when --dependants is used")

	repoDir := setupTestRepo(t)

	// Modify utils.go (independent), service.go (depends on utils.go), and main.go (depends on utils.go).
	modifyFile(t, filepath.Join(repoDir, "utils.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "service.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "main.go"), testComment)

	// With dependants flag - should return utils.go + its dependants.
	files, err := validator.FindCommittableSet(t.Context(), repoDir, true)
	if err != nil {
		t.Fatalf("FindCommittableSet with dependants failed: %v", err)
	}

	// Should include utils.go, service.go (depends only on utils.go), and main.go (depends on utils.go + service.go).
	// Note: main.go depends on both utils.go AND service.go, so it should NOT be included.
	if len(files) < 2 {
		t.Fatalf("With dependants: expected at least 2 files, got %d: %v", len(files), files)
	}

	// Verify utils.go is in the result (base file).
	if !slices.Contains(files, "utils.go") {
		t.Errorf("With dependants: expected utils.go in results, got %v", files)
	}
}

// TestFindCommittableSet_WithDependants_DirectDependantWithUnrelatedChangesetDep
// tests that a direct dependant is still excluded when it depends on a
// changeset file that is neither baseFile nor another accepted dependant --
// unlike the shared-helper case, this dependency has nothing to do with
// baseFile, so pulling it in would produce a non-compiling commit.
func TestFindCommittableSet_WithDependants_DirectDependantWithUnrelatedChangesetDep(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"FindCommittableSet - With Dependants - Unrelated Changeset Dependency",
		"beacon.go (NewBeacon) <- {a_test.go (also uses sibling.go's SidecarValue), b_test.go}; "+
			"sibling.go is unrelated and also in the changeset",
		"Untracked [beacon.go, sibling.go, a_test.go, b_test.go]",
		"Should return [beacon.go, b_test.go], excluding a_test.go (depends on unrelated sibling.go)")

	repoDir := setupTestRepo(t)

	createUntrackedFile(t, repoDir, "beacon.go", `package main

// Beacon is a simple marker type.
type Beacon struct{}

// NewBeacon creates a beacon.
func NewBeacon() *Beacon {
	return &Beacon{}
}
`)
	createUntrackedFile(t, repoDir, "sibling.go", `package main

// SidecarValue is unrelated to Beacon.
const SidecarValue = 42
`)
	createUntrackedFile(t, repoDir, "a_test.go", `package main

import "testing"

// TestAUsesBeaconAndSidecar calls NewBeacon directly but also depends on
// sibling.go's SidecarValue, an unrelated changeset file.
func TestAUsesBeaconAndSidecar(t *testing.T) {
	b := NewBeacon()
	_ = b
	_ = SidecarValue
}
`)
	createUntrackedFile(t, repoDir, "b_test.go", `package main

import "testing"

func TestBUsesBeacon(t *testing.T) {
	b := NewBeacon()
	_ = b
}
`)

	files, err := validator.FindCommittableSet(t.Context(), repoDir, true)
	if err != nil {
		t.Fatalf("FindCommittableSet with dependants failed: %v", err)
	}

	if !slices.Contains(files, "beacon.go") {
		t.Errorf("Expected beacon.go (base file) in results, got %v", files)
	}

	if !slices.Contains(files, "b_test.go") {
		t.Errorf("Expected b_test.go (direct dependant with no other changeset deps) in results, got %v", files)
	}

	if slices.Contains(files, "a_test.go") {
		t.Errorf("Should NOT include a_test.go (depends on unrelated changeset file sibling.go), got %v", files)
	}
}

// TestFindCommittableSet_WithDependants_ExcludeTransitiveDependants tests that
// transitive dependants (dependants of dependants that have no direct
// dependency of their own on baseFile) are excluded. File gamma.go calls
// only BetaFunc, never AlphaFunc directly, so it must stay out of the set
// even though beta.go (a direct dependant of alpha.go) is included.
func TestFindCommittableSet_WithDependants_ExcludeTransitiveDependants(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"FindCommittableSet - With Dependants - Exclude Transitive",
		"alpha.go <- beta.go (calls AlphaFunc) <- gamma.go (calls only BetaFunc, chain)",
		"Modified [alpha.go, beta.go, gamma.go] | Unstaged [alpha.go, beta.go, gamma.go]",
		"Should return [alpha.go, beta.go] excluding gamma.go (transitive)")

	repoDir := setupTestRepo(t)

	// Modify alpha.go, beta.go (depends on alpha.go), and gamma.go (depends
	// only on beta.go, never alpha.go directly).
	modifyFile(t, filepath.Join(repoDir, "alpha.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "beta.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "gamma.go"), testComment)

	// With dependants flag.
	files, err := validator.FindCommittableSet(t.Context(), repoDir, true)
	if err != nil {
		t.Fatalf("FindCommittableSet with dependants failed: %v", err)
	}

	// File gamma.go has no direct dependency on alpha.go (only on beta.go),
	// so it should be excluded even though beta.go is committed alongside
	// alpha.go.
	if slices.Contains(files, "gamma.go") {
		t.Errorf("Should NOT include gamma.go (only a transitive dependant via beta.go), got %v", files)
	}

	if !slices.Contains(files, "beta.go") {
		t.Errorf("Expected beta.go in results (direct dependant of alpha.go), got %v", files)
	}
}

// TestFindCommittableSet_WithDependants_CircularDependency tests behavior with circular deps.
func TestFindCommittableSet_WithDependants_CircularDependency(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"FindCommittableSet - With Dependants - Circular Dependency",
		"circular_a.go <-> circular_b.go (circular)",
		"Modified [circular_a.go, circular_b.go] | Unstaged [circular_a.go, circular_b.go]",
		"Should return empty (no independent file due to circular dependency)")

	repoDir := setupTestRepo(t)

	// Modify both files with circular dependency.
	modifyFile(t, filepath.Join(repoDir, "circular_a.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "circular_b.go"), testComment)

	files, err := validator.FindCommittableSet(t.Context(), repoDir, true)
	if err != nil {
		t.Fatalf("FindCommittableSet failed: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("Expected empty result for circular dependency, got %v", files)
	}
}

// TestFindCommittableSet_WithDependants_NoDependants tests a file with no dependants.
func TestFindCommittableSet_WithDependants_NoDependants(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"FindCommittableSet - With Dependants - No Dependants",
		"alpha.go (independent, no dependants)",
		"Modified [alpha.go] | Unstaged [alpha.go]",
		"Should return [alpha.go] only (no dependants exist)")

	repoDir := setupTestRepo(t)

	// Modify alpha.go (independent, no other files depend on it).
	modifyFile(t, filepath.Join(repoDir, "alpha.go"), testComment)

	files, err := validator.FindCommittableSet(t.Context(), repoDir, true)
	if err != nil {
		t.Fatalf("FindCommittableSet failed: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("Expected 1 file, got %d: %v", len(files), files)
	}

	if files[0] != "alpha.go" {
		t.Errorf("Expected alpha.go, got %s", files[0])
	}
}

// TestFindCommittableSet_WithDependants_SharedHelperAcrossTestFiles reproduces
// a real false negative: a signature change to an exported function
// (NewTracker) with direct callers spread across six _test.go files, two of
// which also share a test helper defined in a third direct-dependant file.
// A dependant must not be excluded merely because it ALSO depends on a
// sibling file that is itself a direct dependant being committed in the same
// set (previously, canCommitWithBase only allowed dependencies on baseFile
// itself, so any dependant sharing a helper with another dependant was
// wrongly dropped).
func TestFindCommittableSet_WithDependants_SharedHelperAcrossTestFiles(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"FindCommittableSet - With Dependants - Shared Helper Across Test Files",
		"tracker.go (NewTracker) <- {tracker_test.go, helpers_test.go, alpha_tracker_test.go,\n"+
			"         gamma_tracker_test.go, delta_tracker_test.go} (direct callers); "+
			"beta_tracker_test.go calls NewTracker only via helpers_test.go's helper (transitive, not direct)",
		"Modified [all 7 files] | Unstaged [all 7 files] (signature change: NewTracker gains an error return)",
		"Should return tracker.go plus all 5 direct dependants, excluding beta_tracker_test.go")

	repoDir := setupTestRepo(t)

	fileOrder := trackerFileOrder()

	for _, name := range fileOrder {
		createUntrackedFile(t, repoDir, name, trackerFilesOldSignature()[name])
	}

	stageFiles(t, repoDir, fileOrder...)
	runGit(t, repoDir, "commit", "-m", "Add tracker with single-return NewTracker")

	// Change NewTracker's signature to add an error return, and fix every
	// caller accordingly -- all seven files become modified and unstaged.
	for _, name := range fileOrder {
		createUntrackedFile(t, repoDir, name, trackerFilesNewSignature()[name])
	}

	files, err := validator.FindCommittableSet(t.Context(), repoDir, true)
	if err != nil {
		t.Fatalf("FindCommittableSet with dependants failed: %v", err)
	}

	assertTrackerDependants(t, files)
}

// trackerFileOrder lists the tracker test fixture files in a deterministic order.
func trackerFileOrder() []string {
	return []string{
		"tracker.go", "tracker_test.go", "helpers_test.go",
		"alpha_tracker_test.go", "beta_tracker_test.go",
		"gamma_tracker_test.go", "delta_tracker_test.go",
	}
}

// assertTrackerDependants checks that every direct caller of NewTracker is
// present, and that the transitive-only caller (beta_tracker_test.go) is not.
func assertTrackerDependants(t *testing.T, files []string) {
	t.Helper()

	expected := []string{
		"tracker.go", "tracker_test.go", "helpers_test.go",
		"alpha_tracker_test.go", "gamma_tracker_test.go", "delta_tracker_test.go",
	}
	for _, want := range expected {
		if !slices.Contains(files, want) {
			t.Errorf("Expected %s in results (direct caller of NewTracker), got %v", want, files)
		}
	}

	if slices.Contains(files, "beta_tracker_test.go") {
		t.Errorf("Should NOT include beta_tracker_test.go (only a transitive dependant via helpers_test.go), got %v", files)
	}

	if len(files) != len(expected) {
		t.Errorf("Expected %d files, got %d: %v", len(expected), len(files), files)
	}
}

// trackerFilesOldSignature returns the tracker fixture files with NewTracker's
// original single-return signature.
func trackerFilesOldSignature() map[string]string {
	return map[string]string{
		"tracker.go": `package main

// Tracker tracks something against a cap.
type Tracker struct {
	Cap float64
}

// NewTracker creates a tracker.
func NewTracker(cap float64) *Tracker {
	return &Tracker{Cap: cap}
}
`,
		"tracker_test.go": `package main

import "testing"

func TestNewTracker(t *testing.T) {
	tr := NewTracker(1.0)
	_ = tr
}
`,
		"helpers_test.go": `package main

import "testing"

// newTestTracker is a shared test helper used by multiple _test.go files.
func newTestTracker(t *testing.T) *Tracker {
	t.Helper()

	return NewTracker(2.0)
}
`,
		"alpha_tracker_test.go": `package main

import "testing"

func TestAlphaUsesTracker(t *testing.T) {
	tr := NewTracker(3.0)
	_ = tr
}
`,
		"beta_tracker_test.go": `package main

import "testing"

// TestBetaUsesTracker only calls NewTracker indirectly, via helpers_test.go's
// helper -- it is a transitive dependant, not a direct one.
func TestBetaUsesTracker(t *testing.T) {
	tr := newTestTracker(t)
	_ = tr
}
`,
		"gamma_tracker_test.go": `package main

import "testing"

// TestGammaUsesTracker calls NewTracker directly AND uses the shared helper
// from helpers_test.go, mirroring cache_test.go in the reported bug.
func TestGammaUsesTracker(t *testing.T) {
	tr := NewTracker(4.0)
	helper := newTestTracker(t)
	_ = tr
	_ = helper
}
`,
		"delta_tracker_test.go": `package main

import "testing"

// TestDeltaUsesTracker calls NewTracker directly AND uses the shared helper
// from helpers_test.go, mirroring observability_test.go in the reported bug.
func TestDeltaUsesTracker(t *testing.T) {
	tr := NewTracker(5.0)
	helper := newTestTracker(t)
	_ = tr
	_ = helper
}
`,
	}
}

// trackerFilesNewSignature returns the tracker fixture files updated for
// NewTracker's new (capUSD, error) signature.
func trackerFilesNewSignature() map[string]string {
	files := trackerFilesNewSignatureCore()
	maps.Copy(files, trackerFilesNewSignatureTests())

	return files
}

// trackerFilesNewSignatureCore returns the non-test tracker fixture files
// updated for NewTracker's new (capUSD, error) signature.
func trackerFilesNewSignatureCore() map[string]string {
	return map[string]string{
		"tracker.go": `package main

import "errors"

// Tracker tracks something against a cap.
type Tracker struct {
	Cap float64
}

// NewTracker creates a tracker, rejecting a negative cap.
func NewTracker(cap float64) (*Tracker, error) {
	if cap < 0 {
		return nil, errors.New("negative cap")
	}

	return &Tracker{Cap: cap}, nil
}
`,
		"tracker_test.go": `package main

import "testing"

func TestNewTracker(t *testing.T) {
	tr, err := NewTracker(1.0)
	if err != nil {
		t.Fatal(err)
	}
	_ = tr
}
`,
		"helpers_test.go": `package main

import "testing"

// newTestTracker is a shared test helper used by multiple _test.go files.
func newTestTracker(t *testing.T) *Tracker {
	t.Helper()

	tr, err := NewTracker(2.0)
	if err != nil {
		t.Fatal(err)
	}

	return tr
}
`,
	}
}

// trackerFilesNewSignatureTests returns the remaining _test.go tracker
// fixture files updated for NewTracker's new (capUSD, error) signature.
func trackerFilesNewSignatureTests() map[string]string {
	return map[string]string{
		"alpha_tracker_test.go": `package main

import "testing"

func TestAlphaUsesTracker(t *testing.T) {
	tr, err := NewTracker(3.0)
	if err != nil {
		t.Fatal(err)
	}
	_ = tr
}
`,
		"beta_tracker_test.go": `package main

import "testing"

// TestBetaUsesTracker only calls NewTracker indirectly, via helpers_test.go's
// helper -- it is a transitive dependant, not a direct one.
func TestBetaUsesTracker(t *testing.T) {
	tr := newTestTracker(t)
	_ = tr
}
`,
		"gamma_tracker_test.go": `package main

import "testing"

// TestGammaUsesTracker calls NewTracker directly AND uses the shared helper
// from helpers_test.go, mirroring cache_test.go in the reported bug.
func TestGammaUsesTracker(t *testing.T) {
	tr, err := NewTracker(4.0)
	if err != nil {
		t.Fatal(err)
	}
	helper := newTestTracker(t)
	_ = tr
	_ = helper
}
`,
		"delta_tracker_test.go": `package main

import "testing"

// TestDeltaUsesTracker calls NewTracker directly AND uses the shared helper
// from helpers_test.go, mirroring observability_test.go in the reported bug.
func TestDeltaUsesTracker(t *testing.T) {
	tr, err := NewTracker(5.0)
	if err != nil {
		t.Fatal(err)
	}
	helper := newTestTracker(t)
	_ = tr
	_ = helper
}
`,
	}
}

// TestValidateAtomicCommit_UntrackedFileDoesNotBreakAnalysis verifies that
// untracked files referencing symbols absent from the staged codebase do not
// cause compilation errors during analysis.
func TestValidateAtomicCommit_UntrackedFileDoesNotBreakAnalysis(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Untracked File With Forward Reference",
		"utils.go (staged+worktree, MM) | untracked_consumer.go (??, references WorkingTreeFunc)",
		"Staged [utils.go comment] | WorkingTree [utils.go+WorkingTreeFunc] | Untracked [untracked_consumer.go]",
		"No compilation error - untracked file is excluded from analysis")

	repoDir := setupTestRepo(t)

	// Stage utils.go with a comment.
	modifyFile(t, filepath.Join(repoDir, fileUtilsGo), testComment)
	stageFiles(t, repoDir, fileUtilsGo)

	// Add WorkingTreeFunc to utils.go working tree only (not staged, MM status).
	wtFunc := "\n// WorkingTreeFunc is only in the working tree.\n" +
		"func WorkingTreeFunc() string { return \"wt\" }\n"
	modifyFile(t, filepath.Join(repoDir, fileUtilsGo), wtFunc)

	// Create an untracked file that calls WorkingTreeFunc (only in working tree of utils.go).
	untrackedContent := "package main\n\n" +
		"// UntrackedConsumer calls a working-tree-only function.\n" +
		"func UntrackedConsumer() string { return WorkingTreeFunc() }\n"
	createUntrackedFile(t, repoDir, "untracked_consumer.go", untrackedContent)

	// Should not return a compilation error - untracked file is neutralised.
	_, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed (expected no error): %v", err)
	}
}

// TestValidateAtomicCommit_StructFieldTypeDependency verifies that a struct type
// whose field type references an untracked subpackage is flagged as a violation.
// This tests that type-level (non-function) symbol usages are tracked correctly.
func TestValidateAtomicCommit_StructFieldTypeDependency(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Struct Field Type Dependency (type-level reference)",
		"repo.go (staged, defines Repo struct with NewStore field typed as store.Writer) | store/writer.go (untracked)",
		"Staged [repo.go] | Untracked [store/writer.go defines store.Writer interface]",
		"Violation detected - Repo struct field type depends on untracked store.Writer")

	repoDir := setupTestRepo(t)
	storeDir := createUntrackedSubpackage(t, repoDir, "store")

	writeFileContent(t, filepath.Join(storeDir, "writer.go"),
		"package store\n\n// Writer is a write interface.\ntype Writer interface { Write(p []byte) (int, error) }\n")

	// Create a staged file whose struct field type references store.Writer.
	writeFileContent(t, filepath.Join(repoDir, "repo.go"),
		"package main\n\nimport \"example.com/testproject/store\"\n\n"+
			"// Repo holds repo operations.\ntype Repo struct {\n"+
			"\tNewWriter func() store.Writer\n}\n")
	stageFiles(t, repoDir, "repo.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violation for struct field type dependency on untracked store.Writer, got none")
	}

	found := false

	for _, v := range violations {
		if v.StagedFile == "repo.go" && v.MissingFile == "store/writer.go" {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Expected violation from repo.go to store/writer.go, violations: %+v", violations)
	}
}

// TestValidateAtomicCommit_VarTypeDependency verifies that a package-level var
// whose type or initializer references an untracked subpackage is flagged.
func TestValidateAtomicCommit_VarTypeDependency(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Var Type + Initializer Dependency",
		"vars.go (staged, defines var with store.Writer type and store.New initializer) | store/writer.go (untracked)",
		"Staged [vars.go] | Untracked [store/writer.go defines store.Writer and store.New]",
		"Violation detected - var type and initializer depend on untracked store package")

	repoDir := setupTestRepo(t)
	storeDir := createUntrackedSubpackage(t, repoDir, "store")

	writeFileContent(t, filepath.Join(storeDir, "writer.go"),
		"package store\n\n"+
			"// Writer is a write interface.\n"+
			"type Writer interface { Write(p []byte) (int, error) }\n\n"+
			"// New creates a new Writer.\n"+
			"func New() Writer { return nil }\n")

	// Create a staged file with a var that depends on store.Writer (type) and store.New (initializer).
	writeFileContent(t, filepath.Join(repoDir, "vars.go"),
		"package main\n\nimport \"example.com/testproject/store\"\n\n"+
			"// DefaultWriter is the default writer.\n"+
			"var DefaultWriter store.Writer = store.New()\n")
	stageFiles(t, repoDir, "vars.go")

	violations, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed: %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violation for var type dependency on untracked store package, got none")
	}

	// Check both the type annotation (store.Writer) and initializer (store.New) are caught.
	foundType := false
	foundInit := false

	for _, v := range violations {
		if v.StagedFile == "vars.go" && v.MissingFile == "store/writer.go" {
			if v.MissingSymbol == "example.com/testproject/store.Writer" {
				foundType = true
			}

			if v.MissingSymbol == "example.com/testproject/store.New" {
				foundInit = true
			}
		}
	}

	if !foundType {
		t.Errorf("Expected violation for store.Writer type annotation, violations: %+v", violations)
	}

	if !foundInit {
		t.Errorf("Expected violation for store.New initializer, violations: %+v", violations)
	}
}

// ===========================.
// DetectChangesets Tests.
// ===========================.

// assertChangesets compares two changeset lists ignoring the top-level order
// of groups. Files within each group are already sorted lexicographically by
// DetectChangesets, so equal groups compare directly.
func assertChangesets(t *testing.T, got, want [][]string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("Expected %d changesets, got %d: %v", len(want), len(got), got)
	}

	gotSorted := sortGroups(got)
	wantSorted := sortGroups(want)

	if !slices.EqualFunc(gotSorted, wantSorted, slices.Equal[[]string]) {
		t.Errorf("Changesets mismatch:\n got:  %v\n want: %v", gotSorted, wantSorted)
	}
}

// sortGroups returns a copy of groups sorted by the first element of each
// group so test assertions are independent of BFS iteration order.
func sortGroups(groups [][]string) [][]string {
	out := make([][]string, len(groups))
	for i, g := range groups {
		cp := make([]string, len(g))
		copy(cp, g)
		slices.Sort(cp)
		out[i] = cp
	}

	slices.SortFunc(out, func(a, b []string) int {
		return strings.Compare(a[0], b[0])
	})

	return out
}

func TestDetectChangesets_SingleFile_OneGroup(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"DetectChangesets - Single Staged File",
		"main.go alone (its committed deps do not create staged-set edges)",
		"Modified [main.go] | Staged [main.go]",
		"1 changeset containing main.go")

	repoDir := setupTestRepo(t)

	modifyFile(t, filepath.Join(repoDir, fileMainGo), testComment)
	stageFiles(t, repoDir, fileMainGo)

	got, err := validator.DetectChangesets(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("DetectChangesets failed: %v", err)
	}

	assertChangesets(t, got, [][]string{{fileMainGo}})
}

func TestDetectChangesets_ConnectedChain_OneGroup(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"DetectChangesets - Connected Dependency Chain",
		"main.go -> service.go -> utils.go/types.go (all staged)",
		"Modified [main.go, service.go, utils.go, types.go] | Staged [all]",
		"1 changeset containing all four files")

	repoDir := setupTestRepo(t)

	modifyFile(t, filepath.Join(repoDir, fileMainGo), testComment)
	modifyFile(t, filepath.Join(repoDir, "service.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, fileUtilsGo), testComment)
	modifyFile(t, filepath.Join(repoDir, fileTypesGo), testComment)
	stageFiles(t, repoDir, fileMainGo, "service.go", fileUtilsGo, fileTypesGo)

	got, err := validator.DetectChangesets(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("DetectChangesets failed: %v", err)
	}

	assertChangesets(t, got, [][]string{{fileMainGo, "service.go", fileTypesGo, fileUtilsGo}})
}

func TestDetectChangesets_TwoIndependentGroups(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"DetectChangesets - Two Independent Groups",
		"{alpha.go, beta.go} (beta -> alpha) and {service.go, types.go} (service -> types)",
		"Modified [alpha.go, beta.go, service.go, types.go] | Staged [all]",
		"2 changesets: {alpha, beta} and {service, types}")

	repoDir := setupTestRepo(t)

	modifyFile(t, filepath.Join(repoDir, "alpha.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "beta.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "service.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, fileTypesGo), testComment)
	stageFiles(t, repoDir, "alpha.go", "beta.go", "service.go", fileTypesGo)

	got, err := validator.DetectChangesets(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("DetectChangesets failed: %v", err)
	}

	assertChangesets(t, got, [][]string{
		{"alpha.go", "beta.go"},
		{"service.go", fileTypesGo},
	})
}

func TestDetectChangesets_ThreeIsolatedFiles(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"DetectChangesets - Three Isolated Files",
		"alpha.go, main.go, consumer.go (each depends only on committed code)",
		"Modified [alpha.go, main.go, consumer.go] | Staged [all]",
		"3 changesets: one per staged file")

	repoDir := setupTestRepo(t)

	modifyFile(t, filepath.Join(repoDir, "alpha.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, fileMainGo), testComment)
	modifyFile(t, filepath.Join(repoDir, fileConsumerGo), testComment)
	stageFiles(t, repoDir, "alpha.go", fileMainGo, fileConsumerGo)

	got, err := validator.DetectChangesets(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("DetectChangesets failed: %v", err)
	}

	assertChangesets(t, got, [][]string{
		{"alpha.go"},
		{fileConsumerGo},
		{fileMainGo},
	})
}

func TestDetectChangesets_TestFileGroupedWithSource(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"DetectChangesets - Test File Grouped With Source",
		"widget_test.go (TestNewWidget) -> widget.go (NewWidget)",
		"Untracked [widget.go, widget_test.go] | Staged [both]",
		"1 changeset containing both files")

	repoDir := setupTestRepo(t)

	createUntrackedFile(t, repoDir, "widget.go", `package main

// NewWidget creates a new widget.
func NewWidget() string {
	return "widget"
}
`)

	createUntrackedFile(t, repoDir, "widget_test.go", `package main

import "testing"

func TestNewWidget(t *testing.T) {
	t.Parallel()

	if NewWidget() == "" {
		t.Error("empty widget")
	}
}
`)

	stageFiles(t, repoDir, "widget.go", "widget_test.go")

	got, err := validator.DetectChangesets(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("DetectChangesets failed: %v", err)
	}

	assertChangesets(t, got, [][]string{{"widget.go", "widget_test.go"}})
}

func TestDetectChangesets_CircularDependency_OneGroup(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"DetectChangesets - Circular Dependency",
		"circular_a.go (FuncA) <-> circular_b.go (FuncB)",
		"Modified [circular_a.go, circular_b.go] | Staged [both]",
		"1 changeset containing both files")

	repoDir := setupTestRepo(t)

	modifyFile(t, filepath.Join(repoDir, "circular_a.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "circular_b.go"), testComment)
	stageFiles(t, repoDir, "circular_a.go", "circular_b.go")

	got, err := validator.DetectChangesets(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("DetectChangesets failed: %v", err)
	}

	assertChangesets(t, got, [][]string{{"circular_a.go", "circular_b.go"}})
}

func TestDetectChangesets_CrossPackageChain_OneGroup(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"DetectChangesets - Cross-Package Chain",
		"processor.go -> models/response.go -> helper/formatter.go",
		"Modified [processor.go, models/response.go, helper/formatter.go] | Staged [all]",
		"1 changeset containing all three files")

	repoDir := setupTestRepo(t)

	modifyFile(t, filepath.Join(repoDir, fileProcessorGo), testComment)
	modifyFile(t, filepath.Join(repoDir, "models", "response.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, "helper", "formatter.go"), testComment)
	stageFiles(t, repoDir, fileProcessorGo, "models/response.go", "helper/formatter.go")

	got, err := validator.DetectChangesets(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("DetectChangesets failed: %v", err)
	}

	assertChangesets(t, got, [][]string{{fileHelperFmtGo, fileModelsResponse, fileProcessorGo}})
}

func TestDetectChangesets_MixedConnectedAndIsolated(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"DetectChangesets - Mixed Connected And Isolated",
		"{service.go, types.go} connected + alpha.go isolated",
		"Modified [service.go, types.go, alpha.go] | Staged [all]",
		"2 changesets: {service, types} and {alpha}")

	repoDir := setupTestRepo(t)

	modifyFile(t, filepath.Join(repoDir, "service.go"), testComment)
	modifyFile(t, filepath.Join(repoDir, fileTypesGo), testComment)
	modifyFile(t, filepath.Join(repoDir, "alpha.go"), testComment)
	stageFiles(t, repoDir, "service.go", fileTypesGo, "alpha.go")

	got, err := validator.DetectChangesets(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("DetectChangesets failed: %v", err)
	}

	assertChangesets(t, got, [][]string{
		{"alpha.go"},
		{"service.go", fileTypesGo},
	})
}

func TestDetectChangesets_NoStagedFiles(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"DetectChangesets - No Staged Files",
		"main.go modified but not staged",
		"Modified [main.go] | Staged [NONE]",
		"nil result")

	repoDir := setupTestRepo(t)

	modifyFile(t, filepath.Join(repoDir, fileMainGo), testComment)

	got, err := validator.DetectChangesets(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("DetectChangesets failed: %v", err)
	}

	if got != nil {
		t.Errorf("Expected nil for no staged files, got %v", got)
	}
}

// TestValidateAtomicCommit_WorktreeOnlyFileDoesNotBreakAnalysis verifies that
// files with working-tree-only changes that introduce invalid references do not
// cause compilation errors during analysis.
func TestValidateAtomicCommit_WorktreeOnlyFileDoesNotBreakAnalysis(t *testing.T) {
	t.Parallel()

	logTestPattern(t,
		"Working-Tree-Only File With Undefined Reference",
		"utils.go ( M, worktree-only adds call to undefined func) | main.go staged",
		"Staged [main.go comment] | WorkingTree [utils.go+undefined call]",
		"No compilation error - working-tree content replaced with index version")

	repoDir := setupTestRepo(t)

	// Stage main.go with a comment.
	modifyFile(t, filepath.Join(repoDir, fileMainGo), testComment)
	stageFiles(t, repoDir, fileMainGo)

	// Add an invalid call to utils.go working tree only (not staged,  M status).
	modifyFile(t, filepath.Join(repoDir, fileUtilsGo), "\nfunc init() { _ = undefinedFuncThatDoesNotExist() }\n")

	// Should not return a compilation error - working tree content is replaced with index version.
	_, err := validator.ValidateAtomicCommit(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("ValidateAtomicCommit failed (expected no error): %v", err)
	}
}
