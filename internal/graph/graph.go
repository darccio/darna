// Package graph provides dependency graph analysis for Go symbols.
package graph

import (
	"go/ast"
	"go/token"
	"go/types"

	"dario.cat/darna/internal/analyzer"
	"golang.org/x/tools/go/packages"
)

// Symbol represents a symbol in the dependency graph.
type Symbol struct {
	ID      string         // "pkg/path.SymbolName".
	Name    string         // Symbol name.
	Package string         // Package path.
	Kind    string         // "func", "type", "var", "const".
	File    string         // Defining file path.
	Pos     token.Position // Source position.
}

// DependencyGraph represents the dependency relationships between symbols.
type DependencyGraph struct {
	Symbols  map[string]*Symbol             // ID -> Symbol.
	FileSyms map[string][]string            // File -> defined symbol IDs.
	OutEdges map[string]map[string]struct{} // Symbol -> symbols it depends on.
	InEdges  map[string]map[string]struct{} // Symbol -> symbols that depend on it.
}

// NewDependencyGraph creates a new empty dependency graph.
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		Symbols:  make(map[string]*Symbol),
		FileSyms: make(map[string][]string),
		OutEdges: make(map[string]map[string]struct{}),
		InEdges:  make(map[string]map[string]struct{}),
	}
}

// AddDependency adds a dependency edge from one symbol to another.
func (g *DependencyGraph) AddDependency(from, to string) {
	if g.OutEdges[from] == nil {
		g.OutEdges[from] = make(map[string]struct{})
	}

	if g.InEdges[to] == nil {
		g.InEdges[to] = make(map[string]struct{})
	}

	g.OutEdges[from][to] = struct{}{}
	g.InEdges[to][from] = struct{}{}
}

// AnalyzePackage analyzes a package and adds its symbols and dependencies to the graph.
// Packages with nil TypesInfo (e.g. those with parse/type errors) are skipped.
func (g *DependencyGraph) AnalyzePackage(pkg *packages.Package) {
	if pkg.TypesInfo == nil {
		return
	}

	g.registerDefinitions(pkg)
	g.trackUsages(pkg)
}

// TransitiveDeps returns all symbols that the given symbol transitively depends on.
func (g *DependencyGraph) TransitiveDeps(startID string) []string {
	visited := make(map[string]bool)

	var result []string

	var dfs func(id string)

	dfs = func(id string) {
		if visited[id] {
			return
		}

		visited[id] = true

		result = append(result, id)
		for depID := range g.OutEdges[id] {
			dfs(depID)
		}
	}
	dfs(startID)

	return result
}

// TransitiveDependents returns all symbols that transitively depend on the given symbol.
func (g *DependencyGraph) TransitiveDependents(targetID string) []string {
	visited := make(map[string]bool)

	var result []string

	var dfs func(id string)

	dfs = func(id string) {
		if visited[id] {
			return
		}

		visited[id] = true

		result = append(result, id)
		for depID := range g.InEdges[id] { // Follow reverse edges.
			dfs(depID)
		}
	}
	dfs(targetID)

	return result
}

func (g *DependencyGraph) registerDefinitions(pkg *packages.Package) {
	for _, obj := range pkg.TypesInfo.Defs {
		if !shouldRegister(obj) {
			continue
		}

		sym := &Symbol{
			ID:      symbolID(obj),
			Name:    obj.Name(),
			Package: obj.Pkg().Path(),
			Kind:    objectKind(obj),
			File:    pkg.Fset.Position(obj.Pos()).Filename,
			Pos:     pkg.Fset.Position(obj.Pos()),
		}
		if sym.ID == "" {
			continue
		}

		if _, exists := g.Symbols[sym.ID]; !exists {
			g.FileSyms[sym.File] = append(g.FileSyms[sym.File], sym.ID)
		}

		g.Symbols[sym.ID] = sym
	}
}

// shouldRegister reports whether obj is a symbol darna tracks: a method
// (regardless of receiver scope, since Object.Parent is nil for methods) or
// a package-level declaration. Locals, parameters, and struct fields are
// excluded.
func shouldRegister(obj types.Object) bool {
	if obj == nil || obj.Pkg() == nil {
		return false
	}

	if _, isMethod := methodOf(obj); isMethod {
		return true
	}

	return isPackageLevel(obj)
}

// objectKind returns the graph's kind label for obj, treating methods as a
// distinct kind from package-level functions.
func objectKind(obj types.Object) string {
	if _, isMethod := methodOf(obj); isMethod {
		return "method"
	}

	return analyzer.ObjectKind(obj)
}

func (g *DependencyGraph) trackUsages(pkg *packages.Package) {
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			switch decl := n.(type) {
			case *ast.FuncDecl:
				callerID := callerSymbolID(pkg, decl)
				if callerID != "" {
					g.trackFuncBodyUsages(pkg, callerID, decl)
				}
			case *ast.TypeSpec:
				g.trackTypeSpecUsages(pkg, decl)
			case *ast.ValueSpec:
				g.trackValueSpecUsages(pkg, decl)
			}

			return true
		})
	}
}

