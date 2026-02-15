// Package analyzer provides utilities for loading and analyzing Go packages.
package analyzer

import (
	"errors"
	"fmt"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// ErrPackagesContainErrors is returned when loaded packages have errors.
var ErrPackagesContainErrors = errors.New("packages contain errors")

// Symbol represents a symbol (function, type, variable, constant) in Go code.
type Symbol struct {
	ID      string         // "pkg/path.SymbolName".
	Name    string         // Symbol name.
	Package string         // Package path.
	Kind    string         // "func", "type", "var", "const".
	File    string         // Defining file path.
	Pos     token.Position // Source position.
}

// LoadPackages loads Go packages with full type information.
func LoadPackages(dir string, overlay map[string][]byte, patterns ...string) ([]*packages.Package, error) {
	cfg := &packages.Config{ //nolint:exhaustruct // Optional fields intentionally omitted.
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedImports |
			packages.NeedDeps,
		Dir:     dir,
		Overlay: overlay,
	}

	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("loading packages: %w", err)
	}

	if packages.PrintErrors(pkgs) > 0 {
		return nil, ErrPackagesContainErrors
	}

	return pkgs, nil
}

// CollectSymbols extracts symbol definitions and usages from a package.
// Returns two maps: defined symbols and used symbols from other packages.
//
//nolint:nonamedreturns // Named returns clarify same-type return values.
func CollectSymbols(pkg *packages.Package) (defined, used map[string]types.Object) {
	defined = make(map[string]types.Object)
	used = make(map[string]types.Object)

	// Symbols DEFINED in this package.
	for _, obj := range pkg.TypesInfo.Defs {
		if obj == nil || obj.Parent() != pkg.Types.Scope() {
			continue // Skip nil and non-package-level definitions.
		}

		id := symbolID(obj)
		if id != "" {
			defined[id] = obj
		}
	}

	// Symbols USED from other packages.
	for _, obj := range pkg.TypesInfo.Uses {
		if obj == nil || obj.Pkg() == nil {
			continue
		}

		if obj.Pkg() != pkg.Types { // External reference.
			id := symbolID(obj)
			if id != "" {
				used[id] = obj
			}
		}
	}

	return defined, used
}

// symbolID generates a unique identifier for a types.Object.
func symbolID(obj types.Object) string {
	if obj.Pkg() == nil {
		return "" // Built-in, skip.
	}

	return obj.Pkg().Path() + "." + obj.Name()
}

// ObjectKind returns a string representation of the object kind.
func ObjectKind(obj types.Object) string {
	switch obj.(type) {
	case *types.Func:
		return "func"
	case *types.TypeName:
		return "type"
	case *types.Var:
		return "var"
	case *types.Const:
		return "const"
	default:
		return "unknown"
	}
}
