// Package codex adapts the Codex CLI to libstall's execution contract.
package codex

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// SandboxMode selects the sandbox requested from Codex. Enforcement is owned
// by the installed Codex runtime and its host environment.
type SandboxMode string

const (
	ReadOnly       SandboxMode = "read-only"
	WorkspaceWrite SandboxMode = "workspace-write"
)

// Config describes an invocation. WorkDir must be absolute and exist.
// Empty Model and ReasoningEffort use Codex's configured defaults.
// Authentication and ambient configuration remain managed by Codex.
type Config struct {
	WorkDir          string
	Model            string
	ReasoningEffort  string
	Sandbox          SandboxMode
	SkipGitRepoCheck bool
}

// ImplementationConfig requests write access to the workspace.
func ImplementationConfig(workDir string) Config {
	return Config{WorkDir: workDir, Sandbox: WorkspaceWrite}
}

// ReviewConfig requests Codex's read-only sandbox; shell tools may still run.
func ReviewConfig(workDir string) Config {
	return Config{WorkDir: workDir, Sandbox: ReadOnly}
}

// Args validates configuration and constructs a fresh, ephemeral invocation.
// The prompt is read from stdin. Arguments must not be joined into a shell command.
func (c Config) Args() ([]string, error) {
	if !filepath.IsAbs(c.WorkDir) {
		return nil, errors.New("codex working directory must be absolute")
	}
	info, err := os.Stat(c.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("codex working directory: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("codex working directory is not a directory")
	}
	if err := c.validateOptions(); err != nil {
		return nil, err
	}
	args := []string{"exec", "--json", "--ephemeral", "--color", "never",
		"--sandbox", string(c.Sandbox), "--config", `approval_policy="never"`}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	if c.ReasoningEffort != "" {
		args = append(args, "--config", "model_reasoning_effort="+strconv.Quote(c.ReasoningEffort))
	}
	if c.SkipGitRepoCheck {
		args = append(args, "--skip-git-repo-check")
	}
	return append(args, "-"), nil
}

func (c Config) validateOptions() error {
	if c.Model != "" && (strings.TrimSpace(c.Model) != c.Model || strings.ContainsAny(c.Model, "\x00\r\n")) {
		return errors.New("codex model must be nonblank and contain no surrounding whitespace, NUL or newlines")
	}
	switch c.Sandbox {
	case ReadOnly, WorkspaceWrite:
	default:
		return fmt.Errorf("unsupported codex sandbox %q: expected read-only or workspace-write", c.Sandbox)
	}
	switch c.ReasoningEffort {
	case "", "none", "minimal", "low", "medium", "high", "xhigh":
	default:
		return fmt.Errorf("unsupported codex reasoning effort %q", c.ReasoningEffort)
	}
	return nil
}
