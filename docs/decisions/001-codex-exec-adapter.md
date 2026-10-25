# ADR-001: Codex exec adapter

## Status

Accepted for the initial implementation. Live model validation and publication are separate work.

## Context

libstall exposes a synchronous Harness contract: one isolated conversational execution returns assistant output and token usage. Codex should be available through this contract without coupling libstall or its consumers to Codex process handling.

## Decision

Provide package `codex` in the standalone Go module `github.com/bboykarlito/codex-in-stall`. Depend only on libstall's public types, with YAML as an optional configuration surface. Use a sibling libstall replacement during local development, as the Pi adapter does.

Launch a fresh `codex exec --json --ephemeral` for each execution. This documented interface supplies the lifecycle, assistant message, failure, and usage events needed by the current contract. Require a completed turn and successful process exit. Preserve available results on error, enforce bounded records/diagnostics, and support cancellation without introducing adapter retries.

Expose explicit read-only/workspace-write sandbox selection and always disable interactive approvals. Defer model selection to the caller or Codex's defaults. Preserve native authentication and configuration loading, documenting that this also retains ambient instructions, skills, and integrations. Keep application policies outside the adapter.

## Alternatives

- App-server provides richer thread management, interactive approvals, and steering, but the current Harness contract exposes none of those capabilities. It would add request correlation, initialization, and server lifecycle management without improving this initial API.
- The TypeScript SDK would add a second runtime to a Go adapter using the same underlying CLI.
- Replacing the Pi adapter would remove an independently useful runtime; separate modules let applications choose their harness.

## Consequences

Tests use fake subprocesses and need no credentials or paid inference. Codex CLI and its event schema remain external compatibility dependencies. Cancellation cannot guarantee termination of tool descendants, usage is unavailable on some failures, and native configuration is not isolated. The adapter's YAML schema intentionally differs from Pi's tool/skill schema.
