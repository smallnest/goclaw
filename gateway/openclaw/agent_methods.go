package openclaw

// RegisterAgentMethods 注册 Agent 方法
func RegisterAgentMethods(mh *MessageHandler) {
	// agents.list
	mh.Register("agents.list", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		// 实际应该从 agent 管理器获取
		return map[string]interface{}{
			"agents": []map[string]interface{}{},
			"count":  0,
		}, nil
	})

	// agents.create
	mh.Register("agents.create", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			ID           string                 `json:"id"`
			Name         string                 `json:"name"`
			Instructions string                 `json:"instructions"`
			Model        string                 `json:"model"`
			Metadata     map[string]interface{} `json:"metadata,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.ID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "id is required")
		}

		// 实际应该创建 agent
		return map[string]interface{}{
			"id":     params.ID,
			"status": "created",
		}, nil
	})

	// agents.update
	mh.Register("agents.update", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			ID           string                 `json:"id"`
			Name         string                 `json:"name,omitempty"`
			Instructions string                 `json:"instructions,omitempty"`
			Model        string                 `json:"model,omitempty"`
			Metadata     map[string]interface{} `json:"metadata,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.ID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "id is required")
		}

		// 实际应该更新 agent
		return map[string]interface{}{
			"id":     params.ID,
			"status": "updated",
		}, nil
	})

	// agents.delete
	mh.Register("agents.delete", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			ID string `json:"id"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.ID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "id is required")
		}

		// 实际应该删除 agent
		return map[string]interface{}{
			"id":     params.ID,
			"status": "deleted",
		}, nil
	})

	// agents.files.list
	mh.Register("agents.files.list", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			AgentID string `json:"agentId"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.AgentID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "agentId is required")
		}

		return map[string]interface{}{
			"files": []string{},
			"count": 0,
		}, nil
	})

	// agents.files.get
	mh.Register("agents.files.get", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			AgentID string `json:"agentId"`
			File    string `json:"file"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.AgentID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "agentId is required")
		}

		if params.File == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "file is required")
		}

		return map[string]interface{}{
			"content": "",
		}, nil
	})

	// agents.files.set
	mh.Register("agents.files.set", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			AgentID string `json:"agentId"`
			File    string `json:"file"`
			Content string `json:"content"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.AgentID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "agentId is required")
		}

		if params.File == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "file is required")
		}

		return map[string]interface{}{
			"status": "set",
		}, nil
	})

	// agent
	mh.Register("agent", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Content string `json:"content"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.Content == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "content is required")
		}

		return map[string]interface{}{
			"status": "queued",
		}, nil
	})

	// agent.identity.get
	mh.Register("agent.identity.get", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		return map[string]interface{}{
			"id":      "default",
			"name":    "Default Agent",
			"version": "1.0.0",
		}, nil
	})

	// agent.wait
	mh.Register("agent.wait", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Content string `json:"content"`
			Timeout int64  `json:"timeoutMs,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.Content == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "content is required")
		}

		return map[string]interface{}{
			"status":  "waiting",
			"timeout": params.Timeout,
		}, nil
	})
}
