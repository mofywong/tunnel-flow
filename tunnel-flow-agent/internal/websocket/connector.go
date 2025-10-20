package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/google/uuid"

	"tunnel-flow-agent/internal/config"
	"tunnel-flow-agent/internal/retry"
)

// ConnectionState 连接状态
type ConnectionState int

const (
	StateDisconnected ConnectionState = iota
	StateConnecting
	StateConnected
	StateReconnecting
)

// String 返回状态字符串
func (s ConnectionState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateReconnecting:
		return "reconnecting"
	default:
		return "unknown"
	}
}

// ConnectionStats 连接统计信息
type ConnectionStats struct {
	ConnectedAt        time.Time     `json:"connected_at"`
	LastMessageTime    time.Time     `json:"last_message_time"`
	MessagesSent       int64         `json:"messages_sent"`
	MessagesReceived   int64         `json:"messages_received"`
	BytesSent          int64         `json:"bytes_sent"`
	BytesReceived      int64         `json:"bytes_received"`
	ReconnectCount     int           `json:"reconnect_count"`
	LastReconnectTime  time.Time     `json:"last_reconnect_time"`
	AverageLatency     time.Duration `json:"average_latency"`
	ConnectionDuration time.Duration `json:"connection_duration"`
	ErrorCount         int64         `json:"error_count"`
	LastError          string        `json:"last_error"`
	LastErrorTime      time.Time     `json:"last_error_time"`
}

