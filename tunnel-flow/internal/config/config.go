package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 配置结构
type Config struct {
	// 服务器配置 - 多端口支持
	APIPort       int    `json:"api_port" yaml:"server.api_port"`             // API接口端口 (8080)
	WebSocketPort int    `json:"websocket_port" yaml:"server.websocket_port"` // WebSocket端口 (8081)
	ProxyPort     int    `json:"proxy_port" yaml:"server.proxy_port"`         // HTTP代理端口 (8082)
	ServerHost    string `json:"server_host" yaml:"server.host"`
	ServerURL     string `json:"server_url"`

	// 兼容性配置 (保持向后兼容)
	ServerPort int `json:"server_port"` // 主端口，用于向后兼容

	// 数据库配置
	DatabasePath string `json:"database_path" yaml:"database.path"`

	// WebSocket配置
	SendQueueSize int `json:"send_queue_size" yaml:"websocket.send_queue_size"`

	// WebSocket SSL/TLS配置
	WebSocketSSLEnabled  bool   `json:"websocket_ssl_enabled" yaml:"websocket.ssl.enabled"`
	WebSocketSSLCertFile string `json:"websocket_ssl_cert_file" yaml:"websocket.ssl.cert_file"`
	WebSocketSSLKeyFile  string `json:"websocket_ssl_key_file" yaml:"websocket.ssl.key_file"`
	WebSocketSSLForceSSL bool   `json:"websocket_ssl_force_ssl" yaml:"websocket.ssl.force_ssl"`

	// 认证配置
	AuthJWTSecret string `json:"auth_jwt_secret" yaml:"auth.jwt_secret"`

	// 超时配置
	ReconnectIntervalMS int `json:"reconnect_interval_ms" yaml:"timeout.reconnect_interval_ms"`
	PingIntervalMS      int `json:"ping_interval_ms" yaml:"timeout.ping_interval_ms"`
	RequestTimeoutMS    int `json:"request_timeout_ms" yaml:"timeout.request_timeout_ms"`

	// 重试配置
	MaxRetries          int     `json:"max_retries" yaml:"retry.max_retries"`
	RetryInitialDelayMS int     `json:"retry_initial_delay_ms" yaml:"retry.initial_delay_ms"`
	RetryMaxDelayMS     int     `json:"retry_max_delay_ms" yaml:"retry.max_delay_ms"`
	RetryMultiplier     float64 `json:"retry_multiplier" yaml:"retry.multiplier"`
	RetryMaxAttempts    int     `json:"retry_max_attempts" yaml:"retry.max_attempts"`

	// 性能优化配置
	WorkerPoolSize   int `json:"worker_pool_size" yaml:"performance.worker_pool_size"`
	WorkerQueueSize  int `json:"worker_queue_size" yaml:"performance.worker_queue_size"`
	MessageQueueSize int `json:"message_queue_size" yaml:"performance.message_queue_size"`
	BatchSize        int `json:"batch_size" yaml:"performance.batch_size"`
	BatchTimeoutMS   int `json:"batch_timeout_ms" yaml:"performance.batch_timeout_ms"`

	// 连接池配置
	MaxIdleConns    int `json:"max_idle_conns" yaml:"connection_pool.max_idle_conns"`
	MaxOpenConns    int `json:"max_open_conns" yaml:"connection_pool.max_open_conns"`
	ConnMaxLifetime int `json:"conn_max_lifetime_seconds" yaml:"connection_pool.conn_max_lifetime_seconds"`

	// 缓存配置
	CacheSize       int `json:"cache_size" yaml:"cache.size"`
	CacheTTLSeconds int `json:"cache_ttl_seconds" yaml:"cache.ttl_seconds"`
}

