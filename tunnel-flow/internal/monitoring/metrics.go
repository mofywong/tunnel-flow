package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics 监控指标
type Metrics struct {
	// 连接指标
	ActiveConnections    int64 `json:"active_connections"`
	TotalConnections     int64 `json:"total_connections"`
	ConnectionsPerSecond int64 `json:"connections_per_second"`
	
	// 消息指标
	MessagesSent         int64 `json:"messages_sent"`
	MessagesReceived     int64 `json:"messages_received"`
	MessagesPerSecond    int64 `json:"messages_per_second"`
	MessageErrors        int64 `json:"message_errors"`
	
	// 性能指标
	AverageResponseTime  int64 `json:"average_response_time_ms"`
	MaxResponseTime      int64 `json:"max_response_time_ms"`
	MinResponseTime      int64 `json:"min_response_time_ms"`
	
	// 错误指标
	TotalErrors          int64 `json:"total_errors"`
	ErrorsPerSecond      int64 `json:"errors_per_second"`
	RetryCount           int64 `json:"retry_count"`
	
	// 系统指标
	MemoryUsage          int64 `json:"memory_usage_bytes"`
	GoroutineCount       int   `json:"goroutine_count"`
	CPUUsage             float64 `json:"cpu_usage_percent"`
	
	// 队列指标
	QueueSize            int64 `json:"queue_size"`
	QueueCapacity        int64 `json:"queue_capacity"`
	QueueUtilization     float64 `json:"queue_utilization_percent"`
	
	// 时间戳
	Timestamp            time.Time `json:"timestamp"`
}

// MetricsCollector 指标收集器
type MetricsCollector struct {
	metrics     *Metrics
	mu          sync.RWMutex
	startTime   time.Time
	lastUpdate  time.Time

	// 历史数据
	history     []Metrics
	maxHistory  int

	// 响应时间统计
	responseTimes []int64
	responseTimeMu sync.Mutex

	// 用于控制goroutine生命周期
	ctx    context.Context
	cancel context.CancelFunc
	ticker *time.Ticker
}

