package websocket

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/google/uuid"

	"tunnel-flow/internal/config"
	"tunnel-flow/internal/database"
	"tunnel-flow/internal/protocol"
	"tunnel-flow/internal/retry"
	"tunnel-flow/internal/performance"
)

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Message 消息结构
type Message struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	TargetID  string                 `json:"target_id,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// ClientConn 客户端连接信息
type ClientConn struct {
	clientID     string
	conn         *websocket.Conn
	sendQueue    chan []byte
	lastSeen     time.Time
	lastActivity time.Time
	connectedAt  time.Time
	messageCount int64
	bytesSent    int64
	bytesRecv    int64
	// 网络质量监控字段
	lastPingTime     time.Time
	lastPongTime     time.Time
	avgRTT           time.Duration
	rttSamples       []time.Duration
	packetLoss       float64
	networkQuality   string
	adaptiveInterval time.Duration
	mu               sync.Mutex
	ctx              context.Context
	cancel           context.CancelFunc
}

// generateMessageID 生成消息ID
func (c *ClientConn) generateMessageID() string {
	return uuid.New().String()
}

// sendMessage 发送消息
func (c *ClientConn) sendMessage(msg Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	
	select {
	case c.sendQueue <- data:
		return nil
	default:
		return fmt.Errorf("send queue is full")
	}
}

// PendingContext 待处理上下文
type PendingContext struct {
	msgID      string
	resultCh   chan *protocol.ResponsePayload
	ctx        context.Context
	cancel     context.CancelFunc
	createdAt  time.Time
	retryCount int
}

// HeartbeatUpdate 心跳更新信息
type HeartbeatUpdate struct {
	ClientID       string
	LastActiveTime time.Time
}

// ConnectionStats 连接统计信息
type ConnectionStats struct {
	TotalConnections    int64     `json:"total_connections"`
	ActiveConnections   int       `json:"active_connections"`
	TotalMessages       int64     `json:"total_messages"`
	TotalBytesSent      int64     `json:"total_bytes_sent"`
	TotalBytesReceived  int64     `json:"total_bytes_received"`
	AverageConnDuration float64   `json:"average_conn_duration_seconds"`
	LastUpdated         time.Time `json:"last_updated"`
	StartTime           time.Time `json:"start_time"`
	Uptime              time.Duration `json:"uptime"`
}

// Manager WebSocket连接管理器
type Manager struct {
	config          *config.Config
	db              *database.Repository
	upgrader        websocket.Upgrader
	clients         map[string]*ClientConn
	pending         map[string]*PendingContext
	routeIndex      map[string][]string
	heartbeatQueue  chan HeartbeatUpdate
	stats           *ConnectionStats
	
	// 自适应心跳配置
	baseHeartbeatInterval time.Duration
	maxHeartbeatInterval  time.Duration
	minHeartbeatInterval  time.Duration
	
	// 性能优化组件
	objectPool     *performance.ObjectPool
	workerPool     *performance.WorkerPool
	messageQueue   *performance.MessageQueue
	batchProcessor *performance.BatchProcessor
	connectionMgr  *performance.ConnectionManager
	retryStrategy  *retry.RetryStrategy
	
	// 监控组件
	metrics interface{}
	
	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// NewManager 创建新的WebSocket管理器
func NewManager(cfg *config.Config, db *database.Repository, objectPool *performance.ObjectPool, workerPool *performance.WorkerPool, metrics interface{}) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	
	// 创建消息队列和连接管理器
	messageQueue := performance.NewMessageQueue(cfg.MessageQueueSize, cfg.BatchSize, time.Duration(cfg.BatchTimeoutMS)*time.Millisecond)
	connectionMgr := performance.NewConnectionManager()
	
	// 创建重试策略
	retryStrategy := retry.NewRetryStrategy()
	
	m := &Manager{
		config:         cfg,
		db:             db,
		clients:        make(map[string]*ClientConn),
		pending:        make(map[string]*PendingContext),
		routeIndex:     make(map[string][]string),
		heartbeatQueue: make(chan HeartbeatUpdate, 1000),
		stats: &ConnectionStats{
			StartTime: time.Now(),
		},
		// 初始化心跳间隔
		baseHeartbeatInterval: 30 * time.Second,
		maxHeartbeatInterval:  120 * time.Second,
		minHeartbeatInterval:  10 * time.Second,
		// 性能优化组件
		objectPool:    objectPool,
		workerPool:    workerPool,
		messageQueue:  messageQueue,
		connectionMgr: connectionMgr,
		retryStrategy: retryStrategy,
		// 监控组件
		metrics: metrics,
		ctx:     ctx,
		cancel:  cancel,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
		},
	}

	// 启动后台任务
	go m.startBackgroundTasks()
	
	// 启动工作池
	m.workerPool.Start()
	
	// 启动批处理器
	m.batchProcessor = performance.NewBatchProcessor(
		m.messageQueue,
		m.processBatch,
		cfg.BatchSize,
		time.Duration(cfg.BatchTimeoutMS)*time.Millisecond,
	)
	m.batchProcessor.Start()

	return m
}

// HandleWebSocket 处理WebSocket连接
func (m *Manager) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		http.Error(w, "client_id is required", http.StatusBadRequest)
		return
	}
	
	// 验证token
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "token is required", http.StatusUnauthorized)
		return
	}
	
	// 验证JWT token
	if !m.validateToken(token) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	
	log.Printf("Client %s connecting", clientID)
	
	// 验证客户端是否存在
	client, err := m.db.GetClient(clientID)
	if err != nil {
		log.Printf("Client %s not found in database: %v", clientID, err)
		http.Error(w, "Client not found", http.StatusNotFound)
		return
	}
	
	// 验证authtoken
	if client.AuthToken != token {
		log.Printf("Client %s provided invalid auth token", clientID)
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}
	
	// 更新客户端最新心跳时间
	now := time.Now().UnixMilli()
	client.LastSeenTS = sql.NullInt64{Int64: now, Valid: true}
	err = m.db.UpdateClientLastSeen(clientID, now)
	if err != nil {
		log.Printf("Failed to update client %s last seen: %v", clientID, err)
	}
	
	// 检查客户端是否被禁用
	if client.Enabled != 1 {
		log.Printf("Client %s is disabled, rejecting connection", clientID)
		http.Error(w, "Client is disabled", http.StatusForbidden)
		return
	}
	
	conn, err := m.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}
	
	m.handleConnection(clientID, conn)
}

// handleConnection 处理单个连接
func (m *Manager) handleConnection(clientID string, conn *websocket.Conn) {
	// 记录连接指标
	if m.metrics != nil {
		if collector, ok := m.metrics.(interface{ IncrementConnections() }); ok {
			collector.IncrementConnections()
		}
	}
	
	ctx, cancel := context.WithCancel(m.ctx)
	client := &ClientConn{
		clientID:         clientID,
		conn:             conn,
		sendQueue:        make(chan []byte, m.config.SendQueueSize),
		lastSeen:         time.Now(),
		connectedAt:      time.Now(),
		adaptiveInterval: m.config.PingInterval(), // 初始化为配置的心跳间隔
		ctx:              ctx,
		cancel:           cancel,
	}
	
	// 注册客户端
	m.registerClient(client)
	
	defer func() {
		// 确保context被取消
		cancel()
		
		// 记录断开连接指标
		if m.metrics != nil {
			if collector, ok := m.metrics.(interface{ DecrementConnections() }); ok {
				collector.DecrementConnections()
			}
		}
		
		// 清理资源
		m.unregisterClient(clientID)
		conn.Close()
		log.Printf("Client %s disconnected", clientID)
	}()
	
	// 使用WaitGroup确保两个goroutine都正确退出
	var wg sync.WaitGroup
	wg.Add(2)
	
	// 启动读写goroutine
	go func() {
		defer wg.Done()
		m.clientReader(client)
	}()
	
	go func() {
		defer wg.Done()
		m.clientWriter(client)
	}()
	
	log.Printf("Client %s connected", clientID)
	
	// 等待两个goroutine都退出
	wg.Wait()
}

// registerClient 注册客户端
func (m *Manager) registerClient(client *ClientConn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.clients[client.clientID] = client
	
	// 更新统计信息
	m.stats.TotalConnections++
	m.stats.ActiveConnections = len(m.clients)
	m.stats.LastUpdated = time.Now()
	
	// 初始化网络质量监控
	client.mu.Lock()
	client.adaptiveInterval = m.baseHeartbeatInterval
	client.networkQuality = "unknown"
	client.rttSamples = make([]time.Duration, 0, 10)
	client.mu.Unlock()
	
	log.Printf("Client %s registered, total clients: %d", client.clientID, len(m.clients))
}

// unregisterClient 注销客户端
func (m *Manager) unregisterClient(clientID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	delete(m.clients, clientID)
	
	// 使用工作池处理数据库更新，避免创建新的goroutine
	task := &DatabaseUpdateTask{
		ClientID: clientID,
		Status:   "offline",
		DB:       m.db,
	}
	
	// 提交到工作池
	if err := m.workerPool.Submit(task); err != nil {
		// 工作池队列满，记录警告但不阻塞
		log.Printf("Worker pool queue full, skipping status update for client %s: %v", clientID, err)
	}
}

// clientReader 客户端读取goroutine
func (m *Manager) clientReader(client *ClientConn) {
	for {
		// 检查context是否被取消
		select {
		case <-client.ctx.Done():
			return
		default:
		}
		
		// 设置读取超时
		client.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		
		messageType, messageBytes, err := client.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error for client %s: %v", client.clientID, err)
			}
			// 取消context通知另一个goroutine退出
			client.cancel()
			return
		}
		
		// 更新最后活跃时间
		client.mu.Lock()
		client.lastSeen = time.Now()
		client.lastActivity = time.Now()
		client.mu.Unlock()
		
		// 处理消息
		m.handleMessage(client, messageType, messageBytes)
	}
}

// clientWriter 客户端写入goroutine
func (m *Manager) clientWriter(client *ClientConn) {
	// 使用配置的心跳间隔，而不是硬编码的30秒
	ticker := time.NewTicker(m.config.PingInterval())
	defer ticker.Stop()

	for {
		select {
		case message, ok := <-client.sendQueue:
			if !ok {
				// sendQueue已关闭，取消context通知另一个goroutine退出
				client.cancel()
				return
			}
			
			// 设置写入超时
			client.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			
			// 记录WebSocket数据发送流向日志
			log.Printf("[WebSocket Send] Sending %d bytes to client %s", len(message), client.clientID)
			
			if err := client.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("Failed to write message to client %s: %v", client.clientID, err)
				// 取消context通知另一个goroutine退出
				client.cancel()
				return
			}
			
			log.Printf("[WebSocket Send] Successfully sent message to client %s", client.clientID)
			
			// 更新统计信息
			client.mu.Lock()
			client.bytesSent += int64(len(message))
			client.mu.Unlock()

		case <-ticker.C:
			// 发送自定义协议的ping消息保持连接
			log.Printf("[WebSocket Send] Sending protocol ping to client %s", client.clientID)
			if err := m.sendProtocolPing(client.clientID); err != nil {
				log.Printf("Failed to send protocol ping to client %s: %v", client.clientID, err)
				// 取消context通知另一个goroutine退出
				client.cancel()
				return
			}
			log.Printf("[WebSocket Send] Successfully sent protocol ping to client %s", client.clientID)

		case <-client.ctx.Done():
			return
		}
	}
}

// handleMessage 处理消息
func (m *Manager) handleMessage(client *ClientConn, messageType int, data []byte) {
	start := time.Now()
	
	// 记录接收消息指标
	if m.metrics != nil {
		if collector, ok := m.metrics.(interface{ IncrementMessagesReceived() }); ok {
			collector.IncrementMessagesReceived()
		}
	}
	
	// 使用工作池处理消息
	task := &MessageTask{
		id:          uuid.New().String(),
		manager:     m,
		client:      client,
		messageType: messageType,
		data:        data,
		startTime:   start,
	}
	
	if err := m.workerPool.Submit(task); err != nil {
		log.Printf("Failed to submit message task: %v", err)
		
		// 记录错误指标
		if m.metrics != nil {
			if collector, ok := m.metrics.(interface{ IncrementErrors() }); ok {
				collector.IncrementErrors()
			}
		}
	}
}

// MessageTask 消息处理任务
// DatabaseUpdateTask 数据库更新任务
type DatabaseUpdateTask struct {
	ClientID string
	Status   string
	DB       *database.Repository
}

func (t *DatabaseUpdateTask) GetID() string {
	return fmt.Sprintf("db_update_%s_%d", t.ClientID, time.Now().UnixNano())
}

func (t *DatabaseUpdateTask) GetPriority() int {
	return 1 // 低优先级
}

func (t *DatabaseUpdateTask) Execute() performance.TaskResult {
	start := time.Now()
	err := t.DB.UpdateClientStatus(t.ClientID, t.Status)
	duration := time.Since(start)
	
	if err != nil {
		return performance.TaskResult{
			Success:  false,
			Error:    err,
			Duration: duration,
		}
	}
	
	return performance.TaskResult{
		Success:  true,
		Duration: duration,
	}
}

type MessageTask struct {
	id          string
	manager     *Manager
	client      *ClientConn
	messageType int
	data        []byte
	startTime   time.Time
}

// GetID 获取任务ID
func (t *MessageTask) GetID() string {
	return t.id
}

// GetPriority 获取任务优先级
func (t *MessageTask) GetPriority() int {
	return 1 // 默认优先级
}

// Execute 执行任务
func (t *MessageTask) Execute() performance.TaskResult {
	result := performance.TaskResult{
		TaskID:    t.id,
		Timestamp: time.Now(),
	}
	
	defer func() {
		result.Duration = time.Since(t.startTime)
		
		// 记录响应时间
		if t.manager.metrics != nil {
			if collector, ok := t.manager.metrics.(interface{ RecordResponseTime(time.Duration) }); ok {
				collector.RecordResponseTime(result.Duration)
			}
		}
	}()
	
	// 首先检查消息类型，只对文本消息进行JSON解析
	switch t.messageType {
	case websocket.PingMessage:
		log.Printf("Received ping from client %s", t.client.clientID)
		// 更新客户端最后活跃时间
		now := time.Now()
		t.client.mu.Lock()
		t.client.lastSeen = now
		t.client.mu.Unlock()
		result.Success = true
		return result
		
	case websocket.PongMessage:
		log.Printf("Received pong from client %s", t.client.clientID)
		// 更新客户端最后活跃时间
		now := time.Now()
		t.client.mu.Lock()
		t.client.lastSeen = now
		t.client.mu.Unlock()
		result.Success = true
		return result
		
	case websocket.CloseMessage:
		log.Printf("Received close message from client %s", t.client.clientID)
		result.Success = true
		return result
		
	case websocket.BinaryMessage:
		log.Printf("Received binary message from client %s, length: %d", t.client.clientID, len(t.data))
		// 对于二进制消息，我们暂时不处理，只记录
		result.Success = true
		return result
		
	case websocket.TextMessage:
		// 只对文本消息进行JSON解析
		break
		
	default:
		log.Printf("Received unknown message type %d from client %s", t.messageType, t.client.clientID)
		result.Success = true
		return result
	}
	
	// 解析JSON消息（只有文本消息会到达这里）
	var msg protocol.Message
	if err := json.Unmarshal(t.data, &msg); err != nil {
		log.Printf("Failed to parse JSON message from client %s: %v", t.client.clientID, err)
		log.Printf("Raw message data (first 200 chars): %s", string(t.data[:min(len(t.data), 200)]))
		
		// 记录错误指标
		if t.manager.metrics != nil {
			if collector, ok := t.manager.metrics.(interface{ IncrementErrors() }); ok {
				collector.IncrementErrors()
			}
		}
		
		// 对于JSON解析失败，我们不应该返回错误，而是忽略这个消息
		// 这通常发生在连接断开时收到的非JSON数据
		log.Printf("Ignoring non-JSON message from client %s", t.client.clientID)
		result.Success = true
		return result
	}

	// 处理消息
	switch msg.Type {
	case protocol.MessageTypeControl:
		t.manager.handleControlMessage(t.client, &msg)
	case protocol.MessageTypeMessage:
		t.manager.handleDataMessage(t.client, &msg)
	case protocol.MessageTypeACK:
		t.manager.handleACKMessage(t.client, &msg)
	case protocol.MessageTypeError:
		t.manager.handleErrorMessage(t.client, &msg)
	default:
		log.Printf("Unknown message type: %s", msg.Type)
	}

	result.Success = true
	return result
}

// handlePing 处理ping消息
func (t *MessageTask) handlePing() {
	pong := Message{
		ID:        t.client.generateMessageID(),
		Type:      "pong",
		Timestamp: time.Now(),
	}
	
	t.client.sendMessage(pong)
}

// handleRequest 处理请求消息
func (t *MessageTask) handleRequest(msg *Message) {
	// 更新客户端最后活动时间
	t.client.mu.Lock()
	t.client.lastActivity = time.Now()
	t.client.mu.Unlock()
	
	// 转发请求到目标客户端
	if targetClient := t.manager.getClient(msg.TargetID); targetClient != nil {
		targetClient.sendMessage(*msg)
		
		// 记录发送消息指标
		if t.manager.metrics != nil {
			if collector, ok := t.manager.metrics.(interface{ IncrementMessagesSent() }); ok {
				collector.IncrementMessagesSent()
			}
		}
	} else {
		// 目标客户端不存在，返回错误
		errorMsg := Message{
			ID:        msg.ID,
			Type:      "error",
			Data:      map[string]interface{}{"error": "Target client not found"},
			Timestamp: time.Now(),
		}
		
		t.client.sendMessage(errorMsg)
		
		// 记录错误指标
		if t.manager.metrics != nil {
			if collector, ok := t.manager.metrics.(interface{ IncrementErrors() }); ok {
				collector.IncrementErrors()
			}
		}
	}
}

// handleResponse 处理响应消息
func (t *MessageTask) handleResponse(msg *Message) {
	// 转发响应到原始客户端
	if targetClient := t.manager.getClient(msg.TargetID); targetClient != nil {
		targetClient.sendMessage(*msg)
		
		// 记录发送消息指标
		if t.manager.metrics != nil {
			if collector, ok := t.manager.metrics.(interface{ IncrementMessagesSent() }); ok {
				collector.IncrementMessagesSent()
			}
		}
	}
}

// getClient 获取客户端
func (m *Manager) getClient(clientID string) *ClientConn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return m.clients[clientID]
}

// GetConnectionCount 获取连接数（用于健康检查）
func (m *Manager) GetConnectionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return len(m.clients)
}

// GetStats 获取统计信息
func (m *Manager) GetStats() ConnectionStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	stats := *m.stats
	stats.ActiveConnections = len(m.clients)
	stats.Uptime = time.Since(stats.StartTime)
	stats.LastUpdated = time.Now()
	
	return stats
}

// validateToken 验证JWT token
func (m *Manager) validateToken(tokenString string) bool {
	// 验证token是否非空
	if tokenString == "" {
		return false
	}
	
	// 这里可以添加更复杂的token验证逻辑
	// 目前简化处理，只要非空就认为有效
	// 真正的验证会在连接建立后通过数据库中的auth_token_hash进行
	return true
}

// processBatch 批处理消息
func (m *Manager) processBatch(messages []*performance.QueueMessage) error {
	for _, msg := range messages {
		log.Printf("Processing batch message: %s", msg.ID)
	}
	return nil
}

// startBackgroundTasks 启动后台任务
func (m *Manager) startBackgroundTasks() {
	// 启动心跳批量处理goroutine
	go m.processHeartbeatUpdates()
	
	// 定期清理超时的待处理请求
	cleanupTicker := time.NewTicker(30 * time.Second)
	
	// 定期健康检查
	healthCheckTicker := time.NewTicker(m.config.PingInterval())
	
	// 在goroutine中运行，确保ticker能被正确停止
	go func() {
		defer func() {
			cleanupTicker.Stop()
			healthCheckTicker.Stop()
		}()
		
		for {
			select {
			case <-m.ctx.Done():
				return
			case <-cleanupTicker.C:
				m.cleanupExpiredPending()
			case <-healthCheckTicker.C:
				m.performHealthCheck()
			}
		}
	}()
}

// cleanupExpiredPending 清理过期的待处理请求
func (m *Manager) cleanupExpiredPending() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	now := time.Now()
	for msgID, pending := range m.pending {
		if now.Sub(pending.createdAt) > m.config.RequestTimeout() {
			pending.cancel()
			delete(m.pending, msgID)
		}
	}
}

// performHealthCheck 执行健康检查
func (m *Manager) performHealthCheck() {
	m.mu.RLock()
	clients := make([]*ClientConn, 0, len(m.clients))
	for _, client := range m.clients {
		clients = append(clients, client)
	}
	m.mu.RUnlock()
	
	now := time.Now()
	pingInterval := m.config.PingInterval()
	
	for _, client := range clients {
		client.mu.Lock()
		lastSeen := client.lastSeen
		client.mu.Unlock()
		
		// 使用配置的心跳间隔检查超时，允许3倍的容错时间
		timeout := pingInterval * 3
		if now.Sub(lastSeen) > timeout {
			log.Printf("Client %s inactive for %v (threshold: %v), disconnecting", 
				client.clientID, now.Sub(lastSeen), timeout)
			client.cancel()
		}
	}
}

// processHeartbeatUpdates 批量处理心跳更新
func (m *Manager) processHeartbeatUpdates() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	updates := make(map[string]time.Time)
	
	for {
		select {
		case <-m.ctx.Done():
			return
		case update := <-m.heartbeatQueue:
			updates[update.ClientID] = update.LastActiveTime
		case <-ticker.C:
			if len(updates) > 0 {
				m.batchUpdateHeartbeats(updates)
				updates = make(map[string]time.Time)
			}
		}
	}
}

// batchUpdateHeartbeats 批量更新心跳时间
func (m *Manager) batchUpdateHeartbeats(updates map[string]time.Time) {
	if len(updates) == 0 {
		return
	}
	
	log.Printf("批量更新心跳时间，客户端数量: %d", len(updates))
	for clientID, lastActiveTime := range updates {
		if err := m.db.UpdateClientLastActiveTime(clientID, lastActiveTime); err != nil {
			log.Printf("Failed to update client %s last active time: %v", clientID, err)
		}
	}
}

// periodicCleanup 定期清理
func (m *Manager) periodicCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			// 执行清理任务
			log.Printf("Performing periodic cleanup")
		}
	}
}

// sendProtocolPing 发送自定义协议的ping消息
func (m *Manager) sendProtocolPing(clientID string) error {
	pingPayload := &protocol.PingPayload{
		Timestamp: time.Now().UnixMilli(),
	}
	
	pingMsg, err := protocol.NewMessage(
		protocol.MessageTypeControl,
		protocol.OpPing,
		clientID,
		nil,
		pingPayload,
	)
	if err != nil {
		return fmt.Errorf("failed to create ping message: %w", err)
	}
	
	return m.SendToClient(clientID, pingMsg)
}

// SendToClient 发送消息到指定客户端
func (m *Manager) SendToClient(clientID string, msg *protocol.Message) error {
	m.mu.RLock()
	client, exists := m.clients[clientID]
	m.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("client %s not found", clientID)
	}
	
	// 序列化消息
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}
	
	// 发送到客户端队列
	select {
	case client.sendQueue <- data:
		// 更新统计信息
		client.mu.Lock()
		client.messageCount++
		client.mu.Unlock()
		
		// 记录发送消息指标
		if m.metrics != nil {
			if collector, ok := m.metrics.(interface{ IncrementMessagesSent() }); ok {
				collector.IncrementMessagesSent()
			}
		}
		
		return nil
	default:
		return fmt.Errorf("client %s send queue is full", clientID)
	}
}

// DisconnectClient 断开指定客户端的连接
func (m *Manager) DisconnectClient(clientID string) error {
	m.mu.RLock()
	client, exists := m.clients[clientID]
	m.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("client %s not found", clientID)
	}
	
	// 取消客户端上下文，这将触发连接关闭
	client.cancel()
	
	log.Printf("Disconnected client %s", clientID)
	return nil
}

// Close 关闭管理器
func (m *Manager) Close() {
	m.cancel()
	
	// 停止批处理器
	if m.batchProcessor != nil {
		m.batchProcessor.Stop()
		log.Println("Batch processor stopped")
	}
	
	// 关闭消息队列
	if m.messageQueue != nil {
		m.messageQueue.Close()
		log.Println("Message queue closed")
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// 关闭所有客户端连接
	for _, client := range m.clients {
		client.cancel()
	}
	
	// 取消所有待处理的请求
	for _, pending := range m.pending {
		pending.cancel()
	}
	
	log.Println("WebSocket manager resources cleaned up")
}