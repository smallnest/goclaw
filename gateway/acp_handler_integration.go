package gateway

import (
	"github.com/smallnest/goclaw/acp"
)

// registerAcpMethods 注册 ACP 方法
func (h *Handler) registerAcpMethods() {
	// Only register ACP methods if ACP manager is available
	if h.acpMgr == nil {
		return
	}

	// Type assert to *acp.Manager
	acpManager, ok := h.acpMgr.(*acp.Manager)
	if !ok {
		// Not a valid ACP manager, skip registration
		return
	}

	// Register ACP methods using the gateway registration function
	RegisterAcpMethods(h.registry, h.cfg, acpManager)
}
