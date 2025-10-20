package monitoring

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ClientMetrics 客户端监控指标
type ClientMetrics struct {
	// 连接指标
	ConnectionsActive    int64 `json:"connections_active"`
	ConnectionsTotal     int64 `json:"connections_total"`
	ConnectionsFailed    int64 `json:"connections_failed"`
	ReconnectCount       int64 `json:"reconnect_count"`
	
	// 消息指标
	MessagesSent         int64 `json:"messages_sent"`
	MessagesReceived     int64 `json:"messages_received"`
	MessagesDropped      int64 `json:"messages_dropped"`
	MessageErrors        int64 `json:"message_errors"`
	
	// 性能指标
	ResponseTimeAvg      float64 `json:"response_time_avg_ms"`
	ResponseTimeMax      float64 `json:"response_time_max_ms"`
	ResponseTimeMin      float64 `json:"response_time_min_ms"`
	ThroughputMBps       float64 `json:"throughput_mbps"`
	
	// 系统指标
	CPUUsage             float64 `json:"cpu_usage_percent"`
	MemoryUsage          int64   `json:"memory_usage_bytes"`
	GoroutineCount       int     `json:"goroutine_count"`
	
	// 时间戳
	Timestamp            time.Time `json:"timestamp"`
	UptimeSeconds        float64   `json:"uptime_seconds"`
}

// MetricsCollector 指标收集器
type MetricsCollector struct {
	metrics     *ClientMetrics
	startTime   time.Time
	mu          sync.RWMutex
	
	// 响应时间统计
	responseTimes []float64
	responseSum   float64
	responseCount int64
	
	// 吞吐量统计
	bytesTransferred int64
	lastThroughputCheck time.Time
}

// NewMetricsCollector 创建新的指标收集器
func NewMetricsCollector() *MetricsCollector {
	now := time.Now()
	return &MetricsCollector{
		metrics: &ClientMetrics{
			Timestamp:       now,
			ResponseTimeMin: 999999,
		},
		startTime:           now,
		lastThroughputCheck: now,
		responseTimes:       make([]float64, 0, 1000),
	}
}

// IncrementConnections 增加连接数
func (mc *MetricsCollector) IncrementConnections() {
	atomic.AddInt64(&mc.metrics.ConnectionsActive, 1)
	atomic.AddInt64(&mc.metrics.ConnectionsTotal, 1)
}

// DecrementConnections 减少连接数
func (mc *MetricsCollector) DecrementConnections() {
	atomic.AddInt64(&mc.metrics.ConnectionsActive, -1)
}

// IncrementConnectionsFailed 增加连接失败数
func (mc *MetricsCollector) IncrementConnectionsFailed() {
	atomic.AddInt64(&mc.metrics.ConnectionsFailed, 1)
}

// IncrementReconnect 增加重连次数
func (mc *MetricsCollector) IncrementReconnect() {
	atomic.AddInt64(&mc.metrics.ReconnectCount, 1)
}

// IncrementMessagesSent 增加发送消息数
func (mc *MetricsCollector) IncrementMessagesSent() {
	atomic.AddInt64(&mc.metrics.MessagesSent, 1)
}

// IncrementMessagesReceived 增加接收消息数
func (mc *MetricsCollector) IncrementMessagesReceived() {
	atomic.AddInt64(&mc.metrics.MessagesReceived, 1)
}

// IncrementMessagesDropped 增加丢弃消息数
func (mc *MetricsCollector) IncrementMessagesDropped() {
	atomic.AddInt64(&mc.metrics.MessagesDropped, 1)
}

// IncrementMessageErrors 增加消息错误数
func (mc *MetricsCollector) IncrementMessageErrors() {
	atomic.AddInt64(&mc.metrics.MessageErrors, 1)
}

// RecordResponseTime 记录响应时间
func (mc *MetricsCollector) RecordResponseTime(duration time.Duration) {
	ms := float64(duration.Nanoseconds()) / 1e6
	
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	// 更新响应时间统计
	mc.responseTimes = append(mc.responseTimes, ms)
	mc.responseSum += ms
	mc.responseCount++
	
	// 保持最近1000个响应时间
	if len(mc.responseTimes) > 1000 {
		removed := mc.responseTimes[0]
		mc.responseTimes = mc.responseTimes[1:]
		mc.responseSum -= removed
		mc.responseCount--
	}
	
	// 更新平均值
	if mc.responseCount > 0 {
		mc.metrics.ResponseTimeAvg = mc.responseSum / float64(mc.responseCount)
	}
	
	// 更新最大值
	if ms > mc.metrics.ResponseTimeMax {
		mc.metrics.ResponseTimeMax = ms
	}
	
	// 更新最小值
	if ms < mc.metrics.ResponseTimeMin {
		mc.metrics.ResponseTimeMin = ms
	}
}

