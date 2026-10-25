package codex

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"go.yaml.in/yaml/v3"
)

// Profile holds reusable Codex-specific settings. WorkDir and task are supplied
// at invocation time; credentials are intentionally absent.
type Profile struct {
	Harness          string      `yaml:"harness"`
	Version          int         `yaml:"version"`
	Model            string      `yaml:"model,omitempty"`
	ReasoningEffort  string      `yaml:"reasoning_effort,omitempty"`
	Sandbox          SandboxMode `yaml:"sandbox"`
	SkipGitRepoCheck bool        `yaml:"skip_git_repo_check,omitempty"`
}

// LoadProfile reads exactly one YAML document, rejecting unknown and duplicate
// fields. Profiles must be regular files no larger than 1 MiB.
func LoadProfile(path string) (Profile, error) {
	f, err := os.Open(path)
	if err != nil {
		return Profile{}, fmt.Errorf("read codex profile: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return Profile{}, err
	}
	if !info.Mode().IsRegular() {
		return Profile{}, errors.New("codex profile must be a regular file")
	}
	const limit = 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return Profile{}, err
	}
	if len(data) > limit {
		return Profile{}, errors.New("codex profile exceeds 1 MiB limit")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var p Profile
	if err := decoder.Decode(&p); err != nil {
		return Profile{}, fmt.Errorf("decode codex profile: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Profile{}, errors.New("codex profile must contain exactly one YAML document")
	}
	if err := p.validate(); err != nil {
		return Profile{}, err
	}
	return p, nil
}

// Config combines the profile with a per-invocation working directory.
func (p Profile) Config(workDir string) (Config, error) {
	if err := p.validate(); err != nil {
		return Config{}, err
	}
	c := p.config(workDir)
	if _, err := c.Args(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (p Profile) config(workDir string) Config {
	return Config{WorkDir: workDir, Model: p.Model, ReasoningEffort: p.ReasoningEffort,
		Sandbox: p.Sandbox, SkipGitRepoCheck: p.SkipGitRepoCheck}
}

func (p Profile) validate() error {
	if p.Harness != "codex" {
		return fmt.Errorf("unsupported harness %q: expected codex", p.Harness)
	}
	if p.Version != 1 {
		return fmt.Errorf("unsupported codex profile version %d: expected 1", p.Version)
	}
	return p.config("").validateOptions()
}
