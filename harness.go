package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	stall "github.com/bboykarlito/libstall"
)

// Harness runs one fresh Codex process per request. Callers should set a deadline.
// A Harness may be reused concurrently; each call owns its process and result.
type Harness struct {
	config  Config
	command func(context.Context, ...string) *exec.Cmd
}

var _ stall.Harness = (*Harness)(nil)

// New validates and snapshots configuration without launching Codex.
// Execute requires an authenticated codex executable on PATH.
func New(config Config) (*Harness, error) {
	if _, err := config.Args(); err != nil {
		return nil, err
	}
	return &Harness{config: config, command: func(ctx context.Context, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "codex", args...)
	}}, nil
}

// Execute returns the last completed agent message and reported token usage.
// Available output and usage are retained on failure. Cancellation kills and
// reaps Codex but does not guarantee termination of all tool descendants.
func (h *Harness) Execute(ctx context.Context, request stall.Request) (result stall.Result, err error) {
	if err := ctx.Err(); err != nil {
		return result, err
	}
	args, err := h.config.Args()
	if err != nil {
		return result, err
	}
	cmd := h.command(ctx, args...)
	cmd.Dir = h.config.WorkDir
	cmd.WaitDelay = time.Second
	cmd.Stdin = strings.NewReader("Instructions:\n" + request.Instructions + "\n\nTask:\n" + request.Task.Input)
	stderr := &tailBuffer{}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return result, err
	}
	if err := cmd.Start(); err != nil {
		stdout.Close()
		return result, fmt.Errorf("start codex: %w", err)
	}
	stopClosing := context.AfterFunc(ctx, func() { _ = stdout.Close() })
	defer func() {
		stopClosing()
		stdout.Close()
		if err != nil {
			_ = cmd.Process.Kill()
		}
		if waitErr := cmd.Wait(); waitErr != nil {
			err = errors.Join(err, fmt.Errorf("codex process: %w", waitErr))
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		if err != nil && stderr.text != "" {
			err = fmt.Errorf("%w; codex stderr: %s", err, stderr.text)
		}
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 16*1024*1024)
	stream := eventStream{result: &result}
	for scanner.Scan() {
		if err := stream.consume(scanner.Bytes()); err != nil {
			return result, err
		}
	}
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("read codex event: %w", err)
	}
	return result, stream.finish()
}

type event struct {
	Type  string          `json:"type"`
	Item  json.RawMessage `json:"item"`
	Usage *struct {
		Input  *int64 `json:"input_tokens"`
		Output *int64 `json:"output_tokens"`
		Cached *int64 `json:"cached_input_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Message string `json:"message"`
}

type eventStream struct {
	result               *stall.Result
	completed, assistant bool
	failure              error
	failed               bool
}

func (s *eventStream) consume(line []byte) error {
	var e event
	if err := json.Unmarshal(line, &e); err != nil {
		return fmt.Errorf("decode codex event: %w", err)
	}
	if e.Type == "" {
		return errors.New("codex event missing type")
	}
	switch e.Type {
	case "item.completed":
		var item struct {
			Type string  `json:"type"`
			Text *string `json:"text"`
		}
		// Decode only agent-message fields: other item types have different schemas.
		var kind struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(e.Item, &kind); err != nil {
			return fmt.Errorf("decode codex item: %w", err)
		}
		if kind.Type != "agent_message" {
			return nil
		}
		if err := json.Unmarshal(e.Item, &item); err != nil {
			return fmt.Errorf("decode codex message: %w", err)
		}
		if item.Text == nil {
			return errors.New("codex agent message missing text")
		}
		s.result.Output = *item.Text
		s.assistant = true
	case "turn.completed":
		if s.completed {
			return errors.New("codex emitted multiple completed turns")
		}
		if e.Usage == nil || e.Usage.Input == nil || e.Usage.Output == nil {
			return errors.New("codex completion missing token counts")
		}
		u := e.Usage
		if *u.Input < 0 || *u.Output < 0 || (u.Cached != nil && (*u.Cached < 0 || *u.Cached > *u.Input)) {
			return errors.New("codex reported invalid token counts")
		}
		// Cached input is a subset of input_tokens, not an additional count.
		s.result.Usage = stall.Usage{InputTokens: *u.Input, OutputTokens: *u.Output}
		s.completed = true
	case "turn.failed":
		s.failed = true
		message := "unknown error"
		if e.Error != nil && e.Error.Message != "" {
			message = e.Error.Message
		}
		s.failure = fmt.Errorf("codex turn failed: %s", message)
	case "error":
		// Standalone errors can be transient reconnect notices. A subsequent
		// completed turn and zero exit status establish recovery.
		s.failure = fmt.Errorf("codex error: %s", e.Message)
	}
	return nil
}

func (s *eventStream) finish() error {
	if s.failed {
		return s.failure
	}
	if !s.completed {
		if s.failure != nil {
			return s.failure
		}
		return errors.New("codex stream ended without turn.completed")
	}
	if !s.assistant {
		return errors.New("codex completed without an assistant message")
	}
	return nil
}

// tailBuffer is written by exec's stderr copier and read only after Wait.
type tailBuffer struct{ text string }

func (b *tailBuffer) Write(p []byte) (int, error) {
	const limit = 8192
	n := len(p)
	if len(p) >= limit {
		b.text = string(p[len(p)-limit:])
		return n, nil
	}
	b.text += string(p)
	if len(b.text) > limit {
		b.text = b.text[len(b.text)-limit:]
	}
	return n, nil
}
