package validator_test

import (
	"testing"

	"dario.cat/darna/internal/validator"
)

func TestViolation(t *testing.T) {
	t.Parallel()

	v := validator.Violation{
		StagedFile:    "foo.go",
		StagedSymbol:  "pkg.Foo",
		MissingFile:   "bar.go",
		MissingSymbol: "pkg.Bar",
	}

	if v.StagedFile != "foo.go" {
		t.Errorf("Expected staged file 'foo.go', got %s", v.StagedFile)
	}

	if v.StagedSymbol != "pkg.Foo" {
		t.Errorf("Expected staged symbol 'pkg.Foo', got %s", v.StagedSymbol)
	}

	if v.MissingFile != "bar.go" {
		t.Errorf("Expected missing file 'bar.go', got %s", v.MissingFile)
	}

	if v.MissingSymbol != "pkg.Bar" {
		t.Errorf("Expected missing symbol 'pkg.Bar', got %s", v.MissingSymbol)
	}
}
