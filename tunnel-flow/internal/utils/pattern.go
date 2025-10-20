package utils

import (
	"strings"
)

// MatchPattern 检查URL路径是否匹配给定的模式
// 支持多种通配符匹配模式：
// * - 匹配单个路径段中的任意字符
// ** - 匹配任意数量的路径段
// 例如: 
//   /api/*/test 匹配 /api/user/test, /api/order/test
//   /api/**/test 匹配 /api/test, /api/v1/test, /api/v1/user/test
//   /api/user* 匹配 /api/user, /api/users, /api/user123
func MatchPattern(pattern, path string) bool {
	// 如果模式和路径完全相同，直接匹配
	if pattern == path {
		return true
	}
	
	// 如果模式不包含通配符，进行精确匹配
	if !strings.Contains(pattern, "*") {
		return pattern == path
	}
	
	// 处理 ** 通配符（匹配多个段）
	if strings.Contains(pattern, "**") {
		return matchWithDoubleWildcard(pattern, path)
	}
	
	// 分割模式和路径为段
	patternParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")
	
	// 如果段数不同，检查是否有前缀或后缀匹配
	if len(patternParts) != len(pathParts) {
		// 检查前缀匹配（模式以*结尾）
		if len(patternParts) > 0 && strings.HasSuffix(patternParts[len(patternParts)-1], "*") {
			return matchPrefix(pattern, path)
		}
		// 检查后缀匹配（模式以*开头）
		if len(patternParts) > 0 && strings.HasPrefix(patternParts[0], "*") {
			return matchSuffix(pattern, path)
		}
		return false
	}
	
	// 逐段比较
	for i, patternPart := range patternParts {
		pathPart := pathParts[i]
		
		// 如果是通配符，跳过这一段
		if patternPart == "*" {
			continue
		}
		
		// 如果段内包含通配符，进行模糊匹配
		if strings.Contains(patternPart, "*") {
			if !matchSegmentWithWildcard(patternPart, pathPart) {
				return false
			}
		} else {
			// 精确匹配
			if patternPart != pathPart {
				return false
			}
		}
	}
	
	return true
}

// matchSegmentWithWildcard 匹配包含通配符的段
func matchSegmentWithWildcard(pattern, segment string) bool {
	// 将模式按 * 分割
	parts := strings.Split(pattern, "*")
	
	// 如果只有一个部分且为空，说明整个段都是 *
	if len(parts) == 1 && parts[0] == "" {
		return true
	}
	
	currentPos := 0
	
	for i, part := range parts {
		if part == "" {
			continue
		}
		
		// 查找当前部分在段中的位置
		pos := strings.Index(segment[currentPos:], part)
		if pos == -1 {
			return false
		}
		
		// 如果是第一个部分，必须从开头匹配
		if i == 0 && pos != 0 {
			return false
		}
		
		// 如果是最后一个部分，必须匹配到结尾
		if i == len(parts)-1 {
			if currentPos+pos+len(part) != len(segment) {
				return false
			}
		}
		
		currentPos += pos + len(part)
	}
	
	return true
}

// matchWithDoubleWildcard 处理包含 ** 的模式匹配
func matchWithDoubleWildcard(pattern, path string) bool {
	// 将 ** 替换为特殊标记进行分割
	parts := strings.Split(pattern, "**")
	if len(parts) == 1 {
		return MatchPattern(pattern, path) // 没有 ** 通配符，使用普通匹配
	}
	
	// 检查前缀部分
	if parts[0] != "" {
		prefix := strings.TrimSuffix(parts[0], "/")
		if prefix != "" && !strings.HasPrefix(path, prefix) {
			return false
		}
		path = strings.TrimPrefix(path, prefix)
	}
	
	// 检查后缀部分
	if len(parts) > 1 && parts[len(parts)-1] != "" {
		suffix := strings.TrimPrefix(parts[len(parts)-1], "/")
		if suffix != "" && !strings.HasSuffix(path, suffix) {
			return false
		}
		path = strings.TrimSuffix(path, suffix)
	}
	
	// 如果有中间部分，需要递归检查
	if len(parts) > 2 {
		for i := 1; i < len(parts)-1; i++ {
			if parts[i] == "" {
				continue
			}
			middlePart := strings.Trim(parts[i], "/")
			if !strings.Contains(path, middlePart) {
				return false
			}
		}
	}
	
	return true
}

