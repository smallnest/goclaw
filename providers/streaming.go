package providers

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// StreamChunk represents a chunk of streaming response
type StreamChunk struct {
	Content     string    `json:"content"`
	Done        bool      `json:"done"`
	ToolCall    *ToolCall `json:"tool_call,omitempty"`
	ThinkingTag string    `json:"thinking,omitempty"`
	IsThinking  bool      `json:"is_thinking,omitempty"`
	IsFinal     bool      `json:"is_final,omitempty"`
	Error       error     `json:"error,omitempty"`
}

// StreamCallback is called for each chunk in a streaming response
type StreamCallback func(chunk StreamChunk)

// StreamingProvider extends Provider with streaming support
type StreamingProvider interface {
	Provider

	// ChatStream chat with streaming response
	ChatStream(ctx context.Context, messages []Message, tools []ToolDefinition, callback StreamCallback, options ...ChatOption) error
}

// StreamBuffer manages streaming chunks and provides buffer management
type StreamBuffer struct {
	mu       sync.Mutex
	chunks   []StreamChunk
	content  strings.Builder
	thinking strings.Builder
	final    strings.Builder
	done     bool
	maxSize  int
}

// NewStreamBuffer creates a new stream buffer
func NewStreamBuffer(maxSize int) *StreamBuffer {
	if maxSize <= 0 {
		maxSize = 100000 // Default max buffer size
	}
	return &StreamBuffer{
		chunks:  make([]StreamChunk, 0),
		maxSize: maxSize,
	}
}

// Add adds a chunk to the buffer
func (b *StreamBuffer) Add(chunk StreamChunk) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.done {
		return fmt.Errorf("stream is already complete")
	}

	// Check buffer size
	if b.content.Len()+b.thinking.Len()+b.final.Len() > b.maxSize {
		return fmt.Errorf("buffer size exceeded")
	}

	b.chunks = append(b.chunks, chunk)

	// Add content to appropriate buffer
	if chunk.IsThinking {
		b.thinking.WriteString(chunk.Content)
	} else if chunk.IsFinal {
		b.final.WriteString(chunk.Content)
	} else {
		b.content.WriteString(chunk.Content)
	}

	if chunk.Done {
		b.done = true
	}

	return nil
}

// GetContent returns the accumulated content
func (b *StreamBuffer) GetContent() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.content.String()
}

// GetThinking returns the accumulated thinking content
func (b *StreamBuffer) GetThinking() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.thinking.String()
}

// GetFinal returns the accumulated final content
func (b *StreamBuffer) GetFinal() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.final.String()
}

// IsDone returns whether the stream is complete
func (b *StreamBuffer) IsDone() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.done
}

// GetChunks returns all chunks
func (b *StreamBuffer) GetChunks() []StreamChunk {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]StreamChunk{}, b.chunks...)
}

// Clear clears the buffer
func (b *StreamBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.chunks = make([]StreamChunk, 0)
	b.content.Reset()
	b.thinking.Reset()
	b.final.Reset()
	b.done = false
}

// ThinkingParser handles parsing of thinking tags in streaming responses
type ThinkingParser struct {
	mu         sync.Mutex
	inThinking bool
	inFinal    bool
}

// NewThinkingParser creates a new thinking parser
func NewThinkingParser() *ThinkingParser {
	return &ThinkingParser{}
}

// Parse parses a chunk and separates thinking, final, and regular content
func (p *ThinkingParser) Parse(content string) []StreamChunk {
	p.mu.Lock()
	defer p.mu.Unlock()

	chunks := make([]StreamChunk, 0)

	for content != "" {
		// Check for thinking tag start
		if !p.inThinking && !p.inFinal && strings.HasPrefix(content, "<thinking>") {
			p.inThinking = true
			content = content[10:]
			continue
		}

		// Check for thinking tag end
		if p.inThinking && strings.HasPrefix(content, "</thinking>") {
			p.inThinking = false
			content = content[11:]
			continue
		}

		// Check for final tag start
		if !p.inFinal && !p.inThinking && strings.HasPrefix(content, "<final>") {
			p.inFinal = true
			content = content[8:]
			continue
		}

		// Check for final tag end
		if p.inFinal && strings.HasPrefix(content, "</final>") {
			p.inFinal = false
			content = content[9:]
			continue
		}

		// Find the next tag or end of content
		nextTag := -1
		indices := []int{
			strings.Index(content, "<thinking>"),
			strings.Index(content, "</thinking>"),
			strings.Index(content, "<final>"),
			strings.Index(content, "</final>"),
		}

		for _, idx := range indices {
			if idx != -1 && (nextTag == -1 || idx < nextTag) {
				nextTag = idx
			}
		}

		var chunkContent string
		if nextTag == -1 {
			chunkContent = content
			content = ""
		} else {
			chunkContent = content[:nextTag]
			content = content[nextTag:]
		}

		if chunkContent != "" {
			if p.inThinking {
				chunks = append(chunks, StreamChunk{
					Content:    chunkContent,
					IsThinking: true,
				})
			} else if p.inFinal {
				chunks = append(chunks, StreamChunk{
					Content: chunkContent,
					IsFinal: true,
				})
			} else {
				chunks = append(chunks, StreamChunk{
					Content: chunkContent,
				})
			}
		}
	}

	return chunks
}

// IsInThinking returns whether parser is currently in thinking tag
func (p *ThinkingParser) IsInThinking() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.inThinking
}

