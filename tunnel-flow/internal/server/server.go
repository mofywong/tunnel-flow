package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"tunnel-flow/internal/auth"
	"tunnel-flow/internal/config"
	"tunnel-flow/internal/database"
	"tunnel-flow/internal/performance"
	"tunnel-flow/internal/protocol"
	"tunnel-flow/internal/utils"
	"tunnel-flow/internal/web"
	"tunnel-flow/internal/websocket"
)

// Server HTTP服务器
type Server struct {
	config         *config.Config
	db             *database.Repository
	authHandler    *auth.AuthHandler
	wsManager      *websocket.Manager
	server         *http.Server
}

// NewServer 创建新的HTTP服务器
func NewServer(cfg *config.Config, db *database.Repository) *Server {
	// 创建性能优化组件
	objectPool := performance.NewObjectPool()
	workerPool := performance.NewWorkerPool(cfg.WorkerPoolSize, cfg.WorkerQueueSize)
	
	return &Server{
		config:         cfg,
		db:             db,
		authHandler:    auth.NewAuthHandler(cfg),
		wsManager:      websocket.NewManager(cfg, db, objectPool, workerPool, nil),
	}
}

// Start 启动HTTP/HTTPS服务器
func (s *Server) Start() error {
	router := s.setupRoutes()
	
	// 配置CORS
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"}, // 生产环境中应该限制
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"*"},
		AllowCredentials: true,
	})
	
	handler := c.Handler(router)
	
	// 构建服务器地址
	addr := fmt.Sprintf("%s:%d", s.config.ServerHost, s.config.ServerPort)
	
	s.server = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	
	log.Printf("Starting HTTP server on %s", addr)
	return s.server.ListenAndServe()
}

