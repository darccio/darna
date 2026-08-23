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

// loadGraph writes files (name -> source) plus a go.mod into a temp module
// named "testpkg", loads it, and returns the resulting dependency graph.
func loadGraph(t *testing.T, files map[string]string) *graph.DependencyGraph {
	t.Helper()

	tmpDir := t.TempDir()

	for name, content := range files {
		path := filepath.Join(tmpDir, name)

		err := os.WriteFile(path, []byte(content), 0o600)
		if err != nil {
			t.Fatalf("Failed to write %s: %v", name, err)
		}
	}

	modPath := filepath.Join(tmpDir, "go.mod")

	err := os.WriteFile(modPath, []byte("module testpkg\n\ngo 1.24\n"), 0o600)
	if err != nil {
		t.Fatalf("Failed to write go.mod: %v", err)
	}

	pkgs, err := analyzer.LoadPackages(tmpDir, nil, "./...")
	if err != nil {
		t.Fatalf("LoadPackages() error = %v", err)
	}

	g := graph.NewDependencyGraph()
	for _, pkg := range pkgs {
		g.AnalyzePackage(pkg)
	}

	return g
}

func TestAnalyzePackage_MethodRegisteredWithQualifiedID(t *testing.T) {
	t.Parallel()

	g := loadGraph(t, map[string]string{
		"rec.go": `package testpkg

type Rec struct {
	Baz int
}
`,
		"emit.go": `package testpkg

func (r *Rec) Emit() int {
	return r.Baz
}
`,
	})

	const emitID = "testpkg.Rec.Emit"

	sym, ok := g.Symbols[emitID]
	if !ok {
		t.Fatalf("Expected %s to be registered, symbols: %v", emitID, g.Symbols)
	}

	if sym.Kind != "method" {
		t.Errorf("Expected Kind = %q, got %q", "method", sym.Kind)
	}

	if filepath.Base(sym.File) != "emit.go" {
		t.Errorf("Expected File to end in emit.go, got %q", sym.File)
	}

	found := false

	for _, id := range g.FileSyms[sym.File] {
		if id == emitID {
			found = true
		}
	}

	if !found {
		t.Errorf("Expected FileSyms[%s] to contain %s, got %v", sym.File, emitID, g.FileSyms[sym.File])
	}
}

func TestAnalyzePackage_MethodDependsOnReceiverType(t *testing.T) {
	t.Parallel()

	g := loadGraph(t, map[string]string{
		"rec.go": `package testpkg

type Rec struct {
	Baz int
}
`,
		"emit.go": `package testpkg

func (r *Rec) Emit() int {
	return 0
}
`,
	})

	const (
		emitID = "testpkg.Rec.Emit"
		recID  = "testpkg.Rec"
	)

	if _, ok := g.OutEdges[emitID][recID]; !ok {
		t.Errorf("Expected %s to depend on its receiver type %s, deps: %v", emitID, recID, g.OutEdges[emitID])
	}
}

func TestAnalyzePackage_MethodNameCollisionNoFalseEdge(t *testing.T) {
	t.Parallel()

	g := loadGraph(t, map[string]string{
		"rec.go": `package testpkg

type Rec struct{}

func (r *Rec) Emit() {}
`,
		"other.go": `package testpkg

func Emit() {}
`,
		"use.go": `package testpkg

func Use(r *Rec) {
	r.Emit()
}
`,
	})

	const (
		methodID = "testpkg.Rec.Emit"
		funcID   = "testpkg.Emit"
	)

	if _, ok := g.OutEdges["testpkg.Use"][methodID]; !ok {
		t.Errorf("Expected Use to depend on %s, deps: %v", methodID, g.OutEdges["testpkg.Use"])
	}

	if _, ok := g.OutEdges["testpkg.Use"][funcID]; ok {
		t.Errorf("Use must not depend on the unrelated package-level %s", funcID)
	}
}

func TestAnalyzePackage_GenericMethodBodyTracked(t *testing.T) {
	t.Parallel()

	g := loadGraph(t, map[string]string{
		"stack.go": `package testpkg

type Stack[T any] struct {
	items []T
}

func (s *Stack[T]) Push(v T) {
	helper()
	s.items = append(s.items, v)
}
`,
		"helper.go": `package testpkg

func helper() {}
`,
	})

	const pushID = "testpkg.Stack.Push"

	if _, ok := g.Symbols[pushID]; !ok {
		t.Fatalf("Expected %s to be registered, symbols: %v", pushID, g.Symbols)
	}

	if _, ok := g.OutEdges[pushID]["testpkg.helper"]; !ok {
		t.Errorf("Expected %s to depend on testpkg.helper, deps: %v", pushID, g.OutEdges[pushID])
	}
}

func TestAnalyzePackage_GenericInstantiationMatchesOrigin(t *testing.T) {
	t.Parallel()

	g := loadGraph(t, map[string]string{
		"stack.go": `package testpkg

type Stack[T any] struct {
	items []T
}

func (s *Stack[T]) Push(v T) {
	s.items = append(s.items, v)
}
`,
		"use.go": `package testpkg

func Use() {
	var s Stack[int]
	s.Push(1)
}
`,
	})

	const pushID = "testpkg.Stack.Push"

	if _, ok := g.OutEdges["testpkg.Use"][pushID]; !ok {
		t.Errorf("Expected Use to depend on %s (not an instantiated form), deps: %v",
			pushID, g.OutEdges["testpkg.Use"])
	}
}

func TestAnalyzePackage_InterfaceMethodRegistered(t *testing.T) {
	t.Parallel()

	g := loadGraph(t, map[string]string{
		"writer.go": `package testpkg

type W interface {
	Write()
}
`,
		"use.go": `package testpkg

func Use(w W) {
	w.Write()
}
`,
	})

	const writeID = "testpkg.W.Write"

	if _, ok := g.Symbols[writeID]; !ok {
		t.Fatalf("Expected %s to be registered, symbols: %v", writeID, g.Symbols)
	}

	if _, ok := g.OutEdges["testpkg.Use"][writeID]; !ok {
		t.Errorf("Expected Use to depend on %s, deps: %v", writeID, g.OutEdges["testpkg.Use"])
	}
}
