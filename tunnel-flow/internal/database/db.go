package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// DB 数据库连接
type DB struct {
	*sql.DB
}

// New 创建新的数据库连接
func New(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 配置连接池 - 减少并发连接数以避免SQLite锁定
	db.SetMaxOpenConns(1)  // SQLite建议使用单连接避免锁定
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	// 测试连接
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	wrapper := &DB{DB: db}

	// 初始化数据库
	if err := wrapper.init(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return wrapper, nil
}

// init 初始化数据库
func (db *DB) init() error {
	// 设置PRAGMA
	pragmas := []string{
		"PRAGMA journal_mode = WAL",     // WAL模式提供更好的并发性能
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 10000",   // 增加超时时间
		"PRAGMA foreign_keys = ON",
		"PRAGMA cache_size = -64000",    // 64MB cache
		"PRAGMA wal_autocheckpoint = 1000", // WAL模式下的自动检查点
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("failed to execute pragma %s: %w", pragma, err)
		}
	}

	// 创建表
	if err := db.createTables(); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	// 执行数据迁移
	if err := db.MigrateClientTables(); err != nil {
		return fmt.Errorf("failed to migrate client tables: %w", err)
	}

	return nil
}

// createTables 创建数据库表
func (db *DB) createTables() error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS clients (
			client_id TEXT PRIMARY KEY,
			name TEXT,
			description TEXT,
			auth_token TEXT,
			status TEXT,
			enabled INTEGER DEFAULT 1,
			last_seen_ts INTEGER,
			heartbeat_interval INTEGER DEFAULT 30,
			heartbeat_timeout INTEGER DEFAULT 90,
			created_at INTEGER,
			updated_at INTEGER,
			local_ips TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS server_routes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url_suffix TEXT NOT NULL,
			client_id TEXT NOT NULL,
			targets_json TEXT NOT NULL,
			delivery_policy TEXT DEFAULT 'first_success',
			active INTEGER DEFAULT 1,
			created_at INTEGER,
			UNIQUE(url_suffix, client_id)
		)`,
		`CREATE TABLE IF NOT EXISTS pending_messages (
			msg_id TEXT PRIMARY KEY,
			client_id TEXT,
			url_suffix TEXT,
			request_meta_json TEXT,
			state TEXT,
			retry_count INTEGER DEFAULT 0,
			next_try_ts INTEGER,
			created_at INTEGER,
			last_update INTEGER,
			response_meta_json TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			msg_id TEXT,
			client_id TEXT,
			direction TEXT,
			payload_summary TEXT,
			ts INTEGER
		)`,
	}

	for _, table := range tables {
		if _, err := db.Exec(table); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	// 数据库迁移：为现有表添加字段
	if err := db.migrateDatabase(); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	// 创建索引
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_server_routes_url_suffix ON server_routes(url_suffix)",
		"CREATE INDEX IF NOT EXISTS idx_server_routes_client_id ON server_routes(client_id)",
		"CREATE INDEX IF NOT EXISTS idx_pending_messages_client_id ON pending_messages(client_id)",
		"CREATE INDEX IF NOT EXISTS idx_pending_messages_state ON pending_messages(state)",
		"CREATE INDEX IF NOT EXISTS idx_pending_messages_next_try_ts ON pending_messages(next_try_ts)",
		"CREATE INDEX IF NOT EXISTS idx_audit_logs_msg_id ON audit_logs(msg_id)",
		"CREATE INDEX IF NOT EXISTS idx_audit_logs_client_id ON audit_logs(client_id)",
		"CREATE INDEX IF NOT EXISTS idx_audit_logs_ts ON audit_logs(ts)",
	}

	for _, index := range indexes {
		if _, err := db.Exec(index); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// migrateDatabase 执行数据库迁移
func (db *DB) migrateDatabase() error {
	// 执行server_routes表迁移
	if err := db.MigrateServerRoutes(); err != nil {
		return fmt.Errorf("failed to migrate server_routes: %w", err)
	}

	// 执行server_routes表V2迁移
	if err := db.MigrateServerRoutesV2(); err != nil {
		return fmt.Errorf("failed to migrate server_routes V2: %w", err)
	}

	return nil
}

// Close 关闭数据库连接
func (db *DB) Close() error {
	return db.DB.Close()
}

// Health 检查数据库健康状态
func (db *DB) Health() error {
	return db.Ping()
}