// Stop 停止HTTP服务器
func (s *Server) Stop(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// setupRoutes 设置路由
func (s *Server) setupRoutes() *mux.Router {
	r := mux.NewRouter()
	
	// 获取认证中间件
	authMiddleware := s.authHandler.GetAuthMiddleware()
	
	// API路由
	api := r.PathPrefix("/api/v1").Subrouter()
	
	// 认证相关路由（公开访问）
	api.HandleFunc("/auth/login", s.authHandler.Login).Methods("POST")
	api.HandleFunc("/auth/register", s.authHandler.Register).Methods("POST")
	api.HandleFunc("/auth/profile", s.authHandler.GetProfile).Methods("GET")
	api.HandleFunc("/auth/users", s.authHandler.ListUsers).Methods("GET")
	
	// 系统状态（公开访问）
	api.HandleFunc("/status", s.handleGetStatus).Methods("GET")
	api.HandleFunc("/server-info", s.handleGetServerInfo).Methods("GET")
	
	// 应用认证中间件到需要保护的路由
	protected := api.PathPrefix("").Subrouter()
	protected.Use(authMiddleware.Middleware)
	
	// 客户端管理（需要认证）
	protected.HandleFunc("/clients", s.handleGetClients).Methods("GET")
	protected.HandleFunc("/clients/{id}", s.handleGetClient).Methods("GET")
	protected.HandleFunc("/clients", s.handleCreateClient).Methods("POST")
	protected.HandleFunc("/clients/{id}", s.handleUpdateClient).Methods("PUT")
	protected.HandleFunc("/clients/{id}/status", s.handleUpdateClientStatus).Methods("PUT")
	protected.HandleFunc("/clients/{id}", s.handleDeleteClient).Methods("DELETE")
	
	// 客户端启用状态管理（需要认证）
	protected.HandleFunc("/clients/{id}/enabled", s.handleUpdateClientEnabled).Methods("PUT")
	
	// 路由管理
	protected.HandleFunc("/routes", s.handleGetRoutes).Methods("GET")
	protected.HandleFunc("/routes", s.handleCreateRoute).Methods("POST")
	protected.HandleFunc("/routes/{id}", s.handleGetRoute).Methods("GET")
	protected.HandleFunc("/routes/{id}", s.handleUpdateRoute).Methods("PUT")
	protected.HandleFunc("/routes/{id}", s.handleDeleteRoute).Methods("DELETE")
	
	// 路由启用状态管理
	protected.HandleFunc("/routes/{id}/enabled", s.handleUpdateRouteEnabled).Methods("PUT")
	protected.HandleFunc("/routes/batch/enabled", s.handleBatchUpdateRoutesEnabled).Methods("PUT")
	protected.HandleFunc("/routes/stats", s.handleGetRouteStats).Methods("GET")
	
	// WebSocket连接（agent连接，不需要认证中间件）
	r.HandleFunc("/ws", s.wsManager.HandleWebSocket).Methods("GET")

	// 代理请求处理（核心功能，需要认证）
	r.PathPrefix("/proxy/").Handler(authMiddleware.Middleware(http.HandlerFunc(s.handleProxyRequest)))

	// 静态文件服务（前端）- 使用嵌入的文件系统
	if handler, err := web.GetDistHandler(); err == nil {
		r.PathPrefix("/").Handler(handler).Methods("GET")
	}

	return r
}

// handleProxyRequest 处理代理请求
func (s *Server) handleProxyRequest(w http.ResponseWriter, r *http.Request) {
	// 提取URL后缀
	urlPath := strings.TrimPrefix(r.URL.Path, "/proxy")
	if urlPath == "" {
		http.Error(w, "Invalid proxy path", http.StatusBadRequest)
		return
	}
	
	// 查找路由
	routes, err := s.db.ListServerRoutes()
	if err != nil {
		log.Printf("Failed to get routes for %s: %v", urlPath, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	
	// 过滤匹配的路由（支持通配符）并排除禁用的路由
	matchedRoutes := make([]*database.ServerRoute, 0)
	for _, route := range routes {
		if utils.MatchPattern(route.URLSuffix, urlPath) && route.IsEnabled() {
			matchedRoutes = append(matchedRoutes, route)
		}
	}
	
	// 按优先级排序路由（优先级高的在前）
	if len(matchedRoutes) > 1 {
		sort.Slice(matchedRoutes, func(i, j int) bool {
			priorityI := utils.GetPatternPriority(matchedRoutes[i].URLSuffix)
			priorityJ := utils.GetPatternPriority(matchedRoutes[j].URLSuffix)
			return priorityI > priorityJ
		})
	}
	
	if len(matchedRoutes) == 0 {
		http.Error(w, "Route not found", http.StatusNotFound)
		return
	}
	
	// 选择第一个可用的路由（简化处理，实际应该有负载均衡）
	var selectedRoute *database.ServerRoute
	for _, route := range matchedRoutes {
		// 检查客户端是否连接且启用
		if s.wsManager.IsClientConnected(route.ClientID) {
			// 检查客户端是否启用
			clientInfo, err := s.db.GetClient(route.ClientID)
			if err != nil || clientInfo.Enabled != 1 {
				continue // 跳过禁用的客户端
			}
			selectedRoute = route
			break
		}
	}
	
	if selectedRoute == nil {
		http.Error(w, "No available backend", http.StatusServiceUnavailable)
		return
	}
	
	// 读取请求体
	body := make([]byte, 0)
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
		r.Body.Close()
	}
	
	// 构建请求消息
	requestPayload := &protocol.RequestPayload{
		HTTPMethod:     r.Method,
		URLSuffix:      urlPath,
		Headers:        make(map[string]string),
		Body:           string(body),
		TargetsJSON:    selectedRoute.TargetsJSON,
		DeliveryPolicy: selectedRoute.DeliveryPolicy,
		RouteMode:      selectedRoute.RouteMode,
	}
	
	// 复制请求头
	for name, values := range r.Header {
		if len(values) > 0 {
			requestPayload.Headers[name] = values[0]
		}
	}
	
	// 发送请求并等待响应
	response, err := s.wsManager.SendRequestAndWait(selectedRoute.ClientID, requestPayload, 30*time.Second)
	if err != nil {
		log.Printf("Failed to send request to client %s: %v", selectedRoute.ClientID, err)
		http.Error(w, "Backend request failed", http.StatusBadGateway)
		return
	}
	
	// 设置响应头
	for name, value := range response.Headers {
		w.Header().Set(name, value)
	}
	
	// 设置状态码
	w.WriteHeader(response.HTTPStatus)
	
	// 写入响应体
	if bodyStr, ok := response.Body.(string); ok {
		w.Write([]byte(bodyStr))
	} else if bodyBytes, ok := response.Body.([]byte); ok {
		w.Write(bodyBytes)
	}
	
	// 如果有错误，记录日志
	if response.Error != nil {
		log.Printf("Backend returned error: %s", *response.Error)
	}
}

// 客户端管理API
func (s *Server) handleGetClients(w http.ResponseWriter, r *http.Request) {
	clients, err := s.db.ListClients()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	// 添加在线状态，考虑客户端的启用状态
	for i := range clients {
		// 检查客户端是否启用
		if clients[i].Enabled != 1 {
			// 如果被禁用，状态为禁用
			clients[i].Status = "disabled"
		} else {
			// 启用时，根据WebSocket连接状态设置
			clients[i].Status = "offline"
			if s.wsManager.IsClientConnected(clients[i].ClientID) {
				clients[i].Status = "online"
				if lastSeen, ok := s.wsManager.GetClientLastSeen(clients[i].ClientID); ok {
					clients[i].LastSeen = lastSeen
				}
			}
		}
	}
	
	// 根据查询参数进行筛选
	statusFilter := r.URL.Query().Get("status")
	if statusFilter != "" {
		filteredClients := make([]*database.Client, 0)
		for _, client := range clients {
			switch statusFilter {
			case "online":
				if client.Status == "online" {
					filteredClients = append(filteredClients, client)
				}
			case "offline":
				if client.Status == "offline" {
					filteredClients = append(filteredClients, client)
				}
			case "enabled":
				if client.Enabled == 1 {
					filteredClients = append(filteredClients, client)
				}
			case "disabled":
				if client.Status == "disabled" {
					filteredClients = append(filteredClients, client)
				}
			}
		}
		clients = filteredClients
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(clients)
}

func (s *Server) handleGetClient(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	clientID := vars["id"]
	
	client, err := s.db.GetClient(clientID)
	if err != nil {
		http.Error(w, "Client not found", http.StatusNotFound)
		return
	}
	
	// 添加在线状态，考虑客户端的启用状态
	if client.Enabled != 1 {
		client.Status = "disabled"
	} else {
		client.Status = "offline"
		if s.wsManager.IsClientConnected(client.ClientID) {
			client.Status = "online"
			if lastSeen, ok := s.wsManager.GetClientLastSeen(client.ClientID); ok {
				client.LastSeen = lastSeen
			}
		}
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(client)
}

func (s *Server) handleCreateClient(w http.ResponseWriter, r *http.Request) {
	var client database.Client
	if err := json.NewDecoder(r.Body).Decode(&client); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	
	// 生成客户端ID和认证token
	client.ClientID = generateClientID()
	authToken := generateAuthToken()
	client.AuthToken = authToken // 直接存储明文令牌
	// 不设置Status，让其保持空值，避免触发last_seen_ts的自动更新
	
	if err := s.db.CreateClient(&client); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	// 返回响应时包含生成的token（仅此一次）
	response := struct {
		database.Client
		AuthToken string `json:"auth_token"`
	}{
		Client:    client,
		AuthToken: authToken,
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleUpdateClient(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	clientID := vars["id"]
	
	// 只接收需要更新的字段
	var updateData struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&updateData); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	
	// 获取现有客户端信息
	existingClient, err := s.db.GetClient(clientID)
	if err != nil {
		http.Error(w, "Client not found", http.StatusNotFound)
		return
	}
	
	// 只更新允许修改的字段
	existingClient.Name = updateData.Name
	existingClient.Description = updateData.Description
	
	if err := s.db.UpdateClient(existingClient); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(existingClient)
}

func (s *Server) handleUpdateClientStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	clientID := vars["id"]
	
	var statusUpdate struct {
		Status string `json:"status"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&statusUpdate); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	
	// 验证状态值
	if statusUpdate.Status != "online" && statusUpdate.Status != "disabled" {
		http.Error(w, "Invalid status. Must be 'online' or 'disabled'", http.StatusBadRequest)
		return
	}
	
	// 更新客户端的启用状态
	enabled := 1
	if statusUpdate.Status == "disabled" {
		enabled = 0
		
		// 如果禁用客户端，强制断开连接
		if s.wsManager.IsClientConnected(clientID) {
			if err := s.wsManager.DisconnectClient(clientID); err != nil {
				log.Printf("Failed to disconnect client %s: %v", clientID, err)
			}
		}
	}
	
	if err := s.db.UpdateClientEnabled(clientID, enabled == 1); err != nil {
		http.Error(w, "Failed to update client status", http.StatusInternalServerError)
		return
	}
	
	// 更新客户端状态
	if err := s.db.UpdateClientStatus(clientID, statusUpdate.Status); err != nil {
		log.Printf("Failed to update client status in database: %v", err)
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": statusUpdate.Status})
}

func (s *Server) handleDeleteClient(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	clientID := vars["id"]
	
	if err := s.db.DeleteClient(clientID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.WriteHeader(http.StatusNoContent)
}

// convertRoutesForAPI 转换路由数据为API返回格式
func convertRoutesForAPI(routes []*database.ServerRoute) []map[string]interface{} {
	result := make([]map[string]interface{}, len(routes))
	for i, route := range routes {
		// 解析targets_json
		targets, err := route.GetTargets()
		var targetsJSON string
		if err != nil || len(targets) == 0 {
			targetsJSON = route.TargetsJSON // 保持原始值
		} else if len(targets) == 1 {
			// 单个目标，返回简单URL字符串
			targetsJSON = targets[0].URL
		} else {
			// 多个目标，保持JSON格式
			targetsJSON = route.TargetsJSON
		}
		
		// 将数据库中的route_mode值映射为前端期望的值
		var frontendRouteMode string
		switch route.RouteMode {
		case database.RouteModeOriginalPath:
			frontendRouteMode = "basic"
		case database.RouteModePathTransform:
			frontendRouteMode = "full"
		default:
			frontendRouteMode = route.RouteMode // 保持原值作为后备
		}
		
		result[i] = map[string]interface{}{
			"id":              route.ID,
			"url_suffix":      route.URLSuffix,
			"client_id":       route.ClientID,
			"targets_json":    targetsJSON,
			"delivery_policy": route.DeliveryPolicy,
			"route_mode":      frontendRouteMode,
			"enabled":         route.Enabled,
			"description":     route.Description,
			"created_at":      route.CreatedAt,
			"updated_at":      route.UpdatedAt,
		}
	}
	return result
}

// 路由管理API
func (s *Server) handleGetRoutes(w http.ResponseWriter, r *http.Request) {
	// 检查是否有客户端ID查询参数
	clientID := r.URL.Query().Get("client_id")
	if clientID != "" {
		// 按客户端ID查询路由
		routes, err := s.db.GetServerRoutesByClientID(clientID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(convertRoutesForAPI(routes))
		return
	}
	
	// 查询所有路由
	routes, err := s.db.ListServerRoutes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(convertRoutesForAPI(routes))
}

func (s *Server) handleGetRoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	routeID := vars["id"]

	id, err := strconv.Atoi(routeID)
	if err != nil {
		http.Error(w, "Invalid route ID", http.StatusBadRequest)
		return
	}

	route, err := s.db.GetServerRoute(id)
	if err != nil {
		http.Error(w, "Route not found", http.StatusNotFound)
		return
	}
	
	// 转换单个路由的 targets_json 格式
	routes := []*database.ServerRoute{route}
	convertedRoutes := convertRoutesForAPI(routes)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(convertedRoutes[0])
}

func (s *Server) handleCreateRoute(w http.ResponseWriter, r *http.Request) {
	var route database.ServerRoute
	if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	
	// 将前端的route_mode值映射为数据库期望的值
	switch route.RouteMode {
	case "basic":
		route.RouteMode = database.RouteModeOriginalPath
	case "full":
		route.RouteMode = database.RouteModePathTransform
	// 如果是其他值，保持不变
	}
	
	route.CreatedAt = time.Now().UnixMilli()

	if err := s.db.CreateServerRoute(&route); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(route)
}

func (s *Server) handleUpdateRoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	routeID := vars["id"]

	id, err := strconv.Atoi(routeID)
	if err != nil {
		http.Error(w, "Invalid route ID", http.StatusBadRequest)
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// 获取现有路由并更新
	existingRoute, err := s.db.GetServerRoute(id)
	if err != nil {
		http.Error(w, "Route not found", http.StatusNotFound)
		return
	}
	
	// 更新字段
	if urlSuffix, ok := updates["url_suffix"].(string); ok {
		existingRoute.URLSuffix = urlSuffix
	}
	if clientID, ok := updates["client_id"].(string); ok {
		existingRoute.ClientID = clientID
	}
	if targetsJSON, ok := updates["targets_json"].(string); ok {
		existingRoute.TargetsJSON = targetsJSON
	}
	if routeMode, ok := updates["route_mode"].(string); ok {
		// 将前端的route_mode值映射为数据库期望的值
		switch routeMode {
		case "basic":
			existingRoute.RouteMode = database.RouteModeOriginalPath
		case "full":
			existingRoute.RouteMode = database.RouteModePathTransform
		default:
			existingRoute.RouteMode = routeMode // 保持原值
		}
	}
	if enabled, ok := updates["enabled"].(bool); ok {
		if enabled {
			existingRoute.Enabled = 1
		} else {
			existingRoute.Enabled = 0
		}
	}
	if description, ok := updates["description"].(string); ok {
		existingRoute.Description = description
	}
	
	if err := s.db.UpdateServerRoute(existingRoute); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteRoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	routeID := vars["id"]

	id, err := strconv.Atoi(routeID)
	if err != nil {
		http.Error(w, "Invalid route ID", http.StatusBadRequest)
		return
	}

	if err := s.db.DeleteServerRoute(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// 更新单个路由的启用状态
func (s *Server) handleUpdateRouteEnabled(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	routeID := vars["id"]

	id, err := strconv.Atoi(routeID)
	if err != nil {
		http.Error(w, "Invalid route ID", http.StatusBadRequest)
		return
	}

	var request struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := s.db.UpdateServerRouteEnabled(id, request.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// 批量更新路由启用状态
func (s *Server) handleBatchUpdateRoutesEnabled(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RouteIDs []int `json:"route_ids"`
		Enabled  bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if len(request.RouteIDs) == 0 {
		http.Error(w, "No route IDs provided", http.StatusBadRequest)
		return
	}

	if err := s.db.BatchUpdateServerRoutesEnabled(request.RouteIDs, request.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// 获取路由统计信息
func (s *Server) handleGetRouteStats(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")

	stats, err := s.db.GetServerRouteStats(clientID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}





func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	// 获取总客户端数量
	allClients, err := s.db.ListClients()
	totalClients := 0
	if err == nil {
		totalClients = len(allClients)
	}

	// 检查数据库连接状态
	dbStatus := "connected"
	if err := s.db.Ping(); err != nil {
		dbStatus = "disconnected"
	}

	status := map[string]interface{}{
		"server": map[string]interface{}{
			"status":    "running",
			"timestamp": time.Now(),
			"version":   "1.0.0",
		},
		"database": map[string]interface{}{
			"status": dbStatus,
		},
		"clients": map[string]interface{}{
			"connected": s.wsManager.GetConnectedClientCount(),
			"total":     totalClients,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleGetServerInfo(w http.ResponseWriter, r *http.Request) {
	// 获取实际IP地址，如果配置为0.0.0.0则使用本地IP
	serverHost := s.config.ServerHost
	if serverHost == "0.0.0.0" {
		if localIP, err := utils.GetLocalIP(); err == nil {
			serverHost = localIP
		}
	}
	
	// 构建代理服务器URL
	proxyURL := fmt.Sprintf("http://%s:%d", serverHost, s.config.ProxyPort)
	
	info := map[string]interface{}{
		"name":        "Tunnel Flow Server",
		"version":     "1.0.0",
		"description": "HTTP tunnel and proxy server",
		"api_port":    s.config.APIPort,
		"ws_port":     s.config.WebSocketPort,
		"proxy_port":  s.config.ProxyPort,
		"server_host": serverHost,
		"proxy_url":   proxyURL,
		"uptime":      time.Since(time.Now()).String(), // 简化处理
		"features": []string{
			"HTTP Tunneling",
			"WebSocket Support",
			"Multi-client Management",
			"Route Management",
			"Authentication",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// 客户端配置管理处理器
// 客户端启用状态管理API
func (s *Server) handleUpdateClientEnabled(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	clientID := vars["id"]
	
	var request struct {
		Enabled bool `json:"enabled"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	if err := s.db.UpdateClientEnabled(clientID, request.Enabled); err != nil {
		http.Error(w, "Failed to update client enabled status", http.StatusInternalServerError)
		return
	}
	
	w.WriteHeader(http.StatusNoContent)
}

// 辅助函数
func generateClientID() string {
	return fmt.Sprintf("client_%d", time.Now().UnixNano())
}

func generateAuthToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", h)
}


// MultiServer 多端口服务器管理器
type MultiServer struct {
	config        *config.Config
	db            *database.Repository
	wsManager     *websocket.Manager
	
	apiServer     *APIServer
	wsServer      *WebSocketServer
	proxyServer   *ProxyServer
	
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
}

// APIServer API服务器（原Server重命名）
type APIServer struct {
	config         *config.Config
	db             *database.Repository
	authHandler    *auth.AuthHandler
	wsManager      *websocket.Manager
	server         *http.Server
}

// NewMultiServer 创建多端口服务器管理器
func NewMultiServer(cfg *config.Config, db *database.Repository, objectPool *performance.ObjectPool, workerPool *performance.WorkerPool, metrics interface{}) *MultiServer {
	// 创建WebSocket管理器
	wsManager := websocket.NewManager(cfg, db, objectPool, workerPool, metrics)
	
	// 创建各个服务器
	apiServer := NewAPIServer(cfg, db, wsManager)
	wsServer := NewWebSocketServer(cfg, wsManager)
	proxyServer := NewProxyServer(cfg, db, wsManager)
	
	return &MultiServer{
		config:      cfg,
		apiServer:   apiServer,
		wsServer:    wsServer,
		proxyServer: proxyServer,
		wsManager:   wsManager,
	}
}

// Start 启动所有服务器
func (ms *MultiServer) Start() error {
	log.Println("Starting multi-port tunnel-flow servers...")
	
	// 启动API服务器
	ms.wg.Add(1)
	go func() {
		defer ms.wg.Done()
		if err := ms.apiServer.Start(); err != nil && err != http.ErrServerClosed {
			log.Printf("API server error: %v", err)
		}
	}()
	
	// 启动WebSocket服务器
	ms.wg.Add(1)
	go func() {
		defer ms.wg.Done()
		if err := ms.wsServer.Start(); err != nil && err != http.ErrServerClosed {
			log.Printf("WebSocket server error: %v", err)
		}
	}()
	
	// 启动代理服务器
	ms.wg.Add(1)
	go func() {
		defer ms.wg.Done()
		if err := ms.proxyServer.Start(); err != nil && err != http.ErrServerClosed {
			log.Printf("Proxy server error: %v", err)
		}
	}()
	
	log.Printf("All servers started successfully:")
	log.Printf("  - API Server: http://%s:%d", ms.config.ServerHost, ms.config.APIPort)
	log.Printf("  - WebSocket Server: ws://%s:%d", ms.config.ServerHost, ms.config.WebSocketPort)
	log.Printf("  - Proxy Server: http://%s:%d", ms.config.ServerHost, ms.config.ProxyPort)
	
	// 等待所有服务器完成
	ms.wg.Wait()
	return nil
}

// Stop 停止所有服务器
func (ms *MultiServer) Stop() error {
	ms.cancel()
	
	// 停止各个服务器
	if err := ms.apiServer.Stop(context.Background()); err != nil {
		log.Printf("Error stopping API server: %v", err)
	}
	
	if err := ms.wsServer.Stop(); err != nil {
		log.Printf("Error stopping WebSocket server: %v", err)
	}
	
	if err := ms.proxyServer.Stop(); err != nil {
		log.Printf("Error stopping Proxy server: %v", err)
	}
	
	// 等待所有goroutine完成
	ms.wg.Wait()
	
	log.Println("Multi-server stopped")
	return nil
}

// NewAPIServer 创建新的API服务器
func NewAPIServer(cfg *config.Config, db *database.Repository, wsManager *websocket.Manager) *APIServer {
	return &APIServer{
		config:         cfg,
		db:             db,
		authHandler:    auth.NewAuthHandler(cfg),
		wsManager:      wsManager,
	}
}

// Start 启动API服务器
func (s *APIServer) Start() error {
	router := s.setupRoutes()
	
	// 配置CORS
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"*"},
		AllowCredentials: true,
	})
	
	handler := c.Handler(router)
	
	s.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.config.ServerHost, s.config.APIPort),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	
	log.Printf("API server starting on %s:%d", s.config.ServerHost, s.config.APIPort)
	return s.server.ListenAndServe()
}

