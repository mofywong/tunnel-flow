package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 配置结构
type Config struct {
	// 服务器配置
	Server struct {
		URL string `yaml:"url" json:"server_url"`
	} `yaml:"server"`

	// 客户端配置
	Client struct {
		ID        string `yaml:"id" json:"client_id"`
		AuthToken string `yaml:"auth_token" json:"auth_token"`
	} `yaml:"client"`

	// SSL/TLS 配置
	SSL struct {
		InsecureSkipVerify bool `yaml:"insecure_skip_verify" json:"insecure_skip_verify"`
	} `yaml:"ssl"`
}

// 配置访问方法
func (c *Config) ServerURL() string {
	return c.Server.URL
}

func (c *Config) ClientID() string {
	return c.Client.ID
}

func (c *Config) AuthToken() string {
	return c.Client.AuthToken
}

func (c *Config) SSLInsecureSkipVerify() bool {
	return c.SSL.InsecureSkipVerify
}

// UseSSL 根据WebSocket URL的协议类型判断是否使用SSL
func (c *Config) UseSSL() bool {
	u, err := url.Parse(c.Server.URL)
	if err != nil {
		return false
	}
	return strings.ToLower(u.Scheme) == "wss"
}

// 以下方法提供默认值，保持向后兼容
func (c *Config) ServerHost() string {
	u, err := url.Parse(c.Server.URL)
	if err != nil {
		return "localhost"
	}
	return u.Hostname()
}

func (c *Config) ServerPort() int {
	u, err := url.Parse(c.Server.URL)
	if err != nil {
		return 8081
	}
	port := u.Port()
	if port == "" {
		if c.UseSSL() {
			return 443
		}
		return 80
	}
	if portInt, err := strconv.Atoi(port); err == nil {
		return portInt
	}
	return 8081
}

func (c *Config) ClientName() string {
	// 为了向后兼容，返回客户端ID作为名称
	return c.Client.ID
}

func (c *Config) MonitoringPort() int {
	return 9092
}

func (c *Config) ReconnectIntervalMS() int {
	return 3000
}

func (c *Config) PingIntervalMS() int {
	return 15000
}

func (c *Config) PingTimeoutMS() int {
	return 45000
}

func (c *Config) RequestTimeoutMS() int {
	return 10000
}

func (c *Config) MaxRetries() int {
	return 5
}

func (c *Config) RetryDelayMS() int {
	return 1000
}

func (c *Config) RetryInitialDelayMS() int {
	return 1000
}

func (c *Config) RetryMaxDelayMS() int {
	return 30000
}

func (c *Config) RetryMultiplier() float64 {
	return 2.0
}

func (c *Config) RetryMaxAttempts() int {
	return 5
}

func (c *Config) SendQueueSize() int {
	return 1000
}

func (c *Config) WorkerPoolSize() int {
	return 10
}

func (c *Config) WorkerQueueSize() int {
	return 100
}

func (c *Config) MessageQueueSize() int {
	return 1000
}

func (c *Config) BatchSize() int {
	return 100
}

func (c *Config) BatchTimeoutMS() int {
	return 1000
}

func (c *Config) CacheSize() int {
	return 1000
}

func (c *Config) CacheTTLSeconds() int {
	return 300
}

// Load 加载配置
func Load() (*Config, error) {
	config := &Config{}

	// 设置默认值
	setDefaults(config)

	// 尝试从配置文件加载
	configFile := getEnv("CONFIG_FILE", "config.yaml")
	if err := loadFromFile(config, configFile); err != nil {
		fmt.Printf("Warning: Failed to load config file %s: %v\n", configFile, err)
		fmt.Println("Using default configuration and environment variables")
	}

	// 从环境变量覆盖配置
	loadFromEnv(config)

	// 验证必要的配置
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	return config, nil
}

// setDefaults 设置默认值
func setDefaults(config *Config) {
	config.Server.URL = "ws://localhost:8081/ws"
	config.Client.ID = ""
	config.Client.AuthToken = ""
}

// loadFromFile 从文件加载配置
func loadFromFile(config *Config, filename string) error {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return fmt.Errorf("配置文件不存在: %s", filename)
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	return nil
}

// loadFromEnv 从环境变量加载配置
func loadFromEnv(config *Config) {
	if serverURL := getEnv("SERVER_URL", ""); serverURL != "" {
		config.Server.URL = serverURL
	}
	if clientID := getEnv("CLIENT_ID", ""); clientID != "" {
		config.Client.ID = clientID
	}
	if authToken := getEnv("AUTH_TOKEN", ""); authToken != "" {
		config.Client.AuthToken = authToken
	}
}

// validateConfig 验证配置
func validateConfig(config *Config) error {
	if config.Client.ID == "" {
		return fmt.Errorf("客户端ID不能为空，请在配置文件中设置client.id或通过环境变量CLIENT_ID设置")
	}
	if config.Client.AuthToken == "" {
		return fmt.Errorf("认证Token不能为空，请在配置文件中设置client.auth_token或通过环境变量AUTH_TOKEN设置")
	}
	if config.Server.URL == "" {
		return fmt.Errorf("服务器URL不能为空")
	}
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt 获取环境变量整数值
func getEnvInt(key string) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return 0
}

// getEnvFloat 获取环境变量浮点数值
func getEnvFloat(key string) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return 0
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func (c *Config) ReconnectInterval() time.Duration {
	return 5 * time.Second
}

func (c *Config) PingInterval() time.Duration {
	return 30 * time.Second
}

func (c *Config) PingTimeout() time.Duration {
	return 60 * time.Second
}

func (c *Config) HTTPTimeout() time.Duration {
	return 30 * time.Second
}
