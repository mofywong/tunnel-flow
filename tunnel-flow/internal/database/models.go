package database

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// MarshalJSON 自定义JSON序列化方法
func (c *Client) MarshalJSON() ([]byte, error) {
	type Alias Client
	aux := &struct {
		LastSeenTS *int64   `json:"last_seen_ts"`
		LocalIPs   []string `json:"local_ips"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}
	
	if c.LastSeenTS.Valid {
		aux.LastSeenTS = &c.LastSeenTS.Int64
	} else {
		aux.LastSeenTS = nil
	}
	
	// 解析local_ips JSON字符串为数组
	if c.LocalIPs != "" {
		var localIPs []string
		if err := json.Unmarshal([]byte(c.LocalIPs), &localIPs); err == nil {
			aux.LocalIPs = localIPs
		} else {
			aux.LocalIPs = []string{}
		}
	} else {
		aux.LocalIPs = []string{}
	}
	
	return json.Marshal(aux)
}

// UnmarshalJSON 自定义JSON反序列化方法
func (c *Client) UnmarshalJSON(data []byte) error {
	type Alias Client
	aux := &struct {
		LastSeenTS *int64 `json:"last_seen_ts"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}
	
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	
	if aux.LastSeenTS != nil {
		c.LastSeenTS = sql.NullInt64{Int64: *aux.LastSeenTS, Valid: true}
	} else {
		c.LastSeenTS = sql.NullInt64{Valid: false}
	}
	
	return nil
}

// Client 客户端模型 (合并了ClientConfig)
type Client struct {
	ClientID          string    `json:"client_id" db:"client_id"`
	Name              string    `json:"name" db:"name"`
	Description       string    `json:"description" db:"description"`
	AuthToken         string    `json:"auth_token" db:"auth_token"`
	HasAuthToken      bool      `json:"has_auth_token" db:"-"`  // 表示是否有认证令牌
	Status            string    `json:"status" db:"status"`
	Enabled           int       `json:"enabled" db:"enabled"`
	LastSeenTS        sql.NullInt64 `json:"last_seen_ts" db:"last_seen_ts"`
	HeartbeatInterval int       `json:"heartbeat_interval" db:"heartbeat_interval"`
	HeartbeatTimeout  int       `json:"heartbeat_timeout" db:"heartbeat_timeout"`
	CreatedAt         int64     `json:"created_at" db:"created_at"`
	UpdatedAt         int64     `json:"updated_at" db:"updated_at"`
	LocalIPs          string    `json:"local_ips" db:"local_ips"`  // JSON格式存储本地IP地址列表
	LastSeen          time.Time `json:"last_seen" db:"-"`
}

// IsEnabled 检查客户端是否启用
func (c *Client) IsEnabled() bool {
	return c.Enabled == 1
}

// SetEnabled 设置客户端启用状态
func (c *Client) SetEnabled(enabled bool) {
	if enabled {
		c.Enabled = 1
	} else {
		c.Enabled = 0
	}
	c.UpdatedAt = time.Now().Unix()
}

// ServerRoute 服务端路由模型
type ServerRoute struct {
	ID             int    `json:"id" db:"id"`
	URLSuffix      string `json:"url_suffix" db:"url_suffix"`        // 服务器路径，支持通配符*
	ClientID       string `json:"client_id" db:"client_id"`
	TargetsJSON    string `json:"targets_json" db:"targets_json"`    // 目标地址JSON
	DeliveryPolicy string `json:"delivery_policy" db:"delivery_policy"`
	RouteMode      string `json:"route_mode" db:"route_mode"`        // 路由配置模式：original_path/path_transform
	Enabled        int    `json:"enabled" db:"enabled"`              // 是否启用：1启用，0禁用
	Description    string `json:"description" db:"description"`      // 路由描述
	CreatedAt      int64  `json:"created_at" db:"created_at"`
	UpdatedAt      int64  `json:"updated_at" db:"updated_at"`
}

// 路由配置模式常量
const (
	RouteModeOriginalPath  = "original_path"  // 原路径模式：目标地址为http://ip:port，请求路径保持不变
	RouteModePathTransform = "path_transform" // 路径转换模式：目标地址为完整URL，直接转发到指定地址
)

// RouteTarget 路由目标
type RouteTarget struct {
	URL    string `json:"url"`
}

// GetTargets 解析路由目标
func (sr *ServerRoute) GetTargets() ([]RouteTarget, error) {
	var targets []RouteTarget
	if sr.TargetsJSON == "" {
		return targets, nil
	}
	
	// 首先尝试解析为JSON数组（兼容旧格式）
	err := json.Unmarshal([]byte(sr.TargetsJSON), &targets)
	if err == nil {
		return targets, nil
	}
	
	// 如果JSON解析失败，尝试作为单个URL字符串处理
	urlStr := strings.TrimSpace(sr.TargetsJSON)
	if urlStr != "" {
		targets = append(targets, RouteTarget{URL: urlStr})
	}
	
	return targets, nil
}

