@echo off
REM Build script for Build Agent Desktop Application (Windows)

echo ========================================
echo Building Build Agent Desktop Application
echo ========================================
echo.

REM Check if Wails CLI is installed
where wails >nul 2>nul
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Wails CLI not found!
    echo Please install it first: go install github.com/wailsapp/wails/v2/cmd/wails@latest
    exit /b 1
)

REM Check if Go is installed
where go >nul 2>nul
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Go not found!
    echo Please install Go 1.24 or later from https://go.dev/dl/
    exit /b 1
)

echo [1/3] Checking dependencies...
go mod download
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Failed to download dependencies
    exit /b 1
)

echo.
echo [2/3] Building desktop application...
wails build
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Build failed
    exit /b 1
)

echo.
echo [3/3] Build completed successfully!
echo.
echo Output: build\bin\build-agent.exe
echo.
echo You can now run the application:
echo   .\build\bin\build-agent.exe
echo.

exit /b 0
