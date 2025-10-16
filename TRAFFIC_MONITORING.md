# 流量监控功能文档

## 概述

新增了实时流量监控功能，可以查看每个端口映射和隧道的流量统计，并以可视化图表的形式展示。

## 新增功能

### 1. 流量统计

系统自动统计以下数据：
- **隧道流量**: 通过tunnel发送/接收的总字节数
- **端口映射流量**: 每个端口映射单独的流量统计
- **总流量**: 所有流量的汇总

### 2. API 接口

#### GET /api/stats/traffic

获取当前最新的流量统计数据。

**响应示例**:
```json
{
  "success": true,
  "message": "获取流量统计成功",
  "data": {
    "tunnel": {
      "bytes_sent": 1048576,
      "bytes_received": 2097152,
      "last_update": 1697462400
    },
    "mappings": [
      {
        "port": 30009,
        "bytes_sent": 524288,
        "bytes_received": 1048576,
        "last_update": 1697462400
      }
    ],
    "total_sent": 1572864,
    "total_received": 3145728,
    "timestamp": 1697462400
  }
}
```

#### GET /api/stats/monitor

访问流量监控Web页面，提供可视化的实时监控界面。

**特性**:
- 📊 实时流量趋势图表
- 📈 流量速率计算 (KB/s)
- 🔄 每3秒自动刷新
- 📱 响应式设计，支持移动设备
- 🎨 美观的UI界面

### 3. Web监控页面

访问地址: `http://localhost:8080/api/stats/monitor`

**页面内容**:

1. **总览卡片**
   - 总发送流量
   - 总接收流量
   - 隧道发送流量
   - 隧道接收流量

2. **实时流量趋势图**
   - 显示最近20个数据点
   - 发送/接收速率曲线
   - 单位: KB/s

3. **端口映射详情**
   - 每个端口的流量统计
   - 实时更新

## 使用方法

### 1. 启动服务

```bash
cd /home/qcqcqc/workspace/go-tunnel/src
make build
make run-server
```

### 2. 访问监控页面

在浏览器中打开:
```
http://localhost:8080/api/stats/monitor
```

### 3. API调用示例

使用curl获取流量数据:
```bash
# 获取流量统计
curl http://localhost:8080/api/stats/traffic

# 格式化输出
curl http://localhost:8080/api/stats/traffic | jq .
```

使用JavaScript获取数据:
```javascript
fetch('http://localhost:8080/api/stats/traffic')
  .then(res => res.json())
  .then(data => {
    console.log('总发送:', data.data.total_sent);
    console.log('总接收:', data.data.total_received);
  });
```

### 4. 集成到自己的前端

你可以定期调用 `/api/stats/traffic` 接口获取数据，然后在自己的前端页面中展示：

```javascript
// 每3秒获取一次数据
setInterval(async () => {
  const response = await fetch('http://localhost:8080/api/stats/traffic');
  const result = await response.json();
  
  if (result.success) {
    const data = result.data;
    
    // 更新UI
    updateTotalSent(data.total_sent);
    updateTotalReceived(data.total_received);
    
    // 更新图表
    updateChart(data);
    
    // 更新端口列表
    updatePortList(data.mappings);
  }
}, 3000);
```

## 实现细节

### 流量统计机制

1. **Tunnel层统计**
   - 在 `writeTunnelMessage` 中记录发送字节数
   - 在 `readTunnelMessage` 中记录接收字节数
   - 使用 `atomic.AddUint64` 保证并发安全

2. **Forwarder层统计**
   - 在 `io.Copy` 返回后记录传输字节数
   - 分别统计客户端→目标和目标→客户端的流量

3. **数据结构**
   ```go
   type TrafficStats struct {
       BytesSent     uint64 // 发送字节数
       BytesReceived uint64 // 接收字节数
       LastUpdate    int64  // 最后更新时间
   }
   ```

### 性能考虑

- 使用原子操作 (`sync/atomic`) 避免锁竞争
- 统计开销极小，几乎不影响转发性能
- 只在需要时才计算速率

## 测试验证

### 1. 基础测试

```bash
# 1. 启动服务器和客户端
make run-server  # 终端1
make run-client  # 终端2

# 2. 创建端口映射
curl -X POST http://localhost:8080/api/mapping/create \
  -H "Content-Type: application/json" \
  -d '{
    "source_port": 30009,
    "target_host": "127.0.0.1",
    "target_port": 22,
    "use_tunnel": true
  }'

# 3. 通过映射传输一些数据
ssh root@localhost -p 30009

# 4. 查看流量统计
curl http://localhost:8080/api/stats/traffic | jq .
```

### 2. 压力测试

```bash
# 使用 scp 传输大文件测试流量统计
dd if=/dev/zero of=test.dat bs=1M count=100
scp -P 30009 test.dat root@localhost:/tmp/

# 观察监控页面的实时变化
```

### 3. 长时间运行测试

```bash
# 保持监控页面打开，观察：
# - 图表是否正常更新
# - 数据是否累积正确
# - 内存占用是否稳定
```

## 故障排查

### 问题1: 流量统计不准确

**可能原因**:
- 连接未正常关闭
- 统计溢出（极少见，uint64可存储18EB）

**解决方法**:
```bash
# 重启服务器重置统计
```

### 问题2: 监控页面无法访问

**检查**:
```bash
# 确认服务器正在运行
curl http://localhost:8080/health

# 检查端口是否被占用
netstat -tlnp | grep 8080
```

### 问题3: 图表不更新

**检查**:
1. 打开浏览器开发者工具 (F12)
2. 查看 Console 是否有错误
3. 查看 Network 标签，确认 API 请求成功

## 未来改进

可能的增强功能：
- [ ] 流量统计持久化（保存到数据库）
- [ ] 历史流量查询
- [ ] 流量告警（超过阈值时通知）
- [ ] 导出流量报表
- [ ] 按时间段统计（小时/天/月）
- [ ] WebSocket实时推送（减少轮询）

## 相关文件

- `src/server/stats/stats.go` - 流量统计数据结构
- `src/server/tunnel/tunnel.go` - Tunnel流量统计实现
- `src/server/forwarder/forwarder.go` - Forwarder流量统计实现
- `src/server/api/api.go` - API接口和监控页面

## 技术栈

- **后端**: Go 1.x
- **前端**: 原生 JavaScript + Chart.js 4.4.0
- **图表库**: Chart.js (CDN)
- **样式**: CSS3 (渐变、动画)

## 许可证

与主项目相同

## 更新日期

2025-10-16
