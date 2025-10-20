#!/bin/bash

echo "========================================"
echo "Building Tunnel Flow - Single Binary"
echo "========================================"

# 设置变量
BUILD_DIR="build"
APP_NAME="tunnel-flow"
AGENT_NAME="tunnel-flow-agent"

# 清理之前的构建
echo "Cleaning previous build..."
rm -rf $BUILD_DIR
mkdir -p $BUILD_DIR

# 清理之前的前端构建
echo "Cleaning previous frontend build..."
rm -rf tunnel-flow/internal/web/dist

# 构建前端
echo "Building frontend..."
cd tunnel-flow/web
npm install
if [ $? -ne 0 ]; then
    echo "Frontend dependency installation failed!"
    exit 1
fi

npm run build
if [ $? -ne 0 ]; then
    echo "Frontend build failed!"
    exit 1
fi
cd ../..

# 检查前端构建结果
if [ ! -f "tunnel-flow/internal/web/dist/index.html" ]; then
    echo "Frontend build output not found!"
    exit 1
fi

echo "Frontend build completed successfully!"

# 检测操作系统
OS=$(uname -s)
case $OS in
    Linux*)
        GOOS=linux
        EXT=""
        ;;
    Darwin*)
        GOOS=darwin
        EXT=""
        ;;
    *)
        echo "Unsupported OS: $OS"
        exit 1
        ;;
esac

# 检测架构
ARCH=$(uname -m)
case $ARCH in
    x86_64)
        GOARCH=amd64
        ;;
    arm64|aarch64)
        GOARCH=arm64
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

echo "Building for $GOOS/$GOARCH..."

# 构建后端服务器（包含嵌入的前端）
echo "Building backend server..."
cd tunnel-flow
export CGO_ENABLED=0
export GOOS=$GOOS
export GOARCH=$GOARCH
go build -ldflags "-s -w" -o ../$BUILD_DIR/$APP_NAME$EXT .
if [ $? -ne 0 ]; then
    echo "Backend server build failed!"
    exit 1
fi
cd ..

# 构建代理客户端
echo "Building agent client..."
cd tunnel-flow-agent
go build -ldflags "-s -w" -o ../$BUILD_DIR/$AGENT_NAME$EXT .
if [ $? -ne 0 ]; then
    echo "Agent client build failed!"
    exit 1
fi
cd ..

# 复制配置文件
echo "Copying configuration files..."
cp tunnel-flow/config.yaml $BUILD_DIR/
if [ -d "tunnel-flow/ssl" ]; then
    mkdir -p $BUILD_DIR/ssl
    cp tunnel-flow/ssl/* $BUILD_DIR/ssl/ 2>/dev/null || true
fi

# 创建数据目录
echo "Creating data directory..."
mkdir -p $BUILD_DIR/data

# 创建启动脚本
echo "Creating startup scripts..."
cat > $BUILD_DIR/start.sh << 'EOF'
#!/bin/bash
echo "Starting Tunnel Flow Server..."
./tunnel-flow
EOF
chmod +x $BUILD_DIR/start.sh

# 创建README
echo "Creating README..."
cat > $BUILD_DIR/README.txt << EOF
Tunnel Flow - Single Binary Distribution

Files:
- $APP_NAME$EXT: Main server (includes web UI)
- $AGENT_NAME$EXT: Agent client
- config.yaml: Configuration file
- ssl/: SSL certificates (if exists)
- start.sh: Quick start script

Usage:
1. Edit config.yaml if needed
2. Run ./start.sh or ./$APP_NAME$EXT directly
3. Open http://localhost:8080 in your browser

The server includes the web UI and can be run standalone.
All frontend assets are embedded in the binary.
EOF

# 设置执行权限
chmod +x $BUILD_DIR/$APP_NAME$EXT
chmod +x $BUILD_DIR/$AGENT_NAME$EXT

echo "========================================"
echo "Build completed successfully!"
echo "========================================"
echo "Output directory: $BUILD_DIR"
echo "Main server: $BUILD_DIR/$APP_NAME$EXT"
echo "Agent client: $BUILD_DIR/$AGENT_NAME$EXT"
echo "========================================"
echo ""
echo "The server includes the web UI and can be run standalone."
echo "All frontend assets are embedded in the binary."