// Load 加载配置
func Load() (*Config, error) {
	config := &Config{
		// 多端口默认值
		APIPort:       8080, // API接口端口
		WebSocketPort: 8081, // WebSocket端口
		ProxyPort:     8082, // HTTP代理端口
		ServerPort:    8080, // 向后兼容
		ServerHost:    "0.0.0.0",
		DatabasePath:  "",
		SendQueueSize: 1000,
		// WebSocket SSL 默认配置
		WebSocketSSLEnabled:  true,
		WebSocketSSLCertFile: "./ssl/server.crt",
		WebSocketSSLKeyFile:  "./ssl/server.key",
		WebSocketSSLForceSSL: true,
		AuthJWTSecret:        "your-secret-key",
		ReconnectIntervalMS:  5000,
		PingIntervalMS:       10000, // 改为10秒，与客户端保持一致
		RequestTimeoutMS:     30000,
		MaxRetries:           3,
		RetryInitialDelayMS:  100,
		RetryMaxDelayMS:      5000,
		RetryMultiplier:      2.0,
		RetryMaxAttempts:     5,
		// 性能优化默认值
		WorkerPoolSize:   10,
		WorkerQueueSize:  1000,
		MessageQueueSize: 10000,
		BatchSize:        100,
		BatchTimeoutMS:   1000,
		// 连接池默认值
		MaxIdleConns:    10,
		MaxOpenConns:    100,
		ConnMaxLifetime: 3600,
		// 缓存默认值
		CacheSize:       1000,
		CacheTTLSeconds: 300,
	}

	// 尝试从YAML文件读取配置
	if err := loadFromYAML(config); err != nil {
		// 如果YAML文件不存在或读取失败，继续使用默认值和环境变量
		fmt.Printf("Warning: Failed to load config.yaml: %v\n", err)
	}

	// 从环境变量读取配置
	if port := getEnvInt("API_PORT"); port > 0 {
		config.APIPort = port
	}
	if port := getEnvInt("WEBSOCKET_PORT"); port > 0 {
		config.WebSocketPort = port
	}
	if port := getEnvInt("PROXY_PORT"); port > 0 {
		config.ProxyPort = port
	}
	if port := getEnvInt("SERVER_PORT"); port > 0 {
		config.ServerPort = port
		// 如果设置了SERVER_PORT，同时更新API_PORT以保持兼容性
		if getEnvInt("API_PORT") == 0 {
			config.APIPort = port
		}
	}

	if host := os.Getenv("SERVER_HOST"); host != "" {
		config.ServerHost = host
	}

	if dbPath := os.Getenv("DATABASE_PATH"); dbPath != "" {
		config.DatabasePath = dbPath
	}

	if queueSize := getEnvInt("SEND_QUEUE_SIZE"); queueSize > 0 {
		config.SendQueueSize = queueSize
	}

	if secret := os.Getenv("AUTH_JWT_SECRET"); secret != "" {
		config.AuthJWTSecret = secret
	}

	if interval := getEnvInt("RECONNECT_INTERVAL_MS"); interval > 0 {
		config.ReconnectIntervalMS = interval
	}

	if interval := getEnvInt("PING_INTERVAL_MS"); interval > 0 {
		config.PingIntervalMS = interval
	}

	if timeout := getEnvInt("REQUEST_TIMEOUT_MS"); timeout > 0 {
		config.RequestTimeoutMS = timeout
	}

	if retries := getEnvInt("MAX_RETRIES"); retries > 0 {
		config.MaxRetries = retries
	}

	if delay := getEnvInt("RETRY_INITIAL_DELAY_MS"); delay > 0 {
		config.RetryInitialDelayMS = delay
	}

	if delay := getEnvInt("RETRY_MAX_DELAY_MS"); delay > 0 {
		config.RetryMaxDelayMS = delay
	}

	if multiplier := getEnvFloat("RETRY_MULTIPLIER"); multiplier > 0 {
		config.RetryMultiplier = multiplier
	}

	if attempts := getEnvInt("RETRY_MAX_ATTEMPTS"); attempts > 0 {
		config.RetryMaxAttempts = attempts
	}

	// 性能优化配置
	if poolSize := getEnvInt("WORKER_POOL_SIZE"); poolSize > 0 {
		config.WorkerPoolSize = poolSize
	}

	if queueSize := getEnvInt("WORKER_QUEUE_SIZE"); queueSize > 0 {
		config.WorkerQueueSize = queueSize
	}

	if queueSize := getEnvInt("MESSAGE_QUEUE_SIZE"); queueSize > 0 {
		config.MessageQueueSize = queueSize
	}

	if batchSize := getEnvInt("BATCH_SIZE"); batchSize > 0 {
		config.BatchSize = batchSize
	}

	if timeout := getEnvInt("BATCH_TIMEOUT_MS"); timeout > 0 {
		config.BatchTimeoutMS = timeout
	}

	// 连接池配置
	if conns := getEnvInt("MAX_IDLE_CONNS"); conns > 0 {
		config.MaxIdleConns = conns
	}

	if conns := getEnvInt("MAX_OPEN_CONNS"); conns > 0 {
		config.MaxOpenConns = conns
	}

	if lifetime := getEnvInt("CONN_MAX_LIFETIME_SECONDS"); lifetime > 0 {
		config.ConnMaxLifetime = lifetime
	}

	// 缓存配置
	if size := getEnvInt("CACHE_SIZE"); size > 0 {
		config.CacheSize = size
	}

	if ttl := getEnvInt("CACHE_TTL_SECONDS"); ttl > 0 {
		config.CacheTTLSeconds = ttl
	}

	// 构建服务器URL
	if config.ServerURL == "" {
		config.ServerURL = fmt.Sprintf("http://%s:%d", config.ServerHost, config.ServerPort)
	}

	return config, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return 0
}

