# Required validations

After each work request - one per prompt - always these steps before yielding the control back to me:

- Run `make lint` and fix any issue.
- Run `make test` and ensure that coverage increases. You can keep track of the coverage wherever you see fit.

# Atomic commits

Atomic commits are the smallest logical unit of change where nothing can be removed and nothing needs to be added — each commit must compile, pass tests, and be independently reviewable. When developing, you build a patch set (a sequence of atomic commits on a feature branch) that tells a coherent story, then submit the entire patch set for review. Reviewers can examine individual commits in isolation or the whole diff, making reviews 1.5–5× faster. This approach requires linear history (no merge commits) and heavy use of interactive rebase (git rebase -i) to restructure, reorder, squash, or split commits before submitting. The key validation rule: every symbol (function, type, variable) used in a commit must be defined either in that same commit or in a previous commit — never in a future commit or unstaged file. This ensures any commit in history can be checked out and will work.

# Comments

Always write comments following `godot` expectations:

- Sentence should start with a capital letter
- Comment should end in a period

This is because `godot` fix has a bug and duplicates the whole line instead of fixing the comment.
