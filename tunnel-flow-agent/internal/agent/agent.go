package agent

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"tunnel-flow-agent/internal/config"
	"tunnel-flow-agent/internal/protocol"
	"tunnel-flow-agent/internal/retry"
	wsconnector "tunnel-flow-agent/internal/websocket"
)

// Agent 客户端代理
type Agent struct {
	config      *config.Config
	conn        *websocket.Conn
	sendQueue   chan []byte
	retryCount  int
	isConnected bool
	running     int32
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	stopCh      chan struct{}
	connectionMu sync.Mutex
	writeMu     sync.Mutex  // WebSocket写入锁
	
	// WebSocket连接器
	connector *wsconnector.Connector
	
	// 性能优化组件
	retryStrategy *retry.RetryStrategy
	messagePool   sync.Pool
	bufferPool    sync.Pool
	workerPool    chan struct{}
	
	// 统计信息
	stats struct {
		messagesSent     int64
		messagesReceived int64
		reconnectCount   int64
		errorCount       int64
		startTime        time.Time
		mu               sync.RWMutex
	}
	
	// 连接相关字段
	reconnectCount   int64
	connMu          sync.RWMutex
	lastConnectTime time.Time
	
	// 网络质量相关字段
	qualityMu       sync.RWMutex
	lastPongTime    time.Time
	lastPingTime    time.Time
	rtt             time.Duration
	pongsReceived   int64
	pingsSent       int64
	networkQuality  float64
}

// NewAgent 创建新的代理实例
func NewAgent(cfg *config.Config) *Agent {
	ctx, cancel := context.WithCancel(context.Background())
	
	agent := &Agent{
		config:    cfg,
		sendQueue: make(chan []byte, cfg.SendQueueSize()),
		ctx:       ctx,
		cancel:    cancel,
		stopCh:    make(chan struct{}),
		workerPool: make(chan struct{}, cfg.WorkerPoolSize()),
	}
	
	// 初始化重试策略
	retryConfig := &retry.RetryConfig{
		MaxAttempts:  cfg.MaxRetries(),
		InitialDelay: time.Duration(cfg.RetryDelayMS()) * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
	}
	agent.retryStrategy = retry.NewRetryStrategy(retryConfig)
	
	// 初始化对象池
	agent.messagePool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 0, 1024)
		},
	}
	
	agent.bufferPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 4096)
		},
	}
	
	// 初始化工作池
	for i := 0; i < cfg.WorkerPoolSize(); i++ {
		agent.workerPool <- struct{}{}
	}
	
	// 创建WebSocket连接器
	agent.connector = wsconnector.NewConnector(cfg, agent)
	
	// 初始化统计信息
	agent.stats.startTime = time.Now()
	
	return agent
}

// Start 启动代理
func (a *Agent) Start() error {
	log.Println("启动客户端代理...")
	
	// 启动连接循环
	a.wg.Add(1)
	go a.connectionLoop()
	
	return nil
}

// Stop 停止代理
func (a *Agent) Stop() {
	log.Println("停止客户端代理...")
	
	// 取消context
	a.cancel()
	
	// 关闭stopCh通道，通知所有goroutine退出
	select {
	case <-a.stopCh:
		// 通道已经关闭
	default:
		close(a.stopCh)
	}
	
	// 关闭WebSocket连接
	a.connMu.Lock()
	if a.conn != nil {
		a.conn.Close()
		a.conn = nil
	}
	a.connMu.Unlock()
	
	// 等待所有goroutine退出
	a.wg.Wait()
	log.Println("客户端代理已停止")
}

// IsRunning 检查代理是否正在运行
func (a *Agent) IsRunning() bool {
	return atomic.LoadInt32(&a.running) == 1
}

// connectionLoop 连接循环
func (a *Agent) connectionLoop() {
	defer a.wg.Done()

	for {
		select {
		case <-a.stopCh:
			return
		default:
		}

		// 尝试连接
		err := a.connectWithRetry()
		if err != nil {
			log.Printf("连接失败: %v", err)
			
			// 等待重连间隔
			select {
			case <-a.stopCh:
				return
			case <-time.After(a.config.ReconnectInterval()):
				continue
			}
		}

		// 运行连接
		a.runConnection()
	}
}

