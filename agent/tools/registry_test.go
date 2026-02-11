package tools

import (
	"context"
	"errors"
	"testing"
)

// mockTool 用于测试的模拟工具
type mockTool struct {
	name   string
	params map[string]interface{}
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Description() string {
	return "mock tool for testing"
}

func (m *mockTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"param1": map[string]interface{}{
				"type":        "string",
				"description": "test parameter",
			},
		},
		"required": []string{"param1"},
	}
}

func (m *mockTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	m.params = params
	return "result", nil
}

// failingTool 模拟会失败的工具
type failingTool struct {
	mockTool
	failError error
}

func (f *failingTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	return "", f.failError
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("Expected non-nil registry")
	}
	if r.Count() != 0 {
		t.Errorf("Expected 0 tools, got %d", r.Count())
	}
}

func TestRegistryRegister(t *testing.T) {
	r := NewRegistry()
	tool := &mockTool{name: "test_tool"}

	err := r.Register(tool)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if r.Count() != 1 {
		t.Errorf("Expected 1 tool, got %d", r.Count())
	}

	if !r.Has("test_tool") {
		t.Error("Expected tool to be registered")
	}
}

func TestRegistryDuplicateRegister(t *testing.T) {
	r := NewRegistry()
	tool := &mockTool{name: "test_tool"}

	_ = r.Register(tool)
	err := r.Register(tool)

	if err == nil {
		t.Error("Expected error when registering duplicate tool")
	}
}

func TestRegistryUnregister(t *testing.T) {
	r := NewRegistry()
	tool := &mockTool{name: "test_tool"}

	_ = r.Register(tool)
	r.Unregister("test_tool")

	if r.Has("test_tool") {
		t.Error("Expected tool to be unregistered")
	}

	if r.Count() != 0 {
		t.Errorf("Expected 0 tools, got %d", r.Count())
	}
}

func TestRegistryGet(t *testing.T) {
	r := NewRegistry()
	tool := &mockTool{name: "test_tool"}

	_ = r.Register(tool)

	retrieved, ok := r.Get("test_tool")
	if !ok {
		t.Fatal("Expected tool to be found")
	}
	if retrieved.Name() != "test_tool" {
		t.Errorf("Expected 'test_tool', got '%s'", retrieved.Name())
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("Expected false for nonexistent tool")
	}
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()

	_ = r.Register(&mockTool{name: "tool1"})
	_ = r.Register(&mockTool{name: "tool2"})
	_ = r.Register(&mockTool{name: "tool3"})

	tools := r.List()
	if len(tools) != 3 {
		t.Errorf("Expected 3 tools, got %d", len(tools))
	}
}

func TestRegistryGetDefinitions(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&mockTool{name: "test_tool"})

	defs := r.GetDefinitions()
	if len(defs) != 1 {
		t.Errorf("Expected 1 definition, got %d", len(defs))
	}
}

func TestRegistryExecute(t *testing.T) {
	r := NewRegistry()
	tool := &mockTool{name: "test_tool"}
	_ = r.Register(tool)

	ctx := context.Background()
	params := map[string]interface{}{"param1": "value"}

	result, err := r.Execute(ctx, "test_tool", params)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != "result" {
		t.Errorf("Expected 'result', got '%s'", result)
	}

	if tool.params["param1"] != "value" {
		t.Error("Expected params to be passed to tool")
	}
}

func TestRegistryExecuteNonexistent(t *testing.T) {
	r := NewRegistry()

	ctx := context.Background()
	_, err := r.Execute(ctx, "nonexistent", nil)

	if err == nil {
		t.Error("Expected error for nonexistent tool")
	}
}

func TestRegistryExecuteFailure(t *testing.T) {
	r := NewRegistry()
	tool := &failingTool{
		mockTool: mockTool{name: "failing_tool"},
		failError: errors.New("execution failed"),
	}
	_ = r.Register(tool)

	ctx := context.Background()
	_, err := r.Execute(ctx, "failing_tool", map[string]interface{}{"param1": "value"})

	if err == nil {
		t.Error("Expected error from failing tool")
	}
}

func TestRegistryClear(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&mockTool{name: "tool1"})
	_ = r.Register(&mockTool{name: "tool2"})

	if r.Count() != 2 {
		t.Errorf("Expected 2 tools before clear, got %d", r.Count())
	}

	r.Clear()

	if r.Count() != 0 {
		t.Errorf("Expected 0 tools after clear, got %d", r.Count())
	}
}
