package openclaw

import (
	"encoding/json"
	"fmt"
)

// ProtocolVersion OpenClaw 协议版本
const ProtocolVersion = 2024_09_01

// FrameType 帧类型
type FrameType string

const (
	FrameTypeRequest  FrameType = "req"
	FrameTypeResponse FrameType = "res"
	FrameTypeEvent    FrameType = "event"
	FrameTypeError    FrameType = "error"
	FrameTypeHelloOK  FrameType = "hello-ok"
)

// Frame 通用帧接口
type Frame interface {
	Type() FrameType
}

// Request 请求帧
type Request struct {
	TypeVal FrameType       `json:"type"`
	ID      string          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Type 实现 Frame 接口
func (r *Request) Type() FrameType {
	return r.TypeVal
}

// Response 响应帧
type Response struct {
	TypeVal FrameType       `json:"type"`
	ID      string          `json:"id"`
	OK      bool            `json:"ok"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *ErrorDetail    `json:"error,omitempty"`
}

// Type 实现 Frame 接口
func (r *Response) Type() FrameType {
	return r.TypeVal
}

// ErrorDetail 错误详情
type ErrorDetail struct {
	Code         string          `json:"code"`
	Message      string          `json:"message"`
	Details      json.RawMessage `json:"details,omitempty"`
	Retryable    bool            `json:"retryable,omitempty"`
	RetryAfterMs int64           `json:"retryAfterMs,omitempty"`
}

// Event 事件帧
type Event struct {
	TypeVal      FrameType       `json:"type"`
	Event        string          `json:"event"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	Seq          int64           `json:"seq,omitempty"`
	StateVersion *StateVersion   `json:"stateVersion,omitempty"`
}

// Type 实现 Frame 接口
func (e *Event) Type() FrameType {
	return e.TypeVal
}

// StateVersion 状态版本
type StateVersion struct {
	Presence *int64 `json:"presence,omitempty"`
	Health   *int64 `json:"health,omitempty"`
}

// HelloOK hello-ok 响应帧
type HelloOK struct {
	TypeVal       FrameType           `json:"type"`
	Protocol      int                 `json:"protocol"`
	Server        ServerInfo          `json:"server"`
	Features      Features            `json:"features"`
	Snapshot      json.RawMessage     `json:"snapshot"`
	CanvasHostURL string              `json:"canvasHostUrl,omitempty"`
	Auth          *AuthInfo           `json:"auth,omitempty"`
	Policy        *HelloConnectPolicy `json:"policy,omitempty"`
}

// Type 实现 Frame 接口
func (h *HelloOK) Type() FrameType {
	return h.TypeVal
}

// ServerInfo 服务器信息
type ServerInfo struct {
	Version string `json:"version"`
	ConnID  string `json:"connId"`
}

// Features 功能列表
type Features struct {
	Methods []string `json:"methods"`
	Events  []string `json:"events"`
}

// AuthInfo 认证信息
type AuthInfo struct {
	DeviceToken string   `json:"deviceToken"`
	Role        string   `json:"role"`
	Scopes      []string `json:"scopes"`
}

// HelloConnectPolicy hello-ok 中的连接策略
type HelloConnectPolicy struct {
	MaxPayload       int64 `json:"maxPayload"`
	MaxBufferedBytes int64 `json:"maxBufferedBytes"`
	TickIntervalMs   int64 `json:"tickIntervalMs"`
}

// ConnectChallenge 连接挑战事件
type ConnectChallenge struct {
	TypeVal FrameType `json:"type"`
	Event   string    `json:"event"`
	Payload struct {
		Nonce string `json:"nonce"`
		Ts    int64  `json:"ts"`
	} `json:"payload"`
}

// Type 实现 Frame 接口
func (c *ConnectChallenge) Type() FrameType {
	return c.TypeVal
}

// NewConnectChallenge 创建连接挑战
func NewConnectChallenge(nonce string, ts int64) *ConnectChallenge {
	return &ConnectChallenge{
		TypeVal: FrameTypeEvent,
		Event:   "connect.challenge",
	}
}

// ParseFrame 解析帧
func ParseFrame(data []byte) (Frame, error) {
	var typeWrapper struct {
		Type FrameType `json:"type"`
	}

	if err := json.Unmarshal(data, &typeWrapper); err != nil {
		return nil, fmt.Errorf("failed to parse frame type: %w", err)
	}

	switch typeWrapper.Type {
	case FrameTypeRequest:
		var req Request
		if err := json.Unmarshal(data, &req); err != nil {
			return nil, fmt.Errorf("failed to parse request: %w", err)
		}
		return &req, nil

	case FrameTypeResponse:
		var res Response
		if err := json.Unmarshal(data, &res); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		return &res, nil

	case FrameTypeEvent:
		var event Event
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, fmt.Errorf("failed to parse event: %w", err)
		}
		return &event, nil

	case FrameTypeHelloOK:
		var hello HelloOK
		if err := json.Unmarshal(data, &hello); err != nil {
			return nil, fmt.Errorf("failed to parse hello-ok: %w", err)
		}
		return &hello, nil

	default:
		return nil, fmt.Errorf("unknown frame type: %s", typeWrapper.Type)
	}
}

// EncodeFrame 编码帧
func EncodeFrame(frame Frame) ([]byte, error) {
	return json.Marshal(frame)
}

// NewRequest 创建请求
func NewRequest(id, method string, params interface{}) (*Request, error) {
	var paramsRaw json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		paramsRaw = data
	}

	return &Request{
		TypeVal: FrameTypeRequest,
		ID:      id,
		Method:  method,
		Params:  paramsRaw,
	}, nil
}

// NewResponse 创建响应
func NewResponse(id string, ok bool, payload interface{}, err *ErrorDetail) (*Response, error) {
	var payloadRaw json.RawMessage
	if payload != nil && ok {
		data, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", marshalErr)
		}
		payloadRaw = data
	}

	return &Response{
		TypeVal: FrameTypeResponse,
		ID:      id,
		OK:      ok,
		Payload: payloadRaw,
		Error:   err,
	}, nil
}

// NewSuccessResponse 创建成功响应
func NewSuccessResponse(id string, payload interface{}) (*Response, error) {
	return NewResponse(id, true, payload, nil)
}

// NewErrorResponse 创建错误响应
func NewErrorResponse(id string, code, message string, details interface{}) (*Response, error) {
	var detailsRaw json.RawMessage
	if details != nil {
		data, err := json.Marshal(details)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal details: %w", err)
		}
		detailsRaw = data
	}

	return NewResponse(id, false, nil, &ErrorDetail{
		Code:    code,
		Message: message,
		Details: detailsRaw,
	})
}

// NewEvent 创建事件
func NewEvent(event string, payload interface{}, seq int64, stateVersion *StateVersion) (*Event, error) {
	var payloadRaw json.RawMessage
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
		payloadRaw = data
	}

	return &Event{
		TypeVal:      FrameTypeEvent,
		Event:        event,
		Payload:      payloadRaw,
		Seq:          seq,
		StateVersion: stateVersion,
	}, nil
}
