# 服务注册和管理指南

## 概述

Go Tunnel 支持将 Server 和 Client 注册为系统服务，实现开机自启和自动重启功能。

- **Windows**: 使用 Windows Service
- **Linux**: 使用 systemd

## 功能特性

✅ **开机自启**: 系统启动时自动运行
✅ **自动重启**: 服务崩溃后自动重启（Windows: 60秒后重启，Linux: 10秒后重启）
✅ **优雅关闭**: 正确处理停止信号，清理资源
✅ **日志记录**: Windows 使用事件查看器，Linux 使用 journalctl
✅ **管理员权限**: 自动检查权限，确保安全安装

## 安装要求

### Windows
- 管理员权限（以管理员身份运行 PowerShell 或 CMD）
- Windows 7 或更高版本

### Linux
- root 权限（使用 sudo）
- systemd 支持（大多数现代 Linux 发行版）

---

## Server 服务管理

### Windows

#### 1. 安装服务

```powershell
# 以管理员身份运行
.\server.exe -reg install

# 指定配置文件
.\server.exe -config custom_config.yaml -reg install
```

#### 2. 启动服务

```powershell
# 方法 1: 使用程序命令
.\server.exe -reg start

# 方法 2: 使用 sc 命令
sc start GoTunnelServer

# 方法 3: 使用服务管理器（services.msc）
```

#### 3. 停止服务

```powershell
.\server.exe -reg stop
# 或
sc stop GoTunnelServer
```

#### 4. 查询服务状态

```powershell
.\server.exe -reg status
# 或
sc query GoTunnelServer
```

#### 5. 卸载服务

```powershell
.\server.exe -reg uninstall
```

#### 6. 查看日志

打开"事件查看器" → "Windows 日志" → "应用程序"，搜索 "GoTunnelServer"

### Linux

#### 1. 安装服务

```bash
# 以 root 身份运行
sudo ./server -reg install

# 指定配置文件
sudo ./server -config /etc/gotunnel/config.yaml -reg install
```

#### 2. 启动服务

```bash
# 方法 1: 使用程序命令
sudo ./server -reg start

# 方法 2: 使用 systemctl
sudo systemctl start GoTunnelServer
```

#### 3. 停止服务

```bash
sudo ./server -reg stop
# 或
sudo systemctl stop GoTunnelServer
```

#### 4. 查询服务状态

```bash
sudo ./server -reg status
# 或
sudo systemctl status GoTunnelServer
```

#### 5. 卸载服务

```bash
sudo ./server -reg uninstall
```

#### 6. 查看日志

```bash
# 实时查看日志
sudo journalctl -u GoTunnelServer -f

# 查看最近的日志
sudo journalctl -u GoTunnelServer -n 100

# 查看今天的日志
sudo journalctl -u GoTunnelServer --since today
```

#### 7. 设置开机自启（安装时自动启用）

```bash
sudo systemctl enable GoTunnelServer
```

---

## Client 服务管理

### Windows

#### 1. 安装服务

```powershell
# 以管理员身份运行
.\client.exe -reg install

# 指定服务器地址
.\client.exe -server example.com:9000 -reg install
```

#### 2. 启动服务

```powershell
.\client.exe -reg start
# 或
sc start GoTunnelClient
```

#### 3. 停止服务

```powershell
.\client.exe -reg stop
# 或
sc stop GoTunnelClient
```

#### 4. 查询服务状态

```powershell
.\client.exe -reg status
# 或
sc query GoTunnelClient
```

#### 5. 卸载服务

```powershell
.\client.exe -reg uninstall
```

### Linux

#### 1. 安装服务

```bash
sudo ./client -reg install

# 指定服务器地址
sudo ./client -server example.com:9000 -reg install
```

#### 2. 启动服务

```bash
sudo ./client -reg start
# 或
sudo systemctl start GoTunnelClient
```

#### 3. 停止服务

```bash
sudo ./client -reg stop
# 或
sudo systemctl stop GoTunnelClient
```

#### 4. 查询服务状态

```bash
sudo ./client -reg status
# 或
sudo systemctl status GoTunnelClient
```

#### 5. 卸载服务

```bash
sudo ./client -reg uninstall
```

#### 6. 查看日志

```bash
# 实时查看日志
sudo journalctl -u GoTunnelClient -f

# 查看最近的日志
sudo journalctl -u GoTunnelClient -n 100
```

---

## 自动重启配置

### Windows

服务失败时的恢复策略：
- **第一次失败**: 60秒后重启
- **第二次失败**: 60秒后重启
- **后续失败**: 60秒后重启
- **重置失败计数**: 24小时

可以通过服务管理器（services.msc）查看和修改恢复选项。

### Linux

systemd 服务配置（自动生成）：
```ini
[Service]
Restart=always          # 总是重启
RestartSec=10          # 10秒后重启
StartLimitBurst=5      # 60秒内最多重启5次
StartLimitInterval=60  # 限制间隔
```

修改重启策略：
```bash
# 编辑服务文件
sudo systemctl edit --full GoTunnelServer

# 修改后重新加载
sudo systemctl daemon-reload
sudo systemctl restart GoTunnelServer
```

---

