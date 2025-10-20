package performance

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"
)

// ConnectionPoolConfig 连接池配置
type ConnectionPoolConfig struct {
	MaxIdleConns        int           // 最大空闲连接数
	MaxIdleConnsPerHost int           // 每个主机最大空闲连接数
	MaxConnsPerHost     int           // 每个主机最大连接数
	IdleConnTimeout     time.Duration // 空闲连接超时时间
	TLSHandshakeTimeout time.Duration // TLS握手超时时间
	DialTimeout         time.Duration // 拨号超时时间
	KeepAlive           time.Duration // Keep-Alive时间
	ResponseHeaderTimeout time.Duration // 响应头超时时间
	ExpectContinueTimeout time.Duration // Expect Continue超时时间
}

// DefaultConnectionPoolConfig 默认连接池配置
func DefaultConnectionPoolConfig() *ConnectionPoolConfig {
	return &ConnectionPoolConfig{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		DialTimeout:           30 * time.Second,
		KeepAlive:             30 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// ConnectionPool HTTP连接池
type ConnectionPool struct {
	config    *ConnectionPoolConfig
	transport *http.Transport
	client    *http.Client
	stats     *ConnectionStats
	mu        sync.RWMutex
}

// ConnectionStats 连接统计信息
type ConnectionStats struct {
	ActiveConnections   int64     `json:"active_connections"`
	IdleConnections     int64     `json:"idle_connections"`
	TotalRequests       int64     `json:"total_requests"`
	SuccessfulRequests  int64     `json:"successful_requests"`
	FailedRequests      int64     `json:"failed_requests"`
	AverageResponseTime time.Duration `json:"average_response_time"`
	LastUpdated         time.Time `json:"last_updated"`
}

// NewConnectionPool 创建新的连接池
func NewConnectionPool(config *ConnectionPoolConfig) *ConnectionPool {
	if config == nil {
		config = DefaultConnectionPoolConfig()
	}

	// 创建自定义的Dialer
	dialer := &net.Dialer{
		Timeout:   config.DialTimeout,
		KeepAlive: config.KeepAlive,
		DualStack: true,
	}

	// 创建Transport
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		MaxIdleConns:          config.MaxIdleConns,
		MaxIdleConnsPerHost:   config.MaxIdleConnsPerHost,
		MaxConnsPerHost:       config.MaxConnsPerHost,
		IdleConnTimeout:       config.IdleConnTimeout,
		TLSHandshakeTimeout:   config.TLSHandshakeTimeout,
		ResponseHeaderTimeout: config.ResponseHeaderTimeout,
		ExpectContinueTimeout: config.ExpectContinueTimeout,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		},
		ForceAttemptHTTP2:     true,
		MaxResponseHeaderBytes: 1 << 20, // 1MB
	}

	// 创建HTTP客户端
	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	return &ConnectionPool{
		config:    config,
		transport: transport,
		client:    client,
		stats:     &ConnectionStats{},
	}
}

// Do 执行HTTP请求
func (cp *ConnectionPool) Do(req *http.Request) (*http.Response, error) {
	start := time.Now()
	
	// 更新统计信息
	cp.mu.Lock()
	cp.stats.TotalRequests++
	cp.stats.LastUpdated = time.Now()
	cp.mu.Unlock()

	resp, err := cp.client.Do(req)
	
	duration := time.Since(start)
	
	// 更新统计信息
	cp.mu.Lock()
	if err != nil {
		cp.stats.FailedRequests++
	} else {
		cp.stats.SuccessfulRequests++
	}
	
	// 更新平均响应时间
	totalRequests := cp.stats.SuccessfulRequests + cp.stats.FailedRequests
	if totalRequests > 0 {
		cp.stats.AverageResponseTime = time.Duration(
			(int64(cp.stats.AverageResponseTime)*(totalRequests-1) + int64(duration)) / totalRequests,
		)
	}
	cp.mu.Unlock()

	return resp, err
}

// DoWithContext 使用上下文执行HTTP请求
func (cp *ConnectionPool) DoWithContext(ctx context.Context, req *http.Request) (*http.Response, error) {
	req = req.WithContext(ctx)
	return cp.Do(req)
}

// GetStats 获取连接统计信息
func (cp *ConnectionPool) GetStats() ConnectionStats {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	
	// 获取Transport的连接统计
	if _, ok := cp.client.Transport.(*http.Transport); ok {
		// 注意：Go的http.Transport没有直接暴露连接数的方法
		// 这里我们使用已有的统计信息
		cp.stats.ActiveConnections = cp.stats.TotalRequests - cp.stats.SuccessfulRequests - cp.stats.FailedRequests
		if cp.stats.ActiveConnections < 0 {
			cp.stats.ActiveConnections = 0
		}
	}
	
	return *cp.stats
}

// Close 关闭连接池
func (cp *ConnectionPool) Close() {
	if cp.transport != nil {
		cp.transport.CloseIdleConnections()
	}
}

// SetTimeout 设置请求超时时间
func (cp *ConnectionPool) SetTimeout(timeout time.Duration) {
	cp.client.Timeout = timeout
}

// GetClient 获取HTTP客户端
func (cp *ConnectionPool) GetClient() *http.Client {
	return cp.client
}

