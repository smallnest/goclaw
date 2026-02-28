package acp

import (
	"sync"

	"github.com/smallnest/goclaw/channels"
)

// AcpSessionRouter implements the channels.AcpSessionRouter interface.
// This provides thread-bound session routing functionality for the ACP manager.
type AcpSessionRouter struct {
	mu                   sync.RWMutex
	threadBindingService *channels.ThreadBindingService
	manager              *Manager
}

// NewAcpSessionRouter creates a new ACP session router.
func NewAcpSessionRouter(manager *Manager) *AcpSessionRouter {
	return &AcpSessionRouter{
		manager: manager,
	}
}

// SetThreadBindingService sets the thread binding service.
func (r *AcpSessionRouter) SetThreadBindingService(service *channels.ThreadBindingService) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.threadBindingService = service
}

// RouteToAcpSession checks if there's a thread-bound ACP session for this conversation
// and returns the ACP session key if found.
func (r *AcpSessionRouter) RouteToAcpSession(channel, accountID, conversationID string) string {
	r.mu.RLock()
	service := r.threadBindingService
	r.mu.RUnlock()

	if service == nil {
		return ""
	}

	binding := service.GetByConversation(channel, accountID, conversationID)
	if binding != nil && binding.TargetKind == "session" {
		return binding.TargetSessionKey
	}

	return ""
}

// IsACPThreadBinding checks if a conversation has an active ACP thread binding.
func (r *AcpSessionRouter) IsACPThreadBinding(channel, accountID, conversationID string) bool {
	r.mu.RLock()
	service := r.threadBindingService
	r.mu.RUnlock()

	if service == nil {
		return false
	}

	binding := service.GetByConversation(channel, accountID, conversationID)
	return binding != nil && binding.TargetKind == "session"
}

// Ensure AcpSessionRouter implements channels.AcpSessionRouter
var _ channels.AcpSessionRouter = (*AcpSessionRouter)(nil)
