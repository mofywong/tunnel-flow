package proxy

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"tunnel-flow/internal/database"
	"tunnel-flow/internal/protocol"
	"tunnel-flow/internal/utils"
	"tunnel-flow/internal/websocket"
)

// Handler 代理处理器
type Handler struct {
	db        *database.Repository
	wsManager *websocket.Manager
}

// NewHandler 创建新的代理处理器
func NewHandler(db *database.Repository, wsManager *websocket.Manager) *Handler {
	return &Handler{
		db:        db,
		wsManager: wsManager,
	}
}

// HandleProxyRequest 处理代理请求
func (h *Handler) HandleProxyRequest(w http.ResponseWriter, r *http.Request) {
	// 记录8082端口请求接收日志
	log.Printf("[8082 Proxy] Received %s request: %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
	
	// 提取URL后缀
	urlPath := strings.TrimPrefix(r.URL.Path, "/proxy")
	if urlPath == "" {
		log.Printf("[8082 Proxy] Invalid proxy path: %s", r.URL.Path)
		http.Error(w, "Invalid proxy path", http.StatusBadRequest)
		return
	}

	// 查找路由
	routes, err := h.db.ListServerRoutes()
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
		log.Printf("[8082 Proxy] No route found for path: %s", urlPath)
		http.Error(w, "Route not found", http.StatusNotFound)
		return
	}

	log.Printf("[8082 Proxy] Found %d matching routes for path: %s", len(matchedRoutes), urlPath)

	// 选择第一个可用的路由
	var selectedRoute *database.ServerRoute
	for _, route := range matchedRoutes {
		// 检查客户端是否连接且启用
		if h.wsManager.IsClientConnected(route.ClientID) {
			// 检查客户端是否启用
			clientInfo, err := h.db.GetClient(route.ClientID)
			if err != nil || clientInfo.Enabled != 1 {
				log.Printf("[8082 Proxy] Skipping disabled client: %s", route.ClientID)
				continue // 跳过禁用的客户端
			}
			selectedRoute = route
			log.Printf("[8082 Proxy] Selected route with client: %s", route.ClientID)
			break
		} else {
			log.Printf("[8082 Proxy] Client not connected: %s", route.ClientID)
		}
	}

	if selectedRoute == nil {
		log.Printf("[8082 Proxy] No available backend for path: %s", urlPath)
		http.Error(w, "No available backend", http.StatusServiceUnavailable)
		return
	}

	// 转发请求到客户端
	h.forwardRequestToClient(w, r, selectedRoute, urlPath)
}

// HandleDirectProxyRequest 处理直接代理请求（不带/proxy前缀）
func (h *Handler) HandleDirectProxyRequest(w http.ResponseWriter, r *http.Request) {
	// 跳过特殊路径
	if r.URL.Path == "/upload" || r.URL.Path == "/health" || r.URL.Path == "/status" || strings.HasPrefix(r.URL.Path, "/proxy/") {
		http.NotFound(w, r)
		return
	}
	
	// 记录8082端口请求接收日志
	log.Printf("[8082 Direct] Received %s request: %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
	
	// 直接使用URL路径，不需要移除前缀
	urlPath := r.URL.Path
	if urlPath == "/" {
		log.Printf("[8082 Direct] Root path access not allowed")
		http.Error(w, "Root path not allowed", http.StatusBadRequest)
		return
	}

	// 查找路由
	routes, err := h.db.ListServerRoutes()
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
		log.Printf("[8082 Direct] No route found for path: %s", urlPath)
		http.Error(w, "Route not found", http.StatusNotFound)
		return
	}

	log.Printf("[8082 Direct] Found %d matching routes for path: %s", len(matchedRoutes), urlPath)

	// 选择第一个可用的路由
	var selectedRoute *database.ServerRoute
	for _, route := range matchedRoutes {
		// 检查客户端是否连接且启用
		if h.wsManager.IsClientConnected(route.ClientID) {
			// 检查客户端是否启用
			clientInfo, err := h.db.GetClient(route.ClientID)
			if err != nil || clientInfo.Enabled != 1 {
				log.Printf("[8082 Direct] Skipping disabled client: %s", route.ClientID)
				continue // 跳过禁用的客户端
			}
			selectedRoute = route
			log.Printf("[8082 Direct] Selected route with client: %s", route.ClientID)
			break
		} else {
			log.Printf("[8082 Direct] Client not connected: %s", route.ClientID)
		}
	}

	if selectedRoute == nil {
		log.Printf("[8082 Direct] No available backend for path: %s", urlPath)
		http.Error(w, "No available backend", http.StatusServiceUnavailable)
		return
	}

	// 转发请求到客户端
	h.forwardRequestToClient(w, r, selectedRoute, urlPath)
}

