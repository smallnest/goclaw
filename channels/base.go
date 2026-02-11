package channels

import (
	"context"
	"fmt"
	"strings"

	"github.com/smallnest/dogclaw/goclaw/bus"
)

// BaseChannel 通道基础接口
type BaseChannel interface {
	// Name 返回通道名称
	Name() string

	// Start 启动通道
	Start(ctx context.Context) error

	// Stop 停止通道
	Stop() error

	// Send 发送消息
	Send(msg *bus.OutboundMessage) error

	// SendStream 发送流式消息
	SendStream(chatID string, stream <-chan *bus.StreamMessage) error

	// IsAllowed 检查发送者是否允许
	IsAllowed(senderID string) bool
}

// BaseChannelConfig 通道基础配置
type BaseChannelConfig struct {
	Enabled    bool     `mapstructure:"enabled" json:"enabled"`
	AllowedIDs []string `mapstructure:"allowed_ids" json:"allowed_ids"`
}

// BaseChannelImpl 通道基础实现
type BaseChannelImpl struct {
	name     string
	config   BaseChannelConfig
	bus      *bus.MessageBus
	running  bool
	stopChan chan struct{}
}

// NewBaseChannelImpl 创建通道基础实现
func NewBaseChannelImpl(name string, config BaseChannelConfig, bus *bus.MessageBus) *BaseChannelImpl {
	return &BaseChannelImpl{
		name:     name,
		config:   config,
		bus:      bus,
		running:  false,
		stopChan: make(chan struct{}),
	}
}

// Name 返回通道名称
func (c *BaseChannelImpl) Name() string {
	return c.name
}

// Start 启动通道
func (c *BaseChannelImpl) Start(ctx context.Context) error {
	if !c.config.Enabled {
		return nil
	}

	c.running = true
	return nil
}

// Stop 停止通道
func (c *BaseChannelImpl) Stop() error {
	if !c.running {
		return nil
	}

	close(c.stopChan)
	c.running = false
	return nil
}

// IsAllowed 检查发送者是否允许
func (c *BaseChannelImpl) IsAllowed(senderID string) bool {
	if !c.config.Enabled {
		return false
	}

	// 如果没有限制列表，允许所有
	if len(c.config.AllowedIDs) == 0 {
		return true
	}

	// 检查是否在允许列表中
	for _, id := range c.config.AllowedIDs {
		if id == senderID {
			return true
		}
	}

	return false
}

// PublishInbound 发布入站消息
func (c *BaseChannelImpl) PublishInbound(ctx context.Context, msg *bus.InboundMessage) error {
	msg.Channel = c.name
	return c.bus.PublishInbound(ctx, msg)
}

// IsRunning 检查是否运行中
func (c *BaseChannelImpl) IsRunning() bool {
	return c.running
}

// WaitForStop 等待停止信号
func (c *BaseChannelImpl) WaitForStop() <-chan struct{} {
	return c.stopChan
}

// SendStream 发送流式消息 (默认实现，收集所有chunk后一次性发送)
func (c *BaseChannelImpl) SendStream(chatID string, stream <-chan *bus.StreamMessage) error {
	var content strings.Builder

	for msg := range stream {
		if msg.Error != "" {
			return fmt.Errorf("stream error: %s", msg.Error)
		}

		if !msg.IsThinking && !msg.IsFinal {
			content.WriteString(msg.Content)
		}

		if msg.IsComplete {
			// Send complete message - note: this needs actual channel implementation
			// Default implementation just collects content
			return nil
		}
	}

	return nil
}
