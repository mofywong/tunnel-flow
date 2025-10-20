package websocket

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"tunnel-flow/internal/database"
	"tunnel-flow/internal/protocol"
)

// handleControlMessage 处理控制消息
func (m *Manager) handleControlMessage(client *ClientConn, msg *protocol.Message) {
	switch msg.Op {
	case protocol.OpRegister:
		m.handleRegister(client, msg)
	case protocol.OpPong:
		m.handlePong(client, msg)
	case protocol.OpPing:
		m.handlePing(client, msg)
	default:
		log.Printf("Unknown control operation %s from client %s", msg.Op, client.clientID)
	}
}

// handleBusinessMessage 处理业务消息
func (m *Manager) handleBusinessMessage(client *ClientConn, msg *protocol.Message) {
	switch msg.Op {
	case protocol.OpResponse:
		m.handleResponse(client, msg)
	default:
		log.Printf("Unknown business operation %s from client %s", msg.Op, client.clientID)
	}
}

// handleACKMessage 处理ACK消息
func (m *Manager) handleACKMessage(client *ClientConn, msg *protocol.Message) {
	var ackPayload protocol.ACKPayload
	if err := msg.ParsePayload(&ackPayload); err != nil {
		log.Printf("Failed to parse ACK payload from client %s: %v", client.clientID, err)
		return
	}
	
	log.Printf("Received ACK from client %s for message %s: success=%v", 
		client.clientID, ackPayload.MsgID, ackPayload.Success)
	
	// 更新待处理消息状态
	if ackPayload.Success {
		if err := m.db.UpdatePendingMessageState(ackPayload.MsgID, database.MessageStateProcessing); err != nil {
			log.Printf("Failed to update pending message state: %v", err)
		}
	} else {
		// ACK失败，可能需要重试或标记为失败
		log.Printf("Client %s failed to process message %s: %s", 
			client.clientID, ackPayload.MsgID, ackPayload.Message)
	}
}

// handleErrorMessage 处理错误消息
func (m *Manager) handleErrorMessage(client *ClientConn, msg *protocol.Message) {
	var errorPayload protocol.ErrorPayload
	if err := msg.ParsePayload(&errorPayload); err != nil {
		log.Printf("Failed to parse error payload from client %s: %v", client.clientID, err)
		return
	}
	
	log.Printf("Received error from client %s: %s - %s", 
		client.clientID, errorPayload.Code, errorPayload.Message)
	
	// 如果有关联的消息ID，更新其状态
	if msg.MsgID != nil {
		if err := m.db.UpdatePendingMessageState(*msg.MsgID, database.MessageStateFailed); err != nil {
			log.Printf("Failed to update pending message state: %v", err)
		}
		
		// 通知等待的请求
		m.mu.RLock()
		if pending, exists := m.pending[*msg.MsgID]; exists {
			select {
			case pending.resultCh <- &protocol.ResponsePayload{
				HTTPStatus: 500,
				Error:      &errorPayload.Message,
			}:
			default:
			}
		}
		m.mu.RUnlock()
	}
}

// handleRegister 处理客户端注册
func (m *Manager) handleRegister(client *ClientConn, msg *protocol.Message) {
	var registerPayload protocol.RegisterPayload
	if err := msg.ParsePayload(&registerPayload); err != nil {
		log.Printf("Failed to parse register payload from client %s: %v", client.clientID, err)
		m.sendRegisterResponse(client, false, "Invalid register payload")
		return
	}
	
	// 验证客户端
	clientInfo, err := m.db.GetClient(client.clientID)
	if err != nil {
		log.Printf("Client %s not found in database: %v", client.clientID, err)
		m.sendRegisterResponse(client, false, "Client not found")
		return
	}
	
	// 验证客户端是否启用
	if clientInfo.Enabled != 1 {
		log.Printf("Client %s is disabled", client.clientID)
		m.sendRegisterResponse(client, false, "Client is disabled")
		return
	}
	
	// 验证auth token
	if registerPayload.AuthToken == "" {
		log.Printf("Client %s provided empty auth token", client.clientID)
		m.sendRegisterResponse(client, false, "Invalid auth token")
		return
	}
	
	// 验证authtoken
	if clientInfo.AuthToken != registerPayload.AuthToken {
		log.Printf("Client %s provided invalid auth token", client.clientID)
		m.sendRegisterResponse(client, false, "Invalid auth token")
		return
	}
	
	// 注册成功
	log.Printf("Client %s registered successfully with version %s", 
		client.clientID, registerPayload.Version)
	
	// 更新客户端状态
	if err := m.db.UpdateClientStatus(client.clientID, "online"); err != nil {
		log.Printf("Failed to update client status: %v", err)
	}
	
	// 保存客户端本地IP地址信息
	if len(registerPayload.LocalIPs) > 0 {
		localIPsJSON, err := json.Marshal(registerPayload.LocalIPs)
		if err != nil {
			log.Printf("Failed to marshal local IPs for client %s: %v", client.clientID, err)
		} else {
			if err := m.db.UpdateClientLocalIPs(client.clientID, string(localIPsJSON)); err != nil {
				log.Printf("Failed to update client local IPs: %v", err)
			} else {
				log.Printf("Updated local IPs for client %s: %v", client.clientID, registerPayload.LocalIPs)
			}
		}
	}
	
	m.sendRegisterResponse(client, true, "Registration successful")
	
	// 发送路由同步（如果需要）
	// 在优化版本中，路由信息直接注入到请求消息中，不需要单独同步
}