// forwardRequestToClient 转发请求到客户端的公共函数
func (h *Handler) forwardRequestToClient(w http.ResponseWriter, r *http.Request, selectedRoute *database.ServerRoute, urlPath string) {
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
	log.Printf("[HTTP Proxy] Sending request to client %s for path: %s", selectedRoute.ClientID, urlPath)
	response, err := h.wsManager.SendRequestAndWait(selectedRoute.ClientID, requestPayload, 30*time.Second)
	if err != nil {
		log.Printf("[HTTP Proxy] Failed to send request to client %s: %v", selectedRoute.ClientID, err)
		http.Error(w, "Backend request failed", http.StatusBadGateway)
		return
	}

	log.Printf("[HTTP Proxy] Received response from client %s - Status: %d", selectedRoute.ClientID, response.HTTPStatus)
	
	// 打印响应详情
	bodyPreview := ""
	bodyLength := 0
	if response.Body != nil {
		switch body := response.Body.(type) {
		case string:
			bodyLength = len(body)
			if bodyLength > 200 {
				bodyPreview = body[:200] + "..."
			} else {
				bodyPreview = body
			}
		case []byte:
			bodyLength = len(body)
			if bodyLength > 200 {
				bodyPreview = string(body[:200]) + "..."
			} else {
				bodyPreview = string(body)
			}
		default:
			bodyStr := fmt.Sprintf("%v", body)
			bodyLength = len(bodyStr)
			if bodyLength > 200 {
				bodyPreview = bodyStr[:200] + "..."
			} else {
				bodyPreview = bodyStr
			}
		}
	}
	
	log.Printf("[HTTP Proxy] Response details - Headers: %v, Body length: %d bytes", response.Headers, bodyLength)
	log.Printf("[HTTP Proxy] Response body preview: %s", bodyPreview)

	// 设置响应头
	for name, value := range response.Headers {
		w.Header().Set(name, value)
		log.Printf("[HTTP Proxy] Setting response header: %s = %s", name, value)
	}

	// 设置状态码
	log.Printf("[HTTP Proxy] Setting response status code: %d", response.HTTPStatus)
	w.WriteHeader(response.HTTPStatus)

	// 写入响应体
	bytesWritten := 0
	if response.Body != nil {
		switch body := response.Body.(type) {
		case string:
			bytesWritten, _ = w.Write([]byte(body))
		case []byte:
			bytesWritten, _ = w.Write(body)
		default:
			// 对于其他类型，尝试转换为字符串
			if bodyStr := fmt.Sprintf("%v", body); bodyStr != "" {
				bytesWritten, _ = w.Write([]byte(bodyStr))
			}
		}
	}
	
	log.Printf("[HTTP Proxy] Successfully wrote %d bytes to HTTP response for path: %s", bytesWritten, urlPath)

	// 如果有错误，记录日志
	if response.Error != nil {
		log.Printf("[HTTP Proxy] Backend returned error: %s", *response.Error)
	}
}

// HandleFileUpload 处理文件上传
func (h *Handler) HandleFileUpload(w http.ResponseWriter, r *http.Request) {
	// 文件上传处理逻辑
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("File upload handled"))
}

// GetStats 获取代理统计信息
func (h *Handler) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total_requests": 0,
		"active_routes":  0,
	}
}