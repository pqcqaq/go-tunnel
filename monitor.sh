#!/bin/bash

# Go Tunnel 监控脚本
# 用于实时监控 goroutine 数量和 CPU 占用

SERVER_PPROF="http://localhost:6060"
CLIENT_PPROF="http://localhost:6061"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "========================================"
echo "   Go Tunnel 实时监控"
echo "========================================"
echo ""
echo "监控项："
echo "  - Goroutine 数量（正常应该 < 15）"
echo "  - CPU 占用率"
echo "  - 进程状态"
echo ""
echo "按 Ctrl+C 停止监控"
echo "========================================"
echo ""

printf "%-12s | %-20s | %-20s | %-10s\n" "时间" "服务器" "客户端" "CPU占用"
printf "%-12s | %-20s | %-20s | %-10s\n" "--------" "--------------------" "--------------------" "----------"

while true; do
    TIMESTAMP=$(date +%H:%M:%S)
    
    # 获取服务器 goroutine 数量
    SERVER_GOROUTINES=$(curl -s --connect-timeout 1 $SERVER_PPROF/debug/pprof/goroutine 2>/dev/null | \
                       grep -oP 'goroutine profile: total \K\d+' || echo "N/A")
    
    # 获取客户端 goroutine 数量  
    CLIENT_GOROUTINES=$(curl -s --connect-timeout 1 $CLIENT_PPROF/debug/pprof/goroutine 2>/dev/null | \
                       grep -oP 'goroutine profile: total \K\d+' || echo "N/A")
    
    # 获取服务器进程 CPU 占用
    SERVER_PID=$(pgrep -f "bin/server" | head -1)
    if [ ! -z "$SERVER_PID" ]; then
        SERVER_CPU=$(ps -p $SERVER_PID -o %cpu= | tr -d ' ')
    else
        SERVER_CPU="未运行"
    fi
    
    # 获取客户端进程 CPU 占用
    CLIENT_PID=$(pgrep -f "bin/client" | head -1)
    if [ ! -z "$CLIENT_PID" ]; then
        CLIENT_CPU=$(ps -p $CLIENT_PID -o %cpu= | tr -d ' ')
    else
        CLIENT_CPU="未运行"
    fi
    
    # 格式化 goroutine 信息
    if [ "$SERVER_GOROUTINES" != "N/A" ]; then
        SERVER_INFO="Goroutines: $SERVER_GOROUTINES"
        # 如果 goroutine 数量异常，高亮显示
        if [ "$SERVER_GOROUTINES" -gt 20 ]; then
            SERVER_INFO="${RED}Goroutines: $SERVER_GOROUTINES ⚠${NC}"
        elif [ "$SERVER_GOROUTINES" -gt 15 ]; then
            SERVER_INFO="${YELLOW}Goroutines: $SERVER_GOROUTINES ⚠${NC}"
        else
            SERVER_INFO="${GREEN}Goroutines: $SERVER_GOROUTINES ✓${NC}"
        fi
    else
        SERVER_INFO="${RED}离线${NC}"
    fi
    
    if [ "$CLIENT_GOROUTINES" != "N/A" ]; then
        CLIENT_INFO="Goroutines: $CLIENT_GOROUTINES"
        if [ "$CLIENT_GOROUTINES" -gt 20 ]; then
            CLIENT_INFO="${RED}Goroutines: $CLIENT_GOROUTINES ⚠${NC}"
        elif [ "$CLIENT_GOROUTINES" -gt 15 ]; then
            CLIENT_INFO="${YELLOW}Goroutines: $CLIENT_GOROUTINES ⚠${NC}"
        else
            CLIENT_INFO="${GREEN}Goroutines: $CLIENT_GOROUTINES ✓${NC}"
        fi
    else
        CLIENT_INFO="${RED}离线${NC}"
    fi
    
    # CPU 信息
    if [ "$SERVER_CPU" != "未运行" ]; then
        CPU_INFO="S:${SERVER_CPU}%"
        # 检查 CPU 占用是否过高
        CPU_VALUE=$(echo $SERVER_CPU | cut -d. -f1)
        if [ "$CPU_VALUE" -gt 50 ] 2>/dev/null; then
            CPU_INFO="${RED}S:${SERVER_CPU}% ⚠${NC}"
        elif [ "$CPU_VALUE" -gt 10 ] 2>/dev/null; then
            CPU_INFO="${YELLOW}S:${SERVER_CPU}%${NC}"
        else
            CPU_INFO="${GREEN}S:${SERVER_CPU}%${NC}"
        fi
    else
        CPU_INFO="${RED}未运行${NC}"
    fi
    
    if [ "$CLIENT_CPU" != "未运行" ]; then
        if [ "$SERVER_CPU" != "未运行" ]; then
            CPU_INFO="${CPU_INFO} "
        else
            CPU_INFO=""
        fi
        CLIENT_CPU_VALUE=$(echo $CLIENT_CPU | cut -d. -f1)
        if [ "$CLIENT_CPU_VALUE" -gt 50 ] 2>/dev/null; then
            CPU_INFO="${CPU_INFO}${RED}C:${CLIENT_CPU}% ⚠${NC}"
        elif [ "$CLIENT_CPU_VALUE" -gt 10 ] 2>/dev/null; then
            CPU_INFO="${CPU_INFO}${YELLOW}C:${CLIENT_CPU}%${NC}"
        else
            CPU_INFO="${CPU_INFO}${GREEN}C:${CLIENT_CPU}%${NC}"
        fi
    fi
    
    # 输出监控信息
    printf "%-12s | %-35s | %-35s | %-30s\n" "$TIMESTAMP" "$SERVER_INFO" "$CLIENT_INFO" "$CPU_INFO"
    
    sleep 2
done
