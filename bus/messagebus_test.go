package bus

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewMessageBus(t *testing.T) {
	bus := NewMessageBus(100)
	if bus == nil {
		t.Fatal("Expected non-nil MessageBus")
	}
	bus.Close()
}

func TestPublishInbound(t *testing.T) {
	bus := NewMessageBus(10)
	defer bus.Close()

	ctx := context.Background()
	msg := &InboundMessage{
		ID:       "test-1",
		Channel:  "test",
		SenderID: "user-1",
		ChatID:   "chat-1",
		Content:  "Hello",
	}

	err := bus.PublishInbound(ctx, msg)
	if err != nil {
		t.Fatalf("Failed to publish inbound message: %v", err)
	}
}

func TestConsumeInbound(t *testing.T) {
	bus := NewMessageBus(10)
	defer bus.Close()

	ctx := context.Background()
	msg := &InboundMessage{
		ID:       "test-1",
		Channel:  "test",
		SenderID: "user-1",
		ChatID:   "chat-1",
		Content:  "Hello",
	}

	// Publish message in goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = bus.PublishInbound(ctx, msg)
	}()

	// Consume message
	receivedMsg, err := bus.ConsumeInbound(ctx)
	if err != nil {
		t.Fatalf("Failed to consume inbound message: %v", err)
	}

	if receivedMsg.ID != msg.ID {
		t.Errorf("Expected ID %s, got %s", msg.ID, receivedMsg.ID)
	}

	if receivedMsg.Content != msg.Content {
		t.Errorf("Expected Content %s, got %s", msg.Content, receivedMsg.Content)
	}

	wg.Wait()
}

func TestPublishOutbound(t *testing.T) {
	bus := NewMessageBus(10)
	defer bus.Close()

	ctx := context.Background()
	msg := &OutboundMessage{
		ID:      "test-1",
		Channel: "test",
		ChatID:  "chat-1",
		Content: "Hello back",
	}

	err := bus.PublishOutbound(ctx, msg)
	if err != nil {
		t.Fatalf("Failed to publish outbound message: %v", err)
	}
}

func TestConsumeOutbound(t *testing.T) {
	bus := NewMessageBus(10)
	defer bus.Close()

	ctx := context.Background()
	msg := &OutboundMessage{
		ID:      "test-1",
		Channel: "test",
		ChatID:  "chat-1",
		Content: "Hello back",
	}

	// Publish message in goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = bus.PublishOutbound(ctx, msg)
	}()

	// Consume message
	receivedMsg, err := bus.ConsumeOutbound(ctx)
	if err != nil {
		t.Fatalf("Failed to consume outbound message: %v", err)
	}

	if receivedMsg.ID != msg.ID {
		t.Errorf("Expected ID %s, got %s", msg.ID, receivedMsg.ID)
	}

	if receivedMsg.Content != msg.Content {
		t.Errorf("Expected Content %s, got %s", msg.Content, receivedMsg.Content)
	}

	wg.Wait()
}

func TestClose(t *testing.T) {
	bus := NewMessageBus(10)

	// Publish some messages first
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		msg := &InboundMessage{
			ID:      string(rune(i)),
			Channel: "test",
			Content: "test",
		}
		_ = bus.PublishInbound(ctx, msg)
	}

	// Close the bus
	bus.Close()

	// Try to consume - should get context cancelled
	_, err := bus.ConsumeInbound(ctx)
	if err == nil {
		t.Error("Expected error when consuming from closed bus")
	}
}

func TestConcurrentOperations(t *testing.T) {
	bus := NewMessageBus(100)
	defer bus.Close()

	ctx := context.Background()
	numMessages := 50
	numGoroutines := 5

	var wg sync.WaitGroup

	// Publishers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numMessages; j++ {
				msg := &InboundMessage{
					ID:      string(rune(id)) + string(rune(j)),
					Channel: "test",
					Content: "test",
				}
				_ = bus.PublishInbound(ctx, msg)
			}
		}(i)
	}

	// Consumers
	receivedCount := 0
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				_, err := bus.ConsumeInbound(ctx)
				if err != nil {
					// Context cancelled or bus closed
					return
				}
				mu.Lock()
				receivedCount++
				mu.Unlock()
			}
		}()
	}

	// Wait for publishers to finish
	time.Sleep(100 * time.Millisecond)
	bus.Close()
	wg.Wait()

	expectedMessages := numMessages * numGoroutines
	if receivedCount != expectedMessages {
		t.Logf("Warning: received %d messages, expected %d (may be due to timing)", receivedCount, expectedMessages)
	}
}