// handleResponse 处理响应消息
func (m *Manager) handleResponse(client *ClientConn, msg *protocol.Message) {
	if msg.MsgID == nil {
		log.Printf("Response message from client %s missing msg_id", client.clientID)
		return
	}
	
	// 打印接收到的原始WebSocket消息
	log.Printf("[WebSocket Receive] Raw message from client %s: %+v", client.clientID, msg)
	
	var responsePayload protocol.ResponsePayload
	if err := msg.ParsePayload(&responsePayload); err != nil {
		log.Printf("Failed to parse response payload from client %s: %v", client.clientID, err)
		return
	}
	
	msgID := *msg.MsgID
	log.Printf("[WebSocket Receive] Parsed response from client %s for message %s: status=%d, latency=%dms, body_length=%d", 
		client.clientID, msgID, responsePayload.HTTPStatus, responsePayload.LatencyMS, len(fmt.Sprintf("%v", responsePayload.Body)))
	
	// 打印响应体内容（前200个字符）
	if responsePayload.Body != nil {
		bodyStr := fmt.Sprintf("%v", responsePayload.Body)
		if len(bodyStr) > 200 {
			bodyStr = bodyStr[:200] + "..."
		}
		log.Printf("[WebSocket Receive] Response body preview: %s", bodyStr)
	}
	
	// 更新数据库中的待处理消息
	responseMeta := &database.ResponseMeta{
		HTTPStatus: responsePayload.HTTPStatus,
		Headers:    responsePayload.Headers,
		Body:       responsePayload.Body,
		LatencyMS:  responsePayload.LatencyMS,
		Error:      responsePayload.Error,
	}
	
	responseMetaJSON, err := json.Marshal(responseMeta)
	if err != nil {
		log.Printf("Failed to marshal response meta: %v", err)
		return
	}
	
	state := database.MessageStateDone
	if responsePayload.HTTPStatus >= 400 || responsePayload.Error != nil {
		state = database.MessageStateFailed
	}
	
	if err := m.db.UpdatePendingMessageResponse(msgID, state, string(responseMetaJSON)); err != nil {
		log.Printf("Failed to update pending message response: %v", err)
	}
	
	// 通知等待的请求
	m.mu.RLock()
	log.Printf("[WebSocket Receive] Looking for pending request with msgID: %s", msgID)
	if pending, exists := m.pending[msgID]; exists {
		log.Printf("[WebSocket Receive] Found pending request for msgID: %s, attempting to notify waiting goroutine", msgID)
		select {
		case pending.resultCh <- &responsePayload:
			log.Printf("[WebSocket Receive] Successfully notified waiting goroutine for msgID: %s", msgID)
		default:
			log.Printf("[WebSocket Receive] Failed to send response to pending request %s - channel blocked or closed", msgID)
		}
	} else {
		log.Printf("[WebSocket Receive] No pending request found for msgID: %s", msgID)
		// 打印当前所有pending请求的ID
		pendingIDs := make([]string, 0, len(m.pending))
		for id := range m.pending {
			pendingIDs = append(pendingIDs, id)
		}
		log.Printf("[WebSocket Receive] Current pending request IDs: %v", pendingIDs)
	}
	m.mu.RUnlock()
}

// handlePong 处理Pong消息
func (m *Manager) handlePong(client *ClientConn, msg *protocol.Message) {
	var pongPayload protocol.PongPayload
	if err := msg.ParsePayload(&pongPayload); err != nil {
		log.Printf("Failed to parse pong payload from client %s: %v", client.clientID, err)
		return
	}
	
	// 计算延迟
	now := time.Now().UnixMilli()
	latency := now - pongPayload.Timestamp
	
	log.Printf("Received pong from client %s, latency: %dms", client.clientID, latency)
	
	// 更新客户端最后活跃时间
	client.mu.Lock()
	client.lastSeen = time.Now()
	client.mu.Unlock()
	
	// 发送心跳更新到队列（非阻塞）
	select {
	case m.heartbeatQueue <- HeartbeatUpdate{
		ClientID:       client.clientID,
		LastActiveTime: time.Now(),
	}:
	default:
		// 队列满时丢弃，避免阻塞
		log.Printf("Heartbeat queue full, dropping update for client %s", client.clientID)
	}
}

// sendRegisterResponse 发送注册响应
func (m *Manager) sendRegisterResponse(client *ClientConn, success bool, message string) {
	responsePayload := &protocol.RegisterAckPayload{
		Success: success,
		Message: message,
	}
	
	responseMsg, err := protocol.NewMessage(
		protocol.MessageTypeControl,
		protocol.OpRegisterAck,
		client.clientID,
		nil,
		responsePayload,
	)
	if err != nil {
		log.Printf("Failed to create register response: %v", err)
		return
	}
	
	if err := m.SendToClient(client.clientID, responseMsg); err != nil {
		log.Printf("Failed to send register response to client %s: %v", client.clientID, err)
	}
}

