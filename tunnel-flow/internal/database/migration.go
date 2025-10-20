package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// MigrateClientTables 执行客户端表合并迁移
func (db *DB) MigrateClientTables() error {
	log.Println("开始执行客户端表合并迁移...")

	// 1. 检查是否需要迁移
	needsMigration, err := db.checkMigrationNeeded()
	if err != nil {
		log.Printf("检查迁移状态失败，跳过迁移: %v", err)
		return nil
	}

	if !needsMigration {
		log.Println("迁移已完成或不需要迁移")
		return nil
	}

	// 2. 开始事务
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %v", err)
	}
	defer tx.Rollback()

	// 3. 创建新的clients表结构
	if err := db.createNewClientsTable(tx); err != nil {
		return fmt.Errorf("创建新表结构失败: %v", err)
	}

	// 4. 迁移数据
	if err := db.migrateClientData(tx); err != nil {
		return fmt.Errorf("迁移数据失败: %v", err)
	}

	// 5. 验证数据完整性
	if err := db.validateMigration(tx); err != nil {
		return fmt.Errorf("数据验证失败: %v", err)
	}

	// 6. 清理旧表
	if err := db.cleanupOldTables(tx); err != nil {
		return fmt.Errorf("清理旧表失败: %v", err)
	}

	// 7. 提交事务
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %v", err)
	}

	log.Println("客户端表合并迁移完成")
	return nil
}

