# darna

Atomic commit validator for Go. Ensures staged files form a self-contained commit where every symbol used is defined in the staged changeset or in already-committed code - never in unstaged or untracked files.

The name comes from Lithuanian *darna*, meaning harmony - the quality of a codebase where every commit is complete and coherent.

## Why

Non-atomic commits break `git bisect`, block cherry-picks, and make code review painful. A commit that references a function defined only in a later commit (or worse, never committed at all) cannot compile when checked out in isolation.

This problem intensifies in [automatic programming](https://antirez.com/news/159) workflows where developers and AI coding agents collaborate on large features. Automatic programming requires maintaining vision and control throughout development - but that control depends on being able to commit working subsets of code at any point. When building complex features incrementally, both human developers and autonomous agents need to safely checkpoint progress without accidentally creating broken commits.

Darna was inspired by Vladislav Shpilevoy's FOSDEM 2026 talk ["An Efficient Git Workflow For High-Stakes Projects"](https://fosdem.org/2026/schedule/event/3VNNBK-efficient-git-for-high-stakes/), which presents "Atomic Flow" - a methodology for maintaining clean commit histories through multi-commit patchsets in mission-critical systems.

Common causes of non-atomic commits:

- Forgetting to stage a newly created file.
- Staging part of a refactor while the rest sits in the working tree.
- Partial staging (`git add -p`) that splits a dependency across staged and unstaged hunks.
- AI agents generating code across multiple files without tracking dependencies.

Darna catches these at commit time. It loads Go packages with full type information, builds a symbol dependency graph, and reports exactly which unstaged files are missing and why.

The `--committable` mode is designed specifically for automatic programming workflows - it identifies which files can be committed independently, enabling both developers and autonomous agents to build multi-commit patchsets incrementally without human intervention.

## Install

```bash
go install dario.cat/darna/cmd/darna@latest
```

Or build from source:

```bash
make build    # produces bin/darna
```

Requires Go 1.24+ and `git` on `PATH`.

## Usage

### Validate staged commit

```bash
darna
```

Returns exit code 0 if the commit is atomic, 1 if violations are found.

### Flags

| Flag | Description |
|---|---|
| `-v` | Verbose - prints confirmation on success |
| `-dir <path>` | Set working directory (default: `.`) |
| `--committable` | Find the next file that can be committed atomically |
| `--select` | Alias for `--committable` |
| `--dependants` | Include direct dependants when using `--committable` |

### Progressive commit workflow

#### Single file mode

`--committable` returns the first independent file (sorted lexicographically by path) that has no dependencies on other uncommitted changes.

```bash
while [ -n "$(darna --committable)" ]; do
    git add $(darna --committable)
    git commit -m "feat: add $(darna --committable)"
done
```

#### Committable set mode

`--committable --dependants` returns the first independent file **plus** direct dependants that only depend on that file and committed code.

```bash
# Commit in larger atomic sets when possible.
while [ -n "$(darna --committable --dependants)" ]; do
    FILES=$(darna --committable --dependants)
    git add $FILES
    git commit -m "feat: add $FILES"
done
```

This mode enables building multi-commit patchsets more efficiently by grouping related changes together while maintaining atomicity.

### Selection algorithm

Files are sorted **lexicographically** by path. The first file that is independent (has no dependencies on other unstaged files) is selected as the base file. When `--dependants` is used, direct dependants are added to the set - files that:

1. Depend on the base file
2. Are in the changeset (unstaged or untracked)
3. Have NO dependencies on OTHER changeset files (only the base file + committed code)

Transitive dependants (dependants of dependants) are excluded to maintain atomicity.

### Git pre-commit hook

```bash
#!/bin/sh
darna
```

Darna exits non-zero on violations, blocking the commit.

## How it works

1. **Git status** - categorize files as staged, unstaged, or untracked using `git status --porcelain -z`.
2. **Package loading** - load all Go packages with full type information via `golang.org/x/tools/go/packages`. For partially staged files (status `MM`), an overlay with `git show :path` content ensures analysis reflects what will actually be committed.
3. **Dependency graph** - walk `types.Info.Defs` and `types.Info.Uses` to build a bidirectional symbol dependency graph with transitive reachability.
4. **Violation detection** - for each symbol in a staged file, check whether its dependencies are all satisfied by staged or committed files. Report any that require unstaged files.

## Project structure

```
cmd/darna/           CLI entry point
internal/analyzer/   Go package loading and symbol extraction
internal/git/        Git command wrappers (staged files, content, status)
internal/graph/      Symbol dependency graph construction and traversal
internal/validator/   Validation orchestration and committable file selection
docs/decisions/      Architecture decision records
```

## Development

```bash
make all      # lint + test + build
make lint     # golangci-lint
make test     # tests with race detector and coverage
make clean    # remove artifacts
```

## License

See [LICENSE](LICENSE).
