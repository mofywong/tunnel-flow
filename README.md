# Tunnel Flow

[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org)
[![Vue Version](https://img.shields.io/badge/Vue-3.4+-green.svg)](https://vuejs.org)

一个基于WebSocket的安全隧道流量管理系统，提供客户端连接管理、实时监控和现代化Web管理界面。

## ✨ 功能特性

- 🔐 **安全通信**: 强制使用WSS(WebSocket Secure)加密通信
- 🌐 **双向代理**: 支持公网到内网的安全隧道连接
- 📊 **实时监控**: 实时连接状态监控和性能指标
- 🎛️ **Web管理**: 现代化Vue3管理界面，支持路由配置和客户端管理
- 🔄 **自动重连**: 智能重连机制，确保连接稳定性
- 📈 **负载均衡**: 支持多种负载均衡策略
- 🗄️ **数据持久化**: SQLite数据库存储配置和连接信息
- 📱 **响应式设计**: 支持桌面和移动设备访问
- 📦 **单体构建**: 前端资源完全嵌入后端二进制，零依赖部署
- 🚀 **一键构建**: 自动化构建脚本，支持Windows/Linux/Mac跨平台

## 🏗️ 系统架构

```
[第三方系统] --HTTPS--> [tunnel-flow (公网服务器)]
                              |
                        Web管理界面 (Vue3)
                              |
                    WebSocket (wss://host:8081/ws)
                              |
                    <--- 加密隧道连接 --->
                              |
                    [tunnel-flow-agent (内网代理)]
                              |
                        内网HTTP服务
```

## 📁 项目结构

```
tunnel-flow/
├── tunnel-flow/                 # 服务端代码
│   ├── main.go                 # 服务端入口
│   ├── embed.go                # 前端资源嵌入
│   ├── internal/               # 内部包
│   │   ├── auth/              # 认证模块
│   │   ├── config/            # 配置管理
│   │   ├── database/          # 数据库操作
│   │   ├── websocket/         # WebSocket处理
│   │   ├── server/            # HTTP服务器
│   │   └── proxy/             # 代理模块
│   ├── web/                   # 前端代码
│   │   ├── src/               # Vue3源码
│   │   ├── public/            # 静态资源
│   │   ├── dist/              # 构建输出
│   │   └── vite.config.js     # Vite配置(支持单体构建)
│   ├── ssl/                   # SSL证书
│   └── config.yaml            # 服务端配置
├── tunnel-flow-agent/          # 客户端代码
│   ├── main.go                # 客户端入口
│   ├── internal/              # 内部包
│   └── config.yaml            # 客户端配置
├── build-win.bat              # Windows构建脚本
├── build-linux.sh             # Linux/Mac构建脚本
└── README.md                  # 项目说明
```

## 📦 单体构建特性

Tunnel Flow 支持将前端资源完全嵌入到后端二进制文件中，实现真正的单体应用部署。这种方式具有以下优势：

### 🎯 核心优势

- **零依赖部署**: 无需单独部署前端文件，一个可执行文件包含所有功能
- **简化运维**: 减少部署复杂度，避免前后端版本不匹配问题
- **跨平台支持**: 支持Windows、Linux、macOS等多平台构建
- **完整功能**: 保持所有原有功能，包括Web管理界面、API接口等
- **高性能**: 静态资源直接从内存提供，响应速度更快

### 🔧 技术实现

- **Go embed**: 使用Go 1.16+的embed指令将前端dist目录嵌入二进制
- **Vite构建**: 优化的Vite配置，确保资源路径正确
- **自动化脚本**: 提供Windows和Linux/Mac的一键构建脚本
- **智能路由**: 自动处理静态资源和API路由的分发

### 📋 构建产物

构建完成后，`build`目录包含：
```
build/
├── tunnel-flow.exe          # 服务端可执行文件(内嵌前端)
├── tunnel-flow-agent.exe    # 客户端可执行文件
├── config.yaml              # 服务端配置文件
├── ssl/                     # SSL证书目录
│   ├── server.crt
│   └── server.key
├── data/                    # 数据库目录(自动创建)
├── start.bat               # 启动脚本
└── README.txt              # 使用说明
```

## 🚀 快速开始

### 🎯 方式一：一键构建（推荐）

使用自动化构建脚本，一键生成单体应用：

**Windows:**
```bash
# 在项目根目录执行
.\build-win.bat
```

**Linux/Mac:**
```bash
# 在项目根目录执行
chmod +x build-linux.sh
./build-linux.sh
```

构建完成后，进入`build`目录：
```bash
cd build
# Windows
.\start.bat
# Linux/Mac
./start.sh
```

访问 `https://localhost:8080` 即可使用Web管理界面。

### 🔧 方式二：手动构建

### 环境要求

- **Go**: 1.21 或更高版本
- **Node.js**: 16.0 或更高版本
- **npm**: 8.0 或更高版本

### 安装步骤

#### 1. 克隆项目

```bash
git clone https://github.com/mofywong/tunnel-flow.git
cd tunnel-flow
```

### 2. 构建项目

#### 使用构建脚本 (推荐)

**Windows (CMD):**
```cmd
build-win.bat
```

**Linux:**
```
build-linux.sh
```

#### 手动构建

#### 构建前端

```bash
cd tunnel-flow/web
npm install
npm run build
```

#### 构建服务端

```bash
cd ../
go mod tidy
go build -o tunnel-flow
```

#### 构建客户端

```bash
cd ../tunnel-flow-agent
go mod tidy
go build -o tunnel-flow-agent
```

### 3. 配置SSL证书

生成自签名证书用于开发环境：

```bash
# 创建SSL目录
mkdir -p tunnel-flow/ssl

# 生成私钥
openssl genrsa -out tunnel-flow/ssl/server.key 2048

# 生成证书
openssl req -new -x509 -key tunnel-flow/ssl/server.key -out tunnel-flow/ssl/server.crt -days 365 -subj "/C=CN/ST=State/L=City/O=Organization/CN=localhost"
```

#### 4. 启动服务

**启动服务端:**
```bash
cd tunnel-flow
./tunnel-flow
```

服务端将启动以下端口：
- **API服务**: `https://localhost:8080`
- **WebSocket服务**: `wss://localhost:8081`
- **HTTP代理服务**: `http://localhost:8082`

**启动客户端:**

1. 修改客户端配置文件 `tunnel-flow-agent/config.yaml`：

```yaml
server:
  url: "wss://your-server-domain:8081/ws"  # 修改为实际服务器地址

client:
  id: "your-client-id"                     # 设置唯一的客户端ID
  auth_token: "your-auth-token"            # 设置认证令牌

ssl:
  insecure_skip_verify: true               # 生产环境请设为false
```

2. 启动客户端：

```bash
cd tunnel-flow-agent
./tunnel-flow-agent
```

### 5. 访问Web管理界面

打开浏览器访问: `https://localhost:8080`

默认登录信息：
- 用户名: `admin`
- 密码: `admin123`

## 🚀 单体部署指南

使用一键构建脚本生成的单体应用，部署极其简单：

### 📦 部署准备

1. **获取构建产物**: 将`build`目录复制到目标服务器
2. **检查端口**: 确保端口8080、8081、8082未被占用
3. **配置权限**: Linux/Mac需要给可执行文件添加执行权限

### 🔧 部署步骤

#### Windows部署

```bash
# 1. 复制build目录到服务器
# 2. 进入build目录
cd build

# 3. 修改配置文件(可选)
notepad config.yaml

# 4. 启动服务
.\start.bat
```

#### Linux/Mac部署

```bash
# 1. 复制build目录到服务器
# 2. 进入build目录
cd build

# 3. 添加执行权限
chmod +x tunnel-flow tunnel-flow-agent start.sh

# 4. 修改配置文件(可选)
vi config.yaml

# 5. 启动服务
./start.sh
```

### 🌐 服务访问

部署完成后，可通过以下地址访问：

- **Web管理界面**: `https://localhost:8080`
- **WebSocket服务**: `wss://localhost:8081/ws`
- **HTTP代理服务**: `http://localhost:8082`

### 🔄 服务管理

**停止服务:**
```bash
# Windows: 按 Ctrl+C 或关闭命令窗口
# Linux/Mac: 按 Ctrl+C 或使用以下命令
pkill tunnel-flow
```

**重启服务:**
```bash
# 停止服务后重新运行启动脚本
# Windows
.\start.bat
# Linux/Mac
./start.sh
```

### 📋 部署检查清单

- [ ] 端口8080、8081、8082未被占用
- [ ] SSL证书文件存在且有效
- [ ] 配置文件参数正确
- [ ] 可执行文件有执行权限(Linux/Mac)
- [ ] 防火墙允许相应端口访问
- [ ] 数据库目录可写

## ⚙️ 配置说明

### 服务端配置 (tunnel-flow/config.yaml)

```yaml
# 服务器配置
server:
  host: "0.0.0.0"
  api_port: 8080        # API接口端口
  websocket_port: 8081  # WebSocket端口
  proxy_port: 8082      # HTTP代理端口

# 数据库配置
database:
  path: "./data/tunnel-flow.db"

# WebSocket配置
websocket:
  send_queue_size: 1000
  ssl:
    enabled: true                    # 强制启用SSL/TLS
    cert_file: "./ssl/server.crt"    # SSL证书文件路径
    key_file: "./ssl/server.key"     # SSL私钥文件路径
    force_ssl: true                  # 强制使用SSL

# 认证配置
auth:
  jwt_secret: "your-secret-key"      # JWT密钥，生产环境请修改

# 超时配置
timeout:
  reconnect_interval_ms: 5000
  ping_interval_ms: 10000
  request_timeout_ms: 30000

# 性能优化配置
performance:
  worker_pool_size: 10
  worker_queue_size: 1000
  message_queue_size: 10000
```

### 客户端配置 (tunnel-flow-agent/config.yaml)

```yaml
# WebSocket服务器地址
server:
  url: "wss://your-server:8081/ws"

# 客户端配置
client:
  id: "unique-client-id"             # 客户端唯一标识
  auth_token: "your-auth-token"      # 认证令牌

# SSL配置
ssl:
  insecure_skip_verify: false        # 生产环境建议设为false
```

## 🔧 使用示例

### 1. 创建路由规则

通过Web界面创建路由规则，将外部请求转发到内网服务：

```
外部请求: https://your-domain/api/service
内网目标: http://192.168.1.100:8080/api/service
```

### 2. API调用示例

**获取客户端列表:**
```bash
curl -X GET "https://localhost:8080/api/clients" \
  -H "Authorization: Bearer your-jwt-token"
```

**创建路由规则:**
```bash
curl -X POST "https://localhost:8080/api/routes" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-jwt-token" \
  -d '{
    "path": "/api/service",
    "target_url": "http://192.168.1.100:8080",
    "client_id": "client-001",
    "method": "GET"
  }'
```

### 3. 负载均衡配置

支持多种负载均衡策略：
- **轮询 (Round Robin)**: 默认策略
- **加权轮询 (Weighted Round Robin)**: 根据权重分配
- **最少连接 (Least Connections)**: 选择连接数最少的服务器

## 🔍 监控和日志

### 日志文件位置

- 服务端日志: `tunnel-flow/logs/tunnel-flow.log`
- 客户端日志: `tunnel-flow-agent/logs/client.log`

### 监控指标

系统提供以下监控指标：
- 连接数统计
- 请求响应时间
- 错误率统计
- 流量统计
- 系统资源使用情况

## 🛠️ 开发指南

### 开发环境设置

1. 安装依赖：
```bash
# 后端依赖
cd tunnel-flow && go mod tidy
cd ../tunnel-flow-agent && go mod tidy

# 前端依赖
cd ../tunnel-flow/web && npm install
```

2. 启动开发服务器：
```bash
# 启动前端开发服务器
cd tunnel-flow/web
npm run dev

# 启动后端服务器
cd ../
go run main.go
```

### 代码结构说明

- **internal/auth**: JWT认证和权限管理
- **internal/config**: 配置文件解析和管理
- **internal/database**: SQLite数据库操作
- **internal/websocket**: WebSocket连接管理和消息处理
- **internal/server**: HTTP服务器和路由处理
- **internal/proxy**: HTTP代理和请求转发

## 🚨 故障排除

### 常见问题

1. **端口冲突**
   - 确保端口8080、8081、8082未被占用
   - 可在配置文件中修改端口号

2. **SSL证书问题**
   - 检查证书文件路径是否正确
   - 确保证书文件权限正确

3. **连接失败**
   - 检查防火墙设置
   - 确认服务器地址和端口配置正确

4. **数据库问题**
   - 检查数据库文件权限
   - 删除数据库文件重新初始化

### 日志级别

可在配置文件中设置日志级别：
- `debug`: 详细调试信息
- `info`: 一般信息 (默认)
- `warn`: 警告信息
- `error`: 错误信息

## 🤝 贡献指南

欢迎贡献代码！请遵循以下步骤：

1. Fork 本项目
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 创建 Pull Request

## 📄 许可证

本项目采用 MIT 许可证。详情请参阅 [LICENSE](LICENSE) 文件。

## 🙏 致谢

- [Gorilla WebSocket](https://github.com/gorilla/websocket) - WebSocket实现
- [Vue.js](https://vuejs.org/) - 前端框架
- [Element Plus](https://element-plus.org/) - UI组件库
- [Gin](https://github.com/gin-gonic/gin) - HTTP框架

## 📞 支持

如果您遇到问题或有建议，请：

1. 查看 [Issues](https://github.com/your-username/tunnel-flow/issues)
2. 创建新的 Issue
3. 联系维护者

---

**注意**: 这是一个开源项目，仅供学习和研究使用。在生产环境中使用前，请确保进行充分的安全评估和测试。