## 故障排查

### Windows

#### 服务无法启动

1. 检查事件查看器中的错误日志
2. 确认可执行文件路径正确
3. 验证配置文件存在且格式正确
4. 检查端口是否被占用

```powershell
# 查看详细错误
sc query GoTunnelServer
sc qc GoTunnelServer

# 手动测试运行
.\server.exe -config config.yaml
```

#### 权限问题

确保以管理员身份运行：
```powershell
# 检查是否以管理员运行
[Security.Principal.WindowsPrincipal] $CurrentUser = [Security.Principal.WindowsIdentity]::GetCurrent()
$CurrentUser.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
```

### Linux

#### 服务无法启动

```bash
# 查看详细状态
sudo systemctl status GoTunnelServer -l

# 查看启动失败日志
sudo journalctl -u GoTunnelServer -xe

# 手动测试运行
sudo ./server -config config.yaml
```

#### 权限问题

```bash
# 检查文件权限
ls -l /etc/systemd/system/GoTunnelServer.service
ls -l ./server

# 确保可执行
sudo chmod +x ./server
```

#### 端口占用

```bash
# 检查端口使用情况
sudo netstat -tlnp | grep :8080
sudo lsof -i :8080
```

---

## 生产环境最佳实践

### 1. 日志管理

#### Windows
- 定期清理事件日志
- 考虑使用第三方日志聚合工具

#### Linux
```bash
# 限制日志大小（编辑 journald 配置）
sudo vim /etc/systemd/journald.conf

# 设置日志保留
SystemMaxUse=1G
SystemMaxFileSize=100M
MaxRetentionSec=30day

# 重启 journald
sudo systemctl restart systemd-journald
```

### 2. 监控和告警

#### Windows
使用 PowerShell 脚本监控服务状态：
```powershell
$service = Get-Service GoTunnelServer
if ($service.Status -ne 'Running') {
    # 发送告警
    Write-Host "Service is not running!"
}
```

#### Linux
使用 systemd 或第三方监控工具：
```bash
# 检查服务状态
systemctl is-active GoTunnelServer

# 配合监控工具（如 Prometheus + Node Exporter）
```

### 3. 配置备份

定期备份配置文件和数据库：
```bash
# Linux
sudo cp /path/to/config.yaml /backup/config.yaml.$(date +%Y%m%d)

# Windows
Copy-Item config.yaml backup\config.yaml.$(Get-Date -Format 'yyyyMMdd')
```

### 4. 安全建议

- ✅ 使用非 root/Administrator 用户运行（可选配置）
- ✅ 限制配置文件权限（仅管理员可读写）
- ✅ 启用防火墙规则
- ✅ 定期更新程序版本
- ✅ 监控异常连接和流量

### 5. 资源限制

#### Linux (systemd)
编辑服务文件添加资源限制：
```ini
[Service]
# 内存限制
MemoryLimit=512M
# 文件描述符限制
LimitNOFILE=65536
# CPU 使用率限制
CPUQuota=200%
```

---

## 常见命令速查表

| 操作 | Windows | Linux |
|------|---------|-------|
| 安装服务 | `.\server.exe -reg install` | `sudo ./server -reg install` |
| 启动服务 | `sc start GoTunnelServer` | `sudo systemctl start GoTunnelServer` |
| 停止服务 | `sc stop GoTunnelServer` | `sudo systemctl stop GoTunnelServer` |
| 重启服务 | `sc stop ... && sc start ...` | `sudo systemctl restart GoTunnelServer` |
| 查看状态 | `sc query GoTunnelServer` | `sudo systemctl status GoTunnelServer` |
| 查看日志 | 事件查看器 | `sudo journalctl -u GoTunnelServer -f` |
| 卸载服务 | `.\server.exe -reg uninstall` | `sudo ./server -reg uninstall` |
| 开机自启 | 自动启用 | `sudo systemctl enable GoTunnelServer` |

---

## 技术细节

### 自动重启机制

#### Windows
- 使用 Windows Service Recovery Options
- 服务崩溃时由 SCM（服务控制管理器）自动重启
- 支持配置延迟和重试次数

#### Linux
- 使用 systemd 的 `Restart=always` 配置
- 支持 `StartLimitBurst` 和 `StartLimitInterval` 防止重启循环
- 系统级别的进程管理和监控

### 优雅关闭

两个平台都实现了优雅关闭：
1. 接收停止信号（SIGTERM/SIGINT）
2. 停止接受新连接
3. 等待现有连接完成
4. 清理资源（关闭数据库、停止转发器）
5. 退出程序

---

## 更新说明

更新服务时的步骤：

### Windows
```powershell
# 1. 停止服务
sc stop GoTunnelServer

# 2. 替换可执行文件
Copy-Item new-server.exe server.exe -Force

# 3. 启动服务
sc start GoTunnelServer
```

### Linux
```bash
# 1. 停止服务
sudo systemctl stop GoTunnelServer

# 2. 替换可执行文件
sudo cp new-server /usr/local/bin/server

# 3. 启动服务
sudo systemctl start GoTunnelServer
```

---

## 支持和反馈

如有问题或建议，请提交 Issue 或 Pull Request。