// sendACK 发送ACK消息
func (m *Manager) sendACK(clientID, msgID string, success bool, message string) {
	ackPayload := &protocol.ACKPayload{
		MsgID:   msgID,
		Success: success,
		Message: message,
	}
	
	ackMsg, err := protocol.NewMessage(
		protocol.MessageTypeACK,
		protocol.OpACK,
		clientID,
		&msgID,
		ackPayload,
	)
	if err != nil {
		log.Printf("Failed to create ACK message: %v", err)
		return
	}
	
	if err := m.SendToClient(clientID, ackMsg); err != nil {
		log.Printf("Failed to send ACK to client %s: %v", clientID, err)
	}
}

// handlePing 处理Ping消息
func (m *Manager) handlePing(client *ClientConn, msg *protocol.Message) {
	var pingPayload protocol.PingPayload
	
	// 如果payload为空或解析失败，使用默认值
	if len(msg.Payload) == 0 {
		log.Printf("Received ping from client %s (empty payload)", client.clientID)
		pingPayload.Timestamp = time.Now().Unix()
	} else if err := msg.ParsePayload(&pingPayload); err != nil {
		log.Printf("Failed to parse ping payload from client %s, using default: %v", client.clientID, err)
		pingPayload.Timestamp = time.Now().Unix()
	} else {
		log.Printf("Received ping from client %s", client.clientID)
	}
	
	// 更新客户端最后活跃时间
	now := time.Now()
	client.mu.Lock()
	client.lastSeen = now
	client.mu.Unlock()
	
	// 发送心跳更新到队列（非阻塞）
	select {
	case m.heartbeatQueue <- HeartbeatUpdate{
		ClientID:       client.clientID,
		LastActiveTime: now,
	}:
	default:
		// 队列满时丢弃，避免阻塞
		log.Printf("Heartbeat queue full, dropping update for client %s", client.clientID)
	}
	
	// 发送Pong响应
	m.sendPong(client.clientID, pingPayload.Timestamp)
}

// sendPong 发送Pong消息
func (m *Manager) sendPong(clientID string, timestamp int64) {
	pongPayload := &protocol.PongPayload{
		Timestamp: timestamp,
	}
	
	pongMsg, err := protocol.NewMessage(
		protocol.MessageTypeControl,
		protocol.OpPong,
		clientID,
		nil,
		pongPayload,
	)
	if err != nil {
		log.Printf("Failed to create pong message: %v", err)
		return
	}
	
	if err := m.SendToClient(clientID, pongMsg); err != nil {
		log.Printf("Failed to send pong to client %s: %v", clientID, err)
	}
}

// sendError 发送错误消息
func (m *Manager) sendError(clientID string, msgID *string, code, message, details string) {
	errorPayload := &protocol.ErrorPayload{
		Code:    code,
		Message: message,
		Details: details,
	}
	
	errorMsg, err := protocol.NewMessage(
		protocol.MessageTypeError,
		protocol.OpError,
		clientID,
		msgID,
		errorPayload,
	)
	if err != nil {
		log.Printf("Failed to create error message: %v", err)
		return
	}
	
	if err := m.SendToClient(clientID, errorMsg); err != nil {
		log.Printf("Failed to send error to client %s: %v", clientID, err)
	}
}

// BroadcastToClients 广播消息到多个客户端
func (m *Manager) BroadcastToClients(clientIDs []string, msg *protocol.Message) map[string]error {
	results := make(map[string]error)
	
	for _, clientID := range clientIDs {
		if err := m.SendToClient(clientID, msg); err != nil {
			results[clientID] = err
		} else {
			results[clientID] = nil
		}
	}
	
	return results
}

// GetRouteClients 根据URL后缀获取路由的客户端列表
func (m *Manager) GetRouteClients(urlSuffix string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if clients, exists := m.routeIndex[urlSuffix]; exists {
		return append([]string(nil), clients...) // 返回副本
	}
	return nil
}

// IsClientConnected 检查客户端是否已连接
func (m *Manager) IsClientConnected(clientID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	_, exists := m.clients[clientID]
	return exists
}

// GetClientLastSeen 获取客户端最后活跃时间
func (m *Manager) GetClientLastSeen(clientID string) (time.Time, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if client, exists := m.clients[clientID]; exists {
		client.mu.Lock()
		lastSeen := client.lastSeen
		client.mu.Unlock()
		return lastSeen, true
	}
	return time.Time{}, false
}

// handleDataMessage 处理数据消息
func (m *Manager) handleDataMessage(client *ClientConn, msg *protocol.Message) {
	switch msg.Op {
	case protocol.OpRequest:
		// 处理请求消息 - 转发到目标客户端
		log.Printf("Handling request message from client %s", client.clientID)
	case protocol.OpResponse:
		// 处理响应消息 - 转发到原始客户端
		log.Printf("Handling response message from client %s", client.clientID)
		m.handleResponse(client, msg)
	default:
		log.Printf("Unknown data operation: %s", msg.Op)
	}
}