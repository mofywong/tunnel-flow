package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"tunnel-flow/internal/config"
	"tunnel-flow/internal/database"
	"tunnel-flow/internal/logging"
	"tunnel-flow/internal/monitoring"
	"tunnel-flow/internal/performance"
	"tunnel-flow/internal/server"
)

func main() {
	// 初始化日志
	logConfig := &logging.Config{
		Level:        "info",
		Filename:     "logs/tunnel-flow.log",
		MaxSize:      100 * 1024 * 1024, // 100MB
		MaxAge:       "168h",            // 7天
		MaxBackups:   10,
		EnableCaller: true,
	}

	if err := logging.InitDefaultLogger(logConfig); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	logging.Info("Starting tunnel-flow server...")

	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	logging.Info("Configuration loaded successfully")

	// 初始化数据库
	db, err := database.New(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// 创建Repository
	repo := database.NewRepository(db)
	logging.Info("Database initialized successfully")

	// 创建性能组件
	objectPool := performance.NewObjectPool()
	workerPool := performance.NewWorkerPool(cfg.WorkerPoolSize, cfg.WorkerQueueSize)
	connectionPool := performance.NewConnectionPool(&performance.ConnectionPoolConfig{
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConns / 2,
		MaxConnsPerHost:       cfg.MaxOpenConns,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		DialTimeout:           30 * time.Second,
		KeepAlive:             30 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	})

	// 启动工作池
	workerPool.Start()
	defer workerPool.Stop()

	logging.Info("Performance components initialized")

	// 创建监控组件
	metricsCollector := monitoring.NewMetricsCollector()
	healthChecker := monitoring.NewHealthChecker(5*time.Second, 30*time.Second)

	// 注册健康检查
	healthChecker.RegisterCheck(monitoring.NewDatabaseHealthCheck("database", func() error {
		return db.Health()
	}))

	// 启动定期监控
	metricsCollector.StartPeriodicCollection(10 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go healthChecker.StartPeriodicCheck(ctx)

	logging.Info("Monitoring components initialized")

	// 测试数据库连接
	if err := db.Health(); err != nil {
		log.Fatalf("Database health check failed: %v", err)
	}
	logging.Info("Database health check passed")

	// 定期检查数据库健康状态
	dbHealthCtx, dbHealthCancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := db.Health(); err != nil {
					logging.Errorf("Database health check failed: %v", err)
				}
			case <-dbHealthCtx.Done():
				return
			}
		}
	}()

	// 创建多端口服务器管理器
	multiServer := server.NewMultiServer(cfg, repo, objectPool, workerPool, metricsCollector)

	// 启动内存监控日志
	memoryMonitorCtx, memoryMonitorCancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(60 * time.Second) // 每分钟记录一次内存使用
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				logging.Infof("Memory Stats - Alloc: %d KB, TotalAlloc: %d KB, Sys: %d KB, NumGC: %d, Goroutines: %d",
					m.Alloc/1024, m.TotalAlloc/1024, m.Sys/1024, m.NumGC, runtime.NumGoroutine())
			case <-memoryMonitorCtx.Done():
				return
			}
		}
	}()

	// 启动多端口服务器
	go func() {
		logging.Info("Starting multi-port servers...")
		if err := multiServer.Start(); err != nil {
			log.Fatalf("Multi-server failed: %v", err)
		}
	}()

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logging.Info("Server started successfully. Press Ctrl+C to stop.")
	<-sigChan

	logging.Info("Shutting down server...")

	// 优雅关闭
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := multiServer.Stop(); err != nil {
		logging.Errorf("Error during server shutdown: %v", err)
	}

	// 停止监控组件
	metricsCollector.Stop()
	logging.Info("Metrics collector stopped")

	// 停止工作池
	workerPool.Stop()
	logging.Info("Worker pool stopped")

	// 停止内存监控
	memoryMonitorCancel()
	logging.Info("Memory monitor stopped")

	// 停止数据库健康检查
	dbHealthCancel()
	logging.Info("Database health monitor stopped")

	// 等待所有goroutine完成
	select {
	case <-shutdownCtx.Done():
		logging.Warn("Shutdown timeout exceeded")
	default:
		logging.Info("Server shutdown completed")
	}

	// 关闭连接池
	connectionPool.Close()

	logging.Info("Tunnel-flow server stopped")
}
