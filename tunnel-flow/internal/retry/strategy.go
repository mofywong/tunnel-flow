package retry

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net"
	"strings"
	"syscall"
	"time"
)

// ErrorType 错误类型
type ErrorType int

const (
	ErrorTypeUnknown ErrorType = iota
	ErrorTypeNetwork           // 网络错误
	ErrorTypeTimeout           // 超时错误
	ErrorTypeConnection        // 连接错误
	ErrorTypeTemporary         // 临时错误
	ErrorTypePermanent         // 永久错误
	ErrorTypeRateLimit         // 限流错误
	ErrorTypeAuth              // 认证错误
	ErrorTypeServer            // 服务器错误
)

// ErrorClassifier 错误分类器
type ErrorClassifier struct{}

// NewErrorClassifier 创建错误分类器
func NewErrorClassifier() *ErrorClassifier {
	return &ErrorClassifier{}
}

// ClassifyError 分类错误
func (ec *ErrorClassifier) ClassifyError(err error) ErrorType {
	if err == nil {
		return ErrorTypeUnknown
	}

	errStr := strings.ToLower(err.Error())

	// 网络相关错误
	if netErr, ok := err.(net.Error); ok {
		if netErr.Timeout() {
			return ErrorTypeTimeout
		}
		if netErr.Temporary() {
			return ErrorTypeTemporary
		}
		return ErrorTypeNetwork
	}

	// 系统调用错误
	if opErr, ok := err.(*net.OpError); ok {
		if syscallErr, ok := opErr.Err.(*syscall.Errno); ok {
			switch *syscallErr {
			case syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ECONNABORTED:
				return ErrorTypeConnection
			case syscall.ETIMEDOUT:
				return ErrorTypeTimeout
			}
		}
		return ErrorTypeNetwork
	}

	// 基于错误消息的分类
	switch {
	case strings.Contains(errStr, "timeout"):
		return ErrorTypeTimeout
	case strings.Contains(errStr, "connection refused"), 
		 strings.Contains(errStr, "connection reset"),
		 strings.Contains(errStr, "connection aborted"):
		return ErrorTypeConnection
	case strings.Contains(errStr, "network"), 
		 strings.Contains(errStr, "dns"):
		return ErrorTypeNetwork
	case strings.Contains(errStr, "rate limit"), 
		 strings.Contains(errStr, "too many requests"):
		return ErrorTypeRateLimit
	case strings.Contains(errStr, "unauthorized"), 
		 strings.Contains(errStr, "forbidden"),
		 strings.Contains(errStr, "authentication"):
		return ErrorTypeAuth
	case strings.Contains(errStr, "server error"), 
		 strings.Contains(errStr, "internal server error"):
		return ErrorTypeServer
	case strings.Contains(errStr, "temporary"):
		return ErrorTypeTemporary
	default:
		return ErrorTypeUnknown
	}
}

// IsRetryable 判断错误是否可重试
func (ec *ErrorClassifier) IsRetryable(errorType ErrorType) bool {
	switch errorType {
	case ErrorTypeNetwork, ErrorTypeTimeout, ErrorTypeConnection, 
		 ErrorTypeTemporary, ErrorTypeRateLimit, ErrorTypeServer:
		return true
	case ErrorTypePermanent, ErrorTypeAuth:
		return false
	default:
		return true // 未知错误默认可重试
	}
}

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries      int           // 最大重试次数
	BaseDelay       time.Duration // 基础延迟
	MaxDelay        time.Duration // 最大延迟
	BackoffFactor   float64       // 退避因子
	Jitter          bool          // 是否添加抖动
	TimeoutPerRetry time.Duration // 每次重试的超时时间
}

// DefaultRetryConfig 默认重试配置
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:      3,
		BaseDelay:       100 * time.Millisecond,
		MaxDelay:        30 * time.Second,
		BackoffFactor:   2.0,
		Jitter:          true,
		TimeoutPerRetry: 30 * time.Second,
	}
}

// GetRetryConfigForErrorType 根据错误类型获取重试配置
func GetRetryConfigForErrorType(errorType ErrorType) *RetryConfig {
	config := DefaultRetryConfig()
	
	switch errorType {
	case ErrorTypeNetwork, ErrorTypeConnection:
		config.MaxRetries = 5
		config.BaseDelay = 200 * time.Millisecond
		config.MaxDelay = 60 * time.Second
	case ErrorTypeTimeout:
		config.MaxRetries = 3
		config.BaseDelay = 500 * time.Millisecond
		config.MaxDelay = 30 * time.Second
	case ErrorTypeRateLimit:
		config.MaxRetries = 10
		config.BaseDelay = 1 * time.Second
		config.MaxDelay = 300 * time.Second
		config.BackoffFactor = 1.5
	case ErrorTypeServer:
		config.MaxRetries = 2
		config.BaseDelay = 1 * time.Second
		config.MaxDelay = 10 * time.Second
	case ErrorTypeTemporary:
		config.MaxRetries = 4
		config.BaseDelay = 100 * time.Millisecond
		config.MaxDelay = 20 * time.Second
	default:
		// 使用默认配置
	}
	
	return config
}

