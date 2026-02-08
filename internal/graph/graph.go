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
func (g *DependencyGraph) AnalyzePackage(pkg *packages.Package) {
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
		if obj == nil || obj.Pkg() == nil || obj.Parent() != pkg.Types.Scope() {
			continue
		}

		sym := &Symbol{
			ID:      symbolID(obj),
			Name:    obj.Name(),
			Package: obj.Pkg().Path(),
			Kind:    analyzer.ObjectKind(obj),
			File:    pkg.Fset.Position(obj.Pos()).Filename,
			Pos:     pkg.Fset.Position(obj.Pos()),
		}
		g.Symbols[sym.ID] = sym
		g.FileSyms[sym.File] = append(g.FileSyms[sym.File], sym.ID)
	}
}

func (g *DependencyGraph) trackUsages(pkg *packages.Package) {
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok {
				return true
			}

			callerID := callerSymbolID(pkg, fn)
			if callerID == "" {
				return true
			}

			g.trackFuncBodyUsages(pkg, callerID, fn)

			return true
		})
	}
}

func (g *DependencyGraph) trackFuncBodyUsages(
	pkg *packages.Package, callerID string, fn *ast.FuncDecl,
) {
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

func callerSymbolID(pkg *packages.Package, fn *ast.FuncDecl) string {
	if fn.Recv == nil {
		return pkg.PkgPath + "." + fn.Name.Name
	}

	if len(fn.Recv.List) == 0 {
		return ""
	}

	recvType := fn.Recv.List[0].Type

	// Handle pointer receivers.
	if star, ok := recvType.(*ast.StarExpr); ok {
		recvType = star.X
	}

	if ident, ok := recvType.(*ast.Ident); ok {
		return pkg.PkgPath + "." + ident.Name + "." + fn.Name.Name
	}

	return ""
}

// symbolID generates a unique identifier for a types.Object.
func symbolID(obj types.Object) string {
	if obj.Pkg() == nil {
		return "" // Built-in, skip.
	}

	return obj.Pkg().Path() + "." + obj.Name()
}
