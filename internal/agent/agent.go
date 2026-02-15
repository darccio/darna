// Package agent provides LLM agent integrations for commit message generation.
package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DefaultTimeout is the maximum time an agent has to generate a commit message.
const DefaultTimeout = 30 * time.Second

// DefaultPrompt is the built-in prompt for generating Conventional Commits messages.
const DefaultPrompt = `Generate a single-line commit message for the following diff.
Follow the Conventional Commits format exactly:

<type>[optional scope]: <description>

Types: feat, fix, refactor, docs, test, chore, ci, perf, style, build
Scope: optional, in parentheses if present
Description: imperative mood, lowercase, no period, max 72 chars after prefix

Output ONLY the commit message line. No explanation, no quotes, no markdown.`

// Agent generates commit messages from staged diffs.
type Agent interface {
	// Generate produces a commit message from the given diff using the provided prompt.
	Generate(ctx context.Context, diff, prompt string) (string, error)
}

// ErrUnknownAgent is returned when an unsupported agent type is requested.
var ErrUnknownAgent = errors.New("unknown agent")

// ErrEmptyDiff is returned when the diff is empty.
var ErrEmptyDiff = errors.New("empty diff")

// ErrEmptyResponse is returned when the agent produces no output.
var ErrEmptyResponse = errors.New("agent returned empty response")

// ErrAgentNotFound is returned when the agent binary is not installed.
var ErrAgentNotFound = errors.New("agent not found")

// NewAgent creates an agent for the given type.
// Supported types: "claude", "codex", "mistral", "opencode".
//
//nolint:ireturn // Factory function intentionally returns interface for polymorphism.
func NewAgent(agentType string) (Agent, error) {
	switch agentType {
	case "claude":
		return &cliAgent{
			args: func(prompt string) []string {
				return []string{"-p", prompt, "--output-format", "text"}
			},
			name: "claude",
		}, nil
	case "codex":
		return &cliAgent{
			args: func(prompt string) []string {
				return []string{"exec", prompt}
			},
			name: "codex",
		}, nil
	case "mistral":
		return &cliAgent{
			args: func(prompt string) []string {
				return []string{"-p", prompt}
			},
			name: "mistral",
		}, nil
	case "opencode":
		return &cliAgent{
			args: func(prompt string) []string {
				return []string{"run", prompt}
			},
			name: "opencode",
		}, nil
	default:
		return nil, fmt.Errorf(
			"%w: %s (supported: claude, codex, mistral, opencode)",
			ErrUnknownAgent, agentType,
		)
	}
}

// cliAgent runs an external CLI tool to generate commit messages.
type cliAgent struct {
	args func(prompt string) []string
	name string
}

// Generate invokes the CLI agent with the diff appended to the prompt.
func (ag *cliAgent) Generate(ctx context.Context, diff, prompt string) (string, error) {
	if diff == "" {
		return "", ErrEmptyDiff
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	fullPrompt := prompt + "\n\nDiff:\n" + diff

	//nolint:gosec // Agent name is validated in NewAgent; args built from user-provided prompt.
	cmd := exec.CommandContext(timeoutCtx, ag.name, ag.args(fullPrompt)...)

	var stdout bytes.Buffer

	var stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if isNotFound(err) {
			return "", fmt.Errorf(
				"%w: %s is not installed (install it and ensure it is in your PATH)",
				ErrAgentNotFound, ag.name,
			)
		}

		return "", fmt.Errorf(
			"running %s: %w (stderr: %s)",
			ag.name, err, strings.TrimSpace(stderr.String()),
		)
	}

	msg := strings.TrimSpace(stdout.String())
	if msg == "" {
		return "", fmt.Errorf("%w from %s", ErrEmptyResponse, ag.name)
	}

	// Extract first line only (summary).
	if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
		msg = msg[:idx]
	}

	return msg, nil
}

// isNotFound checks if the error indicates the binary was not found.
func isNotFound(err error) bool {
	var execErr *exec.Error

	return errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound)
}