// Message WebSocket消息结构
type Message struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	TargetID  string                 `json:"target_id,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// MessageHandler 消息处理器接口
type MessageHandler interface {
	HandleMessage(msg *Message) error
}

// Connector WebSocket连接器
type Connector struct {
	config         *config.Config
	conn           *websocket.Conn
	state          ConnectionState
	stateMu        sync.RWMutex
	
	// 消息处理
	messageHandler MessageHandler
	sendQueue      chan []byte
	
	// 统计信息
	stats          *ConnectionStats
	statsMu        sync.RWMutex
	
	// 心跳和重连
	heartbeatTicker *time.Ticker
	reconnectTimer  *time.Timer
	retryStrategy   *retry.RetryStrategy
	
	// 上下文控制
	ctx            context.Context
	cancel         context.CancelFunc
	
	// 事件回调
	onConnected    func()
	onDisconnected func(error)
	onError        func(error)
	
	mu sync.Mutex
}

// NewConnector 创建新的WebSocket连接器
func NewConnector(cfg *config.Config, handler MessageHandler) *Connector {
	ctx, cancel := context.WithCancel(context.Background())
	
	// 创建重试策略
	retryConfig := &retry.RetryConfig{
		MaxAttempts:  cfg.MaxRetries(),
		InitialDelay: time.Duration(cfg.RetryDelayMS()) * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
	}
	
	return &Connector{
		config:         cfg,
		state:          StateDisconnected,
		messageHandler: handler,
		sendQueue:      make(chan []byte, cfg.SendQueueSize()),
		stats: &ConnectionStats{
			ConnectedAt: time.Now(),
		},
		retryStrategy: retry.NewRetryStrategy(retryConfig),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// SetEventHandlers 设置事件处理器
func (c *Connector) SetEventHandlers(onConnected func(), onDisconnected func(error), onError func(error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.onConnected = onConnected
	c.onDisconnected = onDisconnected
	c.onError = onError
}

// Connect 连接到WebSocket服务器
func (c *Connector) Connect() error {
	c.setState(StateConnecting)
	
	// 构建WebSocket URL
	wsURL := c.buildWebSocketURL()
	
	// 创建HTTP头
	headers := http.Header{}
	headers.Set("User-Agent", "TunnelFlow-Agent/1.0")
	
	log.Printf("Connecting to WebSocket server: %s", wsURL)
	
	// 建立WebSocket连接
	dialer := websocket.Dialer{
		HandshakeTimeout: c.config.HTTPTimeout(),
		ReadBufferSize:   4096,
		WriteBufferSize:  4096,
	}
	
	conn, resp, err := dialer.Dial(wsURL, headers)
	if err != nil {
		c.setState(StateDisconnected)
		c.recordError(fmt.Sprintf("Failed to connect: %v", err))
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}
	
	if resp != nil {
		resp.Body.Close()
	}
	
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	
	c.setState(StateConnected)
	c.updateConnectionStats()
	
	// 启动消息处理goroutine
	go c.readLoop()
	go c.writeLoop()
	go c.heartbeatLoop()
	
	log.Printf("Successfully connected to WebSocket server")
	
	// 触发连接事件
	if c.onConnected != nil {
		c.onConnected()
	}
	
	return nil
}

// Disconnect 断开连接
func (c *Connector) Disconnect() error {
	c.setState(StateDisconnected)
	
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// 停止心跳
	if c.heartbeatTicker != nil {
		c.heartbeatTicker.Stop()
		c.heartbeatTicker = nil
	}
	
	// 停止重连定时器
	if c.reconnectTimer != nil {
		c.reconnectTimer.Stop()
		c.reconnectTimer = nil
	}
	
	// 关闭连接
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	
	return nil
}

// SendMessage 发送消息
func (c *Connector) SendMessage(msg *Message) error {
	if c.getState() != StateConnected {
		return fmt.Errorf("not connected")
	}
	
	// 设置消息ID和时间戳
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	msg.Timestamp = time.Now()
	
	// 序列化消息
	data, err := json.Marshal(msg)
	if err != nil {
		c.recordError(fmt.Sprintf("Failed to marshal message: %v", err))
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	
	// 发送到队列
	select {
	case c.sendQueue <- data:
		c.updateSentStats(int64(len(data)))
		return nil
	case <-c.ctx.Done():
		return fmt.Errorf("connector is closed")
	default:
		c.recordError("Send queue is full")
		return fmt.Errorf("send queue is full")
	}
}

// GetState 获取连接状态
func (c *Connector) GetState() ConnectionState {
	return c.getState()
}

// GetStats 获取统计信息
func (c *Connector) GetStats() ConnectionStats {
	c.statsMu.RLock()
	defer c.statsMu.RUnlock()
	
	stats := *c.stats
	if c.getState() == StateConnected {
		stats.ConnectionDuration = time.Since(stats.ConnectedAt)
	}
	
	return stats
}

// Close 关闭连接器
func (c *Connector) Close() error {
	c.cancel()
	return c.Disconnect()
}

// 内部方法

// buildWebSocketURL 构建WebSocket URL
func (c *Connector) buildWebSocketURL() string {
	scheme := "ws"
	if c.config.UseSSL() {
		scheme = "wss"
	}
	
	u := url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", c.config.ServerHost(), c.config.ServerPort()),
		Path:   "/ws",
	}
	
	// 添加查询参数
	query := u.Query()
	query.Set("client_id", c.config.ClientID())
	query.Set("auth_token", c.config.AuthToken())
	u.RawQuery = query.Encode()
	
	return u.String()
}

// setState 设置连接状态
func (c *Connector) setState(state ConnectionState) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	
	oldState := c.state
	c.state = state
	
	log.Printf("Connection state changed: %s -> %s", oldState, state)
}

// getState 获取连接状态
func (c *Connector) getState() ConnectionState {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	
	return c.state
}

// updateConnectionStats 更新连接统计
func (c *Connector) updateConnectionStats() {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	
	c.stats.ConnectedAt = time.Now()
	c.stats.LastMessageTime = time.Now()
}

// updateSentStats 更新发送统计
func (c *Connector) updateSentStats(bytes int64) {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	
	c.stats.MessagesSent++
	c.stats.BytesSent += bytes
	c.stats.LastMessageTime = time.Now()
}

// updateReceivedStats 更新接收统计
func (c *Connector) updateReceivedStats(bytes int64) {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	
	c.stats.MessagesReceived++
	c.stats.BytesReceived += bytes
	c.stats.LastMessageTime = time.Now()
}

// recordError 记录错误
func (c *Connector) recordError(errMsg string) {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	
	c.stats.ErrorCount++
	c.stats.LastError = errMsg
	c.stats.LastErrorTime = time.Now()
	
	log.Printf("WebSocket error: %s", errMsg)
	
	// 触发错误事件
	if c.onError != nil {
		go c.onError(fmt.Errorf(errMsg))
	}
}

// readLoop 读取消息循环
func (c *Connector) readLoop() {
	defer func() {
		if c.onDisconnected != nil {
			c.onDisconnected(fmt.Errorf("read loop ended"))
		}
		c.setState(StateDisconnected)
	}()
	
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}
		
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		
		if conn == nil {
			return
		}
		
		// 设置读取超时
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.recordError(fmt.Sprintf("Unexpected close error: %v", err))
			}
			return
		}
		
		// 处理不同类型的消息
		switch messageType {
		case websocket.TextMessage:
			c.handleTextMessage(data)
		case websocket.PongMessage:
			c.handlePongMessage()
		case websocket.CloseMessage:
			log.Printf("Received close message")
			return
		}
	}
}

// writeLoop 写入消息循环
func (c *Connector) writeLoop() {
	defer func() {
		c.setState(StateDisconnected)
	}()
	
	for {
		select {
		case <-c.ctx.Done():
			return
		case data := <-c.sendQueue:
			c.mu.Lock()
			conn := c.conn
			c.mu.Unlock()
			
			if conn == nil {
				return
			}
			
			// 设置写入超时
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				c.recordError(fmt.Sprintf("Failed to write message: %v", err))
				return
			}
		}
	}
}

// heartbeatLoop 心跳循环
func (c *Connector) heartbeatLoop() {
	c.heartbeatTicker = time.NewTicker(30 * time.Second)
	defer c.heartbeatTicker.Stop()
	
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.heartbeatTicker.C:
			if c.getState() == StateConnected {
				c.sendPing()
			}
		}
	}
}

// sendPing 发送ping消息
func (c *Connector) sendPing() {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	
	if conn == nil {
		return
	}
	
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
		c.recordError(fmt.Sprintf("Failed to send ping: %v", err))
	}
}

// handleTextMessage 处理文本消息
func (c *Connector) handleTextMessage(data []byte) {
	c.updateReceivedStats(int64(len(data)))
	
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		c.recordError(fmt.Sprintf("Failed to unmarshal message: %v", err))
		return
	}
	
	// 处理特殊消息类型
	switch msg.Type {
	case "pong":
		c.handlePongMessage()
		return
	case "ping":
		c.sendPongMessage()
		return
	}
	
	// 转发给消息处理器
	if c.messageHandler != nil {
		if err := c.messageHandler.HandleMessage(&msg); err != nil {
			c.recordError(fmt.Sprintf("Message handler error: %v", err))
		}
	}
}

// handlePongMessage 处理pong消息
func (c *Connector) handlePongMessage() {
	// 更新延迟统计
	// 这里可以实现RTT计算
	log.Printf("Received pong message")
}

// sendPongMessage 发送pong消息
func (c *Connector) sendPongMessage() {
	pongMsg := &Message{
		ID:        uuid.New().String(),
		Type:      "pong",
		Timestamp: time.Now(),
	}
	
	if err := c.SendMessage(pongMsg); err != nil {
		c.recordError(fmt.Sprintf("Failed to send pong: %v", err))
	}
}

// startReconnect 开始重连
func (c *Connector) startReconnect() {
	if c.getState() == StateReconnecting {
		return
	}
	
	c.setState(StateReconnecting)
	
	go func() {
		err := c.retryStrategy.ExecuteWithRetry(c.ctx, func() error {
			if c.ctx.Err() != nil {
				return fmt.Errorf("connector is closed")
			}
			
			log.Printf("Attempting to reconnect...")
			return c.Connect()
		})
		
		if err != nil {
			c.recordError(fmt.Sprintf("Reconnect failed: %v", err))
			c.setState(StateDisconnected)
		} else {
			c.statsMu.Lock()
			c.stats.ReconnectCount++
			c.stats.LastReconnectTime = time.Now()
			c.statsMu.Unlock()
			
			log.Printf("Reconnected successfully")
		}
	}()
}