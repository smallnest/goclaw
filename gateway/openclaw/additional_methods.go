package openclaw

// RegisterSessionMethods 注册 Session 方法
func RegisterSessionMethods(mh *MessageHandler) {
	// sessions.list
	mh.Register("sessions.list", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		// 实际应该从 session 管理器获取
		return map[string]interface{}{
			"sessions": []map[string]interface{}{},
			"count":    0,
		}, nil
	})

	// sessions.preview
	mh.Register("sessions.preview", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Keys []string `json:"keys"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		previews := make([]map[string]interface{}, 0)
		for _, key := range params.Keys {
			previews = append(previews, map[string]interface{}{
				"key":          key,
				"messageCount": 0,
				"createdAt":    0,
				"updatedAt":    0,
			})
		}

		return previews, nil
	})

	// sessions.patch
	mh.Register("sessions.patch", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Key      string                 `json:"key"`
			Metadata map[string]interface{} `json:"metadata,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.Key == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "key is required")
		}

		return map[string]interface{}{
			"status": "patched",
			"key":    params.Key,
		}, nil
	})

	// sessions.reset
	mh.Register("sessions.reset", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Key string `json:"key"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.Key == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "key is required")
		}

		return map[string]interface{}{
			"status": "reset",
			"key":    params.Key,
		}, nil
	})

	// sessions.delete
	mh.Register("sessions.delete", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Key string `json:"key"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.Key == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "key is required")
		}

		return map[string]interface{}{
			"status": "deleted",
			"key":    params.Key,
		}, nil
	})

	// sessions.compact
	mh.Register("sessions.compact", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Key string `json:"key"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.Key == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "key is required")
		}

		return map[string]interface{}{
			"status": "compacted",
			"key":    params.Key,
		}, nil
	})
}

// RegisterToolsSkillsMethods 注册工具和技能方法
func RegisterToolsSkillsMethods(mh *MessageHandler) {
	// tools.catalog
	mh.Register("tools.catalog", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		return map[string]interface{}{
			"tools": []map[string]interface{}{},
		}, nil
	})

	// models.list
	mh.Register("models.list", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		return map[string]interface{}{
			"models": []map[string]interface{}{
				{"id": "gpt-4", "name": "GPT-4"},
				{"id": "gpt-3.5-turbo", "name": "GPT-3.5 Turbo"},
			},
		}, nil
	})

	// skills.status
	mh.Register("skills.status", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		return map[string]interface{}{
			"enabled": true,
			"count":   0,
		}, nil
	})

	// skills.bins
	mh.Register("skills.bins", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		return map[string]interface{}{
			"bins": []string{},
		}, nil
	})

	// skills.install
	mh.Register("skills.install", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Skill string `json:"skill"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.Skill == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "skill is required")
		}

		return map[string]interface{}{
			"status": "installed",
			"skill":  params.Skill,
		}, nil
	})

	// skills.update
	mh.Register("skills.update", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Skill string `json:"skill"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.Skill == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "skill is required")
		}

		return map[string]interface{}{
			"status": "updated",
			"skill":  params.Skill,
		}, nil
	})

	// update.run
	mh.Register("update.run", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		return map[string]interface{}{
			"status": "checking",
		}, nil
	})
}

// RegisterWizardVoiceMethods 注册 Wizard 和语音方法
func RegisterWizardVoiceMethods(mh *MessageHandler) {
	// wizard.start
	mh.Register("wizard.start", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Type string                 `json:"type"`
			Args map[string]interface{} `json:"args,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		return map[string]interface{}{
			"wizardId": "wizard_001",
			"status":   "started",
			"type":     params.Type,
		}, nil
	})

	// wizard.next
	mh.Register("wizard.next", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			WizardID string                 `json:"wizardId"`
			Input    map[string]interface{} `json:"input"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.WizardID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "wizardId is required")
		}

		return map[string]interface{}{
			"status":   "next",
			"wizardId": params.WizardID,
		}, nil
	})

	// wizard.cancel
	mh.Register("wizard.cancel", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			WizardID string `json:"wizardId"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.WizardID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "wizardId is required")
		}

		return map[string]interface{}{
			"status":   "cancelled",
			"wizardId": params.WizardID,
		}, nil
	})

	// wizard.status
	mh.Register("wizard.status", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			WizardID string `json:"wizardId"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.WizardID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "wizardId is required")
		}

		return map[string]interface{}{
			"wizardId": params.WizardID,
			"status":   "running",
		}, nil
	})

	// talk.config
	mh.Register("talk.config", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Enabled *bool  `json:"enabled,omitempty"`
			Mode    string `json:"mode,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		return map[string]interface{}{
			"enabled": false,
			"mode":    "disabled",
		}, nil
	})

	// talk.mode
	mh.Register("talk.mode", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Mode string `json:"mode"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		return map[string]interface{}{
			"mode": params.Mode,
		}, nil
	})

	// voicewake.get
	mh.Register("voicewake.get", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		return map[string]interface{}{
			"enabled": false,
			"mode":    "disabled",
		}, nil
	})

	// voicewake.set
	mh.Register("voicewake.set", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Enabled bool   `json:"enabled"`
			Mode    string `json:"mode,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		return map[string]interface{}{
			"enabled": params.Enabled,
			"mode":    params.Mode,
		}, nil
	})

	// tts.status
	mh.Register("tts.status", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		return map[string]interface{}{
			"enabled":  false,
			"provider": "",
		}, nil
	})

	// tts.providers
	mh.Register("tts.providers", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		return map[string]interface{}{
			"providers": []string{},
		}, nil
	})

	// tts.enable
	mh.Register("tts.enable", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		return map[string]interface{}{
			"status": "enabled",
		}, nil
	})

	// tts.disable
	mh.Register("tts.disable", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		return map[string]interface{}{
			"status": "disabled",
		}, nil
	})

	// tts.convert
	mh.Register("tts.convert", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Text  string `json:"text"`
			Voice string `json:"voice,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.Text == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "text is required")
		}

		return map[string]interface{}{
			"status": "converted",
		}, nil
	})

	// tts.setProvider
	mh.Register("tts.setProvider", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Provider string `json:"provider"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		return map[string]interface{}{
			"provider": params.Provider,
		}, nil
	})
}