// RetryStrategy 重试策略
type RetryStrategy struct {
	classifier *ErrorClassifier
}

// NewRetryStrategy 创建重试策略
func NewRetryStrategy() *RetryStrategy {
	return &RetryStrategy{
		classifier: NewErrorClassifier(),
	}
}

// CalculateDelay 计算重试延迟
func (rs *RetryStrategy) CalculateDelay(attempt int, config *RetryConfig) time.Duration {
	if attempt <= 0 {
		return 0
	}

	// 指数退避
	delay := float64(config.BaseDelay) * math.Pow(config.BackoffFactor, float64(attempt-1))
	
	// 限制最大延迟
	if delay > float64(config.MaxDelay) {
		delay = float64(config.MaxDelay)
	}

	// 添加抖动
	if config.Jitter {
		jitter := rand.Float64() * 0.1 * delay // 10% 抖动
		delay += jitter
	}

	return time.Duration(delay)
}

// ShouldRetry 判断是否应该重试
func (rs *RetryStrategy) ShouldRetry(err error, attempt int, config *RetryConfig) bool {
	if err == nil {
		return false
	}

	if attempt >= config.MaxRetries {
		return false
	}

	errorType := rs.classifier.ClassifyError(err)
	return rs.classifier.IsRetryable(errorType)
}

// GetRetryConfig 获取错误对应的重试配置
func (rs *RetryStrategy) GetRetryConfig(err error) *RetryConfig {
	errorType := rs.classifier.ClassifyError(err)
	return GetRetryConfigForErrorType(errorType)
}

// RetryFunc 重试函数类型
type RetryFunc func() error

// ExecuteWithRetry 执行带重试的函数
func (rs *RetryStrategy) ExecuteWithRetry(ctx context.Context, fn RetryFunc, config *RetryConfig) error {
	if config == nil {
		config = DefaultRetryConfig()
	}

	var lastErr error
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 执行函数
		if attempt == 0 {
			lastErr = fn()
		} else {
			// 创建带超时的上下文
			if config.TimeoutPerRetry > 0 {
				retryCtx, cancel := context.WithTimeout(ctx, config.TimeoutPerRetry)
				defer cancel()
				_ = retryCtx // 使用变量避免编译错误
			}
			lastErr = fn()
		}

		// 如果成功，返回
		if lastErr == nil {
			return nil
		}

		// 检查是否应该重试
		if !rs.ShouldRetry(lastErr, attempt, config) {
			break
		}

		// 如果不是最后一次尝试，等待延迟
		if attempt < config.MaxRetries {
			delay := rs.CalculateDelay(attempt, config)
			
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return lastErr
}

// RetryStats 重试统计
type RetryStats struct {
	TotalAttempts   int           `json:"total_attempts"`
	SuccessAttempts int           `json:"success_attempts"`
	FailedAttempts  int           `json:"failed_attempts"`
	TotalDelay      time.Duration `json:"total_delay"`
	LastError       string        `json:"last_error,omitempty"`
	ErrorType       ErrorType     `json:"error_type"`
}

// RetryWithStats 带统计的重试执行
func (rs *RetryStrategy) RetryWithStats(ctx context.Context, fn RetryFunc, config *RetryConfig) (*RetryStats, error) {
	stats := &RetryStats{}
	var lastErr error
	startTime := time.Now()
	
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		stats.TotalAttempts++
		
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			stats.LastError = ctx.Err().Error()
			return stats, ctx.Err()
		default:
		}

		// 执行函数
		lastErr = fn()

		// 如果成功，返回
		if lastErr == nil {
			stats.SuccessAttempts++
			stats.TotalDelay = time.Since(startTime)
			return stats, nil
		}

		stats.FailedAttempts++
		stats.LastError = lastErr.Error()
		stats.ErrorType = rs.classifier.ClassifyError(lastErr)

		// 检查是否应该重试
		if !rs.ShouldRetry(lastErr, attempt, config) {
			break
		}

		// 如果不是最后一次尝试，等待重试延迟
		if attempt < config.MaxRetries {
			delay := rs.CalculateDelay(attempt+1, config)
			
			select {
			case <-ctx.Done():
				stats.LastError = ctx.Err().Error()
				return stats, ctx.Err()
			case <-time.After(delay):
				// 继续重试
			}
		}
	}

	stats.TotalDelay = time.Since(startTime)
	return stats, fmt.Errorf("retry failed after %d attempts: %w", config.MaxRetries+1, lastErr)
}