// RecordBytesTransferred 记录传输字节数
func (mc *MetricsCollector) RecordBytesTransferred(bytes int64) {
	atomic.AddInt64(&mc.bytesTransferred, bytes)
}

// UpdateSystemMetrics 更新系统指标
func (mc *MetricsCollector) UpdateSystemMetrics() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	// 更新时间戳和运行时间
	now := time.Now()
	mc.metrics.Timestamp = now
	mc.metrics.UptimeSeconds = now.Sub(mc.startTime).Seconds()
	
	// 更新系统指标
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	mc.metrics.MemoryUsage = int64(memStats.Alloc)
	mc.metrics.GoroutineCount = runtime.NumGoroutine()
	
	// 计算吞吐量
	if now.Sub(mc.lastThroughputCheck) >= time.Second {
		bytes := atomic.SwapInt64(&mc.bytesTransferred, 0)
		duration := now.Sub(mc.lastThroughputCheck).Seconds()
		mc.metrics.ThroughputMBps = float64(bytes) / (1024 * 1024) / duration
		mc.lastThroughputCheck = now
	}
}

// GetMetrics 获取当前指标
func (mc *MetricsCollector) GetMetrics() *ClientMetrics {
	mc.UpdateSystemMetrics()
	
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	// 创建副本
	metrics := *mc.metrics
	return &metrics
}

// GetMetricsJSON 获取JSON格式的指标
func (mc *MetricsCollector) GetMetricsJSON() ([]byte, error) {
	metrics := mc.GetMetrics()
	return json.MarshalIndent(metrics, "", "  ")
}

// HTTPHandler 提供HTTP接口
func (mc *MetricsCollector) HTTPHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	data, err := mc.GetMetricsJSON()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to marshal metrics: %v", err), http.StatusInternalServerError)
		return
	}
	
	w.Write(data)
}

// Reset 重置指标
func (mc *MetricsCollector) Reset() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	// 重置计数器
	atomic.StoreInt64(&mc.metrics.ConnectionsActive, 0)
	atomic.StoreInt64(&mc.metrics.ConnectionsTotal, 0)
	atomic.StoreInt64(&mc.metrics.ConnectionsFailed, 0)
	atomic.StoreInt64(&mc.metrics.ReconnectCount, 0)
	atomic.StoreInt64(&mc.metrics.MessagesSent, 0)
	atomic.StoreInt64(&mc.metrics.MessagesReceived, 0)
	atomic.StoreInt64(&mc.metrics.MessagesDropped, 0)
	atomic.StoreInt64(&mc.metrics.MessageErrors, 0)
	
	// 重置响应时间统计
	mc.responseTimes = mc.responseTimes[:0]
	mc.responseSum = 0
	mc.responseCount = 0
	mc.metrics.ResponseTimeAvg = 0
	mc.metrics.ResponseTimeMax = 0
	mc.metrics.ResponseTimeMin = 999999
	
	// 重置吞吐量统计
	atomic.StoreInt64(&mc.bytesTransferred, 0)
	mc.lastThroughputCheck = time.Now()
	
	// 重置开始时间
	mc.startTime = time.Now()
}

// GetConnectionCount 获取活跃连接数
func (mc *MetricsCollector) GetConnectionCount() int64 {
	return atomic.LoadInt64(&mc.metrics.ConnectionsActive)
}

// GetMessageStats 获取消息统计
func (mc *MetricsCollector) GetMessageStats() (sent, received, dropped, errors int64) {
	return atomic.LoadInt64(&mc.metrics.MessagesSent),
		   atomic.LoadInt64(&mc.metrics.MessagesReceived),
		   atomic.LoadInt64(&mc.metrics.MessagesDropped),
		   atomic.LoadInt64(&mc.metrics.MessageErrors)
}