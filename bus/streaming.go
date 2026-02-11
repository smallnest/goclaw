package bus

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// StreamMessage represents a streaming message update
type StreamMessage struct {
	ID         string                 `json:"id"`
	Channel    string                 `json:"channel"`
	ChatID     string                 `json:"chat_id"`
	Content    string                 `json:"content"`
	ChunkIndex int                    `json:"chunk_index"`
	IsComplete bool                   `json:"is_complete"`
	IsThinking bool                   `json:"is_thinking"`
	IsFinal    bool                   `json:"is_final"`
	Metadata   map[string]interface{} `json:"metadata"`
	Error      string                 `json:"error,omitempty"`
}

// StreamingMessageBus extends MessageBus with streaming support
type StreamingMessageBus struct {
	*MessageBus
	streamStreams map[string]chan *StreamMessage
	streamMu      sync.RWMutex
}

// NewStreamingMessageBus creates a new streaming message bus
func NewStreamingMessageBus(bufferSize int) *StreamingMessageBus {
	return &StreamingMessageBus{
		MessageBus:    NewMessageBus(bufferSize),
		streamStreams: make(map[string]chan *StreamMessage),
	}
}

// CreateStream creates a new stream for a chat
func (b *StreamingMessageBus) CreateStream(chatID string) chan *StreamMessage {
	b.streamMu.Lock()
	defer b.streamMu.Unlock()

	stream := make(chan *StreamMessage, 100)
	b.streamStreams[chatID] = stream

	return stream
}

// GetStream gets an existing stream for a chat
func (b *StreamingMessageBus) GetStream(chatID string) (chan *StreamMessage, bool) {
	b.streamMu.RLock()
	defer b.streamMu.RUnlock()

	stream, ok := b.streamStreams[chatID]
	return stream, ok
}

// PublishStream publishes a streaming message
func (b *StreamingMessageBus) PublishStream(ctx context.Context, msg *StreamMessage) error {
	b.streamMu.RLock()
	defer b.streamMu.RUnlock()

	// Set ID if not set
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}

	// Get the stream
	stream, ok := b.streamStreams[msg.ChatID]
	if !ok {
		return nil // No stream for this chat
	}

	// Publish to stream
	select {
	case stream <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CloseStream closes a stream for a chat
func (b *StreamingMessageBus) CloseStream(chatID string) {
	b.streamMu.Lock()
	defer b.streamMu.Unlock()

	if stream, ok := b.streamStreams[chatID]; ok {
		close(stream)
		delete(b.streamStreams, chatID)
	}
}

// StreamHandler handles streaming messages
type StreamHandler struct {
	bus        *StreamingMessageBus
	chatID     string
	stream     chan *StreamMessage
	content    strings.Builder
	thinking   strings.Builder
	final      strings.Builder
	chunkIndex int
	mu         sync.Mutex
	onChunk    func(*StreamMessage)
	onComplete func(string)
	onError    func(error)
}

// NewStreamHandler creates a new stream handler
func NewStreamHandler(bus *StreamingMessageBus, chatID string) *StreamHandler {
	stream, ok := bus.GetStream(chatID)
	if !ok {
		stream = bus.CreateStream(chatID)
	}

	return &StreamHandler{
		bus:    bus,
		chatID: chatID,
		stream: stream,
	}
}

// OnChunk sets the chunk callback
func (h *StreamHandler) OnChunk(callback func(*StreamMessage)) *StreamHandler {
	h.onChunk = callback
	return h
}

// OnComplete sets the complete callback
func (h *StreamHandler) OnComplete(callback func(string)) *StreamHandler {
	h.onComplete = callback
	return h
}

// OnError sets the error callback
func (h *StreamHandler) OnError(callback func(error)) *StreamHandler {
	h.onError = callback
	return h
}

// Start starts handling streaming messages
func (h *StreamHandler) Start(ctx context.Context) {
	go h.handle(ctx)
}

// handle handles streaming messages
func (h *StreamHandler) handle(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-h.stream:
			if !ok {
				return
			}
			h.processChunk(msg)
		}
	}
}

// processChunk processes a streaming chunk
func (h *StreamHandler) processChunk(msg *StreamMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if msg.Error != "" {
		if h.onError != nil {
			h.onError(fmt.Errorf("stream error: %s", msg.Error))
		}
		return
	}

	h.chunkIndex = msg.ChunkIndex

	if msg.IsThinking {
		h.thinking.WriteString(msg.Content)
	} else if msg.IsFinal {
		h.final.WriteString(msg.Content)
	} else {
		h.content.WriteString(msg.Content)
	}

	if h.onChunk != nil {
		h.onChunk(msg)
	}

	if msg.IsComplete {
		if h.onComplete != nil {
			h.onComplete(h.content.String())
		}
	}
}

// GetContent returns the accumulated content
func (h *StreamHandler) GetContent() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.content.String()
}

// GetThinking returns the accumulated thinking content
func (h *StreamHandler) GetThinking() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.thinking.String()
}

// GetFinal returns the accumulated final content
func (h *StreamHandler) GetFinal() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.final.String()
}

// GetChunkIndex returns the current chunk index
func (h *StreamHandler) GetChunkIndex() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.chunkIndex
}

// Reset resets the handler state
func (h *StreamHandler) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.content.Reset()
	h.thinking.Reset()
	h.final.Reset()
	h.chunkIndex = 0
}

// Close closes the stream handler
func (h *StreamHandler) Close() {
	h.bus.CloseStream(h.chatID)
}