// matchPrefix 处理前缀匹配
func matchPrefix(pattern, path string) bool {
	// 移除模式末尾的通配符
	prefix := strings.TrimSuffix(pattern, "*")
	prefix = strings.TrimSuffix(prefix, "/")
	
	if prefix == "" {
		return true // 纯通配符匹配所有
	}
	
	return strings.HasPrefix(path, prefix)
}

// matchSuffix 处理后缀匹配
func matchSuffix(pattern, path string) bool {
	// 移除模式开头的通配符
	suffix := strings.TrimPrefix(pattern, "*")
	suffix = strings.TrimPrefix(suffix, "/")
	
	if suffix == "" {
		return true // 纯通配符匹配所有
	}
	
	return strings.HasSuffix(path, suffix)
}

// GetPatternPriority 获取模式的优先级
// 优先级规则：
// 1. 精确匹配优先级最高 (1000)
// 2. 单段通配符 (*) 优先级中等 (500-800)
// 3. 多段通配符 (**) 优先级较低 (100-400)
// 4. 前缀/后缀匹配优先级最低 (50-200)
// 5. 通配符越少优先级越高
// 6. 通配符位置越靠后优先级越高
func GetPatternPriority(pattern string) int {
	// 如果没有通配符，优先级最高
	if !strings.Contains(pattern, "*") {
		return 1000
	}
	
	// 检查是否包含双通配符
	if strings.Contains(pattern, "**") {
		doubleWildcardCount := strings.Count(pattern, "**")
		// 双通配符优先级较低：300 - 双通配符数量 * 50
		priority := 300 - doubleWildcardCount*50
		
		// 如果还有单通配符，进一步降低优先级
		singleWildcardCount := strings.Count(pattern, "*") - doubleWildcardCount*2
		priority -= singleWildcardCount * 20
		
		return priority
	}
	
	// 检查前缀匹配（以*结尾）
	if strings.HasSuffix(pattern, "*") {
		// 前缀匹配优先级较低
		prefixLen := len(strings.TrimSuffix(pattern, "*"))
		return 150 + prefixLen // 前缀越长优先级越高
	}
	
	// 检查后缀匹配（以*开头）
	if strings.HasPrefix(pattern, "*") {
		// 后缀匹配优先级较低
		suffixLen := len(strings.TrimPrefix(pattern, "*"))
		return 100 + suffixLen // 后缀越长优先级越高
	}
	
	// 计算单通配符数量
	wildcardCount := strings.Count(pattern, "*")
	
	// 基础优先级：700 - 通配符数量 * 100
	priority := 700 - wildcardCount*100
	
	// 根据通配符位置调整优先级
	parts := strings.Split(pattern, "/")
	for i, part := range parts {
		if part == "*" {
			// 越靠前的通配符，优先级越低
			priority -= (len(parts) - i) * 10
		} else if strings.Contains(part, "*") {
			// 段内通配符，优先级中等
			priority -= (len(parts) - i) * 5
		}
	}
	
	// 确保优先级不为负数
	if priority < 0 {
		priority = 1
	}
	
	return priority
}

// IsValidPattern 检查模式是否有效
func IsValidPattern(pattern string) bool {
	// 空模式无效
	if pattern == "" {
		return false
	}
	
	// 模式必须以 / 开头
	if !strings.HasPrefix(pattern, "/") {
		return false
	}
	
	// 检查是否有连续的斜杠
	if strings.Contains(pattern, "//") {
		return false
	}
	
	return true
}

// NormalizePattern 标准化模式
func NormalizePattern(pattern string) string {
	// 确保以 / 开头
	if !strings.HasPrefix(pattern, "/") {
		pattern = "/" + pattern
	}
	
	// 移除末尾的 /（除非是根路径）
	if len(pattern) > 1 && strings.HasSuffix(pattern, "/") {
		pattern = strings.TrimSuffix(pattern, "/")
	}
	
	return pattern
}