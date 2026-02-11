package providers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smallnest/goclaw/types"
)

func TestNewRotationProvider(t *testing.T) {
	classifier := types.NewSimpleErrorClassifier()
	rp := NewRotationProvider(RotationStrategyRoundRobin, time.Minute, classifier)

	if rp.strategy != RotationStrategyRoundRobin {
		t.Errorf("Expected RoundRobin strategy, got %v", rp.strategy)
	}
}

func TestRotationProviderAddProfile(t *testing.T) {
	classifier := types.NewSimpleErrorClassifier()
	rp := NewRotationProvider(RotationStrategyRoundRobin, time.Minute, classifier)

	provider := &mockProvider{}
	rp.AddProfile("test", provider, "key1", 1)

	profile, ok := rp.GetProfile("test")
	if !ok {
		t.Error("Expected profile to be found")
	}
	if profile.Name != "test" {
		t.Errorf("Expected name 'test', got %s", profile.Name)
	}
}

func TestRotationProviderRemoveProfile(t *testing.T) {
	classifier := types.NewSimpleErrorClassifier()
	rp := NewRotationProvider(RotationStrategyRoundRobin, time.Minute, classifier)

	provider := &mockProvider{}
	rp.AddProfile("test", provider, "key1", 1)

	rp.RemoveProfile("test")

	_, ok := rp.GetProfile("test")
	if ok {
		t.Error("Expected profile to be removed")
	}
}

func TestRotationProviderListProfiles(t *testing.T) {
	classifier := types.NewSimpleErrorClassifier()
	rp := NewRotationProvider(RotationStrategyRoundRobin, time.Minute, classifier)

	provider := &mockProvider{}
	rp.AddProfile("profile1", provider, "key1", 1)
	rp.AddProfile("profile2", provider, "key2", 2)
	rp.AddProfile("profile3", provider, "key3", 3)

	names := rp.ListProfiles()
	if len(names) != 3 {
		t.Errorf("Expected 3 profiles, got %d", len(names))
	}
}

func TestRotationProviderRoundRobin(t *testing.T) {
	classifier := types.NewSimpleErrorClassifier()
	rp := NewRotationProvider(RotationStrategyRoundRobin, time.Minute, classifier)

	provider1 := &mockProvider{response: &Response{Content: "profile1"}}
	provider2 := &mockProvider{response: &Response{Content: "profile2"}}
	provider3 := &mockProvider{response: &Response{Content: "profile3"}}

	rp.AddProfile("profile1", provider1, "key1", 1)
	rp.AddProfile("profile2", provider2, "key2", 2)
	rp.AddProfile("profile3", provider3, "key3", 3)

	ctx := context.Background()

	// Make 3 requests to verify cycling through all profiles
	seen := make(map[string]bool)

	for i := 0; i < 3; i++ {
		resp, _ := rp.Chat(ctx, nil, nil)
		seen[resp.Content] = true
	}

	// All three profiles should have been seen
	if !seen["profile1"] {
		t.Error("Expected profile1 to be called")
	}
	if !seen["profile2"] {
		t.Error("Expected profile2 to be called")
	}
	if !seen["profile3"] {
		t.Error("Expected profile3 to be called")
	}
}

func TestRotationProviderLeastUsed(t *testing.T) {
	classifier := types.NewSimpleErrorClassifier()
	rp := NewRotationProvider(RotationStrategyLeastUsed, time.Minute, classifier)

	provider1 := &mockProvider{response: &Response{Content: "profile1"}}
	provider2 := &mockProvider{response: &Response{Content: "profile2"}}

	rp.AddProfile("profile1", provider1, "key1", 1)
	rp.AddProfile("profile2", provider2, "key2", 2)

	ctx := context.Background()

	// Make 4 requests and verify counts balance out
	// With least-used strategy, requests should be distributed
	counts := make(map[string]int)

	for i := 0; i < 4; i++ {
		resp, _ := rp.Chat(ctx, nil, nil)
		counts[resp.Content]++
	}

	// After 4 requests, both should have been used at least once
	if counts["profile1"] == 0 {
		t.Error("Expected profile1 to be used at least once")
	}
	if counts["profile2"] == 0 {
		t.Error("Expected profile2 to be used at least once")
	}

	// Verify total counts from profiles
	status1, _ := rp.GetProfileStatus("profile1")
	status2, _ := rp.GetProfileStatus("profile2")

	totalCalls := int(status1["request_count"].(int64)) + int(status2["request_count"].(int64))
	if totalCalls != 4 {
		t.Errorf("Expected 4 total calls, got %d", totalCalls)
	}
}

