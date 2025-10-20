package protocol

import (
	"encoding/json"
	"strings"
	"time"
)

// MessageType 消息类型
type MessageType string

const (
	MessageTypeControl MessageType = "CONTROL"
	MessageTypeMessage MessageType = "MESSAGE"
	MessageTypeACK     MessageType = "ACK"
	MessageTypeError   MessageType = "ERROR"
)

// Operation 操作类型
type Operation string

const (
	OpRegister     Operation = "REGISTER"
	OpRegisterAck  Operation = "REGISTER_ACK"
	OpRouteSync    Operation = "ROUTE_SYNC"
	OpRouteSyncAck Operation = "ROUTE_SYNC_ACK"
	OpRequest      Operation = "REQUEST"
	OpResponse     Operation = "RESPONSE"
	OpACK          Operation = "ACK"
	OpPing         Operation = "PING"
	OpPong         Operation = "PONG"
	OpCancel       Operation = "CANCEL"
	OpError        Operation = "ERROR"
)

// Message WebSocket消息结构
type Message struct {
	Type     MessageType     `json:"type"`
	Op       Operation       `json:"op"`
	MsgID    *string         `json:"msg_id"`
	ClientID string          `json:"client_id"`
	TS       int64           `json:"ts"`
	Payload  json.RawMessage `json:"payload"`
}

// NewMessage 创建新消息
func NewMessage(msgType MessageType, op Operation, clientID string, msgID *string, payload interface{}) (*Message, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return &Message{
		Type:     msgType,
		Op:       op,
		MsgID:    msgID,
		ClientID: clientID,
		TS:       time.Now().UnixMilli(),
		Payload:  payloadBytes,
	}, nil
}

// ParsePayload 解析payload到指定结构
func (m *Message) ParsePayload(v interface{}) error {
	return json.Unmarshal(m.Payload, v)
}

// RegisterPayload 注册消息载荷
type RegisterPayload struct {
	AuthToken string   `json:"auth_token"`
	Version   string   `json:"version"`
	LocalIPs  []string `json:"local_ips"` // 本地网卡IP地址列表
}

// RegisterAckPayload 注册确认消息载荷
type RegisterAckPayload struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// RouteTarget 路由目标
type RouteTarget struct {
	URL    string `json:"url"`
}

// RequestPayload 请求消息载荷
type RequestPayload struct {
	URLSuffix     string            `json:"url_suffix"`
	HTTPMethod    string            `json:"http_method"`
	Headers       map[string]string `json:"headers"`
	Params        map[string]string `json:"params"`
	Body          interface{}       `json:"body"`
	TimeoutMS     int               `json:"timeout_ms"`
	TargetsJSON   string            `json:"targets_json"`   // 路由目标JSON字符串
	DeliveryPolicy string           `json:"delivery_policy"` // 投递策略
	RouteMode     string            `json:"route_mode"`     // 路由配置模式：basic/full
}

// GetTargets 解析路由目标
func (r *RequestPayload) GetTargets() ([]RouteTarget, error) {
	var targets []RouteTarget
	if r.TargetsJSON == "" {
		return targets, nil
	}
	
	// 首先尝试解析为JSON数组（兼容旧格式）
	err := json.Unmarshal([]byte(r.TargetsJSON), &targets)
	if err == nil {
		return targets, nil
	}
	
	// 如果JSON解析失败，尝试作为单个URL字符串处理
	urlStr := strings.TrimSpace(r.TargetsJSON)
	if urlStr != "" {
		targets = append(targets, RouteTarget{URL: urlStr})
	}
	
	return targets, nil
}

// ResponsePayload 响应消息载荷
type ResponsePayload struct {
	HTTPStatus int               `json:"http_status"`
	Headers    map[string]string `json:"headers"`
	Body       interface{}       `json:"body"`
	LatencyMS  int64             `json:"latency_ms"`
	Error      *string           `json:"error"`
}

// ACKPayload 确认消息载荷
type ACKPayload struct {
	MsgID   string `json:"msg_id"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// ErrorPayload 错误消息载荷
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// PingPayload Ping消息载荷
type PingPayload struct {
	Timestamp int64 `json:"timestamp"`
}

// PongPayload Pong消息载荷
type PongPayload struct {
	Timestamp int64 `json:"timestamp"`
}