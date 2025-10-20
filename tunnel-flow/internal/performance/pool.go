package performance

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"
)

// ObjectPool 对象池，用于减少GC压力
type ObjectPool struct {
	messagePool   sync.Pool
	bufferPool    sync.Pool
	contextPool   sync.Pool
	responsePool  sync.Pool
}

// NewObjectPool 创建新的对象池
func NewObjectPool() *ObjectPool {
	return &ObjectPool{
		messagePool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, 4096) // 预分配4KB缓冲区
			},
		},
		bufferPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, 1024) // 预分配1KB缓冲区
			},
		},
		contextPool: sync.Pool{
			New: func() interface{} {
				return &PooledContext{}
			},
		},
		responsePool: sync.Pool{
			New: func() interface{} {
				return &PooledResponse{}
			},
		},
	}
}

// GetMessageBuffer 获取消息缓冲区
func (p *ObjectPool) GetMessageBuffer() []byte {
	return p.messagePool.Get().([]byte)[:0]
}

// PutMessageBuffer 归还消息缓冲区
func (p *ObjectPool) PutMessageBuffer(buf []byte) {
	if cap(buf) <= 8192 { // 只回收不超过8KB的缓冲区
		p.messagePool.Put(buf)
	}
}

// GetBuffer 获取通用缓冲区
func (p *ObjectPool) GetBuffer() []byte {
	return p.bufferPool.Get().([]byte)[:0]
}

// PutBuffer 归还通用缓冲区
func (p *ObjectPool) PutBuffer(buf []byte) {
	if cap(buf) <= 2048 { // 只回收不超过2KB的缓冲区
		p.bufferPool.Put(buf)
	}
}

// PooledContext 池化的上下文对象
type PooledContext struct {
	ctx    context.Context
	cancel context.CancelFunc
	data   map[string]interface{}
}

// GetContext 获取池化的上下文
func (p *ObjectPool) GetContext() *PooledContext {
	pc := p.contextPool.Get().(*PooledContext)
	pc.ctx, pc.cancel = context.WithCancel(context.Background())
	if pc.data == nil {
		pc.data = make(map[string]interface{})
	}
	return pc
}

// PutContext 归还池化的上下文
func (p *ObjectPool) PutContext(pc *PooledContext) {
	if pc.cancel != nil {
		pc.cancel()
	}
	// 清理数据
	for k := range pc.data {
		delete(pc.data, k)
	}
	p.contextPool.Put(pc)
}

// PooledResponse 池化的响应对象
type PooledResponse struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
	Timestamp  time.Time
}

// GetResponse 获取池化的响应对象
func (p *ObjectPool) GetResponse() *PooledResponse {
	resp := p.responsePool.Get().(*PooledResponse)
	resp.StatusCode = 0
	if resp.Headers == nil {
		resp.Headers = make(map[string]string)
	}
	resp.Body = resp.Body[:0]
	resp.Timestamp = time.Now()
	return resp
}

// PutResponse 归还池化的响应对象
func (p *ObjectPool) PutResponse(resp *PooledResponse) {
	// 清理headers
	for k := range resp.Headers {
		delete(resp.Headers, k)
	}
	// 只回收不超过4KB的body缓冲区
	if cap(resp.Body) <= 4096 {
		p.responsePool.Put(resp)
	}
}

// WorkerPool 工作池，用于并发处理任务
type WorkerPool struct {
	workers    int
	taskQueue  chan Task
	resultChan chan TaskResult
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	stats      *WorkerStats
}

// Task 任务接口
type Task interface {
	Execute() TaskResult
	GetID() string
	GetPriority() int
}

// TaskResult 任务结果
type TaskResult struct {
	TaskID    string
	Success   bool
	Error     error
	Data      interface{}
	Duration  time.Duration
	Timestamp time.Time
}

// WorkerStats 工作池统计信息
type WorkerStats struct {
	TotalTasks     int64
	CompletedTasks int64
	FailedTasks    int64
	ActiveWorkers  int32
	QueueLength    int32
	AverageLatency time.Duration
	mu             sync.RWMutex
}

// NewWorkerPool 创建新的工作池
func NewWorkerPool(workers int, queueSize int) *WorkerPool {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if queueSize <= 0 {
		queueSize = workers * 10
	}

	ctx, cancel := context.WithCancel(context.Background())
	
	return &WorkerPool{
		workers:    workers,
		taskQueue:  make(chan Task, queueSize),
		resultChan: make(chan TaskResult, queueSize),
		ctx:        ctx,
		cancel:     cancel,
		stats:      &WorkerStats{},
	}
}

// Start 启动工作池
func (wp *WorkerPool) Start() {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}
}

// Stop 停止工作池
func (wp *WorkerPool) Stop() {
	wp.cancel()
	close(wp.taskQueue)
	wp.wg.Wait()
	close(wp.resultChan)
}

// Submit 提交任务
func (wp *WorkerPool) Submit(task Task) error {
	select {
	case wp.taskQueue <- task:
		wp.stats.mu.Lock()
		wp.stats.TotalTasks++
		wp.stats.QueueLength = int32(len(wp.taskQueue))
		wp.stats.mu.Unlock()
		return nil
	case <-wp.ctx.Done():
		return wp.ctx.Err()
	default:
		return ErrQueueFull
	}
}

// GetResults 获取结果通道
func (wp *WorkerPool) GetResults() <-chan TaskResult {
	return wp.resultChan
}

// GetStats 获取统计信息
func (wp *WorkerPool) GetStats() WorkerStats {
	wp.stats.mu.RLock()
	defer wp.stats.mu.RUnlock()
	return *wp.stats
}

// worker 工作协程
func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()
	
	wp.stats.mu.Lock()
	wp.stats.ActiveWorkers++
	wp.stats.mu.Unlock()
	
	defer func() {
		wp.stats.mu.Lock()
		wp.stats.ActiveWorkers--
		wp.stats.mu.Unlock()
	}()

	for {
		select {
		case task, ok := <-wp.taskQueue:
			if !ok {
				return
			}
			
			start := time.Now()
			result := task.Execute()
			result.Duration = time.Since(start)
			result.Timestamp = time.Now()
			
			// 更新统计信息
			wp.stats.mu.Lock()
			wp.stats.QueueLength = int32(len(wp.taskQueue))
			if result.Success {
				wp.stats.CompletedTasks++
			} else {
				wp.stats.FailedTasks++
			}
			// 更新平均延迟
			totalCompleted := wp.stats.CompletedTasks + wp.stats.FailedTasks
			if totalCompleted > 0 {
				wp.stats.AverageLatency = time.Duration(
					(int64(wp.stats.AverageLatency)*(totalCompleted-1) + int64(result.Duration)) / totalCompleted,
				)
			}
			wp.stats.mu.Unlock()
			
			// 发送结果
			select {
			case wp.resultChan <- result:
			case <-wp.ctx.Done():
				return
			default:
				// 结果通道满，丢弃结果
			}
			
		case <-wp.ctx.Done():
			return
		}
	}
}

// 错误定义
var (
	ErrQueueFull = fmt.Errorf("task queue is full")
)

// AdaptiveBuffer 自适应缓冲区
type AdaptiveBuffer struct {
	buffer    []byte
	capacity  int
	maxSize   int
	growthFactor float64
	shrinkThreshold float64
	mu        sync.RWMutex
}

// NewAdaptiveBuffer 创建自适应缓冲区
func NewAdaptiveBuffer(initialSize, maxSize int) *AdaptiveBuffer {
	if initialSize <= 0 {
		initialSize = 1024
	}
	if maxSize <= 0 {
		maxSize = 1024 * 1024 // 1MB
	}
	
	return &AdaptiveBuffer{
		buffer:          make([]byte, 0, initialSize),
		capacity:        initialSize,
		maxSize:         maxSize,
		growthFactor:    2.0,
		shrinkThreshold: 0.25,
	}
}

// Write 写入数据
func (ab *AdaptiveBuffer) Write(data []byte) {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	
	needed := len(ab.buffer) + len(data)
	
	// 检查是否需要扩容
	if needed > ab.capacity {
		newCapacity := ab.capacity
		for newCapacity < needed && newCapacity < ab.maxSize {
			newCapacity = int(float64(newCapacity) * ab.growthFactor)
		}
		if newCapacity > ab.maxSize {
			newCapacity = ab.maxSize
		}
		
		if newCapacity > ab.capacity {
			newBuffer := make([]byte, len(ab.buffer), newCapacity)
			copy(newBuffer, ab.buffer)
			ab.buffer = newBuffer
			ab.capacity = newCapacity
		}
	}
	
	ab.buffer = append(ab.buffer, data...)
}

// Read 读取数据
func (ab *AdaptiveBuffer) Read(n int) []byte {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	
	if n > len(ab.buffer) {
		n = len(ab.buffer)
	}
	
	data := make([]byte, n)
	copy(data, ab.buffer[:n])
	ab.buffer = ab.buffer[n:]
	
	// 检查是否需要缩容
	if float64(len(ab.buffer)) < float64(ab.capacity)*ab.shrinkThreshold && ab.capacity > 1024 {
		newCapacity := ab.capacity / 2
		if newCapacity < 1024 {
			newCapacity = 1024
		}
		
		newBuffer := make([]byte, len(ab.buffer), newCapacity)
		copy(newBuffer, ab.buffer)
		ab.buffer = newBuffer
		ab.capacity = newCapacity
	}
	
	return data
}

// Len 返回缓冲区长度
func (ab *AdaptiveBuffer) Len() int {
	ab.mu.RLock()
	defer ab.mu.RUnlock()
	return len(ab.buffer)
}

// Cap 返回缓冲区容量
func (ab *AdaptiveBuffer) Cap() int {
	ab.mu.RLock()
	defer ab.mu.RUnlock()
	return ab.capacity
}

// Reset 重置缓冲区
func (ab *AdaptiveBuffer) Reset() {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	ab.buffer = ab.buffer[:0]
}