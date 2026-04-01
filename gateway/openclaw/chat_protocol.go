package openclaw

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ChatMessage 聊天消息
type ChatMessage struct {
	Role      string      `json:"role"` // "user" | "assistant" | "system"
	Content   string      `json:"content"`
	Timestamp time.Time   `json:"timestamp"`
	Metadata  interface{} `json:"metadata,omitempty"`
}

// ChatAttachment 聊天附件
type ChatAttachment struct {
	Type     string      `json:"type,omitempty"`
	MimeType string      `json:"mimeType,omitempty"`
	FileName string      `json:"fileName,omitempty"`
	Content  interface{} `json:"content,omitempty"`
}

// ChatSendParams chat.send 参数
type ChatSendParams struct {
	SessionKey     string           `json:"sessionKey"`
	Message        string           `json:"message"`
	Thinking       string           `json:"thinking,omitempty"`
	Deliver        bool             `json:"deliver,omitempty"`
	Attachments    []ChatAttachment `json:"attachments,omitempty"`
	TimeoutMs      int64            `json:"timeoutMs,omitempty"`
	IdempotencyKey string           `json:"idempotencyKey"`
}

// ChatSendResponse chat.send 响应
type ChatSendResponse struct {
	RunID  string `json:"runId"`
	Status string `json:"status"` // "started" | "in_flight" | "ok" | "error"
}

// ChatHistoryParams chat.history 参数
type ChatHistoryParams struct {
	SessionKey string `json:"sessionKey"`
	Limit      int    `json:"limit,omitempty"`
}

// ChatHistoryResponse chat.history 响应
type ChatHistoryResponse struct {
	SessionKey    string        `json:"sessionKey"`
	SessionID     string        `json:"sessionId,omitempty"`
	Messages      []ChatMessage `json:"messages"`
	ThinkingLevel string        `json:"thinkingLevel,omitempty"`
	VerboseLevel  string        `json:"verboseLevel,omitempty"`
}

// ChatAbortParams chat.abort 参数
type ChatAbortParams struct {
	SessionKey string `json:"sessionKey"`
	RunID      string `json:"runId,omitempty"`
}

// ChatEvent 聊天事件（流式）
type ChatEvent struct {
	RunID        string      `json:"runId"`
	SessionKey   string      `json:"sessionKey"`
	Seq          int64       `json:"seq"`
	State        string      `json:"state"` // "delta" | "final" | "aborted" | "error"
	Message      interface{} `json:"message,omitempty"`
	ErrorMessage string      `json:"errorMessage,omitempty"`
	Usage        interface{} `json:"usage,omitempty"`
	StopReason   string      `json:"stopReason,omitempty"`
}

// ChatRunState 聊天运行状态
type ChatRunState struct {
	RunID      string
	SessionKey string
	ConnID     string
	StartedAt  time.Time
	UpdatedAt  time.Time
	State      string
	AbortCh    chan struct{}
	ResultCh   chan *ChatEvent
}

// ChatManager 聊天管理器
type ChatManager struct {
	mu          sync.RWMutex
	runs        map[string]*ChatRunState // runID -> state
	sessionRuns map[string][]string      // sessionKey -> []runID
	runSeq      int64
	eventSeq    int64
}

// NewChatManager 创建聊天管理器
func NewChatManager() *ChatManager {
	return &ChatManager{
		runs:        make(map[string]*ChatRunState),
		sessionRuns: make(map[string][]string),
		runSeq:      0,
		eventSeq:    0,
	}
}

// Send 发送聊天消息
func (cm *ChatManager) Send(params *ChatSendParams, connID string) (*ChatSendResponse, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 生成 run ID
	runID := fmt.Sprintf("run_%d", cm.runSeq)
	cm.runSeq++

	// 创建运行状态
	runState := &ChatRunState{
		RunID:      runID,
		SessionKey: params.SessionKey,
		ConnID:     connID,
		StartedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		State:      "started",
		AbortCh:    make(chan struct{}),
		ResultCh:   make(chan *ChatEvent, 10),
	}

	cm.runs[runID] = runState
	cm.sessionRuns[params.SessionKey] = append(cm.sessionRuns[params.SessionKey], runID)

	// 在实际实现中，这里会：
	// 1. 加载会话
	// 2. 解析附件
	// 3. 调用 Agent
	// 4. 流式返回结果

	return &ChatSendResponse{
		RunID:  runID,
		Status: "started",
	}, nil
}

// GetHistory 获取历史消息
func (cm *ChatManager) GetHistory(params *ChatHistoryParams) (*ChatHistoryResponse, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	// 实际实现中应该从存储中读取历史消息
	return &ChatHistoryResponse{
		SessionKey: params.SessionKey,
		Messages:   []ChatMessage{},
	}, nil
}

// Abort 中止运行
func (cm *ChatManager) Abort(params *ChatAbortParams) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var runIDs []string
	if params.RunID != "" {
		runIDs = []string{params.RunID}
	} else {
		// 中止会话的所有运行
		runIDs = cm.sessionRuns[params.SessionKey]
	}

	for _, runID := range runIDs {
		if runState, ok := cm.runs[runID]; ok {
			// 发送中止信号
			close(runState.AbortCh)
			runState.State = "aborted"

			// 发送中止事件
			event := &ChatEvent{
				RunID:      runID,
				SessionKey: params.SessionKey,
				Seq:        cm.nextSeq(),
				State:      "aborted",
			}
			select {
			case runState.ResultCh <- event:
			default:
			}
		}
	}

	return nil
}

