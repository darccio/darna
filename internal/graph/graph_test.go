package graph_test

import (
	"os"
	"path/filepath"
	"testing"

	"dario.cat/darna/internal/analyzer"
	"dario.cat/darna/internal/graph"
)

func TestNewDependencyGraph(t *testing.T) {
	t.Parallel()

	g := graph.NewDependencyGraph()
	if g.Symbols == nil {
		t.Error("Symbols map should not be nil")
	}

	if g.FileSyms == nil {
		t.Error("FileSyms map should not be nil")
	}

	if g.OutEdges == nil {
		t.Error("OutEdges map should not be nil")
	}

	if g.InEdges == nil {
		t.Error("InEdges map should not be nil")
	}
}

func TestAddDependency(t *testing.T) {
	t.Parallel()

	g := graph.NewDependencyGraph()
	from := "pkg.Foo"
	to := "pkg.Bar"

	g.AddDependency(from, to)

	if _, ok := g.OutEdges[from][to]; !ok {
		t.Errorf("Expected outgoing edge from %s to %s", from, to)
	}

	if _, ok := g.InEdges[to][from]; !ok {
		t.Errorf("Expected incoming edge to %s from %s", to, from)
	}
}

func TestTransitiveDeps(t *testing.T) {
	t.Parallel()

	g := graph.NewDependencyGraph()

	// Create a dependency chain: A -> B -> C.
	g.AddDependency("pkg.A", "pkg.B")
	g.AddDependency("pkg.B", "pkg.C")

	deps := g.TransitiveDeps("pkg.A")

	// Should include A, B, and C.
	expectedDeps := 3
	if len(deps) != expectedDeps {
		t.Errorf("Expected %d dependencies, got %d", expectedDeps, len(deps))
	}

	// Verify all are present.
	found := make(map[string]bool)
	for _, dep := range deps {
		found[dep] = true
	}

	if !found["pkg.A"] || !found["pkg.B"] || !found["pkg.C"] {
		t.Errorf("Missing expected dependencies. Got: %v", deps)
	}
}

func TestTransitiveDependents(t *testing.T) {
	t.Parallel()

	g := graph.NewDependencyGraph()

	// Create a dependency chain: A -> B -> C.
	g.AddDependency("pkg.A", "pkg.B")
	g.AddDependency("pkg.B", "pkg.C")

	dependents := g.TransitiveDependents("pkg.C")

	// Should include C, B, and A.
	expectedDeps := 3
	if len(dependents) != expectedDeps {
		t.Errorf("Expected %d dependents, got %d", expectedDeps, len(dependents))
	}

	// Verify all are present.
	found := make(map[string]bool)
	for _, dep := range dependents {
		found[dep] = true
	}

	if !found["pkg.A"] || !found["pkg.B"] || !found["pkg.C"] {
		t.Errorf("Missing expected dependents. Got: %v", dependents)
	}
}

func TestAnalyzePackage(t *testing.T) {
	t.Parallel()

	// Create a temporary directory with test packages.
	tmpDir := t.TempDir()

	// Create a simple Go file with dependencies.
	testFile := filepath.Join(tmpDir, "test.go")

	content := `package testpkg

func Foo() {
	Bar()
}

func Bar() {
	// Does nothing
}

const Pi = 3.14
`

	err := os.WriteFile(testFile, []byte(content), 0o600)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create go.mod.
	modFile := filepath.Join(tmpDir, "go.mod")

	modContent := "module testpkg\n\ngo 1.24\n"

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

	// Build the dependency graph.
	g := graph.NewDependencyGraph()
	g.AnalyzePackage(pkgs[0])

	// Check that symbols were registered.
	fooID := "testpkg.Foo"
	barID := "testpkg.Bar"
	piID := "testpkg.Pi"

	if _, ok := g.Symbols[fooID]; !ok {
		t.Errorf("Expected to find Foo in symbols")
	}

	if _, ok := g.Symbols[barID]; !ok {
		t.Errorf("Expected to find Bar in symbols")
	}

	if _, ok := g.Symbols[piID]; !ok {
		t.Errorf("Expected to find Pi in symbols")
	}

	// Check that Foo -> Bar dependency exists.
	if _, ok := g.OutEdges[fooID][barID]; !ok {
		t.Errorf("Expected Foo to depend on Bar")
	}
}
