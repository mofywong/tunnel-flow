package performance

import (
	"container/heap"
	"context"
	"fmt"
	"sync"
	"time"
)

// Priority 优先级类型
type Priority int

const (
	PriorityLow Priority = iota
	PriorityNormal
	PriorityHigh
	PriorityCritical
)

// QueueMessage 队列消息
type QueueMessage struct {
	ID        string
	Data      []byte
	Priority  Priority
	Timestamp time.Time
	Retries   int
	MaxRetries int
	Deadline  time.Time
}

// PriorityQueue 优先级队列
type PriorityQueue []*QueueMessage

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	// 优先级高的排在前面
	if pq[i].Priority != pq[j].Priority {
		return pq[i].Priority > pq[j].Priority
	}
	// 相同优先级按时间戳排序
	return pq[i].Timestamp.Before(pq[j].Timestamp)
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *PriorityQueue) Push(x interface{}) {
	*pq = append(*pq, x.(*QueueMessage))
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}

// MessageQueue 消息队列
type MessageQueue struct {
	queue       *PriorityQueue
	mu          sync.RWMutex
	cond        *sync.Cond
	maxSize     int
	batchSize   int
	batchTimeout time.Duration
	closed      bool
	stats       *QueueStats
}

// QueueStats 队列统计信息
type QueueStats struct {
	TotalMessages    int64     `json:"total_messages"`
	ProcessedMessages int64    `json:"processed_messages"`
	DroppedMessages  int64     `json:"dropped_messages"`
	CurrentSize      int       `json:"current_size"`
	MaxSize          int       `json:"max_size"`
	AverageWaitTime  time.Duration `json:"average_wait_time"`
	LastUpdated      time.Time `json:"last_updated"`
}

