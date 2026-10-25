package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review.yaml")
	data := "harness: codex\nversion: 1\nsandbox: read-only\nmodel: chosen-model\nreasoning_effort: high\nskip_git_repo_check: true\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	c, err := p.Config(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Model != "chosen-model" || c.ReasoningEffort != "high" || c.Sandbox != ReadOnly || !c.SkipGitRepoCheck {
		t.Fatalf("wrong config: %+v", c)
	}
	if _, err := p.Config("relative"); err == nil {
		t.Fatal("accepted relative directory")
	}
}

func TestInvalidProfiles(t *testing.T) {
	base := "harness: codex\nversion: 1\nsandbox: read-only\n"
	for name, data := range map[string]string{
		"empty": "", "null": "null", "unknown": base + "tools: [read]\n",
		"duplicate": base + "version: 1\n", "multi document": base + "---\n",
		"wrong harness": strings.Replace(base, "codex", "pi", 1),
		"wrong version": strings.Replace(base, "version: 1", "version: 2", 1),
		"no sandbox":    "harness: codex\nversion: 1\n",
		"credentials":   base + "api_key: secret\n", "work dir": base + "work_dir: /tmp\n",
		"oversize": base + strings.Repeat("#", 1024*1024),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "profile.yaml")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadProfile(path); err == nil {
				t.Fatal("accepted invalid profile")
			}
		})
	}
	if _, err := LoadProfile(t.TempDir()); err == nil {
		t.Fatal("accepted directory")
	}
	if _, err := (Profile{Harness: "pi", Version: 1, Sandbox: ReadOnly}).Config(t.TempDir()); err == nil {
		t.Fatal("accepted invalid in-memory profile")
	}
}