// RegisterExecApprovalMethods 注册执行批准方法
func RegisterExecApprovalMethods(mh *MessageHandler) {
	// exec.approvals.get
	mh.Register("exec.approvals.get", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		return map[string]interface{}{
			"mode": "auto",
		}, nil
	})

	// exec.approvals.set
	mh.Register("exec.approvals.set", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Mode  string                 `json:"mode"`
			Rules map[string]interface{} `json:"rules,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		return map[string]interface{}{
			"status": "set",
			"mode":   params.Mode,
		}, nil
	})

	// exec.approvals.node.get
	mh.Register("exec.approvals.node.get", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		return map[string]interface{}{
			"mode": "inherit",
		}, nil
	})

	// exec.approvals.node.set
	mh.Register("exec.approvals.node.set", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			NodeID string                 `json:"nodeId"`
			Mode   string                 `json:"mode"`
			Rules  map[string]interface{} `json:"rules,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		return map[string]interface{}{
			"status": "set",
			"nodeId": params.NodeID,
		}, nil
	})

	// exec.approval.request
	mh.Register("exec.approval.request", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Command string   `json:"command"`
			Args    []string `json:"args,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		return map[string]interface{}{
			"approvalId": "approval_001",
			"status":     "pending",
		}, nil
	})

	// exec.approval.waitDecision
	mh.Register("exec.approval.waitDecision", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			ApprovalID string `json:"approvalId"`
			TimeoutMs  int64  `json:"timeoutMs,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		return map[string]interface{}{
			"approvalId": params.ApprovalID,
			"approved":   true,
		}, nil
	})

	// exec.approval.resolve
	mh.Register("exec.approval.resolve", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			ApprovalID string `json:"approvalId"`
			Approved   bool   `json:"approved"`
			Reason     string `json:"reason,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		return map[string]interface{}{
			"status":     "resolved",
			"approvalId": params.ApprovalID,
		}, nil
	})
}

// RegisterLoggingMonitoringMethods 注册日志和监控方法
func RegisterLoggingMonitoringMethods(mh *MessageHandler) {
	// usage.status
	mh.Register("usage.status", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		return map[string]interface{}{
			"requests": 0,
			"tokens":   0,
			"cost":     0.0,
		}, nil
	})

	// usage.cost
	mh.Register("usage.cost", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		return map[string]interface{}{
			"totalCost": 0.0,
			"period":    "current",
		}, nil
	})

	// set-heartbeats
	mh.Register("set-heartbeats", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Enabled bool     `json:"enabled"`
			Events  []string `json:"events,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		return map[string]interface{}{
			"status": "set",
		}, nil
	})

	// system-event
	mh.Register("system-event", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Event   string                 `json:"event"`
			Payload map[string]interface{} `json:"payload"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		return map[string]interface{}{
			"status": "recorded",
		}, nil
	})

	// secrets.reload
	mh.Register("secrets.reload", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		return map[string]interface{}{
			"status": "reloaded",
		}, nil
	})
}

// RegisterBrowserMethods 注册浏览器方法
func RegisterBrowserMethods(mh *MessageHandler) {
	// browser.request
	mh.Register("browser.request", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Action  string                 `json:"action"`
			Options map[string]interface{} `json:"options,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.Action == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "action is required")
		}

		return map[string]interface{}{
			"status": "executed",
			"action": params.Action,
		}, nil
	})
}

// RegisterChannelsMethods 注册通道方法
func RegisterChannelsMethods(mh *MessageHandler) {
	// channels.status
	mh.Register("channels.status", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Channel string `json:"channel"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		return map[string]interface{}{
			"channel": params.Channel,
			"status":  "connected",
		}, nil
	})

	// channels.logout
	mh.Register("channels.logout", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Channel string `json:"channel"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		return map[string]interface{}{
			"status": "logged_out",
		}, nil
	})

	// send
	mh.Register("send", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Channel  string                 `json:"channel"`
			ChatID   string                 `json:"chat_id"`
			Content  string                 `json:"content"`
			Metadata map[string]interface{} `json:"metadata,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.Channel == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "channel is required")
		}

		if params.ChatID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "chat_id is required")
		}

		if params.Content == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "content is required")
		}

		return map[string]interface{}{
			"status":  "sent",
			"channel": params.Channel,
			"chatId":  params.ChatID,
		}, nil
	})
}
