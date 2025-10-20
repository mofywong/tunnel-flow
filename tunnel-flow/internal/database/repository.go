package database

import (
	"database/sql"
	"time"
)

// Repository 数据库操作接口
type Repository struct {
	db *DB
}

// NewRepository 创建新的Repository
func NewRepository(db *DB) *Repository {
	return &Repository{db: db}
}

// Ping 检查数据库连接状态
func (r *Repository) Ping() error {
	return r.db.Ping()
}

// Client operations

// CreateClient 创建客户端
func (r *Repository) CreateClient(client *Client) error {
	query := `INSERT INTO clients (client_id, name, description, auth_token, status, enabled, last_seen_ts, heartbeat_interval, heartbeat_timeout, created_at, updated_at) 
			   VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	
	now := time.Now().Unix()
	client.CreatedAt = now
	client.UpdatedAt = now
	// 不手动设置LastSeenTS，让心跳机制自动更新，设置为null
	client.LastSeenTS = sql.NullInt64{}
	
	// 设置默认值
	if client.HeartbeatInterval == 0 {
		client.HeartbeatInterval = 30
	}
	if client.HeartbeatTimeout == 0 {
		client.HeartbeatTimeout = 90
	}
	if client.Enabled == 0 {
		client.Enabled = 1 // 默认启用
	}
	
	_, err := r.db.Exec(query, client.ClientID, client.Name, client.Description, client.AuthToken, 
		client.Status, client.Enabled, client.LastSeenTS, client.HeartbeatInterval, client.HeartbeatTimeout, 
		client.CreatedAt, client.UpdatedAt)
	return err
}

// GetClient 获取客户端
func (r *Repository) GetClient(clientID string) (*Client, error) {
	query := `SELECT client_id, name, description, auth_token, status, enabled, last_seen_ts, heartbeat_interval, heartbeat_timeout, created_at, updated_at, local_ips 
			   FROM clients WHERE client_id = ?`
	
	client := &Client{}
	var description sql.NullString
	var localIPs sql.NullString
	err := r.db.QueryRow(query, clientID).Scan(
		&client.ClientID, &client.Name, &description, &client.AuthToken,
		&client.Status, &client.Enabled, &client.LastSeenTS, &client.HeartbeatInterval, &client.HeartbeatTimeout, 
		&client.CreatedAt, &client.UpdatedAt, &localIPs)
	
	if err != nil {
		return nil, err
	}
	
	if description.Valid {
		client.Description = description.String
	}
	if localIPs.Valid {
		client.LocalIPs = localIPs.String
	}
	// 处理LastSeenTS的null值
	if client.LastSeenTS.Valid {
		client.LastSeen = time.UnixMilli(client.LastSeenTS.Int64)
	}
	// 设置是否有认证令牌
	client.HasAuthToken = client.AuthToken != ""
	return client, nil
}

// UpdateClientStatus 更新客户端状态
func (r *Repository) UpdateClientStatus(clientID, status string) error {
	query := `UPDATE clients SET status = ?, last_seen_ts = ? WHERE client_id = ?`
	_, err := r.db.Exec(query, status, time.Now().UnixMilli(), clientID)
	return err
}

// UpdateClientLastSeen 更新客户端最后心跳时间
func (r *Repository) UpdateClientLastSeen(clientID string, lastSeenTS int64) error {
	query := `UPDATE clients SET last_seen_ts = ? WHERE client_id = ?`
	_, err := r.db.Exec(query, lastSeenTS, clientID)
	return err
}

// UpdateClientLocalIPs 更新客户端本地IP地址列表
func (r *Repository) UpdateClientLocalIPs(clientID string, localIPs string) error {
	query := `UPDATE clients SET local_ips = ? WHERE client_id = ?`
	_, err := r.db.Exec(query, localIPs, clientID)
	return err
}

// UpdateClientLastActiveTime 更新客户端最后活跃时间
func (r *Repository) UpdateClientLastActiveTime(clientID string, lastActiveTime time.Time) error {
	query := `UPDATE clients SET last_seen_ts = ? WHERE client_id = ?`
	_, err := r.db.Exec(query, lastActiveTime.UnixMilli(), clientID)
	return err
}

// CleanupStaleClients 清理过期的客户端状态
func (r *Repository) CleanupStaleClients() error {
	// 将所有在线状态的客户端设置为离线，因为服务器重启后所有连接都已断开
	query := `UPDATE clients SET status = 'offline', last_seen_ts = ? WHERE status = 'online'`
	_, err := r.db.Exec(query, time.Now().UnixMilli())
	return err
}

// GetStaleClients 获取超时的客户端
func (r *Repository) GetStaleClients(timeoutSeconds int) ([]*Client, error) {
	cutoffTime := time.Now().Add(-time.Duration(timeoutSeconds) * time.Second).UnixMilli()
	query := `SELECT client_id, name, description, auth_token, status, last_seen_ts, heartbeat_interval, heartbeat_timeout, created_at 
			   FROM clients WHERE status = 'online' AND last_seen_ts < ?`
	
	rows, err := r.db.Query(query, cutoffTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var clients []*Client
	for rows.Next() {
		client := &Client{}
		var description sql.NullString
		err := rows.Scan(&client.ClientID, &client.Name, &description, &client.AuthToken,
			&client.Status, &client.LastSeenTS, &client.HeartbeatInterval, &client.HeartbeatTimeout, &client.CreatedAt)
		if err != nil {
			return nil, err
		}
		if description.Valid {
			client.Description = description.String
		}
		// 处理LastSeenTS的null值
		if client.LastSeenTS.Valid {
			client.LastSeen = time.UnixMilli(client.LastSeenTS.Int64)
		}
		// 设置是否有认证令牌
		client.HasAuthToken = client.AuthToken != ""
		clients = append(clients, client)
	}
	
	return clients, rows.Err()
}

// ServerRoute additional operations

// UpdateServerRouteEnabled 更新路由启用状态
func (r *Repository) UpdateServerRouteEnabled(id int, enabled bool) error {
	enabledValue := 0
	if enabled {
		enabledValue = 1
	}
	
	query := `UPDATE server_routes SET enabled = ?, updated_at = ? WHERE id = ?`
	_, err := r.db.Exec(query, enabledValue, time.Now().UnixMilli(), id)
	return err
}

// GetEnabledServerRoutes 获取所有启用的路由
func (r *Repository) GetEnabledServerRoutes() ([]*ServerRoute, error) {
	query := `SELECT id, url_suffix, client_id, targets_json, delivery_policy, route_mode, enabled, description, created_at, updated_at 
			   FROM server_routes WHERE enabled = 1 ORDER BY created_at DESC`
	
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var routes []*ServerRoute
	for rows.Next() {
		route := &ServerRoute{}
		var description sql.NullString
		var updatedAt sql.NullInt64
		
		err := rows.Scan(&route.ID, &route.URLSuffix, &route.ClientID, &route.TargetsJSON,
			&route.DeliveryPolicy, &route.RouteMode, &route.Enabled, &description, &route.CreatedAt, &updatedAt)
		if err != nil {
			return nil, err
		}
		
		// 处理可空字段
		if description.Valid {
			route.Description = description.String
		}
		if updatedAt.Valid {
			route.UpdatedAt = updatedAt.Int64
		} else {
			route.UpdatedAt = route.CreatedAt // 兼容旧数据
		}
		
		routes = append(routes, route)
	}
	
	return routes, rows.Err()
}

// GetServerRoutesByPattern 根据URL模式匹配获取路由（支持通配符）
func (r *Repository) GetServerRoutesByPattern(urlPath string) ([]*ServerRoute, error) {
	// 获取所有启用的路由，然后在应用层进行模式匹配
	query := `SELECT id, url_suffix, client_id, targets_json, delivery_policy, route_mode, enabled, description, created_at, updated_at 
			   FROM server_routes WHERE enabled = 1 ORDER BY created_at DESC`
	
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var routes []*ServerRoute
	for rows.Next() {
		route := &ServerRoute{}
		var description sql.NullString
		var updatedAt sql.NullInt64
		
		err := rows.Scan(&route.ID, &route.URLSuffix, &route.ClientID, &route.TargetsJSON,
			&route.DeliveryPolicy, &route.RouteMode, &route.Enabled, &description, &route.CreatedAt, &updatedAt)
		if err != nil {
			return nil, err
		}
		
		// 处理可空字段
		if description.Valid {
			route.Description = description.String
		}
		if updatedAt.Valid {
			route.UpdatedAt = updatedAt.Int64
		} else {
			route.UpdatedAt = route.CreatedAt // 兼容旧数据
		}
		
		routes = append(routes, route)
	}
	
	return routes, rows.Err()
}

// BatchUpdateServerRoutesEnabled 批量更新路由启用状态
func (r *Repository) BatchUpdateServerRoutesEnabled(ids []int, enabled bool) error {
	if len(ids) == 0 {
		return nil
	}
	
	enabledValue := 0
	if enabled {
		enabledValue = 1
	}
	
	// 构建批量更新SQL
	query := `UPDATE server_routes SET enabled = ?, updated_at = ? WHERE id IN (`
	for i := range ids {
		if i > 0 {
			query += ","
		}
		query += "?"
	}
	query += ")"
	
	// 准备参数
	args := []interface{}{enabledValue, time.Now().UnixMilli()}
	for _, id := range ids {
		args = append(args, id)
	}
	
	_, err := r.db.Exec(query, args...)
	return err
}

// GetServerRouteStats 获取路由统计信息
func (r *Repository) GetServerRouteStats(clientID string) (map[string]int, error) {
	stats := make(map[string]int)
	
	var query string
	var args []interface{}
	
	if clientID != "" {
		query = `SELECT 
			COUNT(*) as total,
			SUM(CASE WHEN enabled = 1 AND active = 1 THEN 1 ELSE 0 END) as enabled,
			SUM(CASE WHEN enabled = 0 OR active = 0 THEN 1 ELSE 0 END) as disabled
			FROM server_routes WHERE client_id = ?`
		args = []interface{}{clientID}
	} else {
		query = `SELECT 
			COUNT(*) as total,
			SUM(CASE WHEN enabled = 1 AND active = 1 THEN 1 ELSE 0 END) as enabled,
			SUM(CASE WHEN enabled = 0 OR active = 0 THEN 1 ELSE 0 END) as disabled
			FROM server_routes`
	}
	
	var total, enabled, disabled int
	err := r.db.QueryRow(query, args...).Scan(&total, &enabled, &disabled)
	if err != nil {
		return nil, err
	}
	
	stats["total"] = total
	stats["enabled"] = enabled
	stats["disabled"] = disabled
	
	return stats, nil
}

// UpdateClient 更新客户端信息
func (r *Repository) UpdateClient(client *Client) error {
	query := `UPDATE clients SET name = ?, description = ?, auth_token = ?, enabled = ?, heartbeat_interval = ?, heartbeat_timeout = ?, updated_at = ? WHERE client_id = ?`
	client.UpdatedAt = time.Now().Unix()
	_, err := r.db.Exec(query, client.Name, client.Description, client.AuthToken, client.Enabled, client.HeartbeatInterval, client.HeartbeatTimeout, client.UpdatedAt, client.ClientID)
	return err
}

// DeleteClient 删除客户端
func (r *Repository) DeleteClient(clientID string) error {
	query := `DELETE FROM clients WHERE client_id = ?`
	_, err := r.db.Exec(query, clientID)
	return err
}

// ListClients 列出所有客户端
func (r *Repository) ListClients() ([]*Client, error) {
	query := `SELECT client_id, name, description, auth_token, status, enabled, last_seen_ts, heartbeat_interval, heartbeat_timeout, created_at, updated_at, local_ips 
			   FROM clients ORDER BY created_at DESC`
	
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var clients []*Client
	for rows.Next() {
		client := &Client{}
		var description sql.NullString
		var localIPs sql.NullString
		err := rows.Scan(&client.ClientID, &client.Name, &description, &client.AuthToken,
			&client.Status, &client.Enabled, &client.LastSeenTS, &client.HeartbeatInterval, &client.HeartbeatTimeout, 
			&client.CreatedAt, &client.UpdatedAt, &localIPs)
		if err != nil {
			return nil, err
		}
		if description.Valid {
			client.Description = description.String
		}
		if localIPs.Valid {
			client.LocalIPs = localIPs.String
		}
		// 处理LastSeenTS的null值
		if client.LastSeenTS.Valid {
			client.LastSeen = time.UnixMilli(client.LastSeenTS.Int64)
		}
		// 设置是否有认证令牌
		client.HasAuthToken = client.AuthToken != ""
		clients = append(clients, client)
	}
	
	return clients, rows.Err()
}

// ServerRoute operations

// CreateServerRoute 创建服务端路由
func (r *Repository) CreateServerRoute(route *ServerRoute) error {
	query := `INSERT INTO server_routes (url_suffix, client_id, targets_json, delivery_policy, route_mode, enabled, description, created_at, updated_at) 
			   VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	
	now := time.Now().UnixMilli()
	route.CreatedAt = now
	route.UpdatedAt = now
	
	// 如果RouteMode为空，设置默认值
	if route.RouteMode == "" {
		route.RouteMode = RouteModeOriginalPath
	}
	
	// 如果Enabled未设置，默认禁用（按需求新增后默认禁用）
	if route.Enabled == 0 {
		route.Enabled = 0 // 默认禁用
	}
	
	result, err := r.db.Exec(query, route.URLSuffix, route.ClientID, route.TargetsJSON,
		route.DeliveryPolicy, route.RouteMode, route.Enabled, route.Description, route.CreatedAt, route.UpdatedAt)
	if err != nil {
		return err
	}
	
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	route.ID = int(id)
	
	return nil
}

// GetServerRoute 获取服务端路由
func (r *Repository) GetServerRoute(id int) (*ServerRoute, error) {
	query := `SELECT id, url_suffix, client_id, targets_json, delivery_policy, route_mode, enabled, description, created_at, updated_at
			   FROM server_routes WHERE id = ?`
	
	route := &ServerRoute{}
	var description sql.NullString
	var updatedAt sql.NullInt64
	
	err := r.db.QueryRow(query, id).Scan(&route.ID, &route.URLSuffix, &route.ClientID, &route.TargetsJSON,
		&route.DeliveryPolicy, &route.RouteMode, &route.Enabled, &description, &route.CreatedAt, &updatedAt)
	
	if err != nil {
		return nil, err
	}
	
	// 处理可空字段
	if description.Valid {
		route.Description = description.String
	}
	if updatedAt.Valid {
		route.UpdatedAt = updatedAt.Int64
	} else {
		route.UpdatedAt = route.CreatedAt // 兼容旧数据
	}
	
	return route, nil
}

// GetServerRoutesByURLSuffix 根据URL后缀获取路由
func (r *Repository) GetServerRoutesByURLSuffix(urlSuffix string) ([]*ServerRoute, error) {
	query := `SELECT id, url_suffix, client_id, targets_json, delivery_policy, route_mode, enabled, description, created_at, updated_at 
			   FROM server_routes WHERE url_suffix = ? AND enabled = 1`
	
	rows, err := r.db.Query(query, urlSuffix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var routes []*ServerRoute
	for rows.Next() {
		route := &ServerRoute{}
		var description sql.NullString
		var updatedAt sql.NullInt64
		
		err := rows.Scan(&route.ID, &route.URLSuffix, &route.ClientID, &route.TargetsJSON,
			&route.DeliveryPolicy, &route.RouteMode, &route.Enabled, &description, &route.CreatedAt, &updatedAt)
		if err != nil {
			return nil, err
		}
		
		// 处理可空字段
		if description.Valid {
			route.Description = description.String
		}
		if updatedAt.Valid {
			route.UpdatedAt = updatedAt.Int64
		} else {
			route.UpdatedAt = route.CreatedAt // 兼容旧数据
		}
		
		routes = append(routes, route)
	}
	
	return routes, rows.Err()
}

// ListServerRoutes 列出所有服务端路由
func (r *Repository) ListServerRoutes() ([]*ServerRoute, error) {
	query := `SELECT id, url_suffix, client_id, targets_json, delivery_policy, route_mode, enabled, description, created_at, updated_at 
			   FROM server_routes ORDER BY created_at DESC`
	
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var routes []*ServerRoute
	for rows.Next() {
		route := &ServerRoute{}
		var description sql.NullString
		var updatedAt sql.NullInt64
		
		err := rows.Scan(&route.ID, &route.URLSuffix, &route.ClientID, &route.TargetsJSON,
			&route.DeliveryPolicy, &route.RouteMode, &route.Enabled, &description, &route.CreatedAt, &updatedAt)
		if err != nil {
			return nil, err
		}
		
		// 处理可空字段
		if description.Valid {
			route.Description = description.String
		}
		if updatedAt.Valid {
			route.UpdatedAt = updatedAt.Int64
		} else {
			route.UpdatedAt = route.CreatedAt // 兼容旧数据
		}
		
		routes = append(routes, route)
	}
	
	return routes, rows.Err()
}

// UpdateServerRoute 更新服务端路由
func (r *Repository) UpdateServerRoute(route *ServerRoute) error {
	// 设置更新时间
	route.UpdatedAt = time.Now().UnixMilli()
	
	query := `UPDATE server_routes SET url_suffix = ?, client_id = ?, targets_json = ?, 
			   delivery_policy = ?, route_mode = ?, enabled = ?, description = ?, updated_at = ? 
			   WHERE id = ?`
	
	_, err := r.db.Exec(query, route.URLSuffix, route.ClientID, route.TargetsJSON,
		route.DeliveryPolicy, route.RouteMode, route.Enabled, route.Description, route.UpdatedAt, route.ID)
	return err
}

// DeleteServerRoute 删除服务端路由
func (r *Repository) DeleteServerRoute(id int) error {
	query := `DELETE FROM server_routes WHERE id = ?`
	_, err := r.db.Exec(query, id)
	return err
}

// GetServerRoutesByClientID 根据客户端ID获取路由列表
func (r *Repository) GetServerRoutesByClientID(clientID string) ([]*ServerRoute, error) {
	query := `SELECT id, url_suffix, client_id, targets_json, delivery_policy, route_mode, enabled, description, created_at, updated_at 
			   FROM server_routes WHERE client_id = ? ORDER BY created_at DESC`
	
	rows, err := r.db.Query(query, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var routes []*ServerRoute
	for rows.Next() {
		route := &ServerRoute{}
		var description sql.NullString
		var updatedAt sql.NullInt64
		
		err := rows.Scan(&route.ID, &route.URLSuffix, &route.ClientID, &route.TargetsJSON,
			&route.DeliveryPolicy, &route.RouteMode, &route.Enabled, &description, &route.CreatedAt, &updatedAt)
		if err != nil {
			return nil, err
		}
		
		// 处理可空字段
		if description.Valid {
			route.Description = description.String
		}
		if updatedAt.Valid {
			route.UpdatedAt = updatedAt.Int64
		} else {
			route.UpdatedAt = route.CreatedAt // 兼容旧数据
		}
		
		routes = append(routes, route)
	}
	
	return routes, rows.Err()
}

// PendingMessage operations

// CreatePendingMessage 创建待处理消息
func (r *Repository) CreatePendingMessage(msg *PendingMessage) error {
	query := `INSERT INTO pending_messages (msg_id, client_id, url_suffix, request_meta_json, 
			   state, retry_count, next_try_ts, created_at, last_update) 
			   VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	
	now := time.Now().UnixMilli()
	msg.CreatedAt = now
	msg.LastUpdate = now
	
	_, err := r.db.Exec(query, msg.MsgID, msg.ClientID, msg.URLSuffix, msg.RequestMetaJSON,
		msg.State, msg.RetryCount, msg.NextTryTS, msg.CreatedAt, msg.LastUpdate)
	return err
}

// GetPendingMessage 获取待处理消息
func (r *Repository) GetPendingMessage(msgID string) (*PendingMessage, error) {
	query := `SELECT msg_id, client_id, url_suffix, request_meta_json, state, retry_count, 
			   next_try_ts, created_at, last_update, response_meta_json 
			   FROM pending_messages WHERE msg_id = ?`
	
	msg := &PendingMessage{}
	err := r.db.QueryRow(query, msgID).Scan(
		&msg.MsgID, &msg.ClientID, &msg.URLSuffix, &msg.RequestMetaJSON,
		&msg.State, &msg.RetryCount, &msg.NextTryTS, &msg.CreatedAt,
		&msg.LastUpdate, &msg.ResponseMetaJSON)
	
	return msg, err
}

// UpdatePendingMessageState 更新待处理消息状态
func (r *Repository) UpdatePendingMessageState(msgID, state string) error {
	query := `UPDATE pending_messages SET state = ?, last_update = ? WHERE msg_id = ?`
	_, err := r.db.Exec(query, state, time.Now().UnixMilli(), msgID)
	return err
}

// UpdatePendingMessageResponse 更新待处理消息响应
func (r *Repository) UpdatePendingMessageResponse(msgID, state, responseMetaJSON string) error {
	query := `UPDATE pending_messages SET state = ?, response_meta_json = ?, last_update = ? WHERE msg_id = ?`
	_, err := r.db.Exec(query, state, responseMetaJSON, time.Now().UnixMilli(), msgID)
	return err
}

// ListPendingMessages 列出待处理消息
func (r *Repository) ListPendingMessages(limit int) ([]*PendingMessage, error) {
	query := `SELECT msg_id, client_id, url_suffix, request_meta_json, state, retry_count, 
			   next_try_ts, created_at, last_update, response_meta_json 
			   FROM pending_messages ORDER BY created_at DESC LIMIT ?`
	
	rows, err := r.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var messages []*PendingMessage
	for rows.Next() {
		msg := &PendingMessage{}
		err := rows.Scan(&msg.MsgID, &msg.ClientID, &msg.URLSuffix, &msg.RequestMetaJSON,
			&msg.State, &msg.RetryCount, &msg.NextTryTS, &msg.CreatedAt,
			&msg.LastUpdate, &msg.ResponseMetaJSON)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	
	return messages, rows.Err()
}

// AuditLog operations

// CreateAuditLog 创建审计日志
func (r *Repository) CreateAuditLog(log *AuditLog) error {
	query := `INSERT INTO audit_logs (msg_id, client_id, direction, payload_summary, ts) 
			   VALUES (?, ?, ?, ?, ?)`
	
	log.TS = time.Now().UnixMilli()
	
	result, err := r.db.Exec(query, log.MsgID, log.ClientID, log.Direction, log.PayloadSummary, log.TS)
	if err != nil {
		return err
	}
	
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	log.ID = int(id)
	
	return nil
}

// ListAuditLogs 列出审计日志
func (r *Repository) ListAuditLogs(limit int) ([]*AuditLog, error) {
	query := `SELECT id, msg_id, client_id, direction, payload_summary, ts 
			   FROM audit_logs ORDER BY ts DESC LIMIT ?`
	
	rows, err := r.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var logs []*AuditLog
	for rows.Next() {
		log := &AuditLog{}
		err := rows.Scan(&log.ID, &log.MsgID, &log.ClientID, &log.Direction, &log.PayloadSummary, &log.TS)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	
	return logs, rows.Err()
}

// Client enabled status operations

// UpdateClientEnabled 更新客户端启用状态
func (r *Repository) UpdateClientEnabled(clientID string, enabled bool) error {
	query := `UPDATE clients SET enabled = ?, updated_at = ? WHERE client_id = ?`
	updatedAt := time.Now().Unix()
	_, err := r.db.Exec(query, enabled, updatedAt, clientID)
	return err
}

// ListEnabledClients 列出所有启用的客户端
func (r *Repository) ListEnabledClients() ([]*Client, error) {
	query := `SELECT client_id, name, description, auth_token, status, enabled, last_seen_ts, heartbeat_interval, heartbeat_timeout, created_at, updated_at 
			   FROM clients WHERE enabled = 1 ORDER BY created_at DESC`
	
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var clients []*Client
	for rows.Next() {
		client := &Client{}
		var description sql.NullString
		err := rows.Scan(&client.ClientID, &client.Name, &description, &client.AuthToken,
			&client.Status, &client.Enabled, &client.LastSeenTS, &client.HeartbeatInterval, &client.HeartbeatTimeout, 
			&client.CreatedAt, &client.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if description.Valid {
			client.Description = description.String
		}
		// 处理LastSeenTS的null值
		if client.LastSeenTS.Valid {
			client.LastSeen = time.UnixMilli(client.LastSeenTS.Int64)
		}
		clients = append(clients, client)
	}
	
	return clients, rows.Err()
}