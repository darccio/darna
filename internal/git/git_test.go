package git_test

import (
	"testing"

	"github.com/darccio/darna/internal/git"
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
