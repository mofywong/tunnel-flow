package websocket

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"tunnel-flow/internal/database"
	"tunnel-flow/internal/protocol"
)

// SendRequestAndWait 发送请求并等待响应
func (m *Manager) SendRequestAndWait(clientID string, requestPayload *protocol.RequestPayload, timeout time.Duration) (*protocol.ResponsePayload, error) {
	// 检查客户端是否连接
	if !m.IsClientConnected(clientID) {
		return nil, fmt.Errorf("client %s is not connected", clientID)
	}
	
	// 创建请求消息
	msgID := uuid.New().String()
	requestMsg, err := protocol.NewMessage(
		protocol.MessageTypeMessage,
		protocol.OpRequest,
		clientID,
		&msgID,
		requestPayload,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request message: %w", err)
	}
	
	// 创建等待上下文
	resultCh := make(chan *protocol.ResponsePayload, 1)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	pending := &PendingContext{
		msgID:     msgID,
		resultCh:  resultCh,
		ctx:       ctx,
		cancel:    cancel,
		createdAt: time.Now(),
	}
	
	// 注册等待的请求
	m.mu.Lock()
	m.pending[msgID] = pending
	pendingCount := len(m.pending)
	m.mu.Unlock()
	
	log.Printf("[SendRequestAndWait] Registered pending request %s, total pending: %d", msgID, pendingCount)
	
	// 确保在函数结束时清理
	defer func() {
		m.mu.Lock()
		delete(m.pending, msgID)
		remainingCount := len(m.pending)
		m.mu.Unlock()
		cancel()
		close(resultCh)
		log.Printf("[SendRequestAndWait] Cleaned up pending request %s, remaining pending: %d", msgID, remainingCount)
	}()
	
	// 保存到数据库
	requestMeta := &database.RequestMeta{
		HTTPMethod:     requestPayload.HTTPMethod,
		Headers:        requestPayload.Headers,
		Params:         requestPayload.Params,
		Body:           requestPayload.Body,
		TimeoutMS:      requestPayload.TimeoutMS,
		TargetsJSON:    requestPayload.TargetsJSON,
		DeliveryPolicy: requestPayload.DeliveryPolicy,
	}
	
	// 创建待处理消息
	pendingMsg := &database.PendingMessage{
		MsgID:       msgID,
		ClientID:    clientID,
		URLSuffix:   requestPayload.URLSuffix,
		State:       database.MessageStatePending,
		CreatedAt:   time.Now().UnixMilli(),
		LastUpdate:  time.Now().UnixMilli(),
	}
	
	// 设置请求元数据
	if err := pendingMsg.SetRequestMeta(requestMeta); err != nil {
		log.Printf("Failed to marshal request meta: %v", err)
	}
	
	if err := m.db.CreatePendingMessage(pendingMsg); err != nil {

		log.Printf("Failed to save pending message to database: %v", err)
		// 继续执行，不因为数据库错误而失败
	}
	
	// 发送消息
	log.Printf("[SendRequestAndWait] Sending request %s to client %s: %s %s", msgID, clientID, requestPayload.HTTPMethod, requestPayload.URLSuffix)
	if err := m.SendToClient(clientID, requestMsg); err != nil {
		log.Printf("[SendRequestAndWait] Failed to send request %s to client %s: %v", msgID, clientID, err)
		return nil, fmt.Errorf("failed to send request to client: %w", err)
	}
	
	log.Printf("[SendRequestAndWait] Successfully sent request %s to client %s, waiting for response...", msgID, clientID)
	
	// 等待响应或超时
	log.Printf("[SendRequestAndWait] Waiting for response to request %s (timeout: %v)", msgID, timeout)
	
	select {
	case response := <-resultCh:
		if response == nil {
			log.Printf("[SendRequestAndWait] Received nil response for request %s", msgID)
			return nil, fmt.Errorf("received nil response")
		}
		log.Printf("[SendRequestAndWait] Successfully received response for request %s - Status: %d", msgID, response.HTTPStatus)
		return response, nil

	case <-pending.ctx.Done():
		log.Printf("[SendRequestAndWait] Request %s timed out after %v", msgID, timeout)
		// 超时，更新数据库状态
		if err := m.db.UpdatePendingMessageState(msgID, database.MessageStateFailed); err != nil {
			log.Printf("[SendRequestAndWait] Failed to update message state to timeout for %s: %v", msgID, err)
		}
		return nil, fmt.Errorf("request timeout after %v", timeout)
	}
}



// SendRouteSync 发送路由同步消息（如果需要）
func (m *Manager) SendRouteSync(clientID string) error {
	// 获取客户端的路由
	routes, err := m.db.ListServerRoutes()
	if err != nil {
		return fmt.Errorf("failed to get routes for client %s: %w", clientID, err)
	}
	
	// 过滤属于该客户端的路由
	clientRoutes := make([]*database.ServerRoute, 0)
	for _, route := range routes {
		if route.ClientID == clientID {
			clientRoutes = append(clientRoutes, route)
		}
	}
	
	// 构建路由同步载荷
	routeSyncPayload := map[string]interface{}{
		"routes": clientRoutes,
	}
	
	routeSyncMsg, err := protocol.NewMessage(
		protocol.MessageTypeControl,
		protocol.OpRouteSync,
		clientID,
		nil,
		routeSyncPayload,
	)
	if err != nil {
		return fmt.Errorf("failed to create route sync message: %w", err)
	}
	
	return m.SendToClient(clientID, routeSyncMsg)
}

// GetConnectedClientCount 获取已连接客户端数量
func (m *Manager) GetConnectedClientCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.clients)
}

// GetConnectedClients 获取已连接客户端列表
// GetConnectedClients method is already defined in manager.go

// GetPendingRequestCount 获取待处理请求数量
func (m *Manager) GetPendingRequestCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.pending)
}

// CleanupExpiredPending 清理过期的待处理请求
func (m *Manager) CleanupExpiredPending(maxAge time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	now := time.Now()
	expired := make([]string, 0)
	
	for msgID, pending := range m.pending {
		if now.Sub(pending.createdAt) > maxAge {
			expired = append(expired, msgID)
		}
	}
	
	for _, msgID := range expired {
		if pending, exists := m.pending[msgID]; exists {
			// 发送超时响应
			select {
			case pending.resultCh <- &protocol.ResponsePayload{
				HTTPStatus: 504,
				Error:      stringPtr("Request timeout"),
			}:
			default:
			}
			delete(m.pending, msgID)
		}
	}
	
	if len(expired) > 0 {
		log.Printf("Cleaned up %d expired pending requests", len(expired))
	}
}

// StartPendingCleanup 启动待处理请求清理协程
func (m *Manager) StartPendingCleanup() {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				m.CleanupExpiredPending(5 * time.Minute)
			case <-m.ctx.Done():
				return
			}
		}
	}()
}

// StartHealthCheck 启动健康检查协程
func (m *Manager) StartHealthCheck() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				m.performHealthCheck()
			case <-m.ctx.Done():
				return
			}
		}
	}()
}



// stringPtr 返回字符串指针
func stringPtr(s string) *string {
	return &s
}