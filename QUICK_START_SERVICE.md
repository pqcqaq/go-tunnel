# 服务注册快速开始

## 快速测试（非管理员权限）

### 1. 正常运行模式（不注册服务）

```powershell
# Server
cd bin
.\server.exe -config config.yaml

# Client
.\client.exe -server localhost:9000
```

## 注册为 Windows 服务（需要管理员权限）

### Server 服务

#### 步骤 1: 以管理员身份打开 PowerShell

右键点击 PowerShell → "以管理员身份运行"

#### 步骤 2: 进入程序目录

```powershell
cd D:\Develop\Projects\go-tunnel\bin
```

#### 步骤 3: 安装服务

```powershell
.\server.exe -reg install
```

输出示例：
```
服务 Go Tunnel Server 安装成功
可执行文件: D:\Develop\Projects\go-tunnel\bin\server.exe
启动参数:
工作目录: D:\Develop\Projects\go-tunnel\bin

使用以下命令管理服务:
  启动服务: sc start GoTunnelServer
  停止服务: sc stop GoTunnelServer
  查询状态: sc query GoTunnelServer
  卸载服务: D:\Develop\Projects\go-tunnel\bin\server.exe -reg uninstall
```

#### 步骤 4: 启动服务

```powershell
sc start GoTunnelServer
```

或者使用：
```powershell
.\server.exe -reg start
```

#### 步骤 5: 检查服务状态

```powershell
sc query GoTunnelServer
```

输出示例：
```
SERVICE_NAME: GoTunnelServer
        TYPE               : 10  WIN32_OWN_PROCESS
        STATE              : 4  RUNNING
        ...
```

#### 步骤 6: 查看服务日志

打开"事件查看器"：
1. 按 `Win + R`，输入 `eventvwr.msc`
2. 展开 "Windows 日志" → "应用程序"
3. 在右侧点击"筛选当前日志"
4. 在事件源中搜索相关日志

或者查看 API 接口：
```powershell
# 浏览器访问
http://localhost:8080
```

### Client 服务

#### 安装并启动

```powershell
# 以管理员身份运行
cd D:\Develop\Projects\go-tunnel\bin

# 安装服务
.\client.exe -server your-server.com:9000 -reg install

# 启动服务
sc start GoTunnelClient

# 查看状态
sc query GoTunnelClient
```

## 管理已安装的服务

### 启动服务

```powershell
# Server
sc start GoTunnelServer

# Client
sc start GoTunnelClient
```

### 停止服务

```powershell
# Server
sc stop GoTunnelServer

# Client
sc stop GoTunnelClient
```

### 重启服务

```powershell
# Server
sc stop GoTunnelServer
sc start GoTunnelServer

# Client
sc stop GoTunnelClient
sc start GoTunnelClient
```

### 查看服务状态

```powershell
# Server
sc query GoTunnelServer

# Client  
sc query GoTunnelClient
```

### 卸载服务

```powershell
# Server
.\server.exe -reg uninstall

# Client
.\client.exe -reg uninstall
```

## 使用服务管理器（图形界面）

### 打开服务管理器

1. 按 `Win + R`
2. 输入 `services.msc`
3. 按回车

### 在服务列表中找到服务

- **Go Tunnel Server** (GoTunnelServer)
- **Go Tunnel Client** (GoTunnelClient)

### 可用操作

- 右键点击服务
- 选择：启动、停止、重新启动、属性

### 查看和修改恢复选项

1. 右键点击服务 → "属性"
2. 切换到"恢复"选项卡
3. 可以看到和修改失败后的恢复策略：
   - 第一次失败：重新启动服务（1分钟后）
   - 第二次失败：重新启动服务（1分钟后）
   - 后续失败：重新启动服务（1分钟后）

## 故障排查

### 问题 1: 服务无法启动

#### 检查权限

```powershell
# 确认是否以管理员运行
[Security.Principal.WindowsPrincipal] $user = [Security.Principal.WindowsIdentity]::GetCurrent()
$user.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
# 应该返回 True
```

#### 手动测试运行

```powershell
# 以普通模式运行，查看错误信息
.\server.exe -config config.yaml
```

#### 检查端口占用

```powershell
# 检查 API 端口（默认 8080）
netstat -ano | findstr :8080

# 检查隧道端口（默认 9000）
netstat -ano | findstr :9000
```

### 问题 2: 服务启动但无法访问

#### 检查防火墙

