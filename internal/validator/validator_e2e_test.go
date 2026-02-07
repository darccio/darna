package validator_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darccio/darna/internal/validator"
)

const (
	testComment        = "\n// Comment\n"
	fileMainGo         = "main.go"
	fileUtilsGo        = "utils.go"
	fileConsumerGo     = "consumer.go"
	fileProcessorGo    = "processor.go"
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

	cmd := exec.CommandContext(t.Context(), "git", args...)
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

	err = os.WriteFile(dst, data, 0o600)
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
		if v.StagedFile == "service.go" && v.MissingFile == "types.go" {
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

		if v.StagedFile == "service.go" && v.MissingFile == "types.go" {
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
		"Violation tracking method: UseCalculator -> Calculator.Add or Calculator type")

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

	// Verify method or type symbol is tracked (could be Calculator type or Add method).
	found := false

	for _, v := range violations {
		if v.StagedFile == "calculator_user.go" && v.MissingFile == "calculator.go" {
			// Accept either Calculator type or Calculator.Add method.
			if v.MissingSymbol == "example.com/testproject.Calculator" ||
				v.MissingSymbol == "example.com/testproject.Calculator.Add" {
				found = true

				break
			}
		}
	}

	if !found {
		t.Errorf("Expected violation tracking Calculator type or Add method, violations: %+v", violations)
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

	err := os.WriteFile(path, []byte(content), 0o600)
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

	files, err := validator.FindCommittableFiles(t.Context(), repoDir)
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

	files, err := validator.FindCommittableFiles(t.Context(), repoDir)
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

	files, err = validator.FindCommittableFiles(t.Context(), repoDir)
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

	files, err := validator.FindCommittableFiles(t.Context(), repoDir)
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

	files, err := validator.FindCommittableFiles(t.Context(), repoDir)
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

	files, err := validator.FindCommittableFiles(t.Context(), repoDir)
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

	files, err := validator.FindCommittableFiles(t.Context(), repoDir)
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

	files, err := validator.FindCommittableFiles(t.Context(), repoDir)
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
	files, err := validator.FindCommittableFiles(t.Context(), repoDir)
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
	files, err = validator.FindCommittableFiles(t.Context(), repoDir)
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
	files, err = validator.FindCommittableFiles(t.Context(), repoDir)
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
	files, err = validator.FindCommittableFiles(t.Context(), repoDir)
	if err != nil {
		t.Fatalf("FindCommittableFiles failed (4th call): %v", err)
	}

	if len(files) != 0 {
		t.Errorf("Expected no files after all committed, got %v", files)
	}
}