// connectWithRetry 带重试的连接
func (a *Agent) connectWithRetry() error {
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return a.retryStrategy.ExecuteWithRetry(ctx, func() error {
		return a.connect()
	})
}

// connect 连接到服务器
func (a *Agent) connect() error {
	log.Printf("尝试连接到服务器...")
	log.Printf("配置信息 - ServerURL: %s, ClientID: %s, AuthToken: %s",
		a.config.ServerURL(), a.config.ClientID(), a.config.AuthToken())
	
	u, err := url.Parse(a.config.ServerURL())
	if err != nil {
		log.Printf("解析服务器URL失败: %v", err)
		return fmt.Errorf("解析服务器URL失败: %w", err)
	}

	// 添加认证参数
	q := u.Query()
	q.Set("client_id", a.config.ClientID())
	q.Set("token", a.config.AuthToken())
	u.RawQuery = q.Encode()

	log.Printf("准备连接到WebSocket URL: %s", u.String())

	// 创建WebSocket连接
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	
	// 检查是否为 WSS 连接，配置 TLS
	if u.Scheme == "wss" {
		log.Printf("检测到 WSS 协议，配置 TLS 安全连接")
		dialer.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,                    // 强制使用 TLS 1.2 或更高版本
			InsecureSkipVerify: a.config.SSLInsecureSkipVerify(),   // 根据配置决定是否跳过证书验证
			ServerName:         u.Hostname(),                       // 设置服务器名称用于证书验证
		}
		log.Printf("TLS 配置完成，最小版本: TLS 1.2，服务器名称: %s，跳过证书验证: %v", 
			u.Hostname(), a.config.SSLInsecureSkipVerify())
	}

	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		atomic.AddInt64(&a.reconnectCount, 1)
		log.Printf("WebSocket连接失败: %v", err)
		return fmt.Errorf("连接WebSocket失败: %w", err)
	}

	a.connMu.Lock()
	a.conn = conn
	a.lastConnectTime = time.Now()
	a.connMu.Unlock()

	// 初始化心跳相关时间戳
	a.qualityMu.Lock()
	a.lastPongTime = time.Now() // 初始化为连接时间，避免首次心跳检测误判
	a.lastPingTime = time.Time{} // 重置ping时间
	a.qualityMu.Unlock()

	log.Printf("已连接到服务器: %s", a.config.ServerURL())
	
	// 发送注册消息
	err = a.sendRegisterMessage()
	if err != nil {
		log.Printf("发送注册消息失败: %v", err)
		conn.Close()
		return fmt.Errorf("发送注册消息失败: %w", err)
	}
	
	return nil
}

// runConnection 运行连接
func (a *Agent) runConnection() {
	// 启动心跳
	a.wg.Add(1)
	go a.heartbeatLoop()

	// 处理消息
	for {
		select {
		case <-a.stopCh:
			return
		default:
		}

		a.connMu.RLock()
		conn := a.conn
		a.connMu.RUnlock()

		if conn == nil {
			return
		}

		var msg protocol.Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			log.Printf("读取消息失败: %v", err)
			return
		}

		a.handleMessage(&msg)
	}
}

// sendMessageWithRetry 带重试的消息发送
func (a *Agent) sendMessageWithRetry(msg *protocol.Message) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return a.retryStrategy.ExecuteWithRetry(ctx, func() error {
		a.connMu.RLock()
		conn := a.conn
		a.connMu.RUnlock()

		if conn == nil {
			return fmt.Errorf("连接不可用")
		}

		// 使用写入锁防止并发写入
		a.writeMu.Lock()
		defer a.writeMu.Unlock()
		
		return conn.WriteJSON(msg)
	})
}

// handleMessage 处理消息
func (a *Agent) handleMessage(msg *protocol.Message) {
	switch msg.Op {
	case protocol.OpPing:
		a.handlePing(msg)
	case protocol.OpPong:
		a.handlePong(msg)
	case protocol.OpRouteSync:
		a.handleRouteSync(msg)
	case protocol.OpRequest:
		a.handleRequest(msg)
	case protocol.OpError:
		a.handleError(msg)
	default:
		log.Printf("未知操作类型: %s", msg.Op)
	}
}

