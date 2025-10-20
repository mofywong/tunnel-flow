@echo off
echo ========================================
echo Building Tunnel Flow - Single Binary
echo ========================================

:: 设置变量
set BUILD_DIR=build
set APP_NAME=tunnel-flow
set AGENT_NAME=tunnel-flow-agent

:: 清理之前的构建
echo Cleaning previous build...
if exist %BUILD_DIR% rmdir /s /q %BUILD_DIR%
mkdir %BUILD_DIR%

:: 清理之前的前端构建
echo Cleaning previous frontend build...
if exist tunnel-flow\internal\web\dist rmdir /s /q tunnel-flow\internal\web\dist

:: 构建前端
echo Building frontend...
cd tunnel-flow\web
call npm install
if %errorlevel% neq 0 (
    echo Frontend dependency installation failed!
    exit /b 1
)

call npm run build
if %errorlevel% neq 0 (
    echo Frontend build failed!
    exit /b 1
)
cd ..\..

:: 检查前端构建结果
if not exist tunnel-flow\internal\web\dist\index.html (
    echo Frontend build output not found!
    exit /b 1
)

echo Frontend build completed successfully!

:: 构建后端服务器（包含嵌入的前端）
echo Building backend server...
cd tunnel-flow
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
go build -ldflags "-s -w" -o ..\%BUILD_DIR%\%APP_NAME%.exe .
if %errorlevel% neq 0 (
    echo Backend server build failed!
    exit /b 1
)
cd ..

:: 构建代理客户端
echo Building agent client...
cd tunnel-flow-agent
go build -ldflags "-s -w" -o ..\%BUILD_DIR%\%AGENT_NAME%.exe .
if %errorlevel% neq 0 (
    echo Agent client build failed!
    exit /b 1
)
cd ..

:: 复制配置文件
echo Copying configuration files...
copy tunnel-flow\config.yaml %BUILD_DIR%\
if exist tunnel-flow\ssl mkdir %BUILD_DIR%\ssl
if exist tunnel-flow\ssl\*.* copy tunnel-flow\ssl\*.* %BUILD_DIR%\ssl\

:: 创建数据目录
echo Creating data directory...
mkdir %BUILD_DIR%\data

:: 创建启动脚本
echo Creating startup scripts...
echo @echo off > %BUILD_DIR%\start.bat
echo echo Starting Tunnel Flow Server... >> %BUILD_DIR%\start.bat
echo %APP_NAME%.exe >> %BUILD_DIR%\start.bat
echo pause >> %BUILD_DIR%\start.bat

:: 创建README
echo Creating README...
echo Tunnel Flow - Single Binary Distribution > %BUILD_DIR%\README.txt
echo. >> %BUILD_DIR%\README.txt
echo Files: >> %BUILD_DIR%\README.txt
echo - %APP_NAME%.exe: Main server (includes web UI) >> %BUILD_DIR%\README.txt
echo - %AGENT_NAME%.exe: Agent client >> %BUILD_DIR%\README.txt
echo - config.yaml: Configuration file >> %BUILD_DIR%\README.txt
echo - ssl/: SSL certificates (if exists) >> %BUILD_DIR%\README.txt
echo - start.bat: Quick start script >> %BUILD_DIR%\README.txt
echo. >> %BUILD_DIR%\README.txt
echo Usage: >> %BUILD_DIR%\README.txt
echo 1. Edit config.yaml if needed >> %BUILD_DIR%\README.txt
echo 2. Run start.bat or %APP_NAME%.exe directly >> %BUILD_DIR%\README.txt
echo 3. Open http://localhost:8080 in your browser >> %BUILD_DIR%\README.txt

echo ========================================
echo Build completed successfully!
echo ========================================
echo Output directory: %BUILD_DIR%
echo Main server: %BUILD_DIR%\%APP_NAME%.exe
echo Agent client: %BUILD_DIR%\%AGENT_NAME%.exe
echo ========================================
echo.
echo The server includes the web UI and can be run standalone.
echo All frontend assets are embedded in the binary.
echo.