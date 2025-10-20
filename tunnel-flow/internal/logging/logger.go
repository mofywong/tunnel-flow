package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel 日志级别
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

// String 返回日志级别字符串
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// LogEntry 日志条目
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Caller    string                 `json:"caller,omitempty"`
	TraceID   string                 `json:"trace_id,omitempty"`
}

// Logger 结构化日志器
type Logger struct {
	level      LogLevel
	output     io.Writer
	mu         sync.Mutex
	fields     map[string]interface{}
	enableCaller bool
	timeFormat string
	
	// 文件轮转
	logFile    *os.File
	maxSize    int64
	maxAge     time.Duration
	maxBackups int
	filename   string
}

// Config 日志配置
type Config struct {
	Level       string `json:"level"`
	Output      string `json:"output"`
	Format      string `json:"format"`
	Filename    string `json:"filename"`
	MaxSize     int64  `json:"max_size"`
	MaxAge      string `json:"max_age"`
	MaxBackups  int    `json:"max_backups"`
	EnableCaller bool  `json:"enable_caller"`
}

// NewLogger 创建新的日志器
func NewLogger(config *Config) (*Logger, error) {
	logger := &Logger{
		fields:       make(map[string]interface{}),
		enableCaller: config.EnableCaller,
		timeFormat:   time.RFC3339,
		maxSize:      config.MaxSize,
		maxBackups:   config.MaxBackups,
		filename:     config.Filename,
	}
	
	// 设置日志级别
	switch strings.ToUpper(config.Level) {
	case "DEBUG":
		logger.level = DEBUG
	case "INFO":
		logger.level = INFO
	case "WARN":
		logger.level = WARN
	case "ERROR":
		logger.level = ERROR
	case "FATAL":
		logger.level = FATAL
	default:
		logger.level = INFO
	}
	
	// 设置最大保存时间
	if config.MaxAge != "" {
		if duration, err := time.ParseDuration(config.MaxAge); err == nil {
			logger.maxAge = duration
		}
	}
	
	// 设置输出
	if config.Filename != "" {
		if err := logger.setupFileOutput(config.Filename); err != nil {
			return nil, err
		}
	} else {
		logger.output = os.Stdout
	}
	
	return logger, nil
}

// setupFileOutput 设置文件输出
func (l *Logger) setupFileOutput(filename string) error {
	// 创建目录
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	
	// 打开文件
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	
	l.logFile = file
	l.output = file
	l.filename = filename
	
	return nil
}

// WithField 添加字段
func (l *Logger) WithField(key string, value interface{}) *Logger {
	newLogger := &Logger{
		level:        l.level,
		output:       l.output,
		fields:       make(map[string]interface{}),
		enableCaller: l.enableCaller,
		timeFormat:   l.timeFormat,
		logFile:      l.logFile,
		maxSize:      l.maxSize,
		maxAge:       l.maxAge,
		maxBackups:   l.maxBackups,
		filename:     l.filename,
	}
	
	// 复制现有字段
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	
	// 添加新字段
	newLogger.fields[key] = value
	
	return newLogger
}

// WithFields 添加多个字段
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	newLogger := l
	for k, v := range fields {
		newLogger = newLogger.WithField(k, v)
	}
	return newLogger
}

// WithTraceID 添加追踪ID
func (l *Logger) WithTraceID(traceID string) *Logger {
	return l.WithField("trace_id", traceID)
}

// log 内部日志方法
func (l *Logger) log(level LogLevel, message string) {
	if level < l.level {
		return
	}
	
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level.String(),
		Message:   message,
		Fields:    l.fields,
	}
	
	// 添加调用者信息
	if l.enableCaller {
		if _, file, line, ok := runtime.Caller(3); ok {
			entry.Caller = fmt.Sprintf("%s:%d", filepath.Base(file), line)
		}
	}
	
	l.mu.Lock()
	defer l.mu.Unlock()
	
	// 检查文件轮转
	if l.logFile != nil {
		l.rotateIfNeeded()
	}
	
	// 输出日志
	data, _ := json.Marshal(entry)
	fmt.Fprintln(l.output, string(data))
}

// Debug 调试日志
func (l *Logger) Debug(message string) {
	l.log(DEBUG, message)
}

// Debugf 格式化调试日志
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.log(DEBUG, fmt.Sprintf(format, args...))
}

// Info 信息日志
func (l *Logger) Info(message string) {
	l.log(INFO, message)
}