// GetRun 获取运行状态
func (cm *ChatManager) GetRun(runID string) (*ChatRunState, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	run, ok := cm.runs[runID]
	if !ok {
		return nil, false
	}

	// 返回副本
	copy := *run
	return &copy, true
}

// RemoveRun 移除运行
func (cm *ChatManager) RemoveRun(runID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if run, ok := cm.runs[runID]; ok {
		// 从会话运行列表中移除
		sessionRuns := cm.sessionRuns[run.SessionKey]
		for i, id := range sessionRuns {
			if id == runID {
				cm.sessionRuns[run.SessionKey] = append(sessionRuns[:i], sessionRuns[i+1:]...)
				break
			}
		}

		delete(cm.runs, runID)
		close(run.AbortCh)
		close(run.ResultCh)
	}
}

// GetSessionRuns 获取会话的所有运行
func (cm *ChatManager) GetSessionRuns(sessionKey string) []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	runs := cm.sessionRuns[sessionKey]
	result := make([]string, len(runs))
	copy(result, runs)
	return result
}

// nextSeq 获取下一个序列号
func (cm *ChatManager) nextSeq() int64 {
	cm.eventSeq++
	return cm.eventSeq
}

// StreamDelta 流式发送 delta 消息
func (cm *ChatManager) StreamDelta(runID string, message interface{}) error {
	cm.mu.RLock()
	run, ok := cm.runs[runID]
	cm.mu.RUnlock()

	if !ok {
		return fmt.Errorf("run not found: %s", runID)
	}

	event := &ChatEvent{
		RunID:   runID,
		State:   "delta",
		Message: message,
		Seq:     cm.nextSeq(),
	}

	select {
	case run.ResultCh <- event:
		return nil
	case <-run.AbortCh:
		return fmt.Errorf("run aborted")
	}
}

// StreamFinal 流式发送 final 消息
func (cm *ChatManager) StreamFinal(runID string, message interface{}, usage interface{}, stopReason string) error {
	cm.mu.RLock()
	run, ok := cm.runs[runID]
	cm.mu.RUnlock()

	if !ok {
		return fmt.Errorf("run not found: %s", runID)
	}

	event := &ChatEvent{
		RunID:      runID,
		State:      "final",
		Message:    message,
		Usage:      usage,
		StopReason: stopReason,
		Seq:        cm.nextSeq(),
	}

	select {
	case run.ResultCh <- event:
		return nil
	case <-run.AbortCh:
		return fmt.Errorf("run aborted")
	}
}

// StreamError 流式发送错误消息
func (cm *ChatManager) StreamError(runID, errorMessage string) error {
	cm.mu.RLock()
	run, ok := cm.runs[runID]
	cm.mu.RUnlock()

	if !ok {
		return fmt.Errorf("run not found: %s", runID)
	}

	event := &ChatEvent{
		RunID:        runID,
		State:        "error",
		ErrorMessage: errorMessage,
		Seq:          cm.nextSeq(),
	}

	select {
	case run.ResultCh <- event:
		return nil
	default:
		return fmt.Errorf("failed to send error event")
	}
}

// IsAborted 检查是否已中止
func (cm *ChatManager) IsAborted(runID string) bool {
	cm.mu.RLock()
	run, ok := cm.runs[runID]
	cm.mu.RUnlock()

	if !ok {
		return true
	}

	select {
	case <-run.AbortCh:
		return true
	default:
		return false
	}
}

// RegisterChatMethods 注册聊天方法
func RegisterChatMethods(mh *MessageHandler, chatMgr *ChatManager) {
	// chat.send
	mh.Register("chat.send", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params ChatSendParams
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.SessionKey == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "sessionKey is required")
		}

		if params.Message == "" && len(params.Attachments) == 0 {
			return nil, NewErrorInfo(ErrorInvalidParams, "message or attachments is required")
		}

		if params.IdempotencyKey == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "idempotencyKey is required")
		}

		resp, err := chatMgr.Send(&params, conn.ID())
		if err != nil {
			return nil, NewErrorInfo(ErrorInternalError, err.Error())
		}

		return resp, nil
	})

	// chat.history
	mh.Register("chat.history", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params ChatHistoryParams
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.SessionKey == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "sessionKey is required")
		}

		resp, err := chatMgr.GetHistory(&params)
		if err != nil {
			return nil, NewErrorInfo(ErrorInternalError, err.Error())
		}

		return resp, nil
	})

	// chat.abort
	mh.Register("chat.abort", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params ChatAbortParams
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.SessionKey == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "sessionKey is required")
		}

		if err := chatMgr.Abort(&params); err != nil {
			return nil, NewErrorInfo(ErrorInternalError, err.Error())
		}

		return map[string]interface{}{
			"status": "aborted",
		}, nil
	})
}

// ParseChatEventFromJSON 从 JSON 解析聊天事件
func ParseChatEventFromJSON(data []byte) (*ChatEvent, error) {
	var event ChatEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("failed to parse chat event: %w", err)
	}
	return &event, nil
}
