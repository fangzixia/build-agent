#!/bin/bash

# Build Agent Web UI 启动脚本

set -e

# 颜色定义
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  Build Agent Web UI 启动脚本${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# 检查 Go 环境
if ! command -v go &> /dev/null; then
    echo -e "${YELLOW}错误: 未找到 Go 环境${NC}"
    echo "请先安装 Go: https://golang.org/dl/"
    exit 1
fi

echo -e "${GREEN}✓${NC} Go 环境检查通过"

# 检查 .env 文件
if [ ! -f ".env" ]; then
    echo -e "${YELLOW}警告: 未找到 .env 文件${NC}"
    echo "正在从 .env.example 复制..."
    cp .env.example .env
    echo -e "${GREEN}✓${NC} 已创建 .env 文件，请根据需要修改配置"
fi

# 检查 web 目录
if [ ! -d "web" ]; then
    echo -e "${YELLOW}错误: 未找到 web 目录${NC}"
    echo "请确保在项目根目录下运行此脚本"
    exit 1
fi

echo -e "${GREEN}✓${NC} Web 文件检查通过"

# 创建 .spec 目录（如果不存在）
if [ ! -d ".spec" ]; then
    echo "正在创建 .spec 目录..."
    mkdir -p .spec
    echo -e "${GREEN}✓${NC} 已创建 .spec 目录"
fi

# 获取端口配置
PORT=${HTTP_ADDR:-:8080}
if [ -f ".env" ]; then
    PORT=$(grep "^HTTP_ADDR=" .env | cut -d '=' -f2 || echo ":8080")
fi

# 清理端口前缀
PORT_NUM=$(echo $PORT | sed 's/://')

echo ""
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  启动配置${NC}"
echo -e "${BLUE}========================================${NC}"
echo -e "端口: ${GREEN}${PORT_NUM}${NC}"
echo -e "访问地址: ${GREEN}http://localhost:${PORT_NUM}${NC}"
echo ""

# 启动服务
echo -e "${BLUE}正在启动服务...${NC}"
echo ""

go run ./cmd/agent serve --addr ${PORT}
