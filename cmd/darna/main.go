// Package main provides the darna CLI for atomic commit validation.
package main

import (
	"context"
	"flag"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/darccio/darna/internal/validator"
)

func main() {
	verbose := flag.Bool("v", false, "show detailed analysis")
	workDir := flag.String("dir", ".", "working directory (default: current directory)")
	committable := flag.Bool("committable", false, "output files that can be committed atomically")
	selectFlag := flag.Bool("select", false, "alias for --committable")

	flag.Parse()

	ctx := context.Background()

	// Handle committable mode.
	if *committable || *selectFlag {
		files, err := validator.FindCommittableFiles(ctx, *workDir)
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

		for _, v := range viols {
			writeString(w, "     - "+v.StagedSymbol+" uses "+v.MissingSymbol+"\n")
		}
	}

	writeString(w, "\nTo fix, run:\n")

	for _, file := range files {
		writeString(w, "   git add "+file+"\n")
	}
}

func groupByMissingFile(violations []validator.Violation) map[string][]validator.Violation {
	byFile := make(map[string][]validator.Violation)
	for _, v := range violations {
		byFile[v.MissingFile] = append(byFile[v.MissingFile], v)
	}

	return byFile
}
