package logging

import (
	"encoding/json"
	"fmt"
	"io"
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
	Component string                 `json:"component,omitempty"`
}

// Logger 结构化日志器
type Logger struct {
	level     LogLevel
	output    io.Writer
	file      *os.File
	component string
	fields    map[string]interface{}
	mu        sync.Mutex
	
	// 配置
	config *Config
}

// Config 日志配置
type Config struct {
	Level       LogLevel `json:"level"`
	Format      string   `json:"format"`      // "json" or "text"
	Output      string   `json:"output"`      // "stdout", "stderr", "file"
	FilePath    string   `json:"file_path"`
	MaxSize     int64    `json:"max_size"`    // 文件最大大小(MB)
	MaxBackups  int      `json:"max_backups"` // 保留备份数
	MaxAge      int      `json:"max_age"`     // 保留天数
	Compress    bool     `json:"compress"`    // 是否压缩
	EnableCaller bool    `json:"enable_caller"`
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		Level:        INFO,
		Format:       "json",
		Output:       "stdout",
		FilePath:     "logs/client.log",
		MaxSize:      100,
		MaxBackups:   5,
		MaxAge:       30,
		Compress:     true,
		EnableCaller: true,
	}
}

// NewLogger 创建新的日志器
func NewLogger(config *Config) (*Logger, error) {
	if config == nil {
		config = DefaultConfig()
	}
	
	logger := &Logger{
		level:     config.Level,
		component: "client",
		fields:    make(map[string]interface{}),
		config:    config,
	}
	
	// 设置输出
	if err := logger.setupOutput(); err != nil {
		return nil, err
	}
	
	return logger, nil
}

// setupOutput 设置输出目标
func (l *Logger) setupOutput() error {
	switch l.config.Output {
	case "stdout":
		l.output = os.Stdout
	case "stderr":
		l.output = os.Stderr
	case "file":
		if err := l.setupFileOutput(); err != nil {
			return err
		}
	default:
		l.output = os.Stdout
	}
	
	return nil
}