// NewMessageQueue 创建新的消息队列
func NewMessageQueue(maxSize, batchSize int, batchTimeout time.Duration) *MessageQueue {
	if maxSize <= 0 {
		maxSize = 10000
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	if batchTimeout <= 0 {
		batchTimeout = 100 * time.Millisecond
	}

	pq := &PriorityQueue{}
	heap.Init(pq)
	
	mq := &MessageQueue{
		queue:       pq,
		maxSize:     maxSize,
		batchSize:   batchSize,
		batchTimeout: batchTimeout,
		stats:       &QueueStats{MaxSize: maxSize},
	}
	mq.cond = sync.NewCond(&mq.mu)
	
	return mq
}

// Enqueue 入队消息
func (mq *MessageQueue) Enqueue(msg *QueueMessage) error {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if mq.closed {
		return ErrQueueClosed
	}

	// 检查队列是否已满
	if mq.queue.Len() >= mq.maxSize {
		// 尝试移除低优先级的旧消息
		if !mq.removeOldLowPriorityMessage() {
			mq.stats.DroppedMessages++
			return ErrQueueFull
		}
	}

	// 设置时间戳
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	heap.Push(mq.queue, msg)
	mq.stats.TotalMessages++
	mq.stats.CurrentSize = mq.queue.Len()
	mq.stats.LastUpdated = time.Now()
	
	mq.cond.Signal()
	return nil
}

// Dequeue 出队消息
func (mq *MessageQueue) Dequeue() *QueueMessage {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	for mq.queue.Len() == 0 && !mq.closed {
		mq.cond.Wait()
	}

	if mq.queue.Len() == 0 {
		return nil
	}

	msg := heap.Pop(mq.queue).(*QueueMessage)
	mq.stats.ProcessedMessages++
	mq.stats.CurrentSize = mq.queue.Len()
	
	// 计算等待时间
	waitTime := time.Since(msg.Timestamp)
	if mq.stats.ProcessedMessages > 0 {
		mq.stats.AverageWaitTime = time.Duration(
			(int64(mq.stats.AverageWaitTime)*(mq.stats.ProcessedMessages-1) + int64(waitTime)) / mq.stats.ProcessedMessages,
		)
	}
	
	return msg
}

// DequeueBatch 批量出队消息
func (mq *MessageQueue) DequeueBatch(maxCount int) []*QueueMessage {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if maxCount <= 0 {
		maxCount = mq.batchSize
	}

	var messages []*QueueMessage
	for len(messages) < maxCount && mq.queue.Len() > 0 {
		msg := heap.Pop(mq.queue).(*QueueMessage)
		messages = append(messages, msg)
		mq.stats.ProcessedMessages++
	}

	mq.stats.CurrentSize = mq.queue.Len()
	return messages
}

// DequeueWithTimeout 带超时的出队
func (mq *MessageQueue) DequeueWithTimeout(timeout time.Duration) *QueueMessage {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	
	done := make(chan *QueueMessage, 1)
	
	go func() {
		// 使用非阻塞方式检查队列
		mq.mu.Lock()
		if mq.closed || mq.queue.Len() == 0 {
			mq.mu.Unlock()
			done <- nil
			return
		}
		
		msg := heap.Pop(mq.queue).(*QueueMessage)
		mq.stats.ProcessedMessages++
		mq.stats.CurrentSize = mq.queue.Len()
		mq.mu.Unlock()
		
		select {
		case done <- msg:
		case <-ctx.Done():
			// 如果context已取消，将消息放回队列
			mq.mu.Lock()
			if !mq.closed {
				heap.Push(mq.queue, msg)
				mq.stats.ProcessedMessages--
				mq.stats.CurrentSize = mq.queue.Len()
			}
			mq.mu.Unlock()
		}
	}()

	select {
	case msg := <-done:
		return msg
	case <-ctx.Done():
		return nil
	}
}

// removeOldLowPriorityMessage 移除旧的低优先级消息
func (mq *MessageQueue) removeOldLowPriorityMessage() bool {
	if mq.queue.Len() == 0 {
		return false
	}

	// 查找最旧的低优先级消息
	oldestIndex := -1
	oldestTime := time.Now()
	
	for i, msg := range *mq.queue {
		if msg.Priority == PriorityLow && msg.Timestamp.Before(oldestTime) {
			oldestIndex = i
			oldestTime = msg.Timestamp
		}
	}

	if oldestIndex >= 0 {
		// 移除找到的消息
		heap.Remove(mq.queue, oldestIndex)
		mq.stats.DroppedMessages++
		return true
	}

	return false
}

// Size 获取队列大小
func (mq *MessageQueue) Size() int {
	mq.mu.RLock()
	defer mq.mu.RUnlock()
	return mq.queue.Len()
}

// Close 关闭队列
func (mq *MessageQueue) Close() {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	
	mq.closed = true
	mq.cond.Broadcast()
}

// GetStats 获取统计信息
func (mq *MessageQueue) GetStats() QueueStats {
	mq.mu.RLock()
	defer mq.mu.RUnlock()
	return *mq.stats
}

// BatchProcessor 批处理器
type BatchProcessor struct {
	queue       *MessageQueue
	processor   func([]*QueueMessage) error
	batchSize   int
	timeout     time.Duration
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// NewBatchProcessor 创建批处理器
func NewBatchProcessor(queue *MessageQueue, processor func([]*QueueMessage) error, batchSize int, timeout time.Duration) *BatchProcessor {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &BatchProcessor{
		queue:     queue,
		processor: processor,
		batchSize: batchSize,
		timeout:   timeout,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start 启动批处理器
func (bp *BatchProcessor) Start() {
	bp.wg.Add(1)
	go bp.run()
}

// Stop 停止批处理器
func (bp *BatchProcessor) Stop() {
	bp.cancel()
	bp.wg.Wait()
}

// run 运行批处理器
func (bp *BatchProcessor) run() {
	defer bp.wg.Done()
	
	ticker := time.NewTicker(bp.timeout)
	defer ticker.Stop()
	
	var batch []*QueueMessage
	
	for {
		select {
		case <-bp.ctx.Done():
			// 处理剩余的批次
			if len(batch) > 0 {
				bp.processor(batch)
			}
			return
			
		case <-ticker.C:
			// 超时处理
			if len(batch) > 0 {
				bp.processor(batch)
				batch = batch[:0]
			}
			// 重置ticker以继续下一个周期
			ticker.Reset(bp.timeout)
		}
		
		// 在每个周期内尝试收集消息直到达到批次大小或超时
		for len(batch) < bp.batchSize {
			msg := bp.queue.DequeueWithTimeout(10 * time.Millisecond)
			if msg == nil {
				break // 没有更多消息，退出内层循环
			}
			
			batch = append(batch, msg)
			
			// 检查是否达到批次大小
			if len(batch) >= bp.batchSize {
				bp.processor(batch)
				batch = batch[:0]
				break
			}
		}
	}
}

// CircularBuffer 环形缓冲区
type CircularBuffer struct {
	buffer   [][]byte
	head     int
	tail     int
	size     int
	capacity int
	mu       sync.RWMutex
}

// NewCircularBuffer 创建环形缓冲区
func NewCircularBuffer(capacity int) *CircularBuffer {
	return &CircularBuffer{
		buffer:   make([][]byte, capacity),
		capacity: capacity,
	}
}

// Write 写入数据
func (cb *CircularBuffer) Write(data []byte) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.size >= cb.capacity {
		return false // 缓冲区已满
	}

	// 复制数据
	cb.buffer[cb.tail] = make([]byte, len(data))
	copy(cb.buffer[cb.tail], data)
	
	cb.tail = (cb.tail + 1) % cb.capacity
	cb.size++
	
	return true
}

// Read 读取数据
func (cb *CircularBuffer) Read() []byte {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.size == 0 {
		return nil
	}

	data := cb.buffer[cb.head]
	cb.buffer[cb.head] = nil // 清理引用
	cb.head = (cb.head + 1) % cb.capacity
	cb.size--
	
	return data
}

// Size 获取当前大小
func (cb *CircularBuffer) Size() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.size
}

// IsFull 检查是否已满
func (cb *CircularBuffer) IsFull() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.size >= cb.capacity
}

// IsEmpty 检查是否为空
func (cb *CircularBuffer) IsEmpty() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.size == 0
}