// checkMigrationNeeded 检查是否需要执行迁移
func (db *DB) checkMigrationNeeded() (bool, error) {
	// 检查client_configs表是否存在
	var count int
	err := db.DB.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master 
		WHERE type='table' AND name='client_configs'
	`).Scan(&count)

	if err != nil {
		return false, err
	}

	// 如果client_configs表不存在，说明已经迁移过了
	return count > 0, nil
}

// createNewClientsTable 创建新的clients表结构
func (db *DB) createNewClientsTable(tx *sql.Tx) error {
	log.Println("创建新的clients表结构...")

	// 重命名现有的clients表
	_, err := tx.Exec("ALTER TABLE clients RENAME TO clients_old")
	if err != nil {
		return fmt.Errorf("重命名clients表失败: %v", err)
	}

	// 创建新的clients表
	createSQL := `
		CREATE TABLE IF NOT EXISTS clients (
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
		)
	`

	_, err = tx.Exec(createSQL)
	if err != nil {
		return fmt.Errorf("创建新clients表失败: %v", err)
	}

	return nil
}

// migrateClientData 迁移客户端数据
func (db *DB) migrateClientData(tx *sql.Tx) error {
	log.Println("开始迁移客户端数据...")

	// 合并数据的SQL语句
	migrateSQL := `
		INSERT INTO clients (
			client_id, name, description, auth_token, status, enabled,
			last_seen_ts, heartbeat_interval, heartbeat_timeout, 
			created_at, updated_at
		)
		SELECT 
			c.client_id,
			COALESCE(c.name, cc.client_name) as name,
			COALESCE(c.description, cc.description) as description,
			c.auth_token,
			c.status,
			COALESCE(cc.enabled, 1) as enabled,
			c.last_seen_ts,
			c.heartbeat_interval,
			c.heartbeat_timeout,
			c.created_at,
			COALESCE(cc.updated_at, c.created_at) as updated_at
		FROM clients_old c
		LEFT JOIN client_configs cc ON c.client_id = cc.client_id
	`

	result, err := tx.Exec(migrateSQL)
	if err != nil {
		return fmt.Errorf("合并clients数据失败: %v", err)
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("已迁移 %d 条clients记录", rowsAffected)

	// 处理只在client_configs中存在的记录
	orphanSQL := `
		INSERT INTO clients (
			client_id, name, description, enabled, created_at, updated_at
		)
		SELECT 
			cc.client_id,
			cc.client_name as name,
			cc.description,
			cc.enabled,
			cc.created_at,
			cc.updated_at
		FROM client_configs cc
		LEFT JOIN clients_old c ON cc.client_id = c.client_id
		WHERE c.client_id IS NULL
	`

	orphanResult, err := tx.Exec(orphanSQL)
	if err != nil {
		return fmt.Errorf("迁移孤立的client_configs记录失败: %v", err)
	}

	orphanRows, _ := orphanResult.RowsAffected()
	if orphanRows > 0 {
		log.Printf("已迁移 %d 条孤立的client_configs记录", orphanRows)
	}

	return nil
}

// validateMigration 验证迁移结果
func (db *DB) validateMigration(tx *sql.Tx) error {
	log.Println("验证迁移数据完整性...")

	// 检查记录数量
	var oldClientsCount, oldConfigsCount, newClientsCount int

	err := tx.QueryRow("SELECT COUNT(*) FROM clients_old").Scan(&oldClientsCount)
	if err != nil {
		return fmt.Errorf("查询旧clients表记录数失败: %v", err)
	}

	err = tx.QueryRow("SELECT COUNT(*) FROM client_configs").Scan(&oldConfigsCount)
	if err != nil {
		return fmt.Errorf("查询client_configs表记录数失败: %v", err)
	}

	err = tx.QueryRow("SELECT COUNT(*) FROM clients").Scan(&newClientsCount)
	if err != nil {
		return fmt.Errorf("查询新clients表记录数失败: %v", err)
	}

	log.Printf("迁移前: clients=%d, client_configs=%d", oldClientsCount, oldConfigsCount)
	log.Printf("迁移后: clients=%d", newClientsCount)

	// 验证没有数据丢失
	// 新表的记录数应该 >= 原clients表的记录数
	if newClientsCount < oldClientsCount {
		return fmt.Errorf("数据验证失败: 新表记录数(%d) < 原clients表记录数(%d)", newClientsCount, oldClientsCount)
	}

	// 验证关键字段不为空
	var nullCount int
	err = tx.QueryRow("SELECT COUNT(*) FROM clients WHERE client_id IS NULL OR client_id = ''").Scan(&nullCount)
	if err != nil {
		return fmt.Errorf("验证client_id字段失败: %v", err)
	}

	if nullCount > 0 {
		return fmt.Errorf("数据验证失败: 发现 %d 条记录的client_id为空", nullCount)
	}

	log.Println("数据完整性验证通过")
	return nil
}

// cleanupOldTables 清理旧表
func (db *DB) cleanupOldTables(tx *sql.Tx) error {
	log.Println("清理旧表...")

	// 删除旧的clients表
	_, err := tx.Exec("DROP TABLE clients_old")
	if err != nil {
		return fmt.Errorf("删除clients_old表失败: %v", err)
	}

	// 删除client_configs表
	_, err = tx.Exec("DROP TABLE client_configs")
	if err != nil {
		return fmt.Errorf("删除client_configs表失败: %v", err)
	}

	log.Println("旧表清理完成")
	return nil
}

// MigrateServerRoutes 为server_routes表添加route_mode字段
func (db *DB) MigrateServerRoutes() error {
	log.Println("开始执行server_routes表迁移...")

	// 检查route_mode字段是否已存在
	var columnExists int
	err := db.DB.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('server_routes') 
		WHERE name='route_mode'
	`).Scan(&columnExists)

	if err != nil {
		return fmt.Errorf("检查route_mode字段失败: %v", err)
	}

	if columnExists > 0 {
		log.Println("route_mode字段已存在，跳过迁移")
		return nil
	}

	// 添加route_mode字段
	_, err = db.DB.Exec(`
		ALTER TABLE server_routes 
		ADD COLUMN route_mode TEXT DEFAULT 'basic'
	`)

	if err != nil {
		return fmt.Errorf("添加route_mode字段失败: %v", err)
	}

	log.Println("server_routes表迁移完成")
	return nil
}