// Stop 停止API服务器
func (s *APIServer) Stop(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// setupRoutes 设置API路由
func (s *APIServer) setupRoutes() *mux.Router {
	r := mux.NewRouter()
	
	// API路由组
	api := r.PathPrefix("/api/v1").Subrouter()
	
	// 认证相关路由（公开访问）
	api.HandleFunc("/auth/login", s.authHandler.Login).Methods("POST")
	api.HandleFunc("/auth/register", s.authHandler.Register).Methods("POST")
	api.HandleFunc("/auth/profile", s.authHandler.GetProfile).Methods("GET")
	
	// 系统状态（公开访问）
	api.HandleFunc("/status", s.handleGetStatus).Methods("GET")
	api.HandleFunc("/server-info", s.handleGetServerInfo).Methods("GET")
	
	// 需要认证的路由
	protected := api.PathPrefix("").Subrouter()
	protected.Use(s.authHandler.GetAuthMiddleware().Middleware)
	
	// 客户端管理
	protected.HandleFunc("/clients", s.handleGetClients).Methods("GET")
	protected.HandleFunc("/clients", s.handleCreateClient).Methods("POST")
	protected.HandleFunc("/clients/{id}", s.handleGetClient).Methods("GET")
	protected.HandleFunc("/clients/{id}", s.handleUpdateClient).Methods("PUT")
	protected.HandleFunc("/clients/{id}", s.handleDeleteClient).Methods("DELETE")
	protected.HandleFunc("/clients/{id}/status", s.handleUpdateClientStatus).Methods("PUT")
	protected.HandleFunc("/clients/{id}/enabled", s.handleUpdateClientEnabled).Methods("PUT")
	
	// 路由管理
	protected.HandleFunc("/routes", s.handleGetRoutes).Methods("GET")
	protected.HandleFunc("/routes", s.handleCreateRoute).Methods("POST")
	protected.HandleFunc("/routes/{id}", s.handleGetRoute).Methods("GET")
	protected.HandleFunc("/routes/{id}", s.handleUpdateRoute).Methods("PUT")
	protected.HandleFunc("/routes/{id}", s.handleDeleteRoute).Methods("DELETE")
	
	// 路由启用状态管理
	protected.HandleFunc("/routes/{id}/enabled", s.handleUpdateRouteEnabled).Methods("PUT")
	protected.HandleFunc("/routes/batch/enabled", s.handleBatchUpdateRoutesEnabled).Methods("PUT")
	protected.HandleFunc("/routes/stats", s.handleGetRouteStats).Methods("GET")
	
	// 健康检查（无需认证）
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("API Server OK"))
	}).Methods("GET")
	
	// 静态文件服务（如果需要）- 使用嵌入的文件系统
	// 注意：静态文件服务应该放在最后，并且不应该拦截API路由
	if handler, err := web.GetDistHandler(); err == nil {
		// 只处理非API路径的请求
		r.PathPrefix("/").Handler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// 如果是API路径，返回404让API路由处理
			if strings.HasPrefix(req.URL.Path, "/api/") {
				http.NotFound(w, req)
				return
			}
			// 否则使用静态文件处理器
			handler.ServeHTTP(w, req)
		}))
	}
	
	return r
}

