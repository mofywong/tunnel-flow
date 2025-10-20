package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// 消息类型常量
const (
	MessageTypeControl  = "CONTROL"
	MessageTypeBusiness = "MESSAGE"
	MessageTypeACK      = "ACK"
	MessageTypeError    = "ERROR"
)

// 操作类型常量
const (
	// 控制操作
	OpRegister    = "REGISTER"
	OpRegisterAck = "REGISTER_ACK"
	OpPing        = "PING"
	OpPong        = "PONG"
	OpRouteSync   = "ROUTE_SYNC"
	
	// 业务操作
	OpRequest  = "REQUEST"
	OpResponse = "RESPONSE"
	
	// 通用操作
	OpACK   = "ACK"
	OpError = "ERROR"
)

// Message WebSocket消息结构
type Message struct {
	Type      string      `json:"type"`                // 消息类型
	Op        string      `json:"op"`                  // 操作类型
	ClientID  string      `json:"client_id"`           // 客户端ID
	MsgID     *string     `json:"msg_id,omitempty"`    // 消息ID（可选）
	Timestamp int64       `json:"timestamp"`           // 时间戳
	Payload   interface{} `json:"payload,omitempty"`   // 载荷数据
}

// NewMessage 创建新消息
func NewMessage(msgType, op, clientID string, msgID *string, payload interface{}) (*Message, error) {
	return &Message{
		Type:      msgType,
		Op:        op,
		ClientID:  clientID,
		MsgID:     msgID,
		Timestamp: time.Now().UnixMilli(),
		Payload:   payload,
	}, nil
}

// ParsePayload 解析载荷到指定结构
func (m *Message) ParsePayload(target interface{}) error {
	if m.Payload == nil {
		return nil
	}
	
	payloadBytes, err := json.Marshal(m.Payload)
	if err != nil {
		return err
	}
	
	return json.Unmarshal(payloadBytes, target)
}

// 注册载荷
type RegisterPayload struct {
	ClientID  string   `json:"client_id"`
	AuthToken string   `json:"auth_token"`
	Version   string   `json:"version"`
	LocalIPs  []string `json:"local_ips"` // 本地网卡IP地址列表
}

// 注册确认载荷
type RegisterAckPayload struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// HTTP请求载荷
type RequestPayload struct {
	Method       string            `json:"method"`
	URL          string            `json:"url"`
	Headers      map[string]string `json:"headers"`
	Body         string            `json:"body"`
	Timeout      int               `json:"timeout"`        // 超时时间（毫秒）
	URLSuffix    string            `json:"url_suffix"`     // URL后缀，用于路由匹配
	TargetsJSON  string            `json:"targets_json"`   // 目标地址JSON数组
	Strategy     string            `json:"strategy"`       // 负载均衡策略
	HTTPMethod   string            `json:"http_method"`    // HTTP方法
	RouteMode    string            `json:"route_mode"`     // 路由配置模式：basic/full
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

// HTTP响应载荷
type ResponsePayload struct {
	HTTPStatus int               `json:"http_status"`
	Headers    map[string]string `json:"headers"`
	Body       interface{}       `json:"body"`
	LatencyMS  int64             `json:"latency_ms"`
	Error      *string           `json:"error,omitempty"`
}

// ACK载荷
type ACKPayload struct {
	MsgID   string `json:"msg_id"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// 错误载荷
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// Ping载荷
type PingPayload struct {
	Timestamp int64 `json:"timestamp"`
}

// Pong载荷
type PongPayload struct {
	Timestamp int64 `json:"timestamp"`
}

// 路由目标
type RouteTarget struct {
	URL    string `json:"url"`
	Weight int    `json:"weight,omitempty"`
}

// 路由信息
type RouteInfo struct {
	ID          string        `json:"id"`
	URLSuffix   string        `json:"url_suffix"`
	TargetsJSON string        `json:"targets_json"`
	Strategy    string        `json:"strategy"`
	Targets     []RouteTarget `json:"targets,omitempty"` // 解析后的目标列表
}

// 路由同步载荷
type RouteSyncPayload struct {
	Routes []RouteInfo `json:"routes"`
}

// GenerateMessageID 生成消息ID
func GenerateMessageID() string {
	return uuid.New().String()
}

// ValidateMessage 验证消息格式
func ValidateMessage(msg *Message) error {
	if msg.Type == "" {
		return fmt.Errorf("message type is required")
	}
	
	if msg.Op == "" {
		return fmt.Errorf("operation is required")
	}
	
	if msg.ClientID == "" {
		return fmt.Errorf("client ID is required")
	}
	
	// 业务消息必须有消息ID
	if msg.Type == MessageTypeBusiness && msg.MsgID == nil {
		return fmt.Errorf("business messages must have msg_id")
	}
	
	return nil
}