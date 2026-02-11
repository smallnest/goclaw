package bus

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"go.uber.org/zap"
)

// MessageBus 消息总线
type MessageBus struct {
	inbound   chan *InboundMessage
	outbound  chan *OutboundMessage
	outSubs   map[string]chan *OutboundMessage
	outSubsMu sync.RWMutex
	mu        sync.RWMutex
	closed    bool
}

// NewMessageBus 创建消息总线
func NewMessageBus(bufferSize int) *MessageBus {
	b := &MessageBus{
		inbound:  make(chan *InboundMessage, bufferSize),
		outbound: make(chan *OutboundMessage, bufferSize),
		outSubs:  make(map[string]chan *OutboundMessage),
		closed:   false,
	}
	// 启动广播 goroutine
	go b.broadcastOutbound()
	return b
}

// PublishInbound 发布入站消息
func (b *MessageBus) PublishInbound(ctx context.Context, msg *InboundMessage) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return ErrBusClosed
	}

	// 设置ID和时间戳
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	select {
	case b.inbound <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ConsumeInbound 消费入站消息
func (b *MessageBus) ConsumeInbound(ctx context.Context) (*InboundMessage, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return nil, ErrBusClosed
	}

	select {
	case msg := <-b.inbound:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// PublishOutbound 发布出站消息
func (b *MessageBus) PublishOutbound(ctx context.Context, msg *OutboundMessage) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		logger.Warn("Message bus is closed, cannot publish outbound")
		return ErrBusClosed
	}

	// 设置ID和时间戳
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	logger.Info("Publishing outbound message to bus",
		zap.String("id", msg.ID),
		zap.String("channel", msg.Channel),
		zap.String("chat_id", msg.ChatID),
		zap.Int("content_length", len(msg.Content)),
		zap.Int("outbound_queue_size", len(b.outbound)))

	select {
	case b.outbound <- msg:
		logger.Info("Outbound message published successfully",
			zap.String("id", msg.ID),
			zap.Int("outbound_queue_size", len(b.outbound)))
		return nil
	case <-ctx.Done():
		logger.Warn("PublishOutbound context cancelled",
			zap.String("id", msg.ID))
		return ctx.Err()
	}
}

// ConsumeOutbound 消费出站消息
func (b *MessageBus) ConsumeOutbound(ctx context.Context) (*OutboundMessage, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		logger.Warn("Message bus is closed, cannot consume outbound")
		return nil, ErrBusClosed
	}

	select {
	case msg := <-b.outbound:
		logger.Debug("Outbound message consumed from bus",
			zap.String("id", msg.ID),
			zap.String("channel", msg.Channel),
			zap.String("chat_id", msg.ChatID),
			zap.Int("content_length", len(msg.Content)),
			zap.Int("outbound_queue_size", len(b.outbound)))
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close 关闭消息总线
func (b *MessageBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true

	// 关闭所有订阅者的 channel
	b.outSubsMu.Lock()
	for _, ch := range b.outSubs {
		close(ch)
	}
	// 清空 map
	for k := range b.outSubs {
		delete(b.outSubs, k)
	}
	b.outSubsMu.Unlock()

	close(b.inbound)
	close(b.outbound)

	return nil
}

// IsClosed 检查是否已关闭
func (b *MessageBus) IsClosed() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.closed
}

// InboundCount 获取入站消息数量
func (b *MessageBus) InboundCount() int {
	return len(b.inbound)
}

// OutboundCount 获取出站消息数量
func (b *MessageBus) OutboundCount() int {
	return len(b.outbound)
}

// OutboundSubscription 出站消息订阅
type OutboundSubscription struct {
	ID      string
	Channel <-chan *OutboundMessage
	bus     *MessageBus
}

// Unsubscribe 取消订阅
func (s *OutboundSubscription) Unsubscribe() {
	if s == nil || s.bus == nil {
		return
	}
	s.bus.UnsubscribeOutbound(s.ID)
}

// SubscribeOutbound 订阅出站消息（支持多个消费者）
// 返回一个 OutboundSubscription 对象，包含只读 channel 和取消订阅方法
func (b *MessageBus) SubscribeOutbound() *OutboundSubscription {
	b.outSubsMu.Lock()
	defer b.outSubsMu.Unlock()

	subID := uuid.New().String()
	ch := make(chan *OutboundMessage, 100) // 每个订阅者有独立的缓冲
	b.outSubs[subID] = ch

	logger.Info("New outbound subscriber",
		zap.String("subscription_id", subID),
		zap.Int("total_subscribers", len(b.outSubs)))

	return &OutboundSubscription{
		ID:      subID,
		Channel: ch,
		bus:     b,
	}
}

// UnsubscribeOutbound 取消订阅出站消息
func (b *MessageBus) UnsubscribeOutbound(subID string) {
	b.outSubsMu.Lock()
	defer b.outSubsMu.Unlock()

	ch, ok := b.outSubs[subID]
	if ok {
		delete(b.outSubs, subID)
		close(ch)

		logger.Info("Outbound subscriber removed",
			zap.String("subscription_id", subID),
			zap.Int("remaining_subscribers", len(b.outSubs)))
	}
}

// broadcastOutbound 广播出站消息到所有订阅者
func (b *MessageBus) broadcastOutbound() {
	logger.Info("Outbound broadcaster started, waiting for messages...")
	for msg := range b.outbound {
		b.outSubsMu.RLock()
		subCount := len(b.outSubs)
		b.outSubsMu.RUnlock()

		logger.Info("Broadcasting outbound message",
			zap.Int("subscribers", subCount),
			zap.Int("msg_content_length", len(msg.Content)))

		if subCount == 0 {
			logger.Warn("No subscribers for outbound message, dropping it")
			continue
		}

		// 转发到所有订阅者
		b.outSubsMu.RLock()
		sentCount := 0
		for subID, ch := range b.outSubs {
			// 非阻塞发送，避免一个慢订阅者阻塞其他订阅者
			select {
			case ch <- msg:
				sentCount++
				logger.Debug("Message sent to subscriber",
					zap.String("subscription_id", subID))
			default:
				logger.Warn("Subscriber channel full, message dropped",
					zap.String("subscription_id", subID),
					zap.Int("queue_len", len(ch)))
			}
		}
		b.outSubsMu.RUnlock()

		logger.Info("Broadcast completed",
			zap.Int("sent_to", sentCount),
			zap.Int("total_subscribers", subCount))
	}

	logger.Info("Outbound broadcaster stopped")
}

// OutboundChan 获取出站消息通道（用于直接 select）
// 注意：如果需要多个消费者，请使用 SubscribeOutbound 代替
func (b *MessageBus) OutboundChan() <-chan *OutboundMessage {
	return b.outbound
}

// Errors
var (
	ErrBusClosed = &BusError{Message: "message bus is closed"}
)

// BusError 总线错误
type BusError struct {
	Message string
}

func (e *BusError) Error() string {
	return e.Message
}
