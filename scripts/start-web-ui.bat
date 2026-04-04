@echo off
REM Build Agent Web UI 启动脚本 (Windows)

echo ========================================
echo   Build Agent Web UI 启动脚本
echo ========================================
echo.

REM 检查 Go 环境
where go >nul 2>nul
if %ERRORLEVEL% NEQ 0 (
    echo 错误: 未找到 Go 环境
    echo 请先安装 Go: https://golang.org/dl/
    exit /b 1
)

echo [OK] Go 环境检查通过

REM 检查 .env 文件
if not exist ".env" (
    echo 警告: 未找到 .env 文件
    echo 正在从 .env.example 复制...
    copy .env.example .env >nul
    echo [OK] 已创建 .env 文件，请根据需要修改配置
)

REM 检查 web 目录
if not exist "web" (
    echo 错误: 未找到 web 目录
    echo 请确保在项目根目录下运行此脚本
    exit /b 1
)

echo [OK] Web 文件检查通过

REM 创建 .spec 目录（如果不存在）
if not exist ".spec" (
    echo 正在创建 .spec 目录...
    mkdir .spec
    echo [OK] 已创建 .spec 目录
)

REM 获取端口配置
set PORT=:8080
if exist ".env" (
    for /f "tokens=2 delims==" %%a in ('findstr "^HTTP_ADDR=" .env') do set PORT=%%a
)

REM 清理端口前缀
set PORT_NUM=%PORT::=%

echo.
echo ========================================
echo   启动配置
echo ========================================
echo 端口: %PORT_NUM%
echo 访问地址: http://localhost:%PORT_NUM%
echo.

REM 启动服务
echo 正在启动服务...
echo.

go run ./cmd/agent serve --addr %PORT%
