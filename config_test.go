package codex

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestArgs(t *testing.T) {
	c := ReviewConfig(t.TempDir())
	c.Model = "chosen-model"
	c.ReasoningEffort = "high"
	c.SkipGitRepoCheck = true
	got, err := c.Args()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"exec", "--json", "--ephemeral", "--color", "never", "--sandbox", "read-only", "--config", `approval_policy="never"`, "--model", "chosen-model", "--config", `model_reasoning_effort="high"`, "--skip-git-repo-check", "-"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	if ImplementationConfig(c.WorkDir).Sandbox != WorkspaceWrite {
		t.Fatal("implementation sandbox")
	}
}

func TestInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Config){
		"relative":       func(c *Config) { c.WorkDir = "." },
		"missing":        func(c *Config) { c.WorkDir = filepath.Join(dir, "missing") },
		"file":           func(c *Config) { c.WorkDir = file },
		"blank model":    func(c *Config) { c.Model = " " },
		"NUL model":      func(c *Config) { c.Model = "a\x00b" },
		"no sandbox":     func(c *Config) { c.Sandbox = "" },
		"unsafe sandbox": func(c *Config) { c.Sandbox = "danger-full-access" },
		"effort":         func(c *Config) { c.ReasoningEffort = "bogus" },
	} {
		t.Run(name, func(t *testing.T) {
			c := ReviewConfig(dir)
			mutate(&c)
			if args, err := c.Args(); err == nil || args != nil {
				t.Fatalf("accepted: %q %v", args, err)
			}
			if _, err := New(c); err == nil {
				t.Fatal("New accepted invalid config")
			}
		})
	}
}

func TestNewSnapshotsConfig(t *testing.T) {
	c := ReviewConfig(t.TempDir())
	h, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	c.Sandbox = WorkspaceWrite
	if h.config.Sandbox != ReadOnly {
		t.Fatal("configuration not snapshotted")
	}
}
