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
| `--commit-msg <agent>` | Generate commit message using LLM agent (claude, codex, mistral, opencode) |
| `--prompt-file <path>` | Custom prompt file for `--commit-msg` (default: built-in Conventional Commits prompt) |

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

### Commit message generation

The `--commit-msg` flag generates Conventional Commits format messages from staged changes using local LLM agents.

#### Basic usage

```bash
# Generate message for currently staged files
darna --commit-msg=claude

# Use in commit workflow
git add $(darna --committable)
git commit -m "$(darna --commit-msg=claude)"
```

#### Supported agents

- `claude` - Claude Code CLI (`claude -p`)
- `codex` - OpenAI Codex CLI (`codex exec`)
- `mistral` - Mistral CLI (`mistral -p`)
- `opencode` - OpenCode CLI (`opencode run`)

Agents must be installed separately and available in PATH.

#### Custom prompts

Override the default Conventional Commits prompt with `--prompt-file`:

```bash
# Create custom prompt
cat > .darna-commit-prompt.txt <<'EOF'
Generate a concise commit message for this diff.
Use imperative mood, lowercase, no period.
Max 50 characters.
EOF

# Use custom prompt
darna --commit-msg=claude --prompt-file .darna-commit-prompt.txt
```

#### Default format

The built-in prompt generates messages following Conventional Commits:

```
<type>[optional scope]: <description>

Types: feat, fix, refactor, docs, test, chore, ci, perf, style, build
Scope: optional, in parentheses if present
Description: imperative mood, lowercase, no period, max 72 chars after prefix
```

Only the first line (summary) is generated. Commit bodies are not included because atomic commits are inherently small and focused.

#### Automated commit loops

Combine `--committable` and `--commit-msg` for fully automated atomic commits:

```bash
while [ -n "$(darna --committable)" ]; do
    git add $(darna --committable)
    git commit -m "$(darna --commit-msg=claude)"
done
```

This is particularly useful for autonomous coding agents that need to checkpoint progress incrementally.

#### Error handling

- **No staged changes**: Returns error "no staged changes (stage files with git add first)"
- **Agent not installed**: Returns error "agent not found: <name> is not installed"
- **Agent timeout**: 30 second default timeout for LLM generation

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
5. **Commit message generation (optional)** - when `--commit-msg` is used, invoke the specified LLM agent with `git diff --cached` content and the prompt (default or custom via `--prompt-file`), extract the first line of output, and return as the commit message.

## Project structure

```
cmd/darna/           CLI entry point
internal/agent/      LLM agent integrations for commit message generation
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

## Documentation

For design rationale and implementation details of commit message generation, see [ADR-002: LLM-powered commit message generation](docs/decisions/002.md).

## License

See [LICENSE](LICENSE).
