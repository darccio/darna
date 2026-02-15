package agent_test

import (
	"context"
	"errors"
	"testing"

	"dario.cat/darna/internal/agent"
)

func TestNewAgentSupported(t *testing.T) {
	t.Parallel()

	supported := []string{"claude", "codex", "mistral", "opencode"}

	for _, name := range supported {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ag, err := agent.NewAgent(name)
			if err != nil {
				t.Errorf("NewAgent(%q) unexpected error: %v", name, err)
			}

			if ag == nil {
				t.Errorf("NewAgent(%q) returned nil agent", name)
			}
		})
	}
}

func TestNewAgentUnsupported(t *testing.T) {
	t.Parallel()

	unsupported := []string{"unknown", ""}

	for _, name := range unsupported {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ag, err := agent.NewAgent(name)
			if !errors.Is(err, agent.ErrUnknownAgent) {
				t.Errorf("NewAgent(%q) error = %v, want %v", name, err, agent.ErrUnknownAgent)
			}

			if ag != nil {
				t.Errorf("NewAgent(%q) returned non-nil agent on error", name)
			}
		})
	}
}

func TestGenerateEmptyDiff(t *testing.T) {
	t.Parallel()

	ag, err := agent.NewAgent("claude")
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}

	_, err = ag.Generate(context.Background(), "", agent.DefaultPrompt)
	if !errors.Is(err, agent.ErrEmptyDiff) {
		t.Errorf("Generate with empty diff: got %v, want %v", err, agent.ErrEmptyDiff)
	}
}

func TestGenerateAgentNotFound(t *testing.T) {
	t.Parallel()

	// All supported agents are unlikely to be installed in CI.
	agents := []string{"claude", "codex", "mistral", "opencode"}

	for _, name := range agents {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ag, err := agent.NewAgent(name)
			if err != nil {
				t.Fatalf("NewAgent(%q): %v", name, err)
			}

			_, err = ag.Generate(context.Background(), "some diff content", agent.DefaultPrompt)

			// The agent binary is almost certainly not installed in test environment.
			if err == nil {
				t.Skipf("skipping: %s is installed", name)
			}

			// Verify the error is meaningful.
			if err.Error() == "" {
				t.Errorf("Generate returned error with empty message")
			}
		})
	}
}

func TestDefaultPromptNotEmpty(t *testing.T) {
	t.Parallel()

	if agent.DefaultPrompt == "" {
		t.Error("DefaultPrompt should not be empty")
	}
}
