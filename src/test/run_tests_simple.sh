#!/bin/bash

# 简化的集成测试运行脚本
# 说明：此脚本用于逐个运行集成测试，避免端口冲突

set -e

echo "========================================="
echo "集成测试开始"
echo "========================================="

# 获取脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# 清理函数
cleanup_ports() {
    echo "清理测试端口..."
    for port in 30001 30002 30003 30004 30005 30006 30010 30011 30012; do
        # 尝试清理可能残留的端口映射
        curl -s -X POST http://localhost:8080/api/mapping/remove \
            -H "Content-Type: application/json" \
            -d "{\"port\": $port}" > /dev/null 2>&1 || true
    done
    
    # 等待端口完全释放
    sleep 3
}

# 测试前清理
echo "测试前清理..."
cleanup_ports

# 检查服务器是否运行
echo "检查服务器状态..."
if ! curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo "错误：服务器未运行，请先启动服务器"
    echo "运行: cd .. && make run-server"
    exit 1
fi
echo "✓ 服务器正在运行"

# 检查客户端是否连接
echo "检查客户端连接..."
MAPPING_RESPONSE=$(curl -s http://localhost:8080/api/mapping/list)
if [ -z "$MAPPING_RESPONSE" ]; then
    echo "警告：无法获取映射列表"
else
    echo "✓ 可以访问服务器 API"
fi

# 运行测试
echo "========================================="
echo "运行集成测试（逐个测试）..."
echo "========================================="

TEST_FAILED=0

# 逐个运行测试，避免端口冲突
echo ""
echo "测试 1: TestForwardingBasic"
echo "-----------------------------------------"
if go test -v -timeout 30s -run TestForwardingBasic; then
    echo "✓ TestForwardingBasic 通过"
else
    echo "✗ TestForwardingBasic 失败"
    TEST_FAILED=1
fi
cleanup_ports

echo ""
echo "测试 2: TestMultipleForwards"
echo "-----------------------------------------"
if go test -v -timeout 30s -run TestMultipleForwards; then
    echo "✓ TestMultipleForwards 通过"
else
    echo "✗ TestMultipleForwards 失败"
    TEST_FAILED=1
fi
cleanup_ports

echo ""
echo "测试 3: TestAddAndRemoveMapping"
echo "-----------------------------------------"
if go test -v -timeout 30s -run TestAddAndRemoveMapping; then
    echo "✓ TestAddAndRemoveMapping 通过"
else
    echo "✗ TestAddAndRemoveMapping 失败"
    TEST_FAILED=1
fi
cleanup_ports

echo ""
echo "测试 4: TestConcurrentRequests"
echo "-----------------------------------------"
if go test -v -timeout 30s -run TestConcurrentRequests; then
    echo "✓ TestConcurrentRequests 通过"
else
    echo "✗ TestConcurrentRequests 失败"
    TEST_FAILED=1
fi
cleanup_ports

# 测试后清理
echo ""
echo "测试后清理..."
cleanup_ports

echo "========================================="
if [ $TEST_FAILED -eq 0 ]; then
    echo "✓ 所有测试通过"
    echo "========================================="
    exit 0
else
    echo "✗ 部分测试失败"
    echo "========================================="
    exit 1
fi