// SetTargets 设置路由目标
func (sr *ServerRoute) SetTargets(targets []RouteTarget) error {
	// 如果只有一个目标，直接存储URL字符串
	if len(targets) == 1 {
		sr.TargetsJSON = targets[0].URL
		return nil
	}
	
	// 多个目标时存储为JSON数组
	targetsBytes, err := json.Marshal(targets)
	if err != nil {
		return err
	}
	sr.TargetsJSON = string(targetsBytes)
	return nil
}

// IsEnabled 检查路由是否启用
func (sr *ServerRoute) IsEnabled() bool {
	return sr.Enabled == 1
}

// SetEnabled 设置路由启用状态
func (sr *ServerRoute) SetEnabled(enabled bool) {
	if enabled {
		sr.Enabled = 1
	} else {
		sr.Enabled = 0
	}
	sr.UpdatedAt = time.Now().UnixMilli()
}

// IsOriginalPathMode 检查是否为原路径模式
func (sr *ServerRoute) IsOriginalPathMode() bool {
	return sr.RouteMode == RouteModeOriginalPath
}

// IsPathTransformMode 检查是否为路径转换模式
func (sr *ServerRoute) IsPathTransformMode() bool {
	return sr.RouteMode == RouteModePathTransform
}

// PendingMessage 待处理消息模型
type PendingMessage struct {
	MsgID            string         `json:"msg_id" db:"msg_id"`
	ClientID         string         `json:"client_id" db:"client_id"`
	URLSuffix        string         `json:"url_suffix" db:"url_suffix"`
	RequestMetaJSON  string         `json:"request_meta_json" db:"request_meta_json"`
	State            string         `json:"state" db:"state"`
	RetryCount       int            `json:"retry_count" db:"retry_count"`
	NextTryTS        int64          `json:"next_try_ts" db:"next_try_ts"`
	CreatedAt        int64          `json:"created_at" db:"created_at"`
	LastUpdate       int64          `json:"last_update" db:"last_update"`
	ResponseMetaJSON sql.NullString `json:"response_meta_json" db:"response_meta_json"`
}

// MessageState 消息状态
const (
	MessageStatePending   = "pending"
	MessageStateProcessing = "processing"
	MessageStateDone      = "done"
	MessageStateFailed    = "failed"
	MessageStateCancelled = "cancelled"
)

// RequestMeta 请求元数据
type RequestMeta struct {
	HTTPMethod     string            `json:"http_method"`
	Headers        map[string]string `json:"headers"`
	Params         map[string]string `json:"params"`
	Body           interface{}       `json:"body"`
	TimeoutMS      int               `json:"timeout_ms"`
	TargetsJSON    string            `json:"targets_json"`
	DeliveryPolicy string            `json:"delivery_policy"`
}

// ResponseMeta 响应元数据
type ResponseMeta struct {
	HTTPStatus int               `json:"http_status"`
	Headers    map[string]string `json:"headers"`
	Body       interface{}       `json:"body"`
	LatencyMS  int64             `json:"latency_ms"`
	Error      *string           `json:"error"`
}

// GetRequestMeta 解析请求元数据
func (pm *PendingMessage) GetRequestMeta() (*RequestMeta, error) {
	var meta RequestMeta
	err := json.Unmarshal([]byte(pm.RequestMetaJSON), &meta)
	return &meta, err
}

// SetRequestMeta 设置请求元数据
func (pm *PendingMessage) SetRequestMeta(meta *RequestMeta) error {
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	pm.RequestMetaJSON = string(metaBytes)
	return nil
}

// GetResponseMeta 解析响应元数据
func (pm *PendingMessage) GetResponseMeta() (*ResponseMeta, error) {
	if !pm.ResponseMetaJSON.Valid {
		return nil, nil
	}
	var meta ResponseMeta
	err := json.Unmarshal([]byte(pm.ResponseMetaJSON.String), &meta)
	return &meta, err
}

// SetResponseMeta 设置响应元数据
func (pm *PendingMessage) SetResponseMeta(meta *ResponseMeta) error {
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	pm.ResponseMetaJSON = sql.NullString{
		String: string(metaBytes),
		Valid:  true,
	}
	return nil
}

// AuditLog 审计日志模型
type AuditLog struct {
	ID             int    `json:"id" db:"id"`
	MsgID          string `json:"msg_id" db:"msg_id"`
	ClientID       string `json:"client_id" db:"client_id"`
	Direction      string `json:"direction" db:"direction"`
	PayloadSummary string `json:"payload_summary" db:"payload_summary"`
	TS             int64  `json:"ts" db:"ts"`
}

// Direction 方向
const (
	DirectionInbound  = "inbound"
	DirectionOutbound = "outbound"
)