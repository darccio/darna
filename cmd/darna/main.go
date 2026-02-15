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

	if *verbose {
		writeString(os.Stdout, "Commit is atomic\n")
	}

	os.Exit(0)
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