// AdaptiveConnectionPool 自适应连接池
type AdaptiveConnectionPool struct {
	*ConnectionPool
	config          *ConnectionPoolConfig
	loadThreshold   float64
	scaleUpFactor   float64
	scaleDownFactor float64
	lastAdjustment  time.Time
	adjustmentInterval time.Duration
	mu              sync.RWMutex
}

// NewAdaptiveConnectionPool 创建自适应连接池
func NewAdaptiveConnectionPool(config *ConnectionPoolConfig) *AdaptiveConnectionPool {
	if config == nil {
		config = DefaultConnectionPoolConfig()
	}

	pool := NewConnectionPool(config)
	
	return &AdaptiveConnectionPool{
		ConnectionPool:     pool,
		config:            config,
		loadThreshold:     0.8,  // 80%负载阈值
		scaleUpFactor:     1.5,  // 扩容因子
		scaleDownFactor:   0.7,  // 缩容因子
		adjustmentInterval: 30 * time.Second,
	}
}

// AdjustPoolSize 自适应调整连接池大小
func (acp *AdaptiveConnectionPool) AdjustPoolSize() {
	acp.mu.Lock()
	defer acp.mu.Unlock()

	now := time.Now()
	if now.Sub(acp.lastAdjustment) < acp.adjustmentInterval {
		return
	}

	stats := acp.GetStats()
	
	// 计算负载率
	var loadRatio float64
	if acp.config.MaxConnsPerHost > 0 {
		loadRatio = float64(stats.ActiveConnections) / float64(acp.config.MaxConnsPerHost)
	}

	// 根据负载率调整连接池大小
	if loadRatio > acp.loadThreshold {
		// 扩容
		newMaxConns := int(float64(acp.config.MaxConnsPerHost) * acp.scaleUpFactor)
		if newMaxConns > 200 { // 设置上限
			newMaxConns = 200
		}
		acp.config.MaxConnsPerHost = newMaxConns
		acp.config.MaxIdleConnsPerHost = newMaxConns / 5
		
		// 重新创建Transport
		acp.recreateTransport()
		
	} else if loadRatio < (acp.loadThreshold * 0.5) {
		// 缩容
		newMaxConns := int(float64(acp.config.MaxConnsPerHost) * acp.scaleDownFactor)
		if newMaxConns < 10 { // 设置下限
			newMaxConns = 10
		}
		acp.config.MaxConnsPerHost = newMaxConns
		acp.config.MaxIdleConnsPerHost = newMaxConns / 5
		
		// 重新创建Transport
		acp.recreateTransport()
	}

	acp.lastAdjustment = now
}

// recreateTransport 重新创建Transport
func (acp *AdaptiveConnectionPool) recreateTransport() {
	// 关闭旧的连接
	acp.transport.CloseIdleConnections()
	
	// 创建新的Transport
	dialer := &net.Dialer{
		Timeout:   acp.config.DialTimeout,
		KeepAlive: acp.config.KeepAlive,
		DualStack: true,
	}

	acp.transport = &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		MaxIdleConns:          acp.config.MaxIdleConns,
		MaxIdleConnsPerHost:   acp.config.MaxIdleConnsPerHost,
		MaxConnsPerHost:       acp.config.MaxConnsPerHost,
		IdleConnTimeout:       acp.config.IdleConnTimeout,
		TLSHandshakeTimeout:   acp.config.TLSHandshakeTimeout,
		ResponseHeaderTimeout: acp.config.ResponseHeaderTimeout,
		ExpectContinueTimeout: acp.config.ExpectContinueTimeout,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		},
		ForceAttemptHTTP2:     true,
		MaxResponseHeaderBytes: 1 << 20, // 1MB
	}

	acp.client.Transport = acp.transport
}

// StartAutoAdjustment 启动自动调整
func (acp *AdaptiveConnectionPool) StartAutoAdjustment(ctx context.Context) {
	ticker := time.NewTicker(acp.adjustmentInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			acp.AdjustPoolSize()
		case <-ctx.Done():
			return
		}
	}
}

// ConnectionManager 连接管理器
type ConnectionManager struct {
	pools map[string]*AdaptiveConnectionPool
	mu    sync.RWMutex
}

// NewConnectionManager 创建连接管理器
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		pools: make(map[string]*AdaptiveConnectionPool),
	}
}

// GetPool 获取指定主机的连接池
func (cm *ConnectionManager) GetPool(host string) *AdaptiveConnectionPool {
	cm.mu.RLock()
	pool, exists := cm.pools[host]
	cm.mu.RUnlock()

	if exists {
		return pool
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 双重检查
	if pool, exists := cm.pools[host]; exists {
		return pool
	}

	// 创建新的连接池
	config := DefaultConnectionPoolConfig()
	pool = NewAdaptiveConnectionPool(config)
	cm.pools[host] = pool

	return pool
}

// CloseAll 关闭所有连接池
func (cm *ConnectionManager) CloseAll() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for _, pool := range cm.pools {
		pool.Close()
	}
	cm.pools = make(map[string]*AdaptiveConnectionPool)
}

// GetAllStats 获取所有连接池的统计信息
func (cm *ConnectionManager) GetAllStats() map[string]ConnectionStats {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	stats := make(map[string]ConnectionStats)
	for host, pool := range cm.pools {
		stats[host] = pool.GetStats()
	}
	return stats
}