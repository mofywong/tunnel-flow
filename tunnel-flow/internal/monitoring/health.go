package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// HealthStatus 健康状态
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusUnhealthy HealthStatus = "unhealthy"
	StatusDegraded  HealthStatus = "degraded"
	StatusUnknown   HealthStatus = "unknown"
)

// HealthCheck 健康检查接口
type HealthCheck interface {
	Name() string
	Check(ctx context.Context) HealthCheckResult
}

// HealthCheckResult 健康检查结果
type HealthCheckResult struct {
	Status    HealthStatus `json:"status"`
	Message   string       `json:"message,omitempty"`
	Duration  time.Duration `json:"duration"`
	Timestamp time.Time    `json:"timestamp"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// HealthReport 健康报告
type HealthReport struct {
	Status    HealthStatus                    `json:"status"`
	Timestamp time.Time                       `json:"timestamp"`
	Duration  time.Duration                   `json:"duration"`
	Checks    map[string]HealthCheckResult    `json:"checks"`
	Summary   map[string]interface{}          `json:"summary,omitempty"`
}

// HealthChecker 健康检查器
type HealthChecker struct {
	checks   map[string]HealthCheck
	mu       sync.RWMutex
	timeout  time.Duration
	interval time.Duration
	
	// 缓存
	lastReport *HealthReport
	reportMu   sync.RWMutex
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(timeout, interval time.Duration) *HealthChecker {
	return &HealthChecker{
		checks:   make(map[string]HealthCheck),
		timeout:  timeout,
		interval: interval,
	}
}

// RegisterCheck 注册健康检查
func (hc *HealthChecker) RegisterCheck(check HealthCheck) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	
	hc.checks[check.Name()] = check
}

// UnregisterCheck 取消注册健康检查
func (hc *HealthChecker) UnregisterCheck(name string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	
	delete(hc.checks, name)
}

// Check 执行所有健康检查
func (hc *HealthChecker) Check(ctx context.Context) *HealthReport {
	start := time.Now()
	
	hc.mu.RLock()
	checks := make(map[string]HealthCheck)
	for name, check := range hc.checks {
		checks[name] = check
	}
	hc.mu.RUnlock()
	
	report := &HealthReport{
		Timestamp: start,
		Checks:    make(map[string]HealthCheckResult),
		Summary:   make(map[string]interface{}),
	}
	
	// 并发执行检查
	var wg sync.WaitGroup
	resultChan := make(chan struct {
		name   string
		result HealthCheckResult
	}, len(checks))
	
	for name, check := range checks {
		wg.Add(1)
		go func(name string, check HealthCheck) {
			defer wg.Done()
			
			checkCtx, cancel := context.WithTimeout(ctx, hc.timeout)
			defer cancel()
			
			result := check.Check(checkCtx)
			resultChan <- struct {
				name   string
				result HealthCheckResult
			}{name, result}
		}(name, check)
	}
	
	// 等待所有检查完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()
	
	// 收集结果
	healthyCount := 0
	unhealthyCount := 0
	degradedCount := 0
	
	for result := range resultChan {
		report.Checks[result.name] = result.result
		
		switch result.result.Status {
		case StatusHealthy:
			healthyCount++
		case StatusUnhealthy:
			unhealthyCount++
		case StatusDegraded:
			degradedCount++
		}
	}
	
	// 计算总体状态
	totalChecks := len(checks)
	if totalChecks == 0 {
		report.Status = StatusUnknown
	} else if unhealthyCount > 0 {
		report.Status = StatusUnhealthy
	} else if degradedCount > 0 {
		report.Status = StatusDegraded
	} else {
		report.Status = StatusHealthy
	}
	
	report.Duration = time.Since(start)
	report.Summary = map[string]interface{}{
		"total_checks":     totalChecks,
		"healthy_checks":   healthyCount,
		"unhealthy_checks": unhealthyCount,
		"degraded_checks":  degradedCount,
	}
	
	// 缓存报告
	hc.reportMu.Lock()
	hc.lastReport = report
	hc.reportMu.Unlock()
	
	return report
}

// GetLastReport 获取最后一次检查报告
func (hc *HealthChecker) GetLastReport() *HealthReport {
	hc.reportMu.RLock()
	defer hc.reportMu.RUnlock()
	
	return hc.lastReport
}

// StartPeriodicCheck 启动定期检查
func (hc *HealthChecker) StartPeriodicCheck(ctx context.Context) {
	ticker := time.NewTicker(hc.interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hc.Check(ctx)
		}
	}
}

// HTTPHandler 提供HTTP接口
func (hc *HealthChecker) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), hc.timeout)
		defer cancel()
		
		var report *HealthReport
		
		// 检查是否需要实时检查
		if r.URL.Query().Get("live") == "true" {
			report = hc.Check(ctx)
		} else {
			report = hc.GetLastReport()
			if report == nil {
				report = hc.Check(ctx)
			}
		}
		
		w.Header().Set("Content-Type", "application/json")
		
		// 根据健康状态设置HTTP状态码
		switch report.Status {
		case StatusHealthy:
			w.WriteHeader(http.StatusOK)
		case StatusDegraded:
			w.WriteHeader(http.StatusOK) // 降级但仍可用
		case StatusUnhealthy:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		
		json.NewEncoder(w).Encode(report)
	}
}

// 内置健康检查实现

// DatabaseHealthCheck 数据库健康检查
type DatabaseHealthCheck struct {
	name string
	ping func() error
}

// NewDatabaseHealthCheck 创建数据库健康检查
func NewDatabaseHealthCheck(name string, ping func() error) *DatabaseHealthCheck {
	return &DatabaseHealthCheck{
		name: name,
		ping: ping,
	}
}

func (d *DatabaseHealthCheck) Name() string {
	return d.name
}

func (d *DatabaseHealthCheck) Check(ctx context.Context) HealthCheckResult {
	start := time.Now()
	
	result := HealthCheckResult{
		Timestamp: start,
	}
	
	if err := d.ping(); err != nil {
		result.Status = StatusUnhealthy
		result.Message = fmt.Sprintf("Database ping failed: %v", err)
	} else {
		result.Status = StatusHealthy
		result.Message = "Database is accessible"
	}
	
	result.Duration = time.Since(start)
	return result
}

// MemoryHealthCheck 内存健康检查
type MemoryHealthCheck struct {
	name      string
	threshold float64 // 内存使用率阈值 (0-1)
}

// NewMemoryHealthCheck 创建内存健康检查
func NewMemoryHealthCheck(name string, threshold float64) *MemoryHealthCheck {
	return &MemoryHealthCheck{
		name:      name,
		threshold: threshold,
	}
}

func (m *MemoryHealthCheck) Name() string {
	return m.name
}

func (m *MemoryHealthCheck) Check(ctx context.Context) HealthCheckResult {
	start := time.Now()
	
	// 这里简化实现，实际应该获取系统内存使用情况
	// 可以使用 github.com/shirou/gopsutil 等库
	memUsage := 0.5 // 假设当前内存使用率为50%
	
	result := HealthCheckResult{
		Timestamp: start,
		Details: map[string]interface{}{
			"memory_usage": memUsage,
			"threshold":    m.threshold,
		},
	}
	
	if memUsage > m.threshold {
		result.Status = StatusDegraded
		result.Message = fmt.Sprintf("Memory usage %.2f%% exceeds threshold %.2f%%", 
			memUsage*100, m.threshold*100)
	} else {
		result.Status = StatusHealthy
		result.Message = fmt.Sprintf("Memory usage %.2f%% is within threshold", memUsage*100)
	}
	
	result.Duration = time.Since(start)
	return result
}

// ConnectionHealthCheck 连接健康检查
type ConnectionHealthCheck struct {
	name           string
	getConnCount   func() int
	maxConnections int
}

// NewConnectionHealthCheck 创建连接健康检查
func NewConnectionHealthCheck(name string, getConnCount func() int, maxConnections int) *ConnectionHealthCheck {
	return &ConnectionHealthCheck{
		name:           name,
		getConnCount:   getConnCount,
		maxConnections: maxConnections,
	}
}

func (c *ConnectionHealthCheck) Name() string {
	return c.name
}

func (c *ConnectionHealthCheck) Check(ctx context.Context) HealthCheckResult {
	start := time.Now()
	
	connCount := c.getConnCount()
	usage := float64(connCount) / float64(c.maxConnections)
	
	result := HealthCheckResult{
		Timestamp: start,
		Details: map[string]interface{}{
			"active_connections": connCount,
			"max_connections":    c.maxConnections,
			"usage_percentage":   usage * 100,
		},
	}
	
	if usage > 0.9 {
		result.Status = StatusUnhealthy
		result.Message = fmt.Sprintf("Connection usage %.1f%% is critically high", usage*100)
	} else if usage > 0.7 {
		result.Status = StatusDegraded
		result.Message = fmt.Sprintf("Connection usage %.1f%% is high", usage*100)
	} else {
		result.Status = StatusHealthy
		result.Message = fmt.Sprintf("Connection usage %.1f%% is normal", usage*100)
	}
	
	result.Duration = time.Since(start)
	return result
}