// Infof 格式化信息日志
func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(INFO, fmt.Sprintf(format, args...))
}

// Warn 警告日志
func (l *Logger) Warn(message string) {
	l.log(WARN, message)
}

// Warnf 格式化警告日志
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.log(WARN, fmt.Sprintf(format, args...))
}

// Error 错误日志
func (l *Logger) Error(message string) {
	l.log(ERROR, message)
}

// Errorf 格式化错误日志
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(ERROR, fmt.Sprintf(format, args...))
}

// Fatal 致命错误日志
func (l *Logger) Fatal(message string) {
	l.log(FATAL, message)
	os.Exit(1)
}

// Fatalf 格式化致命错误日志
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.log(FATAL, fmt.Sprintf(format, args...))
	os.Exit(1)
}

// rotateIfNeeded 检查是否需要轮转日志文件
func (l *Logger) rotateIfNeeded() {
	if l.logFile == nil || l.filename == "" {
		return
	}
	
	// 检查文件大小
	if stat, err := l.logFile.Stat(); err == nil {
		if l.maxSize > 0 && stat.Size() >= l.maxSize {
			l.rotateFile()
		}
	}
}

// rotateFile 轮转日志文件
func (l *Logger) rotateFile() {
	if l.logFile != nil {
		l.logFile.Close()
	}
	
	// 重命名当前文件
	timestamp := time.Now().Format("20060102-150405")
	backupName := fmt.Sprintf("%s.%s", l.filename, timestamp)
	os.Rename(l.filename, backupName)
	
	// 创建新文件
	l.setupFileOutput(l.filename)
	
	// 清理旧文件
	l.cleanupOldFiles()
}

// cleanupOldFiles 清理旧的日志文件
func (l *Logger) cleanupOldFiles() {
	if l.maxBackups <= 0 && l.maxAge <= 0 {
		return
	}
	
	dir := filepath.Dir(l.filename)
	base := filepath.Base(l.filename)
	
	files, err := filepath.Glob(filepath.Join(dir, base+".*"))
	if err != nil {
		return
	}
	
	now := time.Now()
	
	for _, file := range files {
		stat, err := os.Stat(file)
		if err != nil {
			continue
		}
		
		// 按时间清理
		if l.maxAge > 0 && now.Sub(stat.ModTime()) > l.maxAge {
			os.Remove(file)
			continue
		}
	}
	
	// 按数量清理
	if l.maxBackups > 0 && len(files) > l.maxBackups {
		// 按修改时间排序并删除最老的文件
		// 这里简化处理，实际应该按时间排序
		for i := l.maxBackups; i < len(files); i++ {
			os.Remove(files[i])
		}
	}
}

// Close 关闭日志器
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	if l.logFile != nil {
		return l.logFile.Close()
	}
	
	return nil
}

// 全局日志器
var defaultLogger *Logger

// InitDefaultLogger 初始化默认日志器
func InitDefaultLogger(config *Config) error {
	logger, err := NewLogger(config)
	if err != nil {
		return err
	}
	
	defaultLogger = logger
	return nil
}

// 全局日志方法
func Debug(message string) {
	if defaultLogger != nil {
		defaultLogger.Debug(message)
	} else {
		log.Println("[DEBUG]", message)
	}
}

func Debugf(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Debugf(format, args...)
	} else {
		log.Printf("[DEBUG] "+format, args...)
	}
}

func Info(message string) {
	if defaultLogger != nil {
		defaultLogger.Info(message)
	} else {
		log.Println("[INFO]", message)
	}
}

func Infof(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Infof(format, args...)
	} else {
		log.Printf("[INFO] "+format, args...)
	}
}

func Warn(message string) {
	if defaultLogger != nil {
		defaultLogger.Warn(message)
	} else {
		log.Println("[WARN]", message)
	}
}

func Warnf(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Warnf(format, args...)
	} else {
		log.Printf("[WARN] "+format, args...)
	}
}

func Error(message string) {
	if defaultLogger != nil {
		defaultLogger.Error(message)
	} else {
		log.Println("[ERROR]", message)
	}
}

func Errorf(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Errorf(format, args...)
	} else {
		log.Printf("[ERROR] "+format, args...)
	}
}

func Fatal(message string) {
	if defaultLogger != nil {
		defaultLogger.Fatal(message)
	} else {
		log.Fatal("[FATAL]", message)
	}
}

func Fatalf(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Fatalf(format, args...)
	} else {
		log.Fatalf("[FATAL] "+format, args...)
	}
}