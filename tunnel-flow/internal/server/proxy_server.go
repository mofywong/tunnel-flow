package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"tunnel-flow/internal/config"
	"tunnel-flow/internal/database"
	"tunnel-flow/internal/proxy"
	"tunnel-flow/internal/websocket"
)

// ProxyServer HTTP代理服务器
type ProxyServer struct {
	config    *config.Config
	db        *database.Repository
	handler   *proxy.Handler
	server    *http.Server
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewProxyServer 创建新的代理服务器
func NewProxyServer(cfg *config.Config, db *database.Repository, wsManager *websocket.Manager) *ProxyServer {
	ctx, cancel := context.WithCancel(context.Background())
	
	handler := proxy.NewHandler(db, wsManager)
	
	return &ProxyServer{
		config:  cfg,
		db:      db,
		handler: handler,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start 启动代理服务器
func (s *ProxyServer) Start() error {
	mux := http.NewServeMux()
	
	// 代理路由
	mux.HandleFunc("/proxy/", s.handler.HandleProxyRequest)
	
	// 文件上传路由
	mux.HandleFunc("/upload", s.handler.HandleFileUpload)
	
	// 健康检查
	mux.HandleFunc("/health", s.handleHealth)
	
	// 状态信息
	mux.HandleFunc("/status", s.handleStatus)
	
	// 添加根路径处理器，支持直接访问路由路径
	mux.HandleFunc("/", s.handler.HandleDirectProxyRequest)
	
	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.ProxyPort),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	
	log.Printf("Starting proxy server on port %d", s.config.ProxyPort)
	
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Proxy server error: %v", err)
		}
	}()
	
	return nil
}

// Stop 停止代理服务器
func (s *ProxyServer) Stop() error {
	s.cancel()
	
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		if err := s.server.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down proxy server: %v", err)
			return err
		}
	}
	
	log.Println("Proxy server stopped")
	return nil
}

// handleHealth 健康检查处理器
func (s *ProxyServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","service":"proxy"}`))
}

// handleStatus 状态信息处理器
func (s *ProxyServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	stats := s.handler.GetStats()
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	
	// 简单的JSON编码
	response := fmt.Sprintf(`{
		"service": "proxy",
		"port": %d,
		"connected_clients": %v,
		"total_routes": %v
	}`, s.config.ProxyPort, stats["connected_clients"], stats["total_routes"])
	
	w.Write([]byte(response))
}