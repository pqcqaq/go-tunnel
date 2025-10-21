# API 认证说明

## 概述

所有 API 接口（除了 `/health` 健康检查接口）都需要提供有效的 API 密钥才能访问。

## 配置 API 密钥

在配置文件 `config.yaml` 中设置 API 密钥：

```yaml
# HTTP API 配置
api:
  listen_port: 8080
  api_key: "your-secret-api-key-here"  # 修改为你的密钥
```

**安全建议：**
- 使用强密码生成器生成复杂的 API 密钥
- 定期更换 API 密钥
- 不要在公共代码库中提交包含真实密钥的配置文件

## 使用 API 密钥

### 方式 1: 通过 HTTP 请求头（推荐）

在请求头中添加 `X-API-Key`：

```bash
curl -X POST http://localhost:8080/api/mapping/create \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-secret-api-key-here" \
  -d '{
    "source_port": 30001,
    "target_host": "192.168.1.100",
    "target_port": 3306,
    "use_tunnel": false
  }'
```

### 方式 2: 通过 URL 查询参数

在 URL 中添加 `api_key` 参数：

```bash
curl -X POST "http://localhost:8080/api/mapping/create?api_key=your-secret-api-key-here" \
  -H "Content-Type: application/json" \
  -d '{
    "source_port": 30001,
    "target_host": "192.168.1.100",
    "target_port": 3306,
    "use_tunnel": false
  }'
```

### 使用 PowerShell

```powershell
$headers = @{
    "Content-Type" = "application/json"
    "X-API-Key" = "your-secret-api-key-here"
}

$body = @{
    source_port = 30001
    target_host = "192.168.1.100"
    target_port = 3306
    use_tunnel = $false
} | ConvertTo-Json

Invoke-RestMethod -Uri "http://localhost:8080/api/mapping/create" `
    -Method Post `
    -Headers $headers `
    -Body $body
```

### 使用 Python

```python
import requests

headers = {
    'Content-Type': 'application/json',
    'X-API-Key': 'your-secret-api-key-here'
}

data = {
    'source_port': 30001,
    'target_host': '192.168.1.100',
    'target_port': 3306,
    'use_tunnel': False
}

response = requests.post(
    'http://localhost:8080/api/mapping/create',
    headers=headers,
    json=data
)

print(response.json())
```

## 需要认证的 API 接口

以下接口都需要提供有效的 API 密钥：

- `POST /api/mapping/create` - 创建端口映射
- `POST /api/mapping/remove` - 删除端口映射
- `GET /api/mapping/list` - 列出所有映射
- `GET /api/stats/traffic` - 获取流量统计
- `GET /api/stats/monitor` - 流量监控页面
- `GET /admin` - 管理页面

## 不需要认证的接口

- `GET /health` - 健康检查接口（公开访问）

## 错误响应

如果 API 密钥无效或缺失，服务器将返回 401 状态码：

```json
{
  "success": false,
  "message": "无效的 API 密钥"
}
```

## 浏览器访问

对于需要通过浏览器访问的页面（如 `/admin` 和 `/api/stats/monitor`），可以在 URL 中添加 `api_key` 参数：

```
http://localhost:8080/admin?api_key=your-secret-api-key-here
http://localhost:8080/api/stats/monitor?api_key=your-secret-api-key-here
```
