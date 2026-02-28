package agent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	agenttools "github.com/smallnest/goclaw/agent/tools"
	"github.com/smallnest/goclaw/bus"
)

type fakeCronTool struct {
	mu         sync.Mutex
	listOutput string
	runCount   int
	lastRunID  string
}

func (t *fakeCronTool) Name() string { return "cron" }

func (t *fakeCronTool) Description() string { return "fake cron tool" }

func (t *fakeCronTool) Label() string { return "cron" }

func (t *fakeCronTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{"command": map[string]interface{}{"type": "string"}},
		"required":   []string{"command"},
	}
}

func (t *fakeCronTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	cmd, _ := params["command"].(string)
	t.mu.Lock()
	defer t.mu.Unlock()

	if cmd == "list" {
		return t.listOutput, nil
	}
	if strings.HasPrefix(cmd, "run ") {
		t.runCount++
		t.lastRunID = strings.TrimSpace(strings.TrimPrefix(cmd, "run "))
		return "ok", nil
	}
	return "ok", nil
}

func TestIsCronOneShotRequest(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{text: "执行一次定时任务", want: true},
		{text: "只测试一次定时任务", want: true},
		{text: "cron run job-abc123", want: true},
		{text: "帮我看下天气", want: false},
	}
	for _, tc := range cases {
		if got := isCronOneShotRequest(tc.text); got != tc.want {
			t.Fatalf("isCronOneShotRequest(%q)=%v want %v", tc.text, got, tc.want)
		}
	}
}

func TestExtractEnabledCronJobIDs(t *testing.T) {
	out := `Found 2 job(s):

job-a1b2c3 (enabled)
  Name: one
job-d4e5f6 (disabled)
  Name: two
job-z9y8x7 (enabled)
  Name: three
`
	got := extractEnabledCronJobIDs(out)
	if len(got) != 2 || got[0] != "job-a1b2c3" || got[1] != "job-z9y8x7" {
		t.Fatalf("unexpected enabled ids: %#v", got)
	}
}

func TestHandleDirectCronOneShotRunsSingleEnabledJob(t *testing.T) {
	messageBus := bus.NewMessageBus(16)
	sub := messageBus.SubscribeOutbound()
	defer sub.Unsubscribe()

	reg := NewToolRegistry()
	fake := &fakeCronTool{
		listOutput: "Found 1 job(s):\n\njob-a1b2c3 (enabled)\n  Name: one\n",
	}
	if err := reg.RegisterExisting(fake); err != nil {
		t.Fatalf("register fake cron tool: %v", err)
	}

	mgr := &AgentManager{
		bus:            messageBus,
		tools:          reg,
		manualCronLast: make(map[string]time.Time),
	}

	handled, err := mgr.handleDirectCronOneShot(context.Background(), &bus.InboundMessage{
		ID:      "msg-1",
		Channel: "feishu",
		ChatID:  "chat-1",
		Content: "只测试一次定时任务",
	})
	if err != nil {
		t.Fatalf("handleDirectCronOneShot error: %v", err)
	}
	if !handled {
		t.Fatalf("expected request to be handled")
	}

	timeout := time.After(2 * time.Second)
	gotMessages := 0
	for gotMessages < 2 {
		select {
		case <-timeout:
			t.Fatalf("timed out waiting outbound messages")
		case out := <-sub.Channel:
			if out != nil {
				gotMessages++
			}
		}
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.runCount != 1 || fake.lastRunID != "job-a1b2c3" {
		t.Fatalf("unexpected run state: count=%d id=%q", fake.runCount, fake.lastRunID)
	}
}

func TestHandleDirectCronOneShotDeduplicatesWithinCooldown(t *testing.T) {
	messageBus := bus.NewMessageBus(16)
	reg := NewToolRegistry()
	fake := &fakeCronTool{
		listOutput: "Found 1 job(s):\n\njob-a1b2c3 (enabled)\n  Name: one\n",
	}
	if err := reg.RegisterExisting(fake); err != nil {
		t.Fatalf("register fake cron tool: %v", err)
	}

	mgr := &AgentManager{
		bus:            messageBus,
		tools:          reg,
		manualCronLast: make(map[string]time.Time),
	}

	msg := &bus.InboundMessage{
		ID:      "msg-dup",
		Channel: "feishu",
		ChatID:  "chat-1",
		Content: "执行一次定时任务 job-a1b2c3",
	}

	handled, err := mgr.handleDirectCronOneShot(context.Background(), msg)
	if err != nil || !handled {
		t.Fatalf("first run handled=%v err=%v", handled, err)
	}
	handled, err = mgr.handleDirectCronOneShot(context.Background(), msg)
	if err != nil || !handled {
		t.Fatalf("second run handled=%v err=%v", handled, err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		fake.mu.Lock()
		count := fake.runCount
		fake.mu.Unlock()
		if count >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting first run")
		}
		time.Sleep(10 * time.Millisecond)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.runCount != 1 {
		t.Fatalf("expected deduplicated run count 1, got %d", fake.runCount)
	}
}

var _ agenttools.Tool = (*fakeCronTool)(nil)
