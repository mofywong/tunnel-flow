package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"time"

	"tunnel-flow/internal/config"
	"tunnel-flow/internal/websocket"
)

// WebSocketServer WebSocket服务器
type WebSocketServer struct {
	config    *config.Config
	wsManager *websocket.Manager
	server    *http.Server
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewWebSocketServer 创建新的WebSocket服务器
func NewWebSocketServer(cfg *config.Config, wsManager *websocket.Manager) *WebSocketServer {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &WebSocketServer{
		config:    cfg,
		wsManager: wsManager,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start 启动WebSocket服务器
func (s *WebSocketServer) Start() error {
	mux := http.NewServeMux()
	
	// WebSocket连接路由
	mux.HandleFunc("/ws", s.wsManager.HandleWebSocket)
	
	// 健康检查
	mux.HandleFunc("/health", s.handleHealth)
	
	// 状态信息
	mux.HandleFunc("/status", s.handleStatus)
	
	s.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.config.ServerHost, s.config.WebSocketPort),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	
	go func() {
		// 检查是否启用 SSL/TLS
		if s.config.WebSocketSSLEnabled {
			// 配置 TLS
			tlsConfig := &tls.Config{
				MinVersion: tls.VersionTLS12, // 强制使用 TLS 1.2 或更高版本
			}
			
			// 如果强制 SSL，禁用不安全的连接
			if s.config.WebSocketSSLForceSSL {
				tlsConfig.InsecureSkipVerify = false
			}
			
			s.server.TLSConfig = tlsConfig
			
			log.Printf("WebSocket Secure (WSS) server starting on %s:%d", s.config.ServerHost, s.config.WebSocketPort)
			log.Printf("Using SSL certificate: %s", s.config.WebSocketSSLCertFile)
			log.Printf("Using SSL key: %s", s.config.WebSocketSSLKeyFile)
			log.Printf("Force SSL enabled: %v", s.config.WebSocketSSLForceSSL)
			
			if err := s.server.ListenAndServeTLS(s.config.WebSocketSSLCertFile, s.config.WebSocketSSLKeyFile); err != nil && err != http.ErrServerClosed {
				log.Printf("WebSocket server error: %v", err)
			}
		} else {
			log.Printf("WebSocket server starting on %s:%d (non-secure)", s.config.ServerHost, s.config.WebSocketPort)
			if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("WebSocket server error: %v", err)
			}
		}
	}()
	
	return nil
}

// Stop 停止WebSocket服务器
func (s *WebSocketServer) Stop() error {
	s.cancel()
	
	// 关闭WebSocket Manager
	if s.wsManager != nil {
		s.wsManager.Close()
		log.Println("WebSocket manager closed")
	}
	
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		if err := s.server.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down WebSocket server: %v", err)
			return err
		}
	}
	
	log.Println("WebSocket server stopped")
	return nil
}

// handleHealth 健康检查处理器
func (s *WebSocketServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","service":"websocket"}`))
}

// handleStatus 状态信息处理器
func (s *WebSocketServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	connectedCount := s.wsManager.GetConnectedClientCount()
	pendingCount := s.wsManager.GetPendingRequestCount()
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	
	// 简单的JSON编码
	response := fmt.Sprintf(`{
		"service": "websocket",
		"port": %d,
		"connected_clients": %d,
		"pending_requests": %d
	}`, s.config.WebSocketPort, connectedCount, pendingCount)
	
	w.Write([]byte(response))
}