# codex-in-stall

A Codex CLI adapter implementing `stall.Harness` from [libstall](https://github.com/bboykarlito/libstall).

Module: `github.com/bboykarlito/codex-in-stall`. Package: `codex`.

Experimental local module; not a published release. Protocol behavior is tested with fake subprocesses. CLI flags were checked against Codex CLI 0.163.0; live model compatibility remains unverified.

## Local development

Requires Go 1.24.4 or later and sibling checkouts:

```text
Projects/
  libstall/
  codex-in-stall/
```

```sh
go test -race ./...
go vet ./...
```

Tests do not require Codex, credentials, or model calls. Real execution requires an authenticated `codex` on PATH supporting `exec --json --ephemeral`. Authentication stays with Codex.

The local `go.mod` replaces libstall with `../libstall`. Consumers must declare both replacements because Go does not inherit dependency replacements:

```go
require (
    github.com/bboykarlito/libstall v0.0.0
    github.com/bboykarlito/codex-in-stall v0.0.0
)

replace github.com/bboykarlito/libstall => ../libstall
replace github.com/bboykarlito/codex-in-stall => ../codex-in-stall
```

## Usage

```go
import (
    "context"
    "time"

    stall "github.com/bboykarlito/libstall"
    codex "github.com/bboykarlito/codex-in-stall"
)

func review(repositoryPath, task string) (stall.Result, error) {
    config := codex.ReviewConfig(repositoryPath) // absolute existing directory
    harness, err := codex.New(config)
    if err != nil {
        return stall.Result{}, err
    }
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
    defer cancel()
    return harness.Execute(ctx, stall.Request{
        Instructions: "Review the requested change and report concrete findings.",
        Task: stall.Task{Input: task},
    })
}
```

The harness also composes with `stall.Runner` and any application-owned `stall.Behaviour`.

- `New(Config)` validates and snapshots configuration without launching a process.
- `Config.Args()` returns validated CLI arguments without execution.
- `ImplementationConfig(workDir)` requests the `workspace-write` sandbox.
- `ReviewConfig(workDir)` requests the `read-only` sandbox. Shell commands can still run under that policy.
- `LoadProfile(path)` loads reusable settings; `Profile.Config(workDir)` adds and validates the working directory.
- `Harness.Execute(ctx, request)` starts a fresh conversation and returns output and usage, including available results on error. A harness can be reused concurrently.

`Model` and `ReasoningEffort` are optional and otherwise use Codex's configured defaults. Set them explicitly for reproducible selection; model access and supported effort levels depend on the installed runtime and provider. `SkipGitRepoCheck` is an explicit opt-in for directories outside Git repositories.

## YAML profiles

```yaml
harness: codex
version: 1
sandbox: read-only
# model: your-model-id
# reasoning_effort: high
# skip_git_repo_check: false
```

`harness`, `version`, and `sandbox` are required. Supported sandbox values are `read-only` and `workspace-write`. Supported explicit reasoning efforts are `none`, `minimal`, `low`, `medium`, `high`, and `xhigh`. Unknown or duplicate fields, multiple documents, unsupported settings, non-regular files, and files over 1 MiB are rejected. Working directories, task text, credentials, arbitrary flags, and Pi tool/skill settings are not profile fields.

## Execution contract

Each call launches `codex exec --json --ephemeral` in `WorkDir` with the selected sandbox, color disabled, and `approval_policy="never"`. Instructions and task are sent together as a labeled user prompt through stdin, keeping task content out of command arguments. No shell is used to construct or launch the command, and no previous conversation is resumed.

The adapter returns the last completed `agent_message` text. It requires `turn.completed` with valid token counts, an assistant message, and a successful process exit. It rejects malformed streams, missing completion/usage, failed turns, and nonzero exits while preserving available results. A standalone `error` event can represent a transient reconnect; a subsequent completed turn and zero exit status establish recovery. Unknown event types are ignored for forward compatibility. JSONL records are limited to 16 MiB; stderr retains its last 8 KiB and is included on errors.

`input_tokens` already includes cached input; the adapter does not add `cached_input_tokens` again. `output_tokens` is copied as reported. No monetary cost is calculated. If execution fails before usage arrives, zero counts mean **unknown**, not free. No adapter-level retries or spend cap are provided; Codex may perform its own retries or compaction.

Use a context deadline. Cancellation closes the output pipe, kills and reaps Codex, and preserves the context error. Tool descendants are not guaranteed to terminate. Ephemeral mode controls conversation persistence, not all Codex logs or caches.

Codex continues to load its native configuration, instructions, skills, plugins, and MCP integrations. The adapter does not promise a clean configuration environment or a Pi-style tool allowlist. Sandbox enforcement belongs to Codex and the host; external integrations can have effects outside the shell sandbox. Applications own configuration trust, repository policy, and any required security boundary. Review/implementation helpers select sandbox settings, not permanent roles.

## Design and sources

- [ADR-001: Codex exec adapter](docs/decisions/001-codex-exec-adapter.md)
- [Official Codex non-interactive mode](https://learn.chatgpt.com/docs/non-interactive-mode)
- [Official Codex app-server protocol](https://learn.chatgpt.com/docs/app-server)