// APIServer的处理函数 - 简单包装Server的方法
func (s *APIServer) handleGetClients(w http.ResponseWriter, r *http.Request) {
	// 创建临时Server实例来复用逻辑
	tempServer := &Server{
		config:      s.config,
		db:          s.db,
		authHandler: s.authHandler,
		wsManager:   s.wsManager,
	}
	tempServer.handleGetClients(w, r)
}

func (s *APIServer) handleGetClient(w http.ResponseWriter, r *http.Request) {
	tempServer := &Server{
		config:      s.config,
		db:          s.db,
		authHandler: s.authHandler,
		wsManager:   s.wsManager,
	}
	tempServer.handleGetClient(w, r)
}

func (s *APIServer) handleCreateClient(w http.ResponseWriter, r *http.Request) {
	tempServer := &Server{
		config:      s.config,
		db:          s.db,
		authHandler: s.authHandler,
		wsManager:   s.wsManager,
	}
	tempServer.handleCreateClient(w, r)
}

func (s *APIServer) handleUpdateClient(w http.ResponseWriter, r *http.Request) {
	tempServer := &Server{
		config:      s.config,
		db:          s.db,
		authHandler: s.authHandler,
		wsManager:   s.wsManager,
	}
	tempServer.handleUpdateClient(w, r)
}

