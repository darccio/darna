package analyzer_test

import (
	"go/types"
	"os"
	"path/filepath"
	"testing"

	"dario.cat/darna/internal/analyzer"
)

func TestObjectKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		obj  types.Object
		want string
	}{
		{
			name: "function",
			obj:  types.NewFunc(0, nil, "foo", nil),
			want: "func",
		},
		{
			name: "type",
			obj:  types.NewTypeName(0, nil, "Foo", nil),
			want: "type",
		},
		{
			name: "variable",
			obj:  types.NewVar(0, nil, "x", nil),
			want: "var",
		},
		{
			name: "constant",
			obj:  types.NewConst(0, nil, "Pi", nil, nil),
			want: "const",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := analyzer.ObjectKind(tt.obj)
			if got != tt.want {
				t.Errorf("ObjectKind() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadPackages(t *testing.T) {
	t.Parallel()

	// Create a temporary directory with a simple Go package.
	tmpDir := t.TempDir()

	// Create a simple Go file.
	testFile := filepath.Join(tmpDir, "test.go")

	content := `package testpkg

func Foo() string {
	return "bar"
}

const Pi = 3.14
`

	err := os.WriteFile(testFile, []byte(content), 0o600)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create go.mod.
	modFile := filepath.Join(tmpDir, "go.mod")

	modContent := "module testpkg\n\ngo 1.23\n"

	err = os.WriteFile(modFile, []byte(modContent), 0o600)
	if err != nil {
		t.Fatalf("Failed to write go.mod: %v", err)
	}

	// Load the package.
	pkgs, err := analyzer.LoadPackages(tmpDir, nil, ".")
	if err != nil {
		t.Fatalf("LoadPackages() error = %v", err)
	}

	if len(pkgs) != 1 {
		t.Fatalf("Expected 1 package, got %d", len(pkgs))
	}

	pkg := pkgs[0]
	if pkg.Name != "testpkg" {
		t.Errorf("Expected package name 'testpkg', got %s", pkg.Name)
	}

	// Test CollectSymbols.
	defined, used := analyzer.CollectSymbols(pkg)

	if len(defined) == 0 {
		t.Error("Expected at least one defined symbol")
	}

	// Check for Foo function.
	fooID := "testpkg.Foo"
	if _, ok := defined[fooID]; !ok {
		t.Errorf("Expected to find Foo function in defined symbols")
	}

	// Check for Pi constant.
	piID := "testpkg.Pi"
	if _, ok := defined[piID]; !ok {
		t.Errorf("Expected to find Pi constant in defined symbols")
	}

	// For this simple package, there should be no external usages.
	if len(used) > 0 {
		t.Logf("Found %d external usages (this is fine for stdlib deps)", len(used))
	}
}