// IsInFinal returns whether parser is currently in final tag
func (p *ThinkingParser) IsInFinal() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.inFinal
}

// Reset resets the parser state
func (p *ThinkingParser) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inThinking = false
	p.inFinal = false
}

// StreamingAdapter wraps a Provider to add streaming capabilities
type StreamingAdapter struct {
	provider Provider
}

// NewStreamingAdapter creates a new streaming adapter
func NewStreamingAdapter(provider Provider) *StreamingAdapter {
	return &StreamingAdapter{provider: provider}
}

// ChatStream implements streaming chat
func (a *StreamingAdapter) ChatStream(ctx context.Context, messages []Message, tools []ToolDefinition, callback StreamCallback, options ...ChatOption) error {
	// Check if provider natively supports streaming
	if sp, ok := a.provider.(StreamingProvider); ok {
		return sp.ChatStream(ctx, messages, tools, callback, options...)
	}

	// Fall back to non-streaming and simulate streaming
	opts := &ChatOptions{}
	for _, opt := range options {
		opt(opts)
	}

	// Add stream option
	opts.Stream = false

	// Make the call
	resp, err := a.provider.Chat(ctx, messages, tools, options...)
	if err != nil {
		callback(StreamChunk{
			Error: err,
			Done:  true,
		})
		return err
	}

	// Parse response for thinking tags
	parser := NewThinkingParser()
	chunks := parser.Parse(resp.Content)

	// Send chunks
	for i, chunk := range chunks {
		chunk.Done = (i == len(chunks)-1)
		callback(chunk)
	}

	// Send tool calls if any
	if len(resp.ToolCalls) > 0 {
		for _, tc := range resp.ToolCalls {
			callback(StreamChunk{
				ToolCall: &tc,
				Done:     true,
			})
		}
	}

	return nil
}

// ConvertToStreaming converts a streaming response chunks to a regular Response
func ConvertToStreaming(chunks []StreamChunk) *Response {
	var content strings.Builder
	var thinking strings.Builder
	var final strings.Builder
	var toolCalls []ToolCall

	for _, chunk := range chunks {
		if chunk.Error != nil {
			continue
		}
		if chunk.IsThinking {
			thinking.WriteString(chunk.Content)
		} else if chunk.IsFinal {
			final.WriteString(chunk.Content)
		} else if chunk.ToolCall != nil {
			toolCalls = append(toolCalls, *chunk.ToolCall)
		} else {
			content.WriteString(chunk.Content)
		}
	}

	return &Response{
		Content:      content.String(),
		ToolCalls:    toolCalls,
		FinishReason: "stop",
	}
}

// StreamProcessor processes streaming chunks with custom handlers
type StreamProcessor struct {
	onContent  func(content string)
	onThinking func(thinking string)
	onFinal    func(final string)
	onToolCall func(toolCall ToolCall)
	onError    func(err error)
	onComplete func()
	buffer     *StreamBuffer
	parser     *ThinkingParser
}

// NewStreamProcessor creates a new stream processor
func NewStreamProcessor() *StreamProcessor {
	return &StreamProcessor{
		buffer: NewStreamBuffer(100000),
		parser: NewThinkingParser(),
	}
}

// OnContent sets handler for content chunks
func (p *StreamProcessor) OnContent(handler func(content string)) *StreamProcessor {
	p.onContent = handler
	return p
}

// OnThinking sets handler for thinking chunks
func (p *StreamProcessor) OnThinking(handler func(thinking string)) *StreamProcessor {
	p.onThinking = handler
	return p
}

// OnFinal sets handler for final chunks
func (p *StreamProcessor) OnFinal(handler func(final string)) *StreamProcessor {
	p.onFinal = handler
	return p
}

// OnToolCall sets handler for tool call chunks
func (p *StreamProcessor) OnToolCall(handler func(toolCall ToolCall)) *StreamProcessor {
	p.onToolCall = handler
	return p
}

// OnError sets handler for errors
func (p *StreamProcessor) OnError(handler func(err error)) *StreamProcessor {
	p.onError = handler
	return p
}

// OnComplete sets handler for stream completion
func (p *StreamProcessor) OnComplete(handler func()) *StreamProcessor {
	p.onComplete = handler
	return p
}

// Process processes a chunk
func (p *StreamProcessor) Process(chunk StreamChunk) error {
	// Add to buffer
	if err := p.buffer.Add(chunk); err != nil {
		if p.onError != nil {
			p.onError(err)
		}
		return err
	}

	// Handle error
	if chunk.Error != nil {
		if p.onError != nil {
			p.onError(chunk.Error)
		}
		return chunk.Error
	}

	// Handle content based on type
	if chunk.ToolCall != nil {
		if p.onToolCall != nil {
			p.onToolCall(*chunk.ToolCall)
		}
	} else if chunk.IsThinking {
		if p.onThinking != nil {
			p.onThinking(chunk.Content)
		}
	} else if chunk.IsFinal {
		if p.onFinal != nil {
			p.onFinal(chunk.Content)
		}
	} else if chunk.Content != "" {
		if p.onContent != nil {
			p.onContent(chunk.Content)
		}
	}

	// Handle completion
	if chunk.Done {
		if p.onComplete != nil {
			p.onComplete()
		}
	}

	return nil
}

// GetBuffer returns the buffer
func (p *StreamProcessor) GetBuffer() *StreamBuffer {
	return p.buffer
}

// Reset resets the processor
func (p *StreamProcessor) Reset() {
	p.buffer.Clear()
	p.parser.Reset()
}