func TestRotationProviderRandom(t *testing.T) {
	classifier := types.NewSimpleErrorClassifier()
	rp := NewRotationProvider(RotationStrategyRandom, time.Minute, classifier)

	provider := &mockProvider{response: &Response{Content: "response"}}
	rp.AddProfile("profile1", provider, "key1", 1)
	rp.AddProfile("profile2", provider, "key2", 2)

	ctx := context.Background()

	resp, err := rp.Chat(ctx, nil, nil)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if resp == nil {
		t.Error("Expected response to be non-nil")
	}
}

func TestRotationProviderCooldown(t *testing.T) {
	classifier := types.NewSimpleErrorClassifier()
	cooldown := 100 * time.Millisecond
	rp := NewRotationProvider(RotationStrategyRoundRobin, cooldown, classifier)

	// Add only the failing profile first to guarantee it gets selected
	provider1 := &mockProvider{
		shouldFail: true,
		failError:  errors.New("rate limit exceeded"),
		response:   &Response{Content: "profile1"},
	}
	rp.AddProfile("profile1", provider1, "key1", 1)

	ctx := context.Background()

	// Make a call that will fail and trigger cooldown
	_, err := rp.Chat(ctx, nil, nil)
	if err == nil {
		t.Error("Expected error from failing provider")
	}

	// Check profile1 is in cooldown
	status, err := rp.GetProfileStatus("profile1")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if status["in_cooldown"] != true {
		t.Error("Expected profile1 to be in cooldown")
	}

	// Now add a working profile
	provider2 := &mockProvider{response: &Response{Content: "profile2"}}
	rp.AddProfile("profile2", provider2, "key2", 2)

	// Next call should use profile2 (since profile1 is in cooldown)
	resp, err := rp.Chat(ctx, nil, nil)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if resp.Content != "profile2" {
		t.Errorf("Expected 'profile2', got '%s'", resp.Content)
	}

	// Wait for cooldown to expire
	time.Sleep(cooldown + 50*time.Millisecond)

	// Reset cooldown and check
	rp.ResetCooldown()
	status, err = rp.GetProfileStatus("profile1")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if status["in_cooldown"] != false {
		t.Error("Expected profile to be out of cooldown after reset")
	}
}

func TestRotationProviderNoProfiles(t *testing.T) {
	classifier := types.NewSimpleErrorClassifier()
	rp := NewRotationProvider(RotationStrategyRoundRobin, time.Minute, classifier)

	ctx := context.Background()
	_, err := rp.Chat(ctx, nil, nil)

	if err == nil {
		t.Error("Expected error when no profiles available")
	}
}

func TestRotationProviderShouldSetCooldown(t *testing.T) {
	rp := &RotationProvider{}

	tests := []struct {
		name   string
		reason types.FailoverReason
		want   bool
	}{
		{"auth error", types.FailoverReasonAuth, true},
		{"rate limit", types.FailoverReasonRateLimit, true},
		{"billing", types.FailoverReasonBilling, true},
		{"timeout", types.FailoverReasonTimeout, false},
		{"unknown", types.FailoverReasonUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rp.shouldSetCooldown(tt.reason)
			if result != tt.want {
				t.Errorf("Expected %v for %s, got %v", tt.want, tt.name, result)
			}
		})
	}
}

func TestRotationProviderClose(t *testing.T) {
	classifier := types.NewSimpleErrorClassifier()
	rp := NewRotationProvider(RotationStrategyRoundRobin, time.Minute, classifier)

	provider := &mockProvider{}
	rp.AddProfile("test", provider, "key1", 1)

	err := rp.Close()
	if err != nil {
		t.Fatalf("Expected no error on close, got %v", err)
	}
}
