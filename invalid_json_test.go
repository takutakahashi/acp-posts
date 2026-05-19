package main

import (
	"strings"
	"testing"
)

func TestProcessBuffer_InvalidJSON(t *testing.T) {
	output := captureOutput(func() {
		processBuffer("{invalid json}", "", "")
	})
	if strings.TrimSpace(output) != "" {
		t.Errorf("expected no output for invalid JSON, got: %q", output)
	}
}

func TestProcessBuffer_InvalidMessageJSON(t *testing.T) {
	output := captureOutput(func() {
		processBuffer(`{"type":"assistant","message":"not-an-object","sessionId":"s1"}`, "", "")
	})
	if strings.TrimSpace(output) != "" {
		t.Errorf("expected no output for invalid message JSON, got: %q", output)
	}
}