// handlePing 处理Ping消息
func (a *Agent) handlePing(msg *protocol.Message) {
	// 发送Pong响应
	pongMsg := &protocol.Message{
		Type:      protocol.MessageTypeControl,
		Op:        protocol.OpPong,
		ClientID:  a.config.ClientID(),
		MsgID:     msg.MsgID,
		Timestamp: time.Now().UnixMilli(),
		Payload:   &protocol.PongPayload{Timestamp: time.Now().UnixMilli()},
	}

	if err := a.sendMessageWithRetry(pongMsg); err != nil {
		log.Printf("发送Pong失败: %v", err)
	}
}

// handlePong 处理Pong消息
func (a *Agent) handlePong(msg *protocol.Message) {
	a.qualityMu.Lock()
	defer a.qualityMu.Unlock()

	now := time.Now()
	a.lastPongTime = now
	
	// 计算RTT
	if !a.lastPingTime.IsZero() {
		a.rtt = now.Sub(a.lastPingTime)
		atomic.AddInt64(&a.pongsReceived, 1)
		
		// 更新网络质量
		a.updateNetworkQuality()
		
		log.Printf("收到pong响应，RTT: %v, 网络质量: %.2f", a.rtt, a.networkQuality)
	} else {
		log.Printf("收到pong响应")
	}
}

// updateNetworkQuality 更新网络质量
func (a *Agent) updateNetworkQuality() {
	// 基于RTT计算网络质量
	rttMs := float64(a.rtt.Milliseconds())
	
	var quality float64
	switch {
	case rttMs <= 50:
		quality = 1.0 // 优秀
	case rttMs <= 100:
		quality = 0.8 // 良好
	case rttMs <= 200:
		quality = 0.6 // 一般
	case rttMs <= 500:
		quality = 0.4 // 较差
	default:
		quality = 0.2 // 很差
	}
	
	// 平滑更新
	a.networkQuality = a.networkQuality*0.7 + quality*0.3
}

// handleRouteSync 处理路由同步
func (a *Agent) handleRouteSync(msg *protocol.Message) {
	log.Printf("收到路由同步: %+v", msg.Payload)
}

// handleRequest 处理HTTP请求
func (a *Agent) handleRequest(msg *protocol.Message) {
	// 解析请求数据为RequestPayload结构
	var reqPayload protocol.RequestPayload
	
	// 首先尝试直接解析Payload
	if err := msg.ParsePayload(&reqPayload); err != nil {
		log.Printf("解析RequestPayload失败: %v", err)
		a.sendErrorResponse(msg, "解析请求数据失败")
		return
	}

	log.Printf("收到请求: Method=%s, URLSuffix=%s", reqPayload.HTTPMethod, reqPayload.URLSuffix)

	// 解析目标地址
	targets, err := reqPayload.GetTargets()
	if err != nil {
		log.Printf("解析目标地址失败: %v", err)
		a.sendErrorResponse(msg, "解析目标地址失败")
		return
	}

	if len(targets) == 0 {
		log.Printf("没有可用的目标地址")
		a.sendErrorResponse(msg, "没有可用的目标地址")
		return
	}

	// 直接使用目标地址，不拼接URL后缀
		targetURL := targets[0].URL
		log.Printf("路由转发：直接转发到目标地址: %s (模式: %s)", targetURL, reqPayload.RouteMode)

	// 创建HTTP客户端
	timeout := time.Duration(reqPayload.Timeout) * time.Millisecond
	if timeout == 0 {
		timeout = a.config.HTTPTimeout()
	}
	
	// 检测目标地址是否为HTTPS协议
	isHTTPS := strings.HasPrefix(strings.ToLower(targets[0].URL), "https://")
	
	client := &http.Client{
		Timeout: timeout,
	}
	
	// 如果是HTTPS协议，配置忽略证书校验
	if isHTTPS {
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
		client.Transport = transport
		log.Printf("目标地址使用HTTPS协议，已配置忽略证书校验: %s", targets[0].URL)
	}

	// 构建请求体
	var reqBody io.Reader
	if reqPayload.Body != "" {
		reqBody = strings.NewReader(reqPayload.Body)
	}

	// 构建HTTP请求
	req, err := http.NewRequest(reqPayload.HTTPMethod, targetURL, reqBody)
	if err != nil {
		log.Printf("创建HTTP请求失败: %v", err)
		a.sendErrorResponse(msg, "创建HTTP请求失败")
		return
	}

	// 设置请求头
	for name, value := range reqPayload.Headers {
		req.Header.Set(name, value)
	}

	log.Printf("发送HTTP请求到: %s", targetURL)

	// 发送请求
	startTime := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(startTime)
	
	if err != nil {
		log.Printf("发送HTTP请求失败: %v", err)
		a.sendErrorResponse(msg, fmt.Sprintf("HTTP请求失败: %v", err))
		return
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("读取响应体失败: %v", err)
		a.sendErrorResponse(msg, "读取响应体失败")
		return
	}

	// 构建响应头
	respHeaders := make(map[string]string)
	for name, values := range resp.Header {
		if len(values) > 0 {
			respHeaders[name] = values[0]
		}
	}

	// 构建响应载荷
	responsePayload := &protocol.ResponsePayload{
		HTTPStatus: resp.StatusCode,
		Headers:    respHeaders,
		Body:       string(respBody),
		LatencyMS:  latency.Milliseconds(),
	}

	// 发送响应
	responseMsg := &protocol.Message{
		MsgID:     msg.MsgID,
		Type:      protocol.MessageTypeBusiness,
		Op:        protocol.OpResponse,
		ClientID:  a.config.ClientID(),
		Timestamp: time.Now().UnixMilli(),
		Payload:   responsePayload,
	}

	if err := a.sendMessageWithRetry(responseMsg); err != nil {
		log.Printf("发送响应失败: %v", err)
	} else {
		log.Printf("成功处理请求，状态码: %d, 延迟: %dms", resp.StatusCode, latency.Milliseconds())
	}
}

