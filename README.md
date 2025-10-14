# 端口转发和内网穿透系统

一个高性能、生产级别的 TCP 端口转发和内网穿透系统，使用 Go 语言编写。

## 功能特性

- ✅ **端口转发**: 支持配置化的 TCP 端口范围管理和转发
- ✅ **内网穿透**: 支持通过隧道连接实现内网服务穿透
- ✅ **持久化存储**: 使用 SQLite 数据库持久化端口映射配置
- ✅ **动态管理**: 通过 HTTP API 动态创建和删除端口映射
- ✅ **自动恢复**: 服务器启动时自动恢复已保存的端口映射
- ✅ **连接池管理**: 高效的连接池和并发管理
- ✅ **优雅关闭**: 支持优雅关闭和信号处理
- ✅ **自动重连**: 客户端支持断线自动重连
- ✅ **生产级代码**: 完善的错误处理、日志记录和性能优化

## 项目结构

```
port-forward/
├── config.yaml              # 配置文件
├── go.mod                   # Go 模块文件
├── server/                  # 服务器端
│   ├── main.go             # 服务器主程序
│   ├── config/             # 配置管理
│   │   └── config.go
│   ├── db/                 # 数据库管理
│   │   └── database.go
│   ├── forwarder/          # 端口转发
│   │   └── forwarder.go
│   ├── tunnel/             # 内网穿透服务端
│   │   └── server.go
│   └── api/                # HTTP API
│       └── handler.go
└── client/                  # 客户端
    ├── main.go             # 客户端主程序
    └── tunnel/             # 内网穿透客户端
        └── client.go
```

## 快速开始

### 1. 安装依赖

```bash
cd port-forward
go mod download
```

### 2. 配置服务器

编辑 `config.yaml` 文件：

```yaml
# 端口范围配置
port_range:
  from: 10000
  end: 10100

# 内网穿透配置
tunnel:
  enabled: true        # 是否启用内网穿透
  listen_port: 9000    # 隧道监听端口

# HTTP API 配置
api:
  listen_port: 8080

# 数据库配置
database:
  path: "./data/mappings.db"
```

### 3. 启动服务器

```bash
# 编译服务器
go build -o server ./server

# 运行服务器
./server -config config.yaml
```

### 4. 启动客户端（如果启用了内网穿透）

```bash
# 编译客户端
go build -o client ./client

# 运行客户端
./client -server <服务器地址:9000>
```

## HTTP API 使用

### 创建端口映射

**请求**:
```bash
curl -X POST http://localhost:8080/api/mapping/create \
  -H "Content-Type: application/json" \
  -d '{
    "port": 10001,
    "target_ip": "192.168.1.100"
  }'
```

**响应**:
```json
{
  "success": true,
  "message": "端口映射创建成功",
  "data": {
    "port": 10001,
    "target_ip": "192.168.1.100",
    "use_tunnel": false
  }
}
```

### 删除端口映射

**请求**:
```bash
curl -X POST http://localhost:8080/api/mapping/remove \
  -H "Content-Type: application/json" \
  -d '{
    "port": 10001
  }'
```

**响应**:
```json
{
  "success": true,
  "message": "端口映射删除成功",
  "data": {
    "port": 10001
  }
}
```

### 列出所有映射

**请求**:
```bash
curl http://localhost:8080/api/mapping/list
```

**响应**:
```json
{
  "success": true,
  "message": "获取映射列表成功",
  "data": {
    "mappings": [
      {
        "id": 1,
        "source_port": 10001,
        "target_ip": "192.168.1.100",
        "target_port": 10001,
        "created_at": "2024-01-01 12:00:00"
      }
    ],
    "count": 1,
    "use_tunnel": false
  }
}
```

### 健康检查

**请求**:
```bash
curl http://localhost:8080/health
```

**响应**:
```json
{
  "status": "ok",
  "tunnel_enabled": true,
  "tunnel_connected": true
}
```

## 使用场景

### 场景 1: 直接端口转发（无隧道）

适用于服务器可以直接访问目标服务的情况。

1. 配置 `tunnel.enabled: false`
2. 启动服务器
3. 创建映射时指定 `target_ip`

```bash
curl -X POST http://localhost:8080/api/mapping/create \
  -H "Content-Type: application/json" \
  -d '{
    "port": 10022,
    "target_ip": "192.168.1.100"
  }'
```

### 场景 2: 内网穿透（隧道模式）

适用于目标服务在内网，服务器无法直接访问的情况。

1. 配置 `tunnel.enabled: true`
2. 启动服务器
3. 在内网机器上启动客户端
4. 创建映射（无需指定 target_ip）

```bash
curl -X POST http://localhost:8080/api/mapping/create \
  -H "Content-Type: application/json" \
  -d '{
    "port": 10022
  }'
```

## 性能优化

### 连接池配置

数据库连接池在 `db/database.go` 中配置：

```go
db.SetMaxOpenConns(25)  // 最大打开连接数
db.SetMaxIdleConns(5)   // 最大空闲连接数
```

### 缓冲区大小

数据转发缓冲区在 `forwarder/forwarder.go` 和 `tunnel/` 中配置：

```go
buffer := make([]byte, 32*1024)  // 32KB 缓冲区
```

### 并发控制

- 每个端口映射使用独立的 goroutine
- 每个连接使用独立的 goroutine 进行双向转发
- 使用 context 和 WaitGroup 进行优雅关闭

## 安全建议

1. **防火墙配置**: 限制 API 端口（8080）的访问
2. **端口范围**: 合理设置端口范围，避免与系统端口冲突
3. **认证**: 在生产环境建议为 API 添加认证机制
4. **TLS**: 建议为隧道连接添加 TLS 加密
5. **监控**: 部署监控系统，监控端口使用和连接状态

## 监控和日志

### 日志级别

系统使用标准日志记录，包括：
- 启动和关闭事件
- 端口映射的创建和删除
- 连接建立和断开
- 错误和警告信息

### 监控指标

建议监控以下指标：
- 活跃端口映射数量
- 活跃连接数
- 数据传输速率
- 错误率
- 隧道连接状态

## 故障排查

### 问题 1: 端口映射创建失败

**原因**: 端口已被占用或超出范围
**解决**: 检查端口是否在配置的范围内，使用 `netstat` 检查端口占用

### 问题 2: 隧道连接断开

**原因**: 网络不稳定或防火墙阻止
**解决**: 客户端会自动重连，检查网络连接和防火墙规则

### 问题 3: 数据传输缓慢

**原因**: 网络延迟或缓冲区配置不当
**解决**: 调整缓冲区大小，检查网络质量

## 开发和贡献

### 编译

```bash
# 编译服务器
go build -o server ./server

# 编译客户端
go build -o client ./client

# 交叉编译（Linux）
GOOS=linux GOARCH=amd64 go build -o server-linux ./server
GOOS=linux GOARCH=amd64 go build -o client-linux ./client
```

### 测试

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./server/db -v
```

## 许可证

MIT License

## 作者

端口转发和内网穿透系统

## 更新日志

### v1.0.0 (2024-01-01)
- 初始版本发布
- 支持端口转发
- 支持内网穿透
- HTTP API 管理
- SQLite 持久化存储