// Clear 清空缓冲区
func (cb *CircularBuffer) Clear() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	for i := range cb.buffer {
		cb.buffer[i] = nil
	}
	cb.head = 0
	cb.tail = 0
	cb.size = 0
}

// 错误定义
var (
	ErrQueueClosed = fmt.Errorf("queue is closed")
)

// LoadBalancer 负载均衡器
type LoadBalancer struct {
	queues    []*MessageQueue
	strategy  LoadBalanceStrategy
	current   int
	mu        sync.RWMutex
}

// LoadBalanceStrategy 负载均衡策略
type LoadBalanceStrategy int

const (
	RoundRobin LoadBalanceStrategy = iota
	LeastConnections
	WeightedRoundRobin
)

// NewLoadBalancer 创建负载均衡器
func NewLoadBalancer(queues []*MessageQueue, strategy LoadBalanceStrategy) *LoadBalancer {
	return &LoadBalancer{
		queues:   queues,
		strategy: strategy,
	}
}

// SelectQueue 选择队列
func (lb *LoadBalancer) SelectQueue() *MessageQueue {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if len(lb.queues) == 0 {
		return nil
	}

	switch lb.strategy {
	case RoundRobin:
		queue := lb.queues[lb.current]
		lb.current = (lb.current + 1) % len(lb.queues)
		return queue
		
	case LeastConnections:
		minSize := int(^uint(0) >> 1) // 最大int值
		var selectedQueue *MessageQueue
		
		for _, queue := range lb.queues {
			if size := queue.Size(); size < minSize {
				minSize = size
				selectedQueue = queue
			}
		}
		return selectedQueue
		
	default:
		return lb.queues[0]
	}
}