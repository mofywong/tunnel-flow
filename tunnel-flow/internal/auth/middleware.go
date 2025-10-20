package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"tunnel-flow/internal/config"
)

// Claims JWT声明
type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// AuthMiddleware 认证中间件
type AuthMiddleware struct {
	jwtSecret []byte
}

// NewAuthMiddleware 创建认证中间件
func NewAuthMiddleware(cfg *config.Config) *AuthMiddleware {
	return &AuthMiddleware{
		jwtSecret: []byte(cfg.AuthJWTSecret),
	}
}

// Middleware HTTP认证中间件
func (a *AuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 跳过某些不需要认证的路径
		if a.shouldSkipAuth(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// 提取token
		token := a.extractToken(r)
		if token == "" {
			http.Error(w, "Missing or invalid authorization header", http.StatusUnauthorized)
			return
		}

		// 验证token
		claims, err := a.ValidateToken(token)
		if err != nil {
			http.Error(w, "Invalid token: "+err.Error(), http.StatusUnauthorized)
			return
		}

		// 将用户信息添加到上下文
		ctx := context.WithValue(r.Context(), "user", claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// shouldSkipAuth 检查是否应该跳过认证
func (a *AuthMiddleware) shouldSkipAuth(path string) bool {
	// 静态文件和前端路由
	if strings.HasPrefix(path, "/assets/") ||
		strings.HasPrefix(path, "/favicon.ico") ||
		path == "/" ||
		path == "/login" {
		return true
	}

	// WebSocket连接（使用查询参数认证）
	if path == "/ws" {
		return true
	}

	// 公开API端点
	publicPaths := []string{
		"/api/v1/auth/login",
		"/api/v1/auth/register",
		"/api/v1/status", // 系统状态可以公开访问
	}

	for _, publicPath := range publicPaths {
		if path == publicPath {
			return true
		}
	}

	return false
}

// extractToken 从请求中提取token
func (a *AuthMiddleware) extractToken(r *http.Request) string {
	// 从Authorization头提取
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			return parts[1]
		}
	}

	// 从查询参数提取（用于WebSocket）
	return r.URL.Query().Get("token")
}

// ValidateToken 验证JWT token
func (a *AuthMiddleware) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return a.jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// GenerateToken 生成JWT token
func (a *AuthMiddleware) GenerateToken(userID, username, role string) (string, error) {
	claims := &Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)), // 24小时过期
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "tunnel-flow",
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(a.jwtSecret)
}

// GetUserFromContext 从上下文获取用户信息
func GetUserFromContext(ctx context.Context) (*Claims, bool) {
	user, ok := ctx.Value("user").(*Claims)
	return user, ok
}

// RequireRole 要求特定角色的中间件
func (a *AuthMiddleware) RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := GetUserFromContext(r.Context())
			if !ok {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if user.Role != role && user.Role != "admin" {
				http.Error(w, "Forbidden: insufficient permissions", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}