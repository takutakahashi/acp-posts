package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func captureOutput(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

func TestProcessBuffer_AssistantTextMessage(t *testing.T) {
	output := captureOutput(func() {
		processBuffer(`{"type":"assistant","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"Hello world"}],"stop_reason":"end_turn"},"sessionId":"session-1"}`, "", "")
	})
	if !strings.Contains(output, "Hello world") {
		t.Errorf("expected output to contain 'Hello world', got: %q", output)
	}
}

func TestProcessBuffer_IgnoresThinkingContent(t *testing.T) {
	output := captureOutput(func() {
		processBuffer(`{"type":"assistant","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"thinking","thinking":"internal thought"},{"type":"text","text":"visible response"}],"stop_reason":"end_turn"},"sessionId":"session-1"}`, "", "")
	})
	if strings.Contains(output, "internal thought") {
		t.Errorf("output should not contain thinking content, got: %q", output)
	}
	if !strings.Contains(output, "visible response") {
		t.Errorf("expected output to contain 'visible response', got: %q", output)
	}
}

func TestProcessBuffer_IgnoresToolUse(t *testing.T) {
	output := captureOutput(func() {
		processBuffer(`{"type":"assistant","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"ls"}}],"stop_reason":"tool_use"},"sessionId":"session-1"}`, "", "")
	})
	if strings.TrimSpace(output) != "" {
		t.Errorf("expected no output for tool_use message, got: %q", output)
	}
}

func TestProcessBuffer_IgnoresNonAssistantTypes(t *testing.T) {
	types := []string{"user", "queue-operation", "attachment", "last-prompt"}
	for _, msgType := range types {
		output := captureOutput(func() {
			processBuffer(fmt.Sprintf(`{"type":%q,"sessionId":"session-1"}`, msgType), "", "")
		})
		if strings.TrimSpace(output) != "" {
			t.Errorf("type %q: expected no output, got: %q", msgType, output)
		}
	}
}

func TestProcessBuffer_EmptyLine(t *testing.T) {
	output := captureOutput(func() {
		processBuffer("", "", "")
		processBuffer("   ", "", "")
	})
	if strings.TrimSpace(output) != "" {
		t.Errorf("expected no output for empty lines, got: %q", output)
	}
}

func TestProcessBuffer_EmptyText(t *testing.T) {
	output := captureOutput(func() {
		processBuffer(`{"type":"assistant","message":{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":""}],"stop_reason":"end_turn"},"sessionId":"session-1"}`, "", "")
	})
	if strings.TrimSpace(output) != "" {
		t.Errorf("expected no output for empty text, got: %q", output)
	}
}

func TestConvertMarkdownToSlack_Bold(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"**bold**", "*bold*"},
		{"__bold__", "*bold*"},
	}
	for _, c := range cases {
		got := convertMarkdownToSlack(c.input)
		if got != c.want {
			t.Errorf("convertMarkdownToSlack(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestConvertMarkdownToSlack_Strikethrough(t *testing.T) {
	got := convertMarkdownToSlack("~~strike~~")
	if got != "~strike~" {
		t.Errorf("expected ~strike~, got %q", got)
	}
}

func TestConvertMarkdownToSlack_Link(t *testing.T) {
	got := convertMarkdownToSlack("[label](https://example.com)")
	if got != "<https://example.com|label>" {
		t.Errorf("got %q", got)
	}
}

func TestConvertMarkdownToSlack_Header(t *testing.T) {
	got := convertMarkdownToSlack("# My Header")
	if got != "*My Header*" {
		t.Errorf("got %q", got)
	}
}

func TestConvertMarkdownToSlack_PreservesCodeBlock(t *testing.T) {
	input := "```go\nfmt.Println(\"hello\")\n```"
	got := convertMarkdownToSlack(input)
	if !strings.Contains(got, "fmt.Println") {
		t.Errorf("code block content should be preserved, got %q", got)
	}
}

func TestWatchFile(t *testing.T) {
	f, err := os.CreateTemp("", "acp-test-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.Close()

	outputs := []string{}
	done := make(chan struct{})

	go func() {
		defer close(done)
		// We simulate by just processing directly
		offset := processFileFrom(f.Name(), 0, "", "")
		if offset != 0 {
			t.Errorf("expected offset 0 for empty file, got %d", offset)
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	_ = outputs
}

func TestProcessBuffer_MultipleTextContent(t *testing.T) {
	output := captureOutput(func() {
		processBuffer(`{"type":"assistant","message":{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"First part"},{"type":"text","text":"Second part"}],"stop_reason":"end_turn"},"sessionId":"session-1"}`, "", "")
	})
	if !strings.Contains(output, "First part") {
		t.Errorf("expected 'First part' in output, got: %q", output)
	}
	if !strings.Contains(output, "Second part") {
		t.Errorf("expected 'Second part' in output, got: %q", output)
	}
}