// sendErrorResponse 发送错误响应
func (a *Agent) sendErrorResponse(msg *protocol.Message, errorMsg string) {
	errorPayload := &protocol.ResponsePayload{
		HTTPStatus: 500,
		Headers:    make(map[string]string),
		Body:       "",
		LatencyMS:  int64(0),
		Error:      &errorMsg,
	}

	responseMsg := &protocol.Message{
		MsgID:     msg.MsgID,
		Type:      protocol.MessageTypeBusiness,
		Op:        protocol.OpResponse,
		ClientID:  a.config.ClientID(),
		Timestamp: time.Now().UnixMilli(),
		Payload:   errorPayload,
	}

	if err := a.sendMessageWithRetry(responseMsg); err != nil {
		log.Printf("发送错误响应失败: %v", err)
	}
}

// handleError 处理错误消息
func (a *Agent) handleError(msg *protocol.Message) {
	log.Printf("收到错误消息: %s", msg.Payload)
}

// heartbeatLoop 心跳循环
func (a *Agent) heartbeatLoop() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.PingInterval())
	defer ticker.Stop()
	
	// 记录连续失败的ping次数
	consecutiveFailures := 0
	maxFailures := 3 // 允许连续失败3次

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			// 检查连接超时
			a.qualityMu.RLock()
			lastPong := a.lastPongTime
			quality := a.networkQuality
			a.qualityMu.RUnlock()

			// 根据网络质量调整心跳间隔
			interval := a.config.PingInterval()
			if quality < 0.5 && quality > 0 {
				interval = interval * 2 / 3 // 网络质量差时适当增加心跳频率
			}
			ticker.Reset(interval)

			// 使用配置的心跳超时时间进行检查
			pingTimeout := a.config.PingTimeout()
			if !lastPong.IsZero() && time.Since(lastPong) > pingTimeout {
				log.Printf("心跳超时，上次pong时间: %v, 超时阈值: %v", lastPong, pingTimeout)
				
				// 主动关闭WebSocket连接，触发重连
				a.connMu.Lock()
				if a.conn != nil {
					log.Printf("主动关闭WebSocket连接以触发重连")
					a.conn.Close()
					a.conn = nil
				}
				a.connMu.Unlock()
				
				return
			}

			// 发送ping
			if err := a.sendPing(); err != nil {
				consecutiveFailures++
				log.Printf("发送ping失败 (%d/%d): %v", consecutiveFailures, maxFailures, err)
				
				// 连续失败达到阈值时才断开连接
				if consecutiveFailures >= maxFailures {
					log.Printf("连续ping失败%d次，主动断开连接", maxFailures)
					
					// 主动关闭WebSocket连接，触发重连
					a.connMu.Lock()
					if a.conn != nil {
						log.Printf("主动关闭WebSocket连接以触发重连")
						a.conn.Close()
						a.conn = nil
					}
					a.connMu.Unlock()
					
					return
				}
			} else {
				// 成功发送ping，重置失败计数
				consecutiveFailures = 0
			}
		}
	}
}