```powershell
# 添加防火墙规则
New-NetFirewallRule -DisplayName "Go Tunnel Server" -Direction Inbound -Protocol TCP -LocalPort 8080 -Action Allow
New-NetFirewallRule -DisplayName "Go Tunnel Server Tunnel" -Direction Inbound -Protocol TCP -LocalPort 9000 -Action Allow
```

#### 测试 API 是否可访问

```powershell
# 使用 curl 测试
curl http://localhost:8080

# 或使用浏览器访问
start http://localhost:8080
```

### 问题 3: 配置文件找不到

服务会在工作目录查找配置文件。确保：

1. `config.yaml` 在可执行文件同一目录
2. 或者安装时指定配置文件路径：

```powershell
.\server.exe -config "C:\full\path\to\config.yaml" -reg install
```

### 问题 4: 数据库权限问题

确保服务有权限访问数据库文件：

```powershell
# 检查数据库文件权限
Get-Acl data\tunnel.db | Format-List

# 如果需要，添加 SYSTEM 账户的权限
$acl = Get-Acl data\tunnel.db
$permission = "NT AUTHORITY\SYSTEM","FullControl","Allow"
$accessRule = New-Object System.Security.AccessControl.FileSystemAccessRule $permission
$acl.SetAccessRule($accessRule)
Set-Acl data\tunnel.db $acl
```

## 测试自动重启

### 模拟服务崩溃

1. 找到服务进程 ID：

```powershell
Get-Process -Name server | Select-Object Id,ProcessName
```

2. 强制结束进程：

```powershell
Stop-Process -Id <进程ID> -Force
```

3. 等待约 60 秒

4. 检查服务状态：

```powershell
sc query GoTunnelServer
```

服务应该会自动重启，状态显示为 RUNNING。

## 完整示例脚本

### 安装和启动 Server 服务

```powershell
# install_server.ps1
# 需要以管理员身份运行

$ErrorActionPreference = "Stop"

Write-Host "检查管理员权限..."
$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Error "请以管理员身份运行此脚本"
    exit 1
}

Write-Host "进入程序目录..."
cd $PSScriptRoot

Write-Host "停止并卸载旧服务（如果存在）..."
sc stop GoTunnelServer 2>$null
.\server.exe -reg uninstall 2>$null

Write-Host "安装新服务..."
.\server.exe -reg install

Write-Host "启动服务..."
sc start GoTunnelServer

Write-Host "等待服务启动..."
Start-Sleep -Seconds 3

Write-Host "检查服务状态..."
sc query GoTunnelServer

Write-Host "`n服务安装完成！"
Write-Host "API 地址: http://localhost:8080"
Write-Host "查看日志: eventvwr.msc"
```

### 卸载服务

```powershell
# uninstall_server.ps1
# 需要以管理员身份运行

$ErrorActionPreference = "Stop"

Write-Host "停止服务..."
sc stop GoTunnelServer

Write-Host "等待服务停止..."
Start-Sleep -Seconds 2

Write-Host "卸载服务..."
.\server.exe -reg uninstall

Write-Host "服务已卸载"
```

## 监控脚本

### 检查服务运行状态

```powershell
# monitor_service.ps1

$serviceName = "GoTunnelServer"
$service = Get-Service -Name $serviceName -ErrorAction SilentlyContinue

if ($null -eq $service) {
    Write-Host "❌ 服务未安装: $serviceName"
    exit 1
}

if ($service.Status -eq 'Running') {
    Write-Host "✅ 服务运行正常: $serviceName"
    
    # 测试 API 访问
    try {
        $response = Invoke-WebRequest -Uri "http://localhost:8080" -TimeoutSec 5
        Write-Host "✅ API 访问正常"
    } catch {
        Write-Host "⚠️  API 访问失败: $_"
    }
} else {
    Write-Host "❌ 服务未运行: $serviceName (状态: $($service.Status))"
    
    # 尝试启动服务
    Write-Host "尝试启动服务..."
    Start-Service -Name $serviceName
    Start-Sleep -Seconds 3
    
    $service.Refresh()
    if ($service.Status -eq 'Running') {
        Write-Host "✅ 服务启动成功"
    } else {
        Write-Host "❌ 服务启动失败"
        exit 1
    }
}
```

可以将监控脚本添加到任务计划程序，定期执行检查。

---

## 下一步

- 📖 查看完整文档：[SERVICE_GUIDE.md](SERVICE_GUIDE.md)
- 🔧 配置文件说明：[README.md](README.md)
- 🐛 问题反馈：提交 Issue
