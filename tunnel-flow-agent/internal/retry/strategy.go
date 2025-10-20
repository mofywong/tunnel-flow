package retry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"syscall"
	"time"
)

// ErrorType 定义错误类型
type ErrorType int

const (
	ErrorTypeNetwork ErrorType = iota
	ErrorTypeTimeout
	ErrorTypeAuth
	ErrorTypeProtocol
	ErrorTypeServer
	ErrorTypeUnknown
)

// ErrorClassifier 错误分类器
type ErrorClassifier struct{}

// ClassifyError 对错误进行分类
func (ec *ErrorClassifier) ClassifyError(err error) ErrorType {
	if err == nil {
		return ErrorTypeUnknown
	}

	// 网络错误
	if netErr, ok := err.(net.Error); ok {
		if netErr.Timeout() {
			return ErrorTypeTimeout
		}
		return ErrorTypeNetwork
	}

	// 系统调用错误
	if opErr, ok := err.(*net.OpError); ok {
		if syscallErr, ok := opErr.Err.(*syscall.Errno); ok {
			switch *syscallErr {
			case syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ECONNABORTED:
				return ErrorTypeNetwork
			case syscall.ETIMEDOUT:
				return ErrorTypeTimeout
			}
		}
		return ErrorTypeNetwork
	}

	// 根据错误消息判断
	errMsg := err.Error()
	switch {
	case contains(errMsg, "timeout", "deadline exceeded"):
		return ErrorTypeTimeout
	case contains(errMsg, "connection", "network", "dial", "socket"):
		return ErrorTypeNetwork
	case contains(errMsg, "unauthorized", "forbidden", "authentication"):
		return ErrorTypeAuth
	case contains(errMsg, "protocol", "invalid", "malformed"):
		return ErrorTypeProtocol
	case contains(errMsg, "server", "internal", "service"):
		return ErrorTypeServer
	default:
		return ErrorTypeUnknown
	}
}

// contains 检查字符串是否包含任一关键词
func contains(s string, keywords ...string) bool {
	for _, keyword := range keywords {
		if len(s) >= len(keyword) {
			for i := 0; i <= len(s)-len(keyword); i++ {
				if s[i:i+len(keyword)] == keyword {
					return true
				}
			}
		}
	}
	return false
}

// RetryConfig 重试配置
type RetryConfig struct {
	MaxAttempts  int           // 最大重试次数
	InitialDelay time.Duration // 初始延迟
	MaxDelay     time.Duration // 最大延迟
	Multiplier   float64       // 延迟倍数
	Jitter       bool          // 是否添加随机抖动
}

// DefaultRetryConfig 默认重试配置
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:  5,
		InitialDelay: time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
	}
}

// RetryStrategy 重试策略
type RetryStrategy struct {
	config     *RetryConfig
	classifier *ErrorClassifier
}

// NewRetryStrategy 创建重试策略
func NewRetryStrategy(config *RetryConfig) *RetryStrategy {
	if config == nil {
		config = DefaultRetryConfig()
	}
	return &RetryStrategy{
		config:     config,
		classifier: &ErrorClassifier{},
	}
}

// CalculateDelay 计算重试延迟
func (rs *RetryStrategy) CalculateDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}

	// 指数退避
	delay := float64(rs.config.InitialDelay) * math.Pow(rs.config.Multiplier, float64(attempt-1))
	
	// 限制最大延迟
	if delay > float64(rs.config.MaxDelay) {
		delay = float64(rs.config.MaxDelay)
	}

	// 添加随机抖动
	if rs.config.Jitter {
		jitter := rand.Float64() * 0.1 * delay // 10% 抖动
		delay += jitter
	}

	return time.Duration(delay)
}

// ShouldRetry 判断是否应该重试
func (rs *RetryStrategy) ShouldRetry(err error, attempt int) bool {
	if err == nil || attempt >= rs.config.MaxAttempts {
		return false
	}

	errorType := rs.classifier.ClassifyError(err)
	
	// 根据错误类型决定是否重试
	switch errorType {
	case ErrorTypeNetwork, ErrorTypeTimeout, ErrorTypeServer:
		return true
	case ErrorTypeAuth, ErrorTypeProtocol:
		return false // 认证和协议错误通常不需要重试
	default:
		return attempt < 3 // 未知错误最多重试3次
	}
}

// ExecuteWithRetry 执行带重试的函数
func (rs *RetryStrategy) ExecuteWithRetry(ctx context.Context, fn func() error) error {
	var lastErr error
	
	for attempt := 1; attempt <= rs.config.MaxAttempts; attempt++ {
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 执行函数
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// 判断是否应该重试
		if !rs.ShouldRetry(err, attempt) {
			break
		}

		// 如果不是最后一次尝试，等待重试延迟
		if attempt < rs.config.MaxAttempts {
			delay := rs.CalculateDelay(attempt)
			
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				// 继续重试
			}
		}
	}

	return fmt.Errorf("重试失败，最后错误: %w", lastErr)
}

// GetErrorType 获取错误类型
func (rs *RetryStrategy) GetErrorType(err error) ErrorType {
	return rs.classifier.ClassifyError(err)
}

// RetryableError 可重试错误包装
type RetryableError struct {
	Err       error
	Retryable bool
	ErrorType ErrorType
}

func (re *RetryableError) Error() string {
	return re.Err.Error()
}

func (re *RetryableError) Unwrap() error {
	return re.Err
}

// WrapError 包装错误
func (rs *RetryStrategy) WrapError(err error) *RetryableError {
	if err == nil {
		return nil
	}

	errorType := rs.classifier.ClassifyError(err)
	retryable := rs.ShouldRetry(err, 1)

	return &RetryableError{
		Err:       err,
		Retryable: retryable,
		ErrorType: errorType,
	}
}

// IsRetryableError 检查是否为可重试错误
func IsRetryableError(err error) bool {
	var retryableErr *RetryableError
	if errors.As(err, &retryableErr) {
		return retryableErr.Retryable
	}
	return false
}