func (s *APIServer) handleDeleteClient(w http.ResponseWriter, r *http.Request) {
	tempServer := &Server{
		config:      s.config,
		db:          s.db,
		authHandler: s.authHandler,
		wsManager:   s.wsManager,
	}
	tempServer.handleDeleteClient(w, r)
}

func (s *APIServer) handleUpdateClientStatus(w http.ResponseWriter, r *http.Request) {
	tempServer := &Server{
		config:      s.config,
		db:          s.db,
		authHandler: s.authHandler,
		wsManager:   s.wsManager,
	}
	tempServer.handleUpdateClientStatus(w, r)
}

func (s *APIServer) handleUpdateClientEnabled(w http.ResponseWriter, r *http.Request) {
	tempServer := &Server{
		config:      s.config,
		db:          s.db,
		authHandler: s.authHandler,
		wsManager:   s.wsManager,
	}
	tempServer.handleUpdateClientEnabled(w, r)
}

func (s *APIServer) handleGetRoutes(w http.ResponseWriter, r *http.Request) {
	tempServer := &Server{
		config:      s.config,
		db:          s.db,
		authHandler: s.authHandler,
		wsManager:   s.wsManager,
	}
	tempServer.handleGetRoutes(w, r)
}

func (s *APIServer) handleGetRoute(w http.ResponseWriter, r *http.Request) {
	tempServer := &Server{
		config:      s.config,
		db:          s.db,
		authHandler: s.authHandler,
		wsManager:   s.wsManager,
	}
	tempServer.handleGetRoute(w, r)
}

