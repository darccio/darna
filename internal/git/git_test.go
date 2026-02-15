package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"dario.cat/darna/internal/git"
)

func TestFilterGoFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files []string
		want  []string
	}{
		{
			name:  "empty list",
			files: []string{},
			want:  nil,
		},
		{
			name:  "no go files",
			files: []string{"README.md", "Makefile", "test.txt"},
			want:  nil,
		},
		{
			name:  "only go files",
			files: []string{"main.go", "test.go", "internal/foo.go"},
			want:  []string{"main.go", "test.go", "internal/foo.go"},
		},
		{
			name:  "mixed files",
			files: []string{"main.go", "README.md", "test.go", "Makefile"},
			want:  []string{"main.go", "test.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := git.FilterGoFiles(tt.files)
			if len(got) != len(tt.want) {
				t.Errorf("FilterGoFiles() = %v, want %v", got, tt.want)

				return
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("FilterGoFiles() = %v, want %v", got, tt.want)

					return
				}
			}
		})
	}
}

func TestFileStatus(t *testing.T) {
	t.Parallel()

	fs := git.FileStatus{
		Staging:  'M',
		Worktree: ' ',
	}

	if fs.Staging != 'M' {
		t.Errorf("Expected staging status 'M', got %c", fs.Staging)
	}

	if fs.Worktree != ' ' {
		t.Errorf("Expected worktree status ' ', got %c", fs.Worktree)
	}
}

func TestGetStagedDiff(t *testing.T) {
	t.Parallel()

	// Create a temporary git repo with staged changes.
	dir := t.TempDir()

	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")

	// Create and commit an initial file.
	initial := filepath.Join(dir, "hello.txt")
	writeTestFile(t, initial, "hello\n")
	runGit(t, dir, "add", "hello.txt")
	runGit(t, dir, "commit", "-m", "initial")

	// Modify and stage the file.
	writeTestFile(t, initial, "hello world\n")
	runGit(t, dir, "add", "hello.txt")

	diff, err := git.GetStagedDiff(context.Background(), dir)
	if err != nil {
		t.Fatalf("GetStagedDiff: %v", err)
	}

	if diff == "" {
		t.Error("GetStagedDiff returned empty diff for staged changes")
	}
}

func TestGetStagedDiffEmpty(t *testing.T) {
	t.Parallel()

	// Create a temporary git repo with no staged changes.
	dir := t.TempDir()

	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")

	// Create and commit a file so HEAD exists.
	initial := filepath.Join(dir, "hello.txt")
	writeTestFile(t, initial, "hello\n")
	runGit(t, dir, "add", "hello.txt")
	runGit(t, dir, "commit", "-m", "initial")

	diff, err := git.GetStagedDiff(context.Background(), dir)
	if err != nil {
		t.Fatalf("GetStagedDiff: %v", err)
	}

	if diff != "" {
		t.Errorf("GetStagedDiff returned non-empty diff for no staged changes: %q", diff)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+dir)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()

	err := os.WriteFile(path, []byte(content), 0o644) //nolint:gosec // Test file permissions.
	if err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