func getEnvFloat(key string) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return 0
}

func (c *Config) PingInterval() time.Duration {
	return time.Duration(c.PingIntervalMS) * time.Millisecond
}

func (c *Config) RequestTimeout() time.Duration {
	return time.Duration(c.RequestTimeoutMS) * time.Millisecond
}

func (c *Config) ReconnectInterval() time.Duration {
	return time.Duration(c.ReconnectIntervalMS) * time.Millisecond
}

func (c *Config) BatchTimeout() time.Duration {
	return time.Duration(c.BatchTimeoutMS) * time.Millisecond
}

func (c *Config) ConnMaxLifetimeDuration() time.Duration {
	return time.Duration(c.ConnMaxLifetime) * time.Second
}

func (c *Config) CacheTTL() time.Duration {
	return time.Duration(c.CacheTTLSeconds) * time.Second
}

// loadFromYAML 从YAML文件加载配置
func loadFromYAML(config *Config) error {
	// 尝试读取config.yaml文件
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return fmt.Errorf("failed to read config.yaml: %w", err)
	}

	// 创建一个嵌套结构来匹配YAML格式
	var yamlConfig struct {
		Server struct {
			APIPort       int    `yaml:"api_port"`
			WebSocketPort int    `yaml:"websocket_port"`
			ProxyPort     int    `yaml:"proxy_port"`
			Host          string `yaml:"host"`
		} `yaml:"server"`
		Database struct {
			Path string `yaml:"path"`
		} `yaml:"database"`
		WebSocket struct {
			SendQueueSize int `yaml:"send_queue_size"`
			SSL           struct {
				Enabled  bool   `yaml:"enabled"`
				CertFile string `yaml:"cert_file"`
				KeyFile  string `yaml:"key_file"`
				ForceSSL bool   `yaml:"force_ssl"`
			} `yaml:"ssl"`
		} `yaml:"websocket"`
		Auth struct {
			JWTSecret string `yaml:"jwt_secret"`
		} `yaml:"auth"`
		Timeout struct {
			ReconnectIntervalMS int `yaml:"reconnect_interval_ms"`
			PingIntervalMS      int `yaml:"ping_interval_ms"`
			RequestTimeoutMS    int `yaml:"request_timeout_ms"`
		} `yaml:"timeout"`
		Retry struct {
			MaxRetries     int     `yaml:"max_retries"`
			InitialDelayMS int     `yaml:"initial_delay_ms"`
			MaxDelayMS     int     `yaml:"max_delay_ms"`
			Multiplier     float64 `yaml:"multiplier"`
			MaxAttempts    int     `yaml:"max_attempts"`
		} `yaml:"retry"`
		Performance struct {
			WorkerPoolSize   int `yaml:"worker_pool_size"`
			WorkerQueueSize  int `yaml:"worker_queue_size"`
			MessageQueueSize int `yaml:"message_queue_size"`
			BatchSize        int `yaml:"batch_size"`
			BatchTimeoutMS   int `yaml:"batch_timeout_ms"`
		} `yaml:"performance"`
		ConnectionPool struct {
			MaxIdleConns           int `yaml:"max_idle_conns"`
			MaxOpenConns           int `yaml:"max_open_conns"`
			ConnMaxLifetimeSeconds int `yaml:"conn_max_lifetime_seconds"`
		} `yaml:"connection_pool"`
		Cache struct {
			Size       int `yaml:"size"`
			TTLSeconds int `yaml:"ttl_seconds"`
		} `yaml:"cache"`
	}

	// 解析YAML
	if err := yaml.Unmarshal(data, &yamlConfig); err != nil {
		return fmt.Errorf("failed to parse config.yaml: %w", err)
	}

	// 将YAML配置映射到Config结构体
	if yamlConfig.Server.APIPort > 0 {
		config.APIPort = yamlConfig.Server.APIPort
	}
	if yamlConfig.Server.WebSocketPort > 0 {
		config.WebSocketPort = yamlConfig.Server.WebSocketPort
	}
	if yamlConfig.Server.ProxyPort > 0 {
		config.ProxyPort = yamlConfig.Server.ProxyPort
	}
	if yamlConfig.Server.Host != "" {
		config.ServerHost = yamlConfig.Server.Host
	}
	if yamlConfig.Database.Path != "" {
		config.DatabasePath = yamlConfig.Database.Path
	}
	if yamlConfig.WebSocket.SendQueueSize > 0 {
		config.SendQueueSize = yamlConfig.WebSocket.SendQueueSize
	}
	// WebSocket SSL 配置
	config.WebSocketSSLEnabled = yamlConfig.WebSocket.SSL.Enabled
	if yamlConfig.WebSocket.SSL.CertFile != "" {
		config.WebSocketSSLCertFile = yamlConfig.WebSocket.SSL.CertFile
	}
	if yamlConfig.WebSocket.SSL.KeyFile != "" {
		config.WebSocketSSLKeyFile = yamlConfig.WebSocket.SSL.KeyFile
	}
	config.WebSocketSSLForceSSL = yamlConfig.WebSocket.SSL.ForceSSL
	if yamlConfig.Auth.JWTSecret != "" {
		config.AuthJWTSecret = yamlConfig.Auth.JWTSecret
	}
	if yamlConfig.Timeout.ReconnectIntervalMS > 0 {
		config.ReconnectIntervalMS = yamlConfig.Timeout.ReconnectIntervalMS
	}
	if yamlConfig.Timeout.PingIntervalMS > 0 {
		config.PingIntervalMS = yamlConfig.Timeout.PingIntervalMS
	}
	if yamlConfig.Timeout.RequestTimeoutMS > 0 {
		config.RequestTimeoutMS = yamlConfig.Timeout.RequestTimeoutMS
	}
	if yamlConfig.Retry.MaxRetries > 0 {
		config.MaxRetries = yamlConfig.Retry.MaxRetries
	}
	if yamlConfig.Retry.InitialDelayMS > 0 {
		config.RetryInitialDelayMS = yamlConfig.Retry.InitialDelayMS
	}
	if yamlConfig.Retry.MaxDelayMS > 0 {
		config.RetryMaxDelayMS = yamlConfig.Retry.MaxDelayMS
	}
	if yamlConfig.Retry.Multiplier > 0 {
		config.RetryMultiplier = yamlConfig.Retry.Multiplier
	}
	if yamlConfig.Retry.MaxAttempts > 0 {
		config.RetryMaxAttempts = yamlConfig.Retry.MaxAttempts
	}
	if yamlConfig.Performance.WorkerPoolSize > 0 {
		config.WorkerPoolSize = yamlConfig.Performance.WorkerPoolSize
	}
	if yamlConfig.Performance.WorkerQueueSize > 0 {
		config.WorkerQueueSize = yamlConfig.Performance.WorkerQueueSize
	}
	if yamlConfig.Performance.MessageQueueSize > 0 {
		config.MessageQueueSize = yamlConfig.Performance.MessageQueueSize
	}
	if yamlConfig.Performance.BatchSize > 0 {
		config.BatchSize = yamlConfig.Performance.BatchSize
	}
	if yamlConfig.Performance.BatchTimeoutMS > 0 {
		config.BatchTimeoutMS = yamlConfig.Performance.BatchTimeoutMS
	}
	if yamlConfig.ConnectionPool.MaxIdleConns > 0 {
		config.MaxIdleConns = yamlConfig.ConnectionPool.MaxIdleConns
	}
	if yamlConfig.ConnectionPool.MaxOpenConns > 0 {
		config.MaxOpenConns = yamlConfig.ConnectionPool.MaxOpenConns
	}
	if yamlConfig.ConnectionPool.ConnMaxLifetimeSeconds > 0 {
		config.ConnMaxLifetime = yamlConfig.ConnectionPool.ConnMaxLifetimeSeconds
	}
	if yamlConfig.Cache.Size > 0 {
		config.CacheSize = yamlConfig.Cache.Size
	}
	if yamlConfig.Cache.TTLSeconds > 0 {
		config.CacheTTLSeconds = yamlConfig.Cache.TTLSeconds
	}

	return nil
}
