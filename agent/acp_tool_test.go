package agent

import "testing"

func TestBuildSpawnContextFromSessionKey(t *testing.T) {
	ctx, err := buildSpawnContextFromSessionKey("telegram:acc-1:thread-42")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if ctx.AgentChannel != "telegram" {
		t.Fatalf("unexpected channel: %q", ctx.AgentChannel)
	}
	if ctx.AgentAccountID != "acc-1" {
		t.Fatalf("unexpected account id: %q", ctx.AgentAccountID)
	}
	if ctx.AgentThreadID != "thread-42" {
		t.Fatalf("unexpected thread id: %q", ctx.AgentThreadID)
	}
	if ctx.AgentTo != "thread-42" {
		t.Fatalf("unexpected recipient: %q", ctx.AgentTo)
	}
}

func TestBuildSpawnContextFromSessionKeyInvalid(t *testing.T) {
	if _, err := buildSpawnContextFromSessionKey("invalid-key"); err == nil {
		t.Fatalf("expected error for invalid session key")
	}
}
