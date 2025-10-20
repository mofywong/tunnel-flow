package utils

import (
	"testing"
)

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
		desc    string
	}{
		// 精确匹配
		{"/api/users", "/api/users", true, "精确匹配"},
		{"/api/users", "/api/user", false, "精确不匹配"},
		
		// 单段通配符
		{"/api/*/users", "/api/v1/users", true, "单段通配符匹配"},
		{"/api/*/users", "/api/v1/v2/users", false, "单段通配符段数不匹配"},
		
		// 段内通配符
		{"/api/user*", "/api/users", true, "段内通配符匹配"},
		{"/api/user*", "/api/user123", true, "段内通配符匹配数字"},
		{"/api/user*", "/api/admin", false, "段内通配符不匹配"},
		
		// 双通配符
		{"/api/**/users", "/api/users", true, "双通配符匹配零段"},
		{"/api/**/users", "/api/v1/users", true, "双通配符匹配一段"},
		{"/api/**/users", "/api/v1/v2/users", true, "双通配符匹配多段"},
		{"/api/**/users", "/api/v1/v2/v3/users", true, "双通配符匹配更多段"},
		{"/api/**/users", "/api/orders", false, "双通配符不匹配"},
		
		// 前缀匹配
		{"/api/*", "/api/users", true, "前缀匹配"},
		{"/api/*", "/api/users/123", true, "前缀匹配子路径"},
		{"/api/*", "/admin/users", false, "前缀不匹配"},
		
		// 后缀匹配
		{"*/users", "/api/users", true, "后缀匹配"},
		{"*/users", "/admin/api/users", true, "后缀匹配长路径"},
		{"*/users", "/api/orders", false, "后缀不匹配"},
		
		// 复杂模式
		{"/api/*/test/*", "/api/v1/test/123", true, "多个单段通配符"},
		{"/api/**/test/**", "/api/v1/v2/test/a/b", true, "多个双通配符"},
		{"/api/user*/test", "/api/users/test", true, "段内通配符组合"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := MatchPattern(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("MatchPattern(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

func TestGetPatternPriority(t *testing.T) {
	tests := []struct {
		pattern string
		desc    string
	}{
		{"/api/users", "精确匹配"},
		{"/api/*/users", "单段通配符"},
		{"/api/**/users", "双通配符"},
		{"/api/*", "前缀匹配"},
		{"*/users", "后缀匹配"},
		{"/api/user*", "段内通配符"},
	}

	priorities := make([]int, len(tests))
	for i, tt := range tests {
		priorities[i] = GetPatternPriority(tt.pattern)
		t.Logf("%s: %s = %d", tt.desc, tt.pattern, priorities[i])
	}

	// 验证优先级顺序
	if priorities[0] <= priorities[1] { // 精确匹配 > 单段通配符
		t.Errorf("精确匹配优先级应该高于单段通配符")
	}
	if priorities[1] <= priorities[2] { // 单段通配符 > 双通配符
		t.Errorf("单段通配符优先级应该高于双通配符")
	}
	if priorities[2] <= priorities[3] { // 双通配符 > 前缀匹配
		t.Errorf("双通配符优先级应该高于前缀匹配")
	}
}

func TestIsValidPattern(t *testing.T) {
	tests := []struct {
		pattern string
		want    bool
		desc    string
	}{
		{"/api/users", true, "有效的精确路径"},
		{"/api/*/users", true, "有效的通配符路径"},
		{"api/users", false, "缺少前导斜杠"},
		{"", false, "空路径"},
		{"/api//users", false, "连续斜杠"},
		{"/api/**/users", true, "有效的双通配符"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := IsValidPattern(tt.pattern)
			if got != tt.want {
				t.Errorf("IsValidPattern(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestNormalizePattern(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
		desc    string
	}{
		{"api/users", "/api/users", "添加前导斜杠"},
		{"/api/users/", "/api/users", "移除末尾斜杠"},
		{"/", "/", "根路径保持不变"},
		{"api/users/", "/api/users", "添加前导斜杠并移除末尾斜杠"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := NormalizePattern(tt.pattern)
			if got != tt.want {
				t.Errorf("NormalizePattern(%q) = %q, want %q", tt.pattern, got, tt.want)
			}
		})
	}
}