func (s *APIServer) handleCreateRoute(w http.ResponseWriter, r *http.Request) {
	tempServer := &Server{
		config:      s.config,
		db:          s.db,
		authHandler: s.authHandler,
		wsManager:   s.wsManager,
	}
	tempServer.handleCreateRoute(w, r)
}

func (s *APIServer) handleUpdateRoute(w http.ResponseWriter, r *http.Request) {
	tempServer := &Server{
		config:      s.config,
		db:          s.db,
		authHandler: s.authHandler,
		wsManager:   s.wsManager,
	}
	tempServer.handleUpdateRoute(w, r)
}

func (s *APIServer) handleDeleteRoute(w http.ResponseWriter, r *http.Request) {
	tempServer := &Server{
		config:      s.config,
		db:          s.db,
		authHandler: s.authHandler,
		wsManager:   s.wsManager,
	}
	tempServer.handleDeleteRoute(w, r)
}

// 更新单个路由的启用状态
func (s *APIServer) handleUpdateRouteEnabled(w http.ResponseWriter, r *http.Request) {
	tempServer := &Server{
		config:      s.config,
		db:          s.db,
		authHandler: s.authHandler,
		wsManager:   s.wsManager,
	}
	tempServer.handleUpdateRouteEnabled(w, r)
}

// 批量更新路由启用状态
func (s *APIServer) handleBatchUpdateRoutesEnabled(w http.ResponseWriter, r *http.Request) {
	tempServer := &Server{
		config:      s.config,
		db:          s.db,
		authHandler: s.authHandler,
		wsManager:   s.wsManager,
	}
	tempServer.handleBatchUpdateRoutesEnabled(w, r)
}

// 获取路由统计信息
func (s *APIServer) handleGetRouteStats(w http.ResponseWriter, r *http.Request) {
	tempServer := &Server{
		config:      s.config,
		db:          s.db,
		authHandler: s.authHandler,
		wsManager:   s.wsManager,
	}
	tempServer.handleGetRouteStats(w, r)
}





func (s *APIServer) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	tempServer := &Server{
		config:      s.config,
		db:          s.db,
		authHandler: s.authHandler,
		wsManager:   s.wsManager,
	}
	tempServer.handleGetStatus(w, r)
}

func (s *APIServer) handleGetServerInfo(w http.ResponseWriter, r *http.Request) {
	tempServer := &Server{
		config:      s.config,
		db:          s.db,
		authHandler: s.authHandler,
		wsManager:   s.wsManager,
	}
	tempServer.handleGetServerInfo(w, r)
}