// sendPing 发送ping消息
func (a *Agent) sendPing() error {
	a.qualityMu.Lock()
	a.lastPingTime = time.Now()
	pingCount := atomic.AddInt64(&a.pingsSent, 1)
	a.qualityMu.Unlock()

	pingMsg := &protocol.Message{
		Type:      protocol.MessageTypeControl,
		Op:        protocol.OpPing,
		ClientID:  a.config.ClientID(),
		Timestamp: time.Now().UnixMilli(),
		Payload:   &protocol.PingPayload{Timestamp: time.Now().UnixMilli()},
	}

	log.Printf("发送ping消息 #%d", pingCount)
	err := a.sendMessageWithRetry(pingMsg)
	if err != nil {
		log.Printf("ping消息发送失败: %v", err)
	}
	return err
}

// GetStats 获取统计信息
func (a *Agent) GetStats() map[string]interface{} {
	a.stats.mu.RLock()
	defer a.stats.mu.RUnlock()

	return map[string]interface{}{
		"running":           a.IsRunning(),
		"messages_sent":     a.stats.messagesSent,
		"messages_received": a.stats.messagesReceived,
		"reconnect_count":   a.stats.reconnectCount,
		"error_count":       a.stats.errorCount,
		"start_time":        a.stats.startTime.Format(time.RFC3339),
	}
}

// 事件处理方法
func (a *Agent) onConnected() {
	log.Println("WebSocket连接已建立")
	atomic.StoreInt32(&a.running, 1)
}

func (a *Agent) onDisconnected(err error) {
	log.Printf("WebSocket连接已断开: %v", err)
	atomic.StoreInt32(&a.running, 0)
}

func (a *Agent) onError(err error) {
	log.Printf("WebSocket连接错误: %v", err)
	a.stats.mu.Lock()
	a.stats.errorCount++
	a.stats.mu.Unlock()
}

// HandleMessage 实现MessageHandler接口
func (a *Agent) HandleMessage(msg *wsconnector.Message) error {
	a.stats.mu.Lock()
	a.stats.messagesReceived++
	a.stats.mu.Unlock()
	
	log.Printf("收到消息: %s", msg.Type)
	return nil
}

// getLocalIPs 获取本地网卡IP地址列表
func (a *Agent) getLocalIPs() []string {
	var ips []string
	
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Printf("获取网络接口失败: %v", err)
		return ips
	}
	
	for _, iface := range interfaces {
		// 跳过回环接口和未启用的接口
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		
		addrs, err := iface.Addrs()
		if err != nil {
			log.Printf("获取接口 %s 地址失败: %v", iface.Name, err)
			continue
		}
		
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			
			// 只获取IPv4地址，跳过回环地址
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				ips = append(ips, ip.String())
			}
		}
	}
	
	log.Printf("获取到本地IP地址: %v", ips)
	return ips
}

// sendRegisterMessage 发送注册消息
func (a *Agent) sendRegisterMessage() error {
	// 获取本地IP地址
	localIPs := a.getLocalIPs()
	
	// 创建注册载荷
	payload := protocol.RegisterPayload{
		ClientID:  a.config.ClientID(),
		AuthToken: a.config.AuthToken(),
		Version:   "1.0.0", // 可以从配置或常量获取
		LocalIPs:  localIPs,
	}
	
	// 创建注册消息
	msg := protocol.Message{
		Type:      protocol.MessageTypeControl,
		Op:        protocol.OpRegister,
		ClientID:  a.config.ClientID(),
		Timestamp: time.Now().Unix(),
	}
	
	// 直接设置载荷对象，让WebSocket库自动序列化
	msg.Payload = payload
	
	// 发送消息
	a.connMu.RLock()
	conn := a.conn
	a.connMu.RUnlock()
	
	if conn == nil {
		return fmt.Errorf("连接不可用")
	}
	
	log.Printf("发送注册消息: ClientID=%s, LocalIPs=%v",
		payload.ClientID, payload.LocalIPs)
	
	err := conn.WriteJSON(msg)
	if err != nil {
		return fmt.Errorf("发送注册消息失败: %w", err)
	}
	
	log.Printf("注册消息发送成功")
	return nil
}