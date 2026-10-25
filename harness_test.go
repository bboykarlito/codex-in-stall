package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	stall "github.com/bboykarlito/libstall"
)

func TestFakeCodexProcess(t *testing.T) {
	scenario := os.Getenv("STALL_FAKE_CODEX")
	if scenario == "" {
		return
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil || string(input) != "Instructions:\nimplement\n\nTask:\n/task" {
		os.Exit(3)
	}
	cwd, _ := os.Getwd()
	actual, actualErr := os.Stat(cwd)
	expected, expectedErr := os.Stat(os.Getenv("STALL_EXPECT_CWD"))
	if actualErr != nil || expectedErr != nil || !os.SameFile(actual, expected) {
		os.Exit(4)
	}
	args := strings.Join(os.Args, " ")
	for _, want := range []string{"exec --json --ephemeral", `approval_policy="never"`, "--sandbox read-only"} {
		if !strings.Contains(args, want) {
			os.Exit(5)
		}
	}
	if scenario == "exit" {
		fmt.Fprintln(os.Stderr, "fake crash")
		os.Exit(1)
	}
	if scenario == "malformed" {
		fmt.Println("not-json")
		os.Exit(0)
	}
	fmt.Println(`{"type":"thread.started","thread_id":"fresh"}`)
	fmt.Println(`{"type":"turn.started"}`)
	fmt.Println(`{"type":"item.completed","item":{"type":"command_execution","text":{}}}`)
	if scenario != "no-response" {
		fmt.Println(`{"type":"item.completed","item":{"type":"agent_message","text":"working"}}`)
		fmt.Println(`{"type":"item.completed","item":{"type":"agent_message","text":"done\u2028text"}}`)
	}
	switch scenario {
	case "cancel":
		for {
			time.Sleep(time.Second)
		}
	case "missing-completion":
		os.Exit(0)
	case "failed":
		fmt.Println(`{"type":"turn.failed","error":{"message":"provider failure"}}`)
		os.Exit(1)
	case "missing-usage":
		fmt.Println(`{"type":"turn.completed"}`)
	case "negative-usage":
		fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":-1,"output_tokens":5}}`)
	default:
		if scenario == "recovered" {
			fmt.Println(`{"type":"error","message":"reconnecting"}`)
		}
		fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":3,"output_tokens":5}}`)
	}
	if scenario == "nonzero-after-completion" {
		os.Exit(7)
	}
	os.Exit(0)
}

func fakeHarness(t *testing.T, scenario string) *Harness {
	t.Helper()
	h, err := New(ReviewConfig(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	h.command = func(ctx context.Context, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFakeCodexProcess$", "--")
		cmd.Args = append(cmd.Args, args...)
		cmd.Env = append(os.Environ(), "STALL_FAKE_CODEX="+scenario, "STALL_EXPECT_CWD="+h.config.WorkDir)
		return cmd
	}
	return h
}

var testRequest = stall.Request{Instructions: "implement", Task: stall.Task{Input: "/task"}}

func TestExecute(t *testing.T) {
	for _, scenario := range []string{"success", "recovered", "exit", "malformed", "failed", "no-response", "missing-completion", "missing-usage", "negative-usage", "nonzero-after-completion"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			h := fakeHarness(t, scenario)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			result, err := h.Execute(ctx, testRequest)
			if scenario == "success" || scenario == "recovered" {
				if err != nil {
					t.Fatal(err)
				}
				if result.Output != "done\u2028text" || result.Usage.InputTokens != 10 || result.Usage.OutputTokens != 5 {
					t.Fatalf("unexpected result: %+v", result)
				}
			} else if err == nil {
				t.Fatal("expected failure")
			}
			if scenario == "failed" && (result.Output == "" || !strings.Contains(err.Error(), "provider failure")) {
				t.Fatalf("lost failure/output: %+v, %v", result, err)
			}
			if scenario == "nonzero-after-completion" && (result.Output == "" || result.Usage.InputTokens != 10) {
				t.Fatalf("lost partial result: %+v", result)
			}
			if scenario == "exit" && !strings.Contains(err.Error(), "fake crash") {
				t.Fatalf("missing stderr: %v", err)
			}
		})
	}
}

func TestCancellation(t *testing.T) {
	h := fakeHarness(t, "cancel")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := h.Execute(ctx, testRequest)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("cancellation took too long")
	}
}

func TestCancelledBeforeStart(t *testing.T) {
	h := fakeHarness(t, "success")
	h.command = func(context.Context, ...string) *exec.Cmd { t.Fatal("launched cancelled request"); return nil }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.Execute(ctx, testRequest); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestStartFailure(t *testing.T) {
	h := fakeHarness(t, "success")
	h.command = func(ctx context.Context, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "/nonexistent/codex")
	}
	if _, err := h.Execute(context.Background(), testRequest); err == nil {
		t.Fatal("expected start failure")
	}
}

func TestEventValidation(t *testing.T) {
	for _, line := range []string{
		`null`, `{}`, `{"type":"item.completed","item":{"type":"agent_message"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":1}}`,
		`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":0,"cached_input_tokens":2}}`,
		`{"type":"turn.completed","usage":{"input_tokens":9223372036854775808,"output_tokens":0}}`,
	} {
		s := eventStream{result: &stall.Result{}}
		if err := s.consume([]byte(line)); err == nil {
			t.Fatalf("accepted %s", line)
		}
	}
	var result stall.Result
	s := eventStream{result: &result}
	for _, line := range []string{
		`{"type":"item.completed","item":{"type":"agent_message","text":"partial"}}`,
		`{"type":"turn.failed","error":{"message":"failed"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":5}}`,
	} {
		if err := s.consume([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	if s.finish() == nil || result.Output != "partial" || result.Usage.InputTokens != 10 {
		t.Fatalf("lost failure/result: %+v", result)
	}
}

func TestTailBuffer(t *testing.T) {
	b := &tailBuffer{}
	input := strings.Repeat("x", 20000) + "end"
	n, err := b.Write([]byte(input))
	if err != nil || n != len(input) || len(b.text) != 8192 || !strings.HasSuffix(b.text, "end") {
		t.Fatal("incorrect stderr tail")
	}
}
