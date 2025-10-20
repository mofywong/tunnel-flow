package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"tunnel-flow/internal/config"
)

// User 用户结构
type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"` // 不在JSON中显示
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	LastLogin    time.Time `json:"last_login"`
}

// AuthHandler 认证处理器
type AuthHandler struct {
	auth  *AuthMiddleware
	users map[string]*User // 简单的内存存储，生产环境应使用数据库
	config *config.Config
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(cfg *config.Config) *AuthHandler {
	auth := NewAuthMiddleware(cfg)
	h := &AuthHandler{
		auth:   auth,
		users:  make(map[string]*User),
		config: cfg,
	}

	// 创建默认管理员用户
	h.createDefaultAdmin()
	return h
}

// createDefaultAdmin 创建默认管理员用户
func (h *AuthHandler) createDefaultAdmin() {
	adminID := "admin"
	adminPassword := "admin123" // 生产环境应该使用更安全的默认密码

	// 检查是否已存在
	if _, exists := h.users[adminID]; exists {
		return
	}

	passwordHash := h.hashPassword(adminPassword)
	h.users[adminID] = &User{
		ID:           adminID,
		Username:     "admin",
		PasswordHash: passwordHash,
		Role:         "admin",
		CreatedAt:    time.Now(),
	}
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token string `json:"token"`
	User  *User  `json:"user"`
}

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role,omitempty"`
}

// Login 用户登录
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 验证用户凭据
	user, err := h.validateCredentials(req.Username, req.Password)
	if err != nil {
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}

	// 生成token
	token, err := h.auth.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	// 更新最后登录时间
	user.LastLogin = time.Now()

	// 返回响应
	resp := LoginResponse{
		Token: token,
		User:  user,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Register 用户注册
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 验证输入
	if req.Username == "" || req.Password == "" {
		http.Error(w, "Username and password are required", http.StatusBadRequest)
		return
	}

	// 检查用户名是否已存在
	for _, user := range h.users {
		if user.Username == req.Username {
			http.Error(w, "Username already exists", http.StatusConflict)
			return
		}
	}

	// 设置默认角色
	if req.Role == "" {
		req.Role = "user"
	}

	// 只有管理员可以创建管理员用户
	if req.Role == "admin" {
		user, ok := GetUserFromContext(r.Context())
		if !ok || user.Role != "admin" {
			http.Error(w, "Only admins can create admin users", http.StatusForbidden)
			return
		}
	}

	// 创建用户
	userID := h.generateUserID()
	passwordHash := h.hashPassword(req.Password)

	newUser := &User{
		ID:           userID,
		Username:     req.Username,
		PasswordHash: passwordHash,
		Role:         req.Role,
		CreatedAt:    time.Now(),
	}

	h.users[userID] = newUser

	// 生成token
	token, err := h.auth.GenerateToken(newUser.ID, newUser.Username, newUser.Role)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	// 返回响应
	resp := LoginResponse{
		Token: token,
		User:  newUser,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// GetProfile 获取用户信息
func (h *AuthHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 获取完整用户信息
	fullUser, exists := h.users[user.UserID]
	if !exists {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fullUser)
}

// validateCredentials 验证用户凭据
func (h *AuthHandler) validateCredentials(username, password string) (*User, error) {
	for _, user := range h.users {
		if user.Username == username {
			if h.verifyPassword(password, user.PasswordHash) {
				return user, nil
			}
			break
		}
	}
	return nil, fmt.Errorf("invalid credentials")
}

// hashPassword 哈希密码
func (h *AuthHandler) hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password + h.config.AuthJWTSecret))
	return hex.EncodeToString(hash[:])
}

// verifyPassword 验证密码
func (h *AuthHandler) verifyPassword(password, hash string) bool {
	return h.hashPassword(password) == hash
}

// generateUserID 生成用户ID
func (h *AuthHandler) generateUserID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// GetAuthMiddleware 获取认证中间件
func (h *AuthHandler) GetAuthMiddleware() *AuthMiddleware {
	return h.auth
}

// ListUsers 列出所有用户（仅管理员）
func (h *AuthHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := GetUserFromContext(r.Context())
	if !ok || user.Role != "admin" {
		http.Error(w, "Forbidden: admin access required", http.StatusForbidden)
		return
	}

	users := make([]*User, 0, len(h.users))
	for _, u := range h.users {
		users = append(users, u)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}