// setupFileOutput 设置文件输出
func (l *Logger) setupFileOutput() error {
	// 创建日志目录
	dir := filepath.Dir(l.config.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}
	
	// 打开日志文件
	file, err := os.OpenFile(l.config.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	
	l.file = file
	l.output = file
	
	return nil
}

// WithField 添加字段
func (l *Logger) WithField(key string, value interface{}) *Logger {
	newLogger := &Logger{
		level:     l.level,
		output:    l.output,
		file:      l.file,
		component: l.component,
		fields:    make(map[string]interface{}),
		config:    l.config,
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
	newLogger := &Logger{
		level:     l.level,
		output:    l.output,
		file:      l.file,
		component: l.component,
		fields:    make(map[string]interface{}),
		config:    l.config,
	}
	
	// 复制现有字段
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	
	// 添加新字段
	for k, v := range fields {
		newLogger.fields[k] = v
	}
	
	return newLogger
}

// WithComponent 设置组件名
func (l *Logger) WithComponent(component string) *Logger {
	newLogger := &Logger{
		level:     l.level,
		output:    l.output,
		file:      l.file,
		component: component,
		fields:    make(map[string]interface{}),
		config:    l.config,
	}
	
	// 复制现有字段
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	
	return newLogger
}

// log 写入日志
func (l *Logger) log(level LogLevel, message string) {
	if level < l.level {
		return
	}
	
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level.String(),
		Message:   message,
		Component: l.component,
	}
	
	// 添加字段
	if len(l.fields) > 0 {
		entry.Fields = make(map[string]interface{})
		for k, v := range l.fields {
			entry.Fields[k] = v
		}
	}
	
	// 添加调用者信息
	if l.config.EnableCaller {
		if caller := l.getCaller(); caller != "" {
			entry.Caller = caller
		}
	}
	
	// 格式化输出
	var output string
	if l.config.Format == "json" {
		data, _ := json.Marshal(entry)
		output = string(data) + "\n"
	} else {
		output = l.formatText(entry)
	}
	
	// 写入输出
	l.mu.Lock()
	defer l.mu.Unlock()
	
	if l.output != nil {
		l.output.Write([]byte(output))
	}
	
	// 检查文件大小并轮转
	if l.file != nil {
		l.checkRotation()
	}
}

// formatText 格式化文本输出
func (l *Logger) formatText(entry LogEntry) string {
	var parts []string
	
	// 时间戳
	parts = append(parts, entry.Timestamp.Format("2006-01-02 15:04:05"))
	
	// 级别
	parts = append(parts, fmt.Sprintf("[%s]", entry.Level))
	
	// 组件
	if entry.Component != "" {
		parts = append(parts, fmt.Sprintf("[%s]", entry.Component))
	}
	
	// 调用者
	if entry.Caller != "" {
		parts = append(parts, fmt.Sprintf("[%s]", entry.Caller))
	}
	
	// 消息
	parts = append(parts, entry.Message)
	
	// 字段
	if entry.Fields != nil && len(entry.Fields) > 0 {
		var fieldParts []string
		for k, v := range entry.Fields {
			fieldParts = append(fieldParts, fmt.Sprintf("%s=%v", k, v))
		}
		parts = append(parts, fmt.Sprintf("{%s}", strings.Join(fieldParts, ", ")))
	}
	
	return strings.Join(parts, " ") + "\n"
}

// getCaller 获取调用者信息
func (l *Logger) getCaller() string {
	// 跳过日志框架的调用栈
	_, file, line, ok := runtime.Caller(3)
	if !ok {
		return ""
	}
	
	// 只保留文件名
	filename := filepath.Base(file)
	return fmt.Sprintf("%s:%d", filename, line)
}

// checkRotation 检查文件轮转
func (l *Logger) checkRotation() {
	if l.file == nil {
		return
	}
	
	// 获取文件信息
	info, err := l.file.Stat()
	if err != nil {
		return
	}
	
	// 检查文件大小
	maxSize := l.config.MaxSize * 1024 * 1024 // 转换为字节
	if info.Size() > maxSize {
		l.rotateFile()
	}
}

// rotateFile 轮转文件
func (l *Logger) rotateFile() {
	if l.file == nil {
		return
	}
	
	// 关闭当前文件
	l.file.Close()
	
	// 重命名当前文件
	timestamp := time.Now().Format("20060102-150405")
	backupPath := fmt.Sprintf("%s.%s", l.config.FilePath, timestamp)
	os.Rename(l.config.FilePath, backupPath)
	
	// 创建新文件
	l.setupFileOutput()
	
	// 清理旧文件
	l.cleanupOldFiles()
}

// cleanupOldFiles 清理旧文件
func (l *Logger) cleanupOldFiles() {
	dir := filepath.Dir(l.config.FilePath)
	basename := filepath.Base(l.config.FilePath)
	
	// 读取目录
	files, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	
	// 查找备份文件
	var backupFiles []os.DirEntry
	for _, file := range files {
		if strings.HasPrefix(file.Name(), basename+".") {
			backupFiles = append(backupFiles, file)
		}
	}
	
	// 删除超过保留数量的文件
	if len(backupFiles) > l.config.MaxBackups {
		// 按修改时间排序并删除最旧的文件
		// 这里简化处理，实际应该按时间排序
		for i := l.config.MaxBackups; i < len(backupFiles); i++ {
			os.Remove(filepath.Join(dir, backupFiles[i].Name()))
		}
	}
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

// Close 关闭日志器
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	if l.file != nil {
		return l.file.Close()
	}
	
	return nil
}

// SetLevel 设置日志级别
func (l *Logger) SetLevel(level LogLevel) {
	l.level = level
}

// GetLevel 获取日志级别
func (l *Logger) GetLevel() LogLevel {
	return l.level
}