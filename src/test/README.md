# 集成测试文档

本目录包含 go-tunnel 项目的集成测试，用于测试服务器和客户端之间的端口转发功能。

## 前置条件

在运行测试之前，需要：

1. **启动服务器**
   ```bash
   cd src
   make run-server
   # 或者
   ./bin/server -config config.yaml
   ```

2. **启动客户端**
   ```bash
   cd src
   make run-client
   # 或者
   ./bin/client -server localhost:9000
   ```

## 测试配置

测试使用的默认配置如下（可以在 `integration_test.go` 中修改）：

```go
ServerAPIAddr:    "http://localhost:8080",  // 服务器 API 地址
ServerTunnelPort: 9000,                     // 服务器隧道端口
TestPort:         30001,                    // 测试使用的端口
LocalServicePort: 8888,                     // 本地 echo 服务端口
```

如果你的服务器使用不同的端口，请修改这些配置。

## 运行测试

### 运行所有测试

```bash
cd src/test
go test -v
```

### 运行特定测试

```bash
# 测试基本转发功能
go test -v -run TestForwardingBasic

# 测试多端口转发
go test -v -run TestMultipleForwards

# 测试动态添加/删除映射
go test -v -run TestAddAndRemoveMapping

# 测试并发请求
go test -v -run TestConcurrentRequests

# 测试映射列表
go test -v -run TestListMappings

# 测试健康检查
go test -v -run TestHealthCheck
```

## 测试说明

### 1. TestForwardingBasic - 基本转发功能测试

测试流程：
1. 在本地 8888 端口启动一个 echo 服务器
2. 通过 API 创建端口映射（30001 -> 8888）
3. 向服务器的 30001 端口发送消息
4. 验证能否通过隧道转发到客户端的 8888 端口并收到响应

**预期结果**：发送 "Hello, Tunnel!" 能收到 "ECHO: Hello, Tunnel!"

### 2. TestMultipleForwards - 多端口转发测试

测试流程：
1. 创建多个端口映射（30002, 30003, 30004）
2. 同时向这些端口发送请求
3. 验证所有端口都能正确转发

**预期结果**：所有端口都能正常转发并收到正确响应

### 3. TestAddAndRemoveMapping - 动态管理测试

测试流程：
1. 创建端口映射并验证工作
2. 删除端口映射并验证无法连接
3. 重新创建映射并验证恢复工作

**预期结果**：映射的创建、删除、重建都能正确工作

### 4. TestConcurrentRequests - 并发请求测试

测试流程：
1. 创建一个端口映射
2. 并发发送 10 个请求
3. 验证所有请求都能正确处理

**预期结果**：10 个并发请求全部成功

### 5. TestListMappings - 映射列表测试

测试流程：
1. 创建多个端口映射
2. 通过 API 获取映射列表
3. 验证所有创建的映射都在列表中

**预期结果**：API 返回完整的映射列表

### 6. TestHealthCheck - 健康检查测试

测试流程：
1. 向服务器发送健康检查请求

**预期结果**：返回 200 状态码

## 测试端口使用

测试使用以下端口，请确保这些端口未被占用：

- 8888: 本地 echo 测试服务
- 30001: TestForwardingBasic
- 30002-30004: TestMultipleForwards
- 30005: TestAddAndRemoveMapping
- 30006: TestConcurrentRequests
- 30010-30012: TestListMappings

## 自定义测试服务器地址

如果你的服务器运行在不同的地址，可以在测试代码中修改 `defaultConfig`：

```go
var defaultConfig = TestConfig{
	ServerAPIAddr:    "http://your-server:8080",  // 修改这里
	ServerTunnelPort: 9000,
	TestPort:         30001,
	LocalServicePort: 8888,
}
```

或者创建环境变量：

```bash
export TEST_SERVER_API="http://your-server:8080"
export TEST_SERVER_TUNNEL_PORT="9000"
```

## 故障排查

### 测试失败：连接被拒绝

**原因**：服务器或客户端未启动

**解决**：确保服务器和客户端都在运行

### 测试失败：隧道未连接

**原因**：客户端未成功连接到服务器

**解决**：
1. 检查客户端日志，确认连接成功
2. 检查服务器地址配置是否正确

### 测试失败：端口已被占用

**原因**：测试端口被其他程序占用

**解决**：
1. 使用 `netstat -tuln | grep <port>` 查看端口占用
2. 关闭占用端口的程序
3. 或在测试代码中修改端口号

### 测试超时

**原因**：网络延迟或服务响应慢

**解决**：
1. 增加测试中的 `time.Sleep` 时间
2. 检查服务器和客户端是否正常运行
3. 查看服务器和客户端日志

## 示例输出

成功运行测试的输出示例：

```
=== RUN   TestForwardingBasic
    integration_test.go:54: 启动本地测试服务...
    integration_test.go:493: Echo 服务器启动在端口 8888
    integration_test.go:63: 创建端口映射: 30001 -> localhost:8888
    integration_test.go:68: 端口映射创建成功
    integration_test.go:82: 发送测试消息: Hello, Tunnel!
    integration_test.go:95: ✓ 转发成功，收到响应: ECHO: Hello, Tunnel!
    integration_test.go:73: 清理端口映射...
--- PASS: TestForwardingBasic (1.52s)
=== RUN   TestHealthCheck
    integration_test.go:346: ✓ 健康检查成功: {"success":true,"message":"服务器运行正常"}
--- PASS: TestHealthCheck (0.01s)
PASS
ok      test    1.534s
```

## 扩展测试

你可以根据需要添加更多测试用例：

1. 测试不同大小的数据传输
2. 测试长时间连接
3. 测试异常情况处理
4. 测试性能和吞吐量
5. 测试错误恢复机制

## 注意事项

1. 测试会自动清理创建的端口映射，但如果测试中断可能需要手动清理
2. 每个测试都是独立的，可以单独运行
3. 测试内置了 echo 服务器，不需要额外准备测试服务
4. 建议在开发环境运行测试，避免影响生产环境
