// Package main provides the darna CLI for atomic commit validation.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"dario.cat/darna/internal/agent"
	"dario.cat/darna/internal/git"
	"dario.cat/darna/internal/validator"
)

func main() {
	verbose := flag.Bool("v", false, "show detailed analysis")
	workDir := flag.String("dir", ".", "working directory (default: current directory)")
	committable := flag.Bool("committable", false, "output files that can be committed atomically")
	selectFlag := flag.Bool("select", false, "alias for --committable")
	dependants := flag.Bool("dependants", false, "include direct dependants when using --committable")
	commitMsg := flag.String("commit-msg", "", "generate commit message using agent (claude, codex, mistral, opencode)")
	promptFile := flag.String("prompt-file", "", "custom prompt file for --commit-msg")
	singleChangeset := flag.Bool("single-changeset", false, "fail if staged files contain multiple independent changesets")

	flag.Parse()

	ctx := context.Background()

	// Handle commit message generation mode.
	if *commitMsg != "" {
		msg, err := generateCommitMsg(ctx, *commitMsg, *promptFile, *workDir)
		if err != nil {
			writeString(os.Stderr, "Error: "+err.Error()+"\n")
			os.Exit(1)
		}

		writeString(os.Stdout, msg+"\n")
		os.Exit(0)
	}

	if *promptFile != "" {
		writeString(os.Stderr, "Error: --prompt-file can only be used with --commit-msg\n")
		os.Exit(1)
	}

	// Handle committable mode.
	if *committable || *selectFlag {
		files, err := validator.FindCommittableSet(ctx, *workDir, *dependants)
		if err != nil {
			writeString(os.Stderr, "Error: "+err.Error()+"\n")
			os.Exit(1)
		}

		if len(files) > 0 {
			writeString(os.Stdout, strings.Join(files, " ")+"\n")
		}

		os.Exit(0)
	}

	// Run validation.
	violations, err := validator.ValidateAtomicCommit(ctx, *workDir)
	if err != nil {
		writeString(os.Stderr, "Error: "+err.Error()+"\n")
		os.Exit(1)
	}

	if len(violations) > 0 {
		printViolations(os.Stdout, violations)
		os.Exit(1)
	}

	if *singleChangeset && !runSingleChangesetCheck(ctx, *workDir) {
		os.Exit(1)
	}

	if *verbose {
		writeString(os.Stdout, "Commit is atomic\n")
	}

	os.Exit(0)
}

// runSingleChangesetCheck runs the changeset detection pass and returns true
// when the staged files form zero or one changeset. Returns false (with a
// message printed to stderr on error, or the changeset list to stdout on
// split) so the caller can exit non-zero. A split can be overridden per
// invocation with DARNA_ALLOW_MULTI_CHANGESET, since the check is static
// analysis and cannot see runtime coupling.
func runSingleChangesetCheck(ctx context.Context, workDir string) bool {
	changesets, err := validator.DetectChangesets(ctx, workDir)
	if err != nil {
		writeString(os.Stderr, "Error: "+err.Error()+"\n")

		return false
	}

	if len(changesets) <= 1 {
		return true
	}

	printChangesets(os.Stdout, changesets)

	if allowMultiChangeset(os.Getenv) {
		writeString(os.Stderr, fmt.Sprintf(
			"darna: multi-changeset check overridden by DARNA_ALLOW_MULTI_CHANGESET (%d changesets detected)\n",
			len(changesets)))

		return true
	}

	return false
}

// allowMultiChangeset reports whether DARNA_ALLOW_MULTI_CHANGESET (read via
// getenv, injected so tests need not touch process environment) opts out of
// the -single-changeset split failure. Accepts "1", "true", or "yes"
// case-insensitively; anything else, including unset, is off.
func allowMultiChangeset(getenv func(string) string) bool {
	switch strings.ToLower(getenv("DARNA_ALLOW_MULTI_CHANGESET")) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

var errNoStagedChanges = errors.New("no staged changes (stage files with git add first)")

// generateCommitMsg produces a commit message from staged changes using an LLM agent.
func generateCommitMsg(ctx context.Context, agentType, promptPath, workDir string) (string, error) {
	ag, err := agent.NewAgent(agentType)
	if err != nil {
		return "", fmt.Errorf("creating agent: %w", err)
	}

	diff, err := git.GetStagedDiff(ctx, workDir)
	if err != nil {
		return "", fmt.Errorf("getting staged diff: %w", err)
	}

	if strings.TrimSpace(diff) == "" {
		return "", errNoStagedChanges
	}

	prompt := agent.DefaultPrompt

	if promptPath != "" {
		data, readErr := os.ReadFile(promptPath) //nolint:gosec // User-provided prompt file path is intentional.
		if readErr != nil {
			return "", fmt.Errorf("reading prompt file: %w", readErr)
		}

		prompt = string(data)
	}

	msg, genErr := ag.Generate(ctx, diff, prompt)
	if genErr != nil {
		return "", fmt.Errorf("generating commit message: %w", genErr)
	}

	return msg, nil
}

func writeString(w io.Writer, s string) {
	_, err := io.WriteString(w, s)
	if err != nil {
		panic(err)
	}
}

func printViolations(w io.Writer, violations []validator.Violation) {
	writeString(w, "Commit is not atomic. Missing files need to be staged:\n\n")

	// Group violations by missing file for cleaner output.
	byFile := groupByMissingFile(violations)

	// Sort files for consistent output.
	files := make([]string, 0, len(byFile))
	for file := range byFile {
		files = append(files, file)
	}

	sort.Strings(files)

	for _, file := range files {
		viols := byFile[file]
		writeString(w, "  "+file+"\n")

		for _, vv := range viols {
			writeString(w, "     - "+vv.StagedSymbol+" uses "+vv.MissingSymbol+"\n")
		}
	}

	writeString(w, "\nTo fix, run:\n")

	for _, file := range files {
		writeString(w, "   git add "+file+"\n")
	}
}

func groupByMissingFile(violations []validator.Violation) map[string][]validator.Violation {
	byFile := make(map[string][]validator.Violation)
	for _, vv := range violations {
		byFile[vv.MissingFile] = append(byFile[vv.MissingFile], vv)
	}

	return byFile
}

func printChangesets(w io.Writer, changesets [][]string) {
	writeString(w, fmt.Sprintf(
		"Commit is atomic but appears to contain %d independent changesets.\n\n",
		len(changesets),
	))
	writeString(w, "darna found no compile-time dependency between these groups. It cannot see\n")
	writeString(w, "runtime coupling: string keys, reflection, build tags, go:generate output,\n")
	writeString(w, "table-driven test data, or init-order effects. If these files really belong\n")
	writeString(w, "together, this is a false positive.\n\n")

	for i, group := range changesets {
		writeString(w, fmt.Sprintf("  Changeset %d:\n", i+1))

		for _, file := range group {
			writeString(w, "    "+file+"\n")
		}

		writeString(w, "\n")
	}

	writeString(w, "If they are separate concerns, split them:\n")
	writeString(w, "   git reset\n")

	for _, group := range changesets {
		writeString(w, "   git add "+strings.Join(group, " ")+"\n")
		writeString(w, "   git commit -m \"...\"\n")
	}

	writeString(w, "\nIf they belong together, keep the commit as is:\n")
	writeString(w, "   DARNA_ALLOW_MULTI_CHANGESET=1 git commit ...\n")
}