func (g *DependencyGraph) trackTypeSpecUsages(pkg *packages.Package, ts *ast.TypeSpec) {
	obj := pkg.TypesInfo.Defs[ts.Name]
	if obj == nil {
		return
	}

	callerID := symbolID(obj)
	if callerID == "" {
		return
	}

	ast.Inspect(ts.Type, func(inner ast.Node) bool {
		g.recordUsage(pkg, callerID, inner)

		return true
	})
}

func (g *DependencyGraph) trackValueSpecUsages(pkg *packages.Package, vs *ast.ValueSpec) {
	callerIDs := collectValueSpecCallerIDs(pkg, vs)
	if len(callerIDs) == 0 {
		return
	}

	inspect := func(node ast.Node) {
		ast.Inspect(node, func(inner ast.Node) bool {
			for _, callerID := range callerIDs {
				g.recordUsage(pkg, callerID, inner)
			}

			return true
		})
	}

	if vs.Type != nil {
		inspect(vs.Type)
	}

	for _, val := range vs.Values {
		inspect(val)
	}
}

func collectValueSpecCallerIDs(pkg *packages.Package, vs *ast.ValueSpec) []string {
	var ids []string

	for _, name := range vs.Names {
		obj := pkg.TypesInfo.Defs[name]
		if obj == nil || !isPackageLevel(obj) {
			continue
		}

		callerID := symbolID(obj)
		if callerID != "" {
			ids = append(ids, callerID)
		}
	}

	return ids
}

func (g *DependencyGraph) trackFuncBodyUsages(
	pkg *packages.Package, callerID string, fn *ast.FuncDecl,
) {
	// Track usages in function signature (parameter and return types).
	ast.Inspect(fn.Type, func(inner ast.Node) bool {
		g.recordUsage(pkg, callerID, inner)

		return true
	})

	if fn.Body == nil {
		return
	}

	ast.Inspect(fn.Body, func(inner ast.Node) bool {
		g.recordUsage(pkg, callerID, inner)

		return true
	})
}

func (g *DependencyGraph) recordUsage(
	pkg *packages.Package, callerID string, inner ast.Node,
) {
	switch node := inner.(type) {
	case *ast.Ident:
		if obj := pkg.TypesInfo.Uses[node]; obj != nil {
			if calleeID := symbolID(obj); calleeID != "" {
				g.AddDependency(callerID, calleeID)
			}
		}
	case *ast.SelectorExpr:
		if obj := pkg.TypesInfo.Uses[node.Sel]; obj != nil {
			if calleeID := symbolID(obj); calleeID != "" {
				g.AddDependency(callerID, calleeID)
			}
		}
	}
}

// callerSymbolID resolves the graph ID of the symbol a FuncDecl defines.
// Delegating to symbolID (rather than parsing fn.Recv's AST) keeps the
// caller and callee sides of every method call in agreement, and picks up
// generic receivers for free.
func callerSymbolID(pkg *packages.Package, fn *ast.FuncDecl) string {
	obj := pkg.TypesInfo.Defs[fn.Name]
	if obj == nil {
		return ""
	}

	return symbolID(obj)
}

// symbolID generates a unique identifier for a types.Object. Methods are
// qualified by their receiver type ("pkg.Type.Method") so they never
// collide with a package-level function of the same name. Anything that is
// neither a method nor a package-level declaration (locals, parameters,
// struct fields, type parameters, imported package names) returns "".
func symbolID(obj types.Object) string {
	if obj == nil || obj.Pkg() == nil {
		return "" // Nil or built-in, skip.
	}

	if fn, isMethod := methodOf(obj); isMethod {
		return methodSymbolID(fn)
	}

	if !isPackageLevel(obj) {
		return ""
	}

	return obj.Pkg().Path() + "." + obj.Name()
}

// methodOf reports whether obj is a method (a *types.Func with a non-nil
// receiver) and returns it.
func methodOf(obj types.Object) (*types.Func, bool) {
	fn, ok := obj.(*types.Func)
	if !ok || fn.Signature().Recv() == nil {
		return nil, false
	}

	return fn, true
}

// methodSymbolID builds the receiver-qualified ID for a method, e.g.
// "pkg/path.Type.Method". Returns "" when the receiver type is unnamed
// (anonymous interface methods have no declaration file to attribute to).
func methodSymbolID(fn *types.Func) string {
	recvName := recvTypeName(fn.Signature().Recv().Type())
	if recvName == "" {
		return ""
	}

	return fn.Pkg().Path() + "." + recvName + "." + fn.Name()
}

// recvTypeName extracts the declared type name from a method receiver type,
// unwrapping pointers and resolving generic instantiations back to their
// origin so a call on Stack[int] and the declaration of Stack[T] agree on
// the same ID.
func recvTypeName(recv types.Type) string {
	recv = types.Unalias(recv)

	if ptr, ok := recv.(*types.Pointer); ok {
		recv = types.Unalias(ptr.Elem())
	}

	named, ok := recv.(*types.Named)
	if !ok {
		return ""
	}

	return named.Origin().Obj().Name()
}

// isPackageLevel reports whether obj is declared directly in its package
// scope, excluding locals, parameters, type parameters, and struct fields.
func isPackageLevel(obj types.Object) bool {
	return obj.Parent() == obj.Pkg().Scope()
}
