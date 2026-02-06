package channels

import (
	"context"
	"fmt"
	"sync"

	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/config"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"go.uber.org/zap"
)

// Manager 通道管理器
type Manager struct {
	channels map[string]BaseChannel
	bus      *bus.MessageBus
	mu       sync.RWMutex
}

// NewManager 创建通道管理器
func NewManager(bus *bus.MessageBus) *Manager {
	return &Manager{
		channels: make(map[string]BaseChannel),
		bus:      bus,
	}
}

// Register 注册通道
func (m *Manager) Register(channel BaseChannel) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := channel.Name()
	if _, ok := m.channels[name]; ok {
		return fmt.Errorf("channel %s already registered", name)
	}

	m.channels[name] = channel
	logger.Info("Channel registered", zap.String("channel", name))
	return nil
}

// Start 启动所有通道
func (m *Manager) Start(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, channel := range m.channels {
		logger.Info("Starting channel", zap.String("channel", name))
		if err := channel.Start(ctx); err != nil {
			logger.Error("Failed to start channel",
				zap.String("channel", name),
				zap.Error(err),
			)
			continue
		}
	}

	return nil
}

// Stop 停止所有通道
func (m *Manager) Stop() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errors []error
	for name, channel := range m.channels {
		if err := channel.Stop(); err != nil {
			logger.Error("Failed to stop channel",
				zap.String("channel", name),
				zap.Error(err),
			)
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to stop some channels: %d errors", len(errors))
	}

	return nil
}

// Get 获取通道
func (m *Manager) Get(name string) (BaseChannel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channel, ok := m.channels[name]
	return channel, ok
}

// DispatchOutbound 分发出站消息
func (m *Manager) DispatchOutbound(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			msg, err := m.bus.ConsumeOutbound(ctx)
			if err != nil {
				continue
			}

			// 查找对应的通道
			channel, ok := m.Get(msg.Channel)
			if !ok {
				logger.Warn("Channel not found for outbound message",
					zap.String("channel", msg.Channel),
				)
				continue
			}

			// 发送消息
			if err := channel.Send(msg); err != nil {
				logger.Error("Failed to send message via channel",
					zap.String("channel", msg.Channel),
					zap.Error(err),
				)
			}
		}
	}
}

// SetupFromConfig 从配置设置通道
func (m *Manager) SetupFromConfig(cfg *config.Config) error {
	// Telegram 通道
	if cfg.Channels.Telegram.Enabled && cfg.Channels.Telegram.Token != "" {
		tgCfg := TelegramConfig{
			BaseChannelConfig: BaseChannelConfig{
				Enabled:    cfg.Channels.Telegram.Enabled,
				AllowedIDs: cfg.Channels.Telegram.AllowedIDs,
			},
			Token: cfg.Channels.Telegram.Token,
		}

		channel, err := NewTelegramChannel(tgCfg, m.bus)
		if err != nil {
			logger.Error("Failed to create Telegram channel", zap.Error(err))
		} else {
			if err := m.Register(channel); err != nil {
				logger.Error("Failed to register Telegram channel", zap.Error(err))
			}
		}
	}

	// WhatsApp 通道
	if cfg.Channels.WhatsApp.Enabled && cfg.Channels.WhatsApp.BridgeURL != "" {
		waCfg := WhatsAppConfig{
			BaseChannelConfig: BaseChannelConfig{
				Enabled:    cfg.Channels.WhatsApp.Enabled,
				AllowedIDs: cfg.Channels.WhatsApp.AllowedIDs,
			},
			BridgeURL: cfg.Channels.WhatsApp.BridgeURL,
		}

		channel, err := NewWhatsAppChannel(waCfg, m.bus)
		if err != nil {
			logger.Error("Failed to create WhatsApp channel", zap.Error(err))
		} else {
			if err := m.Register(channel); err != nil {
				logger.Error("Failed to register WhatsApp channel", zap.Error(err))
			}
		}
	}

	// 飞书通道
	if cfg.Channels.Feishu.Enabled && cfg.Channels.Feishu.AppID != "" {
		channel, err := NewFeishuChannel(cfg.Channels.Feishu, m.bus)
		if err != nil {
			logger.Error("Failed to create Feishu channel", zap.Error(err))
		} else {
			if err := m.Register(channel); err != nil {
				logger.Error("Failed to register Feishu channel", zap.Error(err))
			}
		}
	}

	// QQ 通道 (使用官方 API)
	if cfg.Channels.QQ.Enabled {
		if cfg.Channels.QQ.AppID != "" {
			channel, err := NewQQChannel(cfg.Channels.QQ, m.bus)
			if err != nil {
				logger.Error("Failed to create QQ channel", zap.Error(err))
			} else {
				if err := m.Register(channel); err != nil {
					logger.Error("Failed to register QQ channel", zap.Error(err))
				}
			}
		} else {
			logger.Warn("QQ channel enabled but app_id not configured")
		}
	}

	// 企业微信通道
	if cfg.Channels.WeWork.Enabled && cfg.Channels.WeWork.CorpID != "" {
		channel, err := NewWeWorkChannel(cfg.Channels.WeWork, m.bus)
		if err != nil {
			logger.Error("Failed to create WeWork channel", zap.Error(err))
		} else {
			if err := m.Register(channel); err != nil {
				logger.Error("Failed to register WeWork channel", zap.Error(err))
			}
		}
	}

	return nil
}
