#!/bin/bash

# 集成测试运行脚本
# 此脚本用于运行 go-tunnel 的集成测试

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置
SERVER_API=${SERVER_API:-"http://localhost:8080"}
SERVER_TUNNEL_PORT=${SERVER_TUNNEL_PORT:-9000}
TEST_TIMEOUT=${TEST_TIMEOUT:-30s}

echo -e "${BLUE}================================${NC}"
echo -e "${BLUE}Go-Tunnel 集成测试${NC}"
echo -e "${BLUE}================================${NC}"
echo ""

# 检查服务器是否运行
echo -e "${YELLOW}检查服务器状态...${NC}"
if curl -s "${SERVER_API}/health" > /dev/null 2>&1; then
    echo -e "${GREEN}✓ 服务器运行正常${NC}"
else
    echo -e "${RED}✗ 服务器未响应${NC}"
    echo -e "${YELLOW}请确保服务器已启动：${NC}"
    echo "  cd ../src && make run-server"
    exit 1
fi

# 检查客户端是否连接（通过创建一个测试映射来验证）
echo -e "${YELLOW}检查隧道连接...${NC}"
if curl -s -X POST "${SERVER_API}/api/mapping/create" \
    -H "Content-Type: application/json" \
    -d '{"port": 39999}' | grep -q "success"; then
    echo -e "${GREEN}✓ 隧道连接正常${NC}"
    # 清理测试映射
    curl -s -X POST "${SERVER_API}/api/mapping/remove" \
        -H "Content-Type: application/json" \
        -d '{"port": 39999}' > /dev/null 2>&1 || true
else
    echo -e "${YELLOW}⚠ 无法验证隧道连接，测试可能失败${NC}"
    echo -e "${YELLOW}请确保客户端已启动：${NC}"
    echo "  cd ../src && make run-client"
fi

echo ""
echo -e "${BLUE}开始运行测试...${NC}"
echo ""

# 切换到测试目录
cd "$(dirname "$0")"

# 运行测试
if [ "$1" = "verbose" ] || [ "$1" = "-v" ]; then
    go test -v -timeout ${TEST_TIMEOUT}
elif [ -n "$1" ]; then
    # 运行特定测试
    echo -e "${BLUE}运行测试: $1${NC}"
    go test -v -timeout ${TEST_TIMEOUT} -run "$1"
else
    go test -timeout ${TEST_TIMEOUT}
fi

TEST_EXIT_CODE=$?

echo ""
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}================================${NC}"
    echo -e "${GREEN}✓ 所有测试通过${NC}"
    echo -e "${GREEN}================================${NC}"
else
    echo -e "${RED}================================${NC}"
    echo -e "${RED}✗ 测试失败${NC}"
    echo -e "${RED}================================${NC}"
fi

exit $TEST_EXIT_CODE