// NewMetricsCollector 创建新的指标收集器
func NewMetricsCollector() *MetricsCollector {
	ctx, cancel := context.WithCancel(context.Background())
	return &MetricsCollector{
		metrics: &Metrics{
			MinResponseTime: 999999999, // 初始化为最大值
			Timestamp:       time.Now(),
		},
		startTime:  time.Now(),
		lastUpdate: time.Now(),
		maxHistory: 100,
		history:    make([]Metrics, 0, 100),
		responseTimes: make([]int64, 0, 1000),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// IncrementConnections 增加连接数
func (mc *MetricsCollector) IncrementConnections() {
	atomic.AddInt64(&mc.metrics.ActiveConnections, 1)
	atomic.AddInt64(&mc.metrics.TotalConnections, 1)
}

// DecrementConnections 减少连接数
func (mc *MetricsCollector) DecrementConnections() {
	atomic.AddInt64(&mc.metrics.ActiveConnections, -1)
}

// IncrementMessagesSent 增加发送消息数
func (mc *MetricsCollector) IncrementMessagesSent() {
	atomic.AddInt64(&mc.metrics.MessagesSent, 1)
}

// IncrementMessagesReceived 增加接收消息数
func (mc *MetricsCollector) IncrementMessagesReceived() {
	atomic.AddInt64(&mc.metrics.MessagesReceived, 1)
}

// IncrementErrors 增加错误数
func (mc *MetricsCollector) IncrementErrors() {
	atomic.AddInt64(&mc.metrics.TotalErrors, 1)
	atomic.AddInt64(&mc.metrics.MessageErrors, 1)
}

// IncrementRetries 增加重试数
func (mc *MetricsCollector) IncrementRetries() {
	atomic.AddInt64(&mc.metrics.RetryCount, 1)
}

// RecordResponseTime 记录响应时间
func (mc *MetricsCollector) RecordResponseTime(duration time.Duration) {
	ms := duration.Milliseconds()
	
	mc.responseTimeMu.Lock()
	mc.responseTimes = append(mc.responseTimes, ms)
	if len(mc.responseTimes) > 1000 {
		mc.responseTimes = mc.responseTimes[1:]
	}
	mc.responseTimeMu.Unlock()
	
	// 更新最大最小值
	if ms > atomic.LoadInt64(&mc.metrics.MaxResponseTime) {
		atomic.StoreInt64(&mc.metrics.MaxResponseTime, ms)
	}
	
	if ms < atomic.LoadInt64(&mc.metrics.MinResponseTime) {
		atomic.StoreInt64(&mc.metrics.MinResponseTime, ms)
	}
}

// UpdateQueueMetrics 更新队列指标
func (mc *MetricsCollector) UpdateQueueMetrics(size, capacity int64) {
	atomic.StoreInt64(&mc.metrics.QueueSize, size)
	atomic.StoreInt64(&mc.metrics.QueueCapacity, capacity)
	
	if capacity > 0 {
		utilization := float64(size) / float64(capacity) * 100
		mc.mu.Lock()
		mc.metrics.QueueUtilization = utilization
		mc.mu.Unlock()
	}
}

// UpdateSystemMetrics 更新系统指标
func (mc *MetricsCollector) UpdateSystemMetrics() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	atomic.StoreInt64(&mc.metrics.MemoryUsage, int64(m.Alloc))
	
	mc.mu.Lock()
	mc.metrics.GoroutineCount = runtime.NumGoroutine()
	mc.mu.Unlock()
}

// CalculateRates 计算速率指标
func (mc *MetricsCollector) CalculateRates() {
	now := time.Now()
	duration := now.Sub(mc.lastUpdate).Seconds()
	
	if duration > 0 {
		mc.mu.Lock()
		
		// 计算平均响应时间
		mc.responseTimeMu.Lock()
		if len(mc.responseTimes) > 0 {
			var total int64
			for _, rt := range mc.responseTimes {
				total += rt
			}
			mc.metrics.AverageResponseTime = total / int64(len(mc.responseTimes))
		}
		mc.responseTimeMu.Unlock()
		
		mc.metrics.Timestamp = now
		mc.lastUpdate = now
		mc.mu.Unlock()
	}
}

// GetMetrics 获取当前指标
func (mc *MetricsCollector) GetMetrics() Metrics {
	mc.UpdateSystemMetrics()
	mc.CalculateRates()
	
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	return *mc.metrics
}

// GetHistory 获取历史指标
func (mc *MetricsCollector) GetHistory() []Metrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	result := make([]Metrics, len(mc.history))
	copy(result, mc.history)
	return result
}

// SaveSnapshot 保存快照
func (mc *MetricsCollector) SaveSnapshot() {
	metrics := mc.GetMetrics()
	
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	mc.history = append(mc.history, metrics)
	if len(mc.history) > mc.maxHistory {
		mc.history = mc.history[1:]
	}
}

// Reset 重置指标
func (mc *MetricsCollector) Reset() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	mc.metrics = &Metrics{
		MinResponseTime: 999999999,
		Timestamp:       time.Now(),
	}
	mc.responseTimes = mc.responseTimes[:0]
}

// HTTPHandler 提供HTTP接口
func (mc *MetricsCollector) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		switch r.URL.Path {
		case "/metrics":
			metrics := mc.GetMetrics()
			json.NewEncoder(w).Encode(metrics)
		case "/metrics/history":
			history := mc.GetHistory()
			json.NewEncoder(w).Encode(history)
		case "/metrics/reset":
			if r.Method == "POST" {
				mc.Reset()
				w.WriteHeader(http.StatusOK)
				fmt.Fprintf(w, `{"status": "reset"}`)
			} else {
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// StartPeriodicCollection 启动定期收集
func (mc *MetricsCollector) StartPeriodicCollection(interval time.Duration) {
	// 如果已经有ticker在运行，先停止它
	if mc.ticker != nil {
		mc.ticker.Stop()
	}
	
	mc.ticker = time.NewTicker(interval)
	go func() {
		defer mc.ticker.Stop()
		for {
			select {
			case <-mc.ticker.C:
				mc.SaveSnapshot()
			case <-mc.ctx.Done():
				return
			}
		}
	}()
}

// Stop 停止指标收集器并清理资源
func (mc *MetricsCollector) Stop() {
	if mc.cancel != nil {
		mc.cancel()
	}
	if mc.ticker != nil {
		mc.ticker.Stop()
	}
}