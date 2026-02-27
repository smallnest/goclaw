package channels

import (
	"strings"
)

// CapabilityType represents a channel capability
type CapabilityType string

const (
	// CapabilityReactions - add/remove reactions to messages
	CapabilityReactions CapabilityType = "reactions"
	// CapabilityInlineButtons - send messages with inline buttons
	CapabilityInlineButtons CapabilityType = "inline_buttons"
	// CapabilityThreads - create and manage message threads
	CapabilityThreads CapabilityType = "threads"
	// CapabilityPolls - create and manage polls
	CapabilityPolls CapabilityType = "polls"
	// CapabilityStreaming - stream message responses
	CapabilityStreaming CapabilityType = "streaming"
	// CapabilityMedia - send/receive media files
	CapabilityMedia CapabilityType = "media"
	// CapabilityNativeCommands - native slash commands
	CapabilityNativeCommands CapabilityType = "native_commands"
)

// CapabilityScope defines where a capability is available
type CapabilityScope string

const (
	CapabilityScopeOff      CapabilityScope = "off"      // disabled
	CapabilityScopeDM       CapabilityScope = "dm"       // direct messages only
	CapabilityScopeGroup    CapabilityScope = "group"    // groups only
	CapabilityScopeAll      CapabilityScope = "all"      // everywhere
	CapabilityScopeAllowlist CapabilityScope = "allowlist" // whitelisted chats only
)

// ChannelCapabilities defines what features a channel supports
type ChannelCapabilities struct {
	// Reactions support
	Reactions CapabilityScope `mapstructure:"reactions" json:"reactions"`
	// Inline buttons support
	InlineButtons CapabilityScope `mapstructure:"inline_buttons" json:"inline_buttons"`
	// Thread support
	Threads CapabilityScope `mapstructure:"threads" json:"threads"`
	// Poll support
	Polls CapabilityScope `mapstructure:"polls" json:"polls"`
	// Streaming support
	Streaming CapabilityScope `mapstructure:"streaming" json:"streaming"`
	// Media support
	Media CapabilityScope `mapstructure:"media" json:"media"`
	// Native slash commands
	NativeCommands CapabilityScope `mapstructure:"native_commands" json:"native_commands"`
}

// DefaultCapabilities returns default capabilities for a channel
func DefaultCapabilities() ChannelCapabilities {
	return ChannelCapabilities{
		Reactions:      CapabilityScopeAll,
		InlineButtons:  CapabilityScopeOff,
		Threads:         CapabilityScopeOff,
		Polls:           CapabilityScopeOff,
		Streaming:       CapabilityScopeOff,
		Media:           CapabilityScopeAll,
		NativeCommands:  CapabilityScopeAll,
	}
}

// ParseCapabilityScope parses a capability scope from string
func ParseCapabilityScope(s string) CapabilityScope {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "off":
		return CapabilityScopeOff
	case "dm":
		return CapabilityScopeDM
	case "group":
		return CapabilityScopeGroup
	case "all":
		return CapabilityScopeAll
	case "allowlist":
		return CapabilityScopeAllowlist
	default:
		return CapabilityScopeOff
	}
}

// IsCapabilityEnabled checks if a capability is enabled for the given context
func IsCapabilityEnabled(capabilities ChannelCapabilities, capability CapabilityType, scope CapabilityScope, isWhitelisted bool) bool {
	var enabled CapabilityScope

	switch capability {
	case CapabilityReactions:
		enabled = capabilities.Reactions
	case CapabilityInlineButtons:
		enabled = capabilities.InlineButtons
	case CapabilityThreads:
		enabled = capabilities.Threads
	case CapabilityPolls:
		enabled = capabilities.Polls
	case CapabilityStreaming:
		enabled = capabilities.Streaming
	case CapabilityMedia:
		enabled = capabilities.Media
	case CapabilityNativeCommands:
		enabled = capabilities.NativeCommands
	default:
		return false
	}

	// Check if enabled at all
	if enabled == CapabilityScopeOff {
		return false
	}

	// Check scope-specific rules
	switch enabled {
	case CapabilityScopeAll:
		return true
	case CapabilityScopeDM:
		return scope == CapabilityScopeDM
	case CapabilityScopeGroup:
		return scope == CapabilityScopeGroup
	case CapabilityScopeAllowlist:
		return isWhitelisted
	default:
		return false
	}
}

// MergeCapabilities merges multiple capability configs, later configs override
func MergeCapabilities(base ChannelCapabilities, overrides []ChannelCapabilities) ChannelCapabilities {
	result := base

	for _, override := range overrides {
		if override.Reactions != "" {
			result.Reactions = ParseCapabilityScope(string(override.Reactions))
		}
		if override.InlineButtons != "" {
			result.InlineButtons = ParseCapabilityScope(string(override.InlineButtons))
		}
		if override.Threads != "" {
			result.Threads = ParseCapabilityScope(string(override.Threads))
		}
		if override.Polls != "" {
			result.Polls = ParseCapabilityScope(string(override.Polls))
		}
		if override.Streaming != "" {
			result.Streaming = ParseCapabilityScope(string(override.Streaming))
		}
		if override.Media != "" {
			result.Media = ParseCapabilityScope(string(override.Media))
		}
		if override.NativeCommands != "" {
			result.NativeCommands = ParseCapabilityScope(string(override.NativeCommands))
		}
	}

	return result
}

// GetDefaultCapabilitiesForChannel returns default capabilities for a channel type
func GetDefaultCapabilitiesForChannel(channelType string) ChannelCapabilities {
	// Define default capabilities per channel type
	switch channelType {
	case "discord":
		return ChannelCapabilities{
			Reactions:      CapabilityScopeAll,
			InlineButtons:  CapabilityScopeAll,
			Threads:         CapabilityScopeAll,
			Polls:           CapabilityScopeAll,
			Streaming:       CapabilityScopeAll,
			Media:           CapabilityScopeAll,
			NativeCommands:  CapabilityScopeAll,
		}
	case "telegram":
		return ChannelCapabilities{
			Reactions:      CapabilityScopeAll,
			InlineButtons:  CapabilityScopeDM,
			Threads:         CapabilityScopeGroup, // Topics
			Polls:           CapabilityScopeAll,
			Streaming:       CapabilityScopeDM,
			Media:           CapabilityScopeAll,
			NativeCommands:  CapabilityScopeAll,
		}
	case "slack":
		return ChannelCapabilities{
			Reactions:      CapabilityScopeAll,
			InlineButtons:  CapabilityScopeAll,
			Threads:         CapabilityScopeAll,
			Polls:           CapabilityScopeOff,
			Streaming:       CapabilityScopeOff,
			Media:           CapabilityScopeAll,
			NativeCommands:  CapabilityScopeAll,
		}
	case "whatsapp":
		return ChannelCapabilities{
			Reactions:      CapabilityScopeAll,
			InlineButtons:  CapabilityScopeOff,
			Threads:         CapabilityScopeOff,
			Polls:           CapabilityScopeAll,
			Streaming:       CapabilityScopeOff,
			Media:           CapabilityScopeAll,
			NativeCommands:  CapabilityScopeOff,
		}
	case "signal":
		return ChannelCapabilities{
			Reactions:      CapabilityScopeAll,
			InlineButtons:  CapabilityScopeOff,
			Threads:         CapabilityScopeOff,
			Polls:           CapabilityScopeOff,
			Streaming:       CapabilityScopeOff,
			Media:           CapabilityScopeAll,
			NativeCommands:  CapabilityScopeOff,
		}
	default:
		return DefaultCapabilities()
	}
}

// ChannelCapabilityConfig is used in config parsing
type ChannelCapabilityConfig struct {
	// Reactions support: "off", "dm", "group", "all", "allowlist"
	Reactions string `mapstructure:"reactions" json:"reactions"`
	// Inline buttons support
	InlineButtons string `mapstructure:"inline_buttons" json:"inline_buttons"`
	// Thread support
	Threads string `mapstructure:"threads" json:"threads"`
	// Poll support
	Polls string `mapstructure:"polls" json:"polls"`
	// Streaming support
	Streaming string `mapstructure:"streaming" json:"streaming"`
	// Media support
	Media string `mapstructure:"media" json:"media"`
	// Native commands support
	NativeCommands string `mapstructure:"native_commands" json:"native_commands"`
}

// ToCapabilities converts config to ChannelCapabilities
func ToCapabilities(cfg ChannelCapabilityConfig) ChannelCapabilities {
	return ChannelCapabilities{
		Reactions:      ParseCapabilityScope(cfg.Reactions),
		InlineButtons:  ParseCapabilityScope(cfg.InlineButtons),
		Threads:         ParseCapabilityScope(cfg.Threads),
		Polls:           ParseCapabilityScope(cfg.Polls),
		Streaming:       ParseCapabilityScope(cfg.Streaming),
		Media:           ParseCapabilityScope(cfg.Media),
		NativeCommands:  ParseCapabilityScope(cfg.NativeCommands),
	}
}

// ChatContext provides context for capability checks
type ChatContext struct {
	IsPrivateMessage bool
	IsGroupMessage   bool
	IsWhitelisted    bool
	ChatType         string // "private", "group", "supergroup", etc.
	ChatID            string
}

// NewChatContext creates a ChatContext from metadata
func NewChatContext(metadata map[string]interface{}) ChatContext {
	ctx := ChatContext{}

	// Check chat type from metadata
	if chatType, ok := metadata["chat_type"].(string); ok {
		ctx.ChatType = chatType
		ctx.IsPrivateMessage = chatType == "private"
		ctx.IsGroupMessage = chatType == "group" || chatType == "supergroup"
	}

	// Check if whitelisted
	// (This would need to be passed in from the channel's allowed list)

	return ctx
}

// CheckCapability checks if a capability is enabled in the given context
func CheckCapability(capabilities ChannelCapabilities, capability CapabilityType, ctx ChatContext) bool {
	var scope CapabilityScope

	// Determine scope from context
	switch {
	case ctx.IsPrivateMessage:
		scope = CapabilityScopeDM
	case ctx.IsGroupMessage:
		scope = CapabilityScopeGroup
	default:
		scope = CapabilityScopeAll
	}

	return IsCapabilityEnabled(capabilities, capability, scope, ctx.IsWhitelisted)
}
