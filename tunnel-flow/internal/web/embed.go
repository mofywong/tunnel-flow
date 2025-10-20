package web

import (
	"embed"
	"io/fs"
	"net/http"
)

// 嵌入前端构建的静态文件
//go:embed all:dist
var distFS embed.FS

// GetDistFS 获取前端静态文件系统
func GetDistFS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// GetDistHandler 获取前端静态文件处理器
func GetDistHandler() (http.Handler, error) {
	distSubFS, err := GetDistFS()
	if err != nil {
		return nil, err
	}
	return http.FileServer(http.FS(distSubFS)), nil
}