// MigrateServerRoutesV2 为server_routes表添加新的字段以支持增强的路由管理
func (db *DB) MigrateServerRoutesV2() error {
	log.Println("开始执行server_routes表V2迁移...")

	// 检查enabled字段是否已存在
	var enabledExists int
	err := db.DB.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('server_routes') 
		WHERE name='enabled'
	`).Scan(&enabledExists)

	if err != nil {
		return fmt.Errorf("检查enabled字段失败: %v", err)
	}

	if enabledExists == 0 {
		// 添加enabled字段，默认启用
		_, err = db.DB.Exec(`
			ALTER TABLE server_routes 
			ADD COLUMN enabled INTEGER DEFAULT 1
		`)
		if err != nil {
			return fmt.Errorf("添加enabled字段失败: %v", err)
		}
		log.Println("添加enabled字段成功")
	}

	// 检查description字段是否已存在
	var descExists int
	err = db.DB.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('server_routes') 
		WHERE name='description'
	`).Scan(&descExists)

	if err != nil {
		return fmt.Errorf("检查description字段失败: %v", err)
	}

	if descExists == 0 {
		// 添加description字段
		_, err = db.DB.Exec(`
			ALTER TABLE server_routes 
			ADD COLUMN description TEXT DEFAULT ''
		`)
		if err != nil {
			return fmt.Errorf("添加description字段失败: %v", err)
		}
		log.Println("添加description字段成功")
	}

	// 检查updated_at字段是否已存在
	var updatedAtExists int
	err = db.DB.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('server_routes') 
		WHERE name='updated_at'
	`).Scan(&updatedAtExists)

	if err != nil {
		return fmt.Errorf("检查updated_at字段失败: %v", err)
	}

	if updatedAtExists == 0 {
		// 添加updated_at字段
		_, err = db.DB.Exec(`
			ALTER TABLE server_routes 
			ADD COLUMN updated_at INTEGER DEFAULT 0
		`)
		if err != nil {
			return fmt.Errorf("添加updated_at字段失败: %v", err)
		}
		
		// 将现有记录的updated_at设置为created_at的值
		_, err = db.DB.Exec(`
			UPDATE server_routes 
			SET updated_at = created_at 
			WHERE updated_at = 0
		`)
		if err != nil {
			return fmt.Errorf("初始化updated_at字段失败: %v", err)
		}
		log.Println("添加updated_at字段成功")
	}

	// 更新路由模式值：将basic改为original_path，full改为path_transform
	_, err = db.DB.Exec(`
		UPDATE server_routes 
		SET route_mode = CASE 
			WHEN route_mode = 'basic' THEN 'original_path'
			WHEN route_mode = 'full' THEN 'path_transform'
			ELSE route_mode
		END
	`)
	if err != nil {
		return fmt.Errorf("更新路由模式值失败: %v", err)
	}

	log.Println("server_routes表V2迁移完成")
	return nil
}

// GetMigrationStatus 获取迁移状态
func (db *DB) GetMigrationStatus() (map[string]interface{}, error) {
	status := make(map[string]interface{})

	// 检查client_configs表是否存在
	var configTableExists int
	err := db.DB.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master 
		WHERE type='table' AND name='client_configs'
	`).Scan(&configTableExists)

	if err != nil {
		return nil, err
	}

	status["migration_completed"] = configTableExists == 0
	status["timestamp"] = time.Now().Unix()

	// 获取clients表记录数
	var clientsCount int
	err = db.DB.QueryRow("SELECT COUNT(*) FROM clients").Scan(&clientsCount)
	if err != nil {
		status["clients_count"] = 0
	} else {
		status["clients_count"] = clientsCount
	}

	// 如果迁移未完成，获取client_configs表记录数
	if configTableExists > 0 {
		var configsCount int
		err = db.DB.QueryRow("SELECT COUNT(*) FROM client_configs").Scan(&configsCount)
		if err != nil {
			status["client_configs_count"] = 0
		} else {
			status["client_configs_count"] = configsCount
		}
	}

	return status, nil
}