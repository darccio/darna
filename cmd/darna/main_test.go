package main

import (
	"bytes"
	"strings"
	"testing"

	"dario.cat/darna/internal/validator"
)

func TestPrintChangesets_MentionsStaticAnalysisLimits(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	printChangesets(&buf, [][]string{{"logic.go", "logic_test.go"}, {"emit.go"}})

	out := buf.String()

	for _, want := range []string{
		"appears to contain",
		"cannot see",
		"false positive",
		"git reset",
		"git add logic.go logic_test.go",
		"git add emit.go",
		"DARNA_ALLOW_MULTI_CHANGESET=1 git commit",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestPrintChangesets_ListsEveryFile(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	changesets := [][]string{{"alpha.go", "beta.go"}, {"service.go", "types.go"}}
	printChangesets(&buf, changesets)

	out := buf.String()

	for _, group := range changesets {
		for _, file := range group {
			if !strings.Contains(out, file) {
				t.Errorf("Expected output to mention %s, got:\n%s", file, out)
			}
		}
	}

	if !strings.Contains(out, "Changeset 1:") || !strings.Contains(out, "Changeset 2:") {
		t.Errorf("Expected both changesets to be numbered, got:\n%s", out)
	}
}

func TestAllowMultiChangeset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"unset", "", false},
		{"zero", "0", false},
		{"one", "1", true},
		{"true", "true", true},
		{"upperTrue", "TRUE", true},
		{"yes", "yes", true},
		{"garbage", "maybe", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			getenv := func(key string) string {
				if key != "DARNA_ALLOW_MULTI_CHANGESET" {
					t.Fatalf("Unexpected env lookup: %s", key)
				}

				return tt.value
			}

			if got := allowMultiChangeset(getenv); got != tt.want {
				t.Errorf("allowMultiChangeset(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestPrintViolations_GroupsByMissingFile(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	violations := []validator.Violation{
		{StagedFile: "a.go", StagedSymbol: "pkg.A", MissingFile: "b.go", MissingSymbol: "pkg.B"},
		{StagedFile: "a.go", StagedSymbol: "pkg.A2", MissingFile: "b.go", MissingSymbol: "pkg.B2"},
		{StagedFile: "c.go", StagedSymbol: "pkg.C", MissingFile: "d.go", MissingSymbol: "pkg.D"},
	}

	printViolations(&buf, violations)

	out := buf.String()

	for _, want := range []string{
		"Commit is not atomic",
		"b.go", "d.go",
		"pkg.A uses pkg.B",
		"pkg.A2 uses pkg.B2",
		"pkg.C uses pkg.D",
		"git add b.go",
		"git add d.go",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Expected output to contain %q, got:\n%s", want, out)
		}
	}
}
