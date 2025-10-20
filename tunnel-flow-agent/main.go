package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tunnel-flow-agent/internal/agent"
	"tunnel-flow-agent/internal/config"
	"tunnel-flow-agent/internal/logging"
	"tunnel-flow-agent/internal/monitoring"
)

func main() {
	// 初始化日志
	logConfig := logging.DefaultConfig()
	logConfig.Level = logging.INFO
	logConfig.Output = "file"
	logConfig.FilePath = "logs/client.log"
	
	logger, err := logging.NewLogger(logConfig)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()
	
	logger.Info("启动客户端代理...")
	
	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		logger.Errorf("Failed to load config: %v", err)
		os.Exit(1)
	}
	
	logger.WithField("config", cfg).Info("配置加载成功")
	
	// 创建监控收集器
	metricsCollector := monitoring.NewMetricsCollector()
	
	// 创建代理
	agentInstance := agent.NewAgent(cfg)
	
	// 启动监控HTTP服务器
	monitoringServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.MonitoringPort),
		Handler: setupMonitoringRoutes(metricsCollector, agentInstance, logger),
	}
	
	// 启动监控服务器
	go func() {
		logger.Infof("启动监控服务器在端口 %d", cfg.MonitoringPort)
		if err := monitoringServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Errorf("监控服务器启动失败: %v", err)
		}
	}()
	
	// 启动定期指标更新
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				metricsCollector.UpdateSystemMetrics()
				
				// 记录统计信息
				stats := agentInstance.GetStats()
				logger.WithFields(map[string]interface{}{
					"messages_sent":     stats["messages_sent"],
					"messages_received": stats["messages_received"],
					"error_count":       stats["error_count"],
					"uptime_seconds":    stats["uptime_seconds"],
				}).Debug("代理统计信息")
				
			case <-context.Background().Done():
				return
			}
		}
	}()
	
	// 启动代理
	if err := agentInstance.Start(); err != nil {
		logger.Errorf("Failed to start agent: %v", err)
		os.Exit(1)
	}
	
	logger.Info("客户端代理启动成功")
	
	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	
	<-sigChan
	logger.Info("接收到停止信号，正在关闭...")
	
	// 创建关闭上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// 关闭监控服务器
	if err := monitoringServer.Shutdown(ctx); err != nil {
		logger.Errorf("监控服务器关闭失败: %v", err)
	}
	
	// 关闭代理
	agentInstance.Stop()
	
	logger.Info("客户端代理已关闭")
}

// setupMonitoringRoutes 设置监控路由
func setupMonitoringRoutes(metricsCollector *monitoring.MetricsCollector, agentInstance *agent.Agent, logger *logging.Logger) http.Handler {
	mux := http.NewServeMux()
	
	// 指标接口
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		logger.Debug("处理指标请求")
		metricsCollector.HTTPHandler(w, r)
	})
	
	// 健康检查接口
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		logger.Debug("处理健康检查请求")
		
		w.Header().Set("Content-Type", "application/json")
		
		status := "healthy"
		if !agentInstance.IsRunning() {
			status = "unhealthy"
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"%s","timestamp":"%s"}`, status, time.Now().Format(time.RFC3339))
	})
	
	// 统计信息接口
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		logger.Debug("处理统计信息请求")
		
		stats := agentInstance.GetStats()
		w.Header().Set("Content-Type", "application/json")
		
		// 简单的JSON输出
		fmt.Fprintf(w, `{
			"messages_sent": %v,
			"messages_received": %v,
			"error_count": %v,
			"uptime_seconds": %v,
			"connection_state": "%v"
		}`, 
			stats["messages_sent"],
			stats["messages_received"], 
			stats["error_count"],
			stats["uptime_seconds"],
			stats["connection_state"])
	})
	
	// 配置信息接口
	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		logger.Debug("处理配置信息请求")
		
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"message": "Configuration endpoint"}`)
	})
	
	// 日志级别控制接口
	mux.HandleFunc("/log-level", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			level := r.URL.Query().Get("level")
			switch level {
			case "debug":
				logger.SetLevel(logging.DEBUG)
			case "info":
				logger.SetLevel(logging.INFO)
			case "warn":
				logger.SetLevel(logging.WARN)
			case "error":
				logger.SetLevel(logging.ERROR)
			default:
				http.Error(w, "Invalid log level", http.StatusBadRequest)
				return
			}
			
			logger.Infof("日志级别已更改为: %s", level)
			fmt.Fprintf(w, `{"message": "Log level changed to %s"}`, level)
		} else {
			currentLevel := logger.GetLevel().String()
			fmt.Fprintf(w, `{"current_level": "%s"}`, currentLevel)
		}
	})
	
	return mux
}