# Go Tunnel 调试指南

## 概述
本指南介绍如何使用 Go 的内置性能分析工具来调试和诊断 CPU 占用、内存泄漏和 goroutine 泄漏等问题。

## 已添加的调试功能

### 1. pprof HTTP 接口
- **服务器**: `http://localhost:6060/debug/pprof/`
- **客户端**: `http://localhost:6061/debug/pprof/`

### 2. Goroutine 监控
每10秒自动打印当前 goroutine 数量到日志。

## 使用方法

### 方法一：实时查看 Goroutine 堆栈（最有用！）

这是找出 CPU 占用问题的最直接方法。

#### 1. 启动服务器和客户端
```bash
# 终端1：启动服务器
cd /home/qcqcqc/workspace/go-tunnel/src
make run-server

# 终端2：启动客户端
make run-client

# 终端3：建立 SSH 连接测试
ssh root@localhost -p 30009
# 执行一些操作后断开
```

#### 2. SSH 断开后，立即查看 goroutine 堆栈

**查看服务器的 goroutine：**
```bash
# 在浏览器打开或使用 curl
curl http://localhost:6060/debug/pprof/goroutine?debug=2

# 或者保存到文件
curl http://localhost:6060/debug/pprof/goroutine?debug=2 > server_goroutines.txt

# 使用 less 查看（方便搜索）
curl http://localhost:6060/debug/pprof/goroutine?debug=2 | less
```

**查看客户端的 goroutine：**
```bash
curl http://localhost:6061/debug/pprof/goroutine?debug=2 > client_goroutines.txt
```

#### 3. 分析 goroutine 堆栈

查看输出中的重复模式：
- **正常情况**: 应该只有几个基础 goroutine（监听器、心跳等）
- **异常情况**: 如果有大量相同的堆栈，说明有 goroutine 泄漏

**关键搜索词**：
```bash
# 在堆栈文件中搜索
grep -n "forwardData" server_goroutines.txt
grep -n "Read" server_goroutines.txt
grep -n "runtime.gopark" server_goroutines.txt
```

**如何解读堆栈**：
```
goroutine 123 [running]:
port-forward/server/tunnel.(*Server).forwardData(0xc000120000, 0xc000130000)
        /path/to/tunnel.go:456 +0x123

这表示 goroutine 123 正在执行 forwardData 函数的第456行
```

### 方法二：CPU Profile（找出高 CPU 占用的函数）

#### 1. 收集 30 秒的 CPU profile
```bash
# 服务器
curl http://localhost:6060/debug/pprof/profile?seconds=30 > server_cpu.prof

# 客户端  
curl http://localhost:6061/debug/pprof/profile?seconds=30 > client_cpu.prof
```

**注意**: 在这30秒内，让程序保持在高CPU占用状态（SSH连接和断开）

#### 2. 分析 CPU profile
```bash
# 使用 go tool pprof 交互式分析
go tool pprof server_cpu.prof

# 进入交互式界面后的常用命令：
(pprof) top           # 显示占用CPU最多的函数
(pprof) top -cum      # 按累计时间排序
(pprof) list forwardData  # 显示 forwardData 函数的详细分析
(pprof) web           # 生成可视化图表（需要 graphviz）
(pprof) quit          # 退出
```

#### 3. Web界面分析（推荐）
```bash
# 启动 Web UI
go tool pprof -http=:8080 server_cpu.prof

# 然后在浏览器打开: http://localhost:8080
# 可以看到火焰图、调用图等可视化分析
```

### 方法三：实时 CPU Profile（找出正在运行的热点）

```bash
# 查看当前正在执行的函数
curl http://localhost:6060/debug/pprof/profile?seconds=5 | go tool pprof -http=:8080 -
```

### 方法四：查看所有 goroutine 数量

```bash
# 服务器
curl http://localhost:6060/debug/pprof/goroutine

# 或者在浏览器访问，会显示友好的界面
```

### 方法五：内存分析（如果怀疑内存泄漏）

```bash
# 堆内存 profile
curl http://localhost:6060/debug/pprof/heap > heap.prof
go tool pprof -http=:8080 heap.prof

# 内存分配统计
curl http://localhost:6060/debug/pprof/allocs > allocs.prof
go tool pprof -http=:8080 allocs.prof
```

### 方法六：使用 trace 进行详细跟踪

```bash
# 收集 5 秒的执行跟踪
curl http://localhost:6060/debug/pprof/trace?seconds=5 > trace.out

# 查看跟踪
go tool trace trace.out

# 这会启动一个 Web 界面，可以看到：
# - 每个 goroutine 的时间线
# - 系统调用
# - GC 事件
# - goroutine 创建和销毁
```

## 典型问题诊断流程

### 场景：SSH 断开后 CPU 仍然高占用

#### 步骤 1：确认问题
```bash
# 观察日志中的 goroutine 数量
# 正常情况下应该在 10 个以内
# 如果持续增长或高于 50，说明有泄漏
```

#### 步骤 2：抓取 goroutine 堆栈
```bash
# SSH 连接前
curl http://localhost:6060/debug/pprof/goroutine?debug=2 > before.txt

# SSH 连接中
curl http://localhost:6060/debug/pprof/goroutine?debug=2 > during.txt

# SSH 断开后（等待 10 秒）
curl http://localhost:6060/debug/pprof/goroutine?debug=2 > after.txt

# 比较文件
diff before.txt after.txt | less
```

#### 步骤 3：查找泄漏的 goroutine
```bash
# 统计每种堆栈的数量
grep -A 20 "^goroutine" after.txt | grep "port-forward" | sort | uniq -c | sort -rn
```

#### 步骤 4：分析特定函数
```bash
# 如果发现大量 forwardData goroutine
grep -A 30 "forwardData" after.txt | head -50
```

#### 步骤 5：CPU Profile
```bash
# 在 CPU 高占用时收集
curl http://localhost:6060/debug/pprof/profile?seconds=10 > high_cpu.prof

# 分析
go tool pprof -http=:8080 high_cpu.prof

# 查看 top 函数，通常会看到：
# - Read 操作占用高 -> 说明在忙循环读取
# - syscall 占用高 -> 可能是网络调用问题
# - runtime.schedule -> goroutine 调度开销大（goroutine 太多）
```

## 预期的正常状态

### Goroutine 数量
- **服务器空闲**: 8-12 个 goroutine
  - acceptLoop (1)
  - handleTunnelRead (1)
  - handleTunnelWrite (1)
  - goroutine 监控 (1)
  - pprof HTTP (1)
  - API HTTP (1)
  - 其他系统 goroutine (2-6)

- **有 SSH 连接时**: +2 个 goroutine
  - forwardData (1) 服务器端
  - forwardData (1) 客户端

- **SSH 断开后**: 应该回到空闲状态的数量

### CPU 占用
- **空闲**: < 5%
- **传输数据时**: 取决于传输速度，但不应该持续高占用
- **连接断开后**: 立即回到 < 5%

## 常见 CPU 占用原因及特征

### 1. 忙循环读取
**特征**:
- CPU 占用 80-100%
- goroutine 堆栈卡在 `Read()` 或 `conn.Read()`
- pprof 显示大量时间在 `syscall.Read`

**原因**: 连接已关闭，但循环继续调用 Read，立即返回错误

### 2. Channel 阻塞循环
**特征**:
- CPU 占用 20-40%
- goroutine 堆栈在 `select` 或 channel 操作
- 大量 goroutine 在 `runtime.gopark`

**原因**: channel 满了或没有接收者，发送阻塞

### 3. Goroutine 泄漏
**特征**:
- goroutine 数量持续增长
- 内存占用增长
- CPU 占用逐渐升高

**原因**: goroutine 没有正确退出

## 快速命令参考

```bash
# 查看 goroutine 数量
curl -s http://localhost:6060/debug/pprof/ | grep goroutine

# 查看详细的 goroutine 堆栈
curl http://localhost:6060/debug/pprof/goroutine?debug=2 | less

# 收集 CPU profile 并分析
curl http://localhost:6060/debug/pprof/profile?seconds=10 | go tool pprof -http=:8080 -

# 查看实时监控日志
tail -f /path/to/server.log | grep "监控"

# 统计当前各种 goroutine 的数量
curl -s http://localhost:6060/debug/pprof/goroutine?debug=2 | \
  grep -E "^goroutine|^\w+\(" | \
  paste - - | \
  awk '{print $NF}' | \
  sort | uniq -c | sort -rn

# 只看 port-forward 相关的 goroutine
curl -s http://localhost:6060/debug/pprof/goroutine?debug=2 | \
  grep -A 15 "port-forward"
```

## 监控脚本

创建一个监控脚本 `monitor.sh`:

```bash
#!/bin/bash

echo "开始监控 go-tunnel..."
echo "时间戳 | Goroutines | CPU%"
echo "------|------------|-----"

while true; do
    # 获取 goroutine 数量
    GOROUTINES=$(curl -s http://localhost:6060/debug/pprof/goroutine | \
                 grep -oP 'goroutine profile: total \K\d+' || echo "N/A")
    
    # 获取进程 CPU 占用
    PID=$(pgrep -f "server.*9000" | head -1)
    if [ ! -z "$PID" ]; then
        CPU=$(ps -p $PID -o %cpu= || echo "N/A")
    else
        CPU="N/A"
    fi
    
    # 输出
    echo "$(date +%H:%M:%S) | $GOROUTINES | $CPU%"
    
    sleep 2
done
```

使用方法：
```bash
chmod +x monitor.sh
./monitor.sh
```

## 故障排查清单

- [ ] 检查日志中的 goroutine 数量是否异常增长
- [ ] 抓取 goroutine 堆栈，查看是否有重复的堆栈
- [ ] 使用 CPU profile 找出占用最多的函数
- [ ] 检查是否有 goroutine 卡在 Read/Write 操作
- [ ] 验证连接关闭后，相关 goroutine 是否退出
- [ ] 检查 channel 是否有阻塞
- [ ] 使用 trace 查看详细的执行流程

## 其他有用的命令

```bash
# 查看所有可用的 pprof endpoints
curl http://localhost:6060/debug/pprof/

# 查看程序版本和编译信息
curl http://localhost:6060/debug/pprof/cmdline

# 查看当前的调度器状态
GODEBUG=schedtrace=1000 ./server

# 查看 GC 统计
curl http://localhost:6060/debug/pprof/heap?debug=1 | head -30
```

## 参考资料

- [Go pprof 官方文档](https://golang.org/pkg/net/http/pprof/)
- [Go tool pprof 使用指南](https://github.com/google/pprof/blob/master/doc/README.md)
- [Go 性能分析最佳实践](https://go.dev/blog/pprof)

## 下一步

在发现问题后：
1. 记录问题出现时的堆栈信息
2. 确认是哪个函数/循环导致的
3. 检查该位置的退出条件
4. 验证错误处理逻辑
5. 添加更多的日志和退出检查
