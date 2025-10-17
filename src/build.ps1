# Go-Tunnel PowerShell 构建脚本
# 用于在 Windows 环境下构建项目

param(
    [Parameter(Position=0)]
    [string]$Command = "help",
    [string]$Config = "config.yaml",
    [string]$Server = "localhost:9000"
)

# 颜色输出函数
function Write-ColorOutput {
    param(
        [string]$Message,
        [string]$Color = "Green"
    )
    Write-Host $Message -ForegroundColor $Color
}

function Write-Error-Output {
    param([string]$Message)
    Write-Host $Message -ForegroundColor Red
}

function Write-Info-Output {
    param([string]$Message)
    Write-Host $Message -ForegroundColor Cyan
}

# 检查 Go 环境
function Test-GoEnvironment {
    try {
        $goVersion = go version 2>$null
        if ($LASTEXITCODE -ne 0) {
            Write-Error-Output "错误: 未找到 Go 环境,请先安装 Go"
            return $false
        }
        Write-Info-Output "Go 环境: $goVersion"
        return $true
    }
    catch {
        Write-Error-Output "错误: 无法检测 Go 环境"
        return $false
    }
}

# 安装依赖
function Install-Dependencies {
    Write-ColorOutput "正在安装依赖..."
    go mod download
    if ($LASTEXITCODE -ne 0) {
        Write-Error-Output "依赖下载失败"
        return $false
    }
    go mod tidy
    if ($LASTEXITCODE -ne 0) {
        Write-Error-Output "依赖整理失败"
        return $false
    }
    Write-ColorOutput "依赖安装完成"
    return $true
}

# 初始化目录
function Initialize-Directories {
    Write-ColorOutput "正在初始化项目目录..."
    $binDir = "..\bin"
    $dataDir = "..\bin\data"
    
    if (-not (Test-Path $binDir)) {
        New-Item -ItemType Directory -Path $binDir -Force | Out-Null
        Write-Info-Output "已创建目录: $binDir"
    }
    
    if (-not (Test-Path $dataDir)) {
        New-Item -ItemType Directory -Path $dataDir -Force | Out-Null
        Write-Info-Output "已创建目录: $dataDir"
    }
    
    Write-ColorOutput "目录初始化完成"
}

# 编译服务器
function Build-Server {
    Write-ColorOutput "正在编译服务器..."
    Initialize-Directories
    
    $serverExe = "..\bin\server.exe"
    go build -ldflags "-s -w" -o $serverExe .\server
    
    if ($LASTEXITCODE -ne 0) {
        Write-Error-Output "服务器编译失败"
        return $false
    }
    
    Write-ColorOutput "服务器编译完成: bin\server.exe"
    return $true
}

# 编译客户端
function Build-Client {
    Write-ColorOutput "正在编译客户端..."
    Initialize-Directories
    
    $clientExe = "..\bin\client.exe"
    $env:CGO_ENABLED = "0"
    go build -a -ldflags "-s -w -extldflags `"-static`"" -o $clientExe .\client
    
    if ($LASTEXITCODE -ne 0) {
        Write-Error-Output "客户端编译失败"
        return $false
    }
    
    Write-ColorOutput "客户端编译完成: bin\client.exe"
    return $true
}

# 编译所有程序
function Build-All {
    Write-ColorOutput "正在编译所有程序..."
    
    if (-not (Build-Server)) {
        return $false
    }
    
    if (-not (Build-Client)) {
        return $false
    }
    
    Write-ColorOutput "所有程序编译完成" "Green"
    return $true
}

# 运行服务器
function Run-Server {
    Write-ColorOutput "正在启动服务器..."
    
    if (-not (Build-Server)) {
        return
    }
    
    $serverExe = "..\bin\server.exe"
    if (Test-Path $serverExe) {
        Write-Info-Output "服务器启动中,配置文件: $Config"
        & $serverExe -config $Config
    }
    else {
        Write-Error-Output "服务器可执行文件不存在: $serverExe"
    }
}

# 运行客户端
function Run-Client {
    Write-ColorOutput "正在启动客户端..."
    
    if (-not (Build-Client)) {
        return
    }
    
    $clientExe = "..\bin\client.exe"
    if (Test-Path $clientExe) {
        Write-Info-Output "客户端启动中,服务器地址: $Server"
        & $clientExe -server $Server
    }
    else {
        Write-Error-Output "客户端可执行文件不存在: $clientExe"
    }
}

# 清理编译文件
function Clean-Build {
    Write-ColorOutput "正在清理编译文件..."
    
    $binDir = "..\bin"
    $dataDir = "data"
    
    if (Test-Path $binDir) {
        Remove-Item -Path $binDir -Recurse -Force
        Write-Info-Output "已删除: $binDir"
    }
    
    if (Test-Path $dataDir) {
        Remove-Item -Path $dataDir -Recurse -Force
        Write-Info-Output "已删除: $dataDir"
    }
    
    Write-ColorOutput "清理完成"
}

# 运行测试
function Run-Tests {
    Write-ColorOutput "正在运行测试..."
    go test -v ./...
    
    if ($LASTEXITCODE -ne 0) {
        Write-Error-Output "测试失败"
        return $false
    }
    
    Write-ColorOutput "测试通过"
    return $true
}

# 交叉编译 Linux 版本
function Build-Linux {
    Write-ColorOutput "正在交叉编译 Linux 版本..."
    Initialize-Directories
    
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    
    Write-Info-Output "编译 Linux 服务器..."
    go build -o ..\bin\server-linux .\server
    if ($LASTEXITCODE -ne 0) {
        Write-Error-Output "Linux 服务器编译失败"
        return $false
    }
    
    Write-Info-Output "编译 Linux 客户端..."
    go build -o ..\bin\client-linux .\client
    if ($LASTEXITCODE -ne 0) {
        Write-Error-Output "Linux 客户端编译失败"
        return $false
    }
    
    # 恢复环境变量
    Remove-Item Env:\GOOS
    Remove-Item Env:\GOARCH
    
    Write-ColorOutput "Linux 版本编译完成"
    return $true
}

# 格式化代码
function Format-Code {
    Write-ColorOutput "正在格式化代码..."
    go fmt ./...
    Write-ColorOutput "代码格式化完成"
}

# 代码检查
function Lint-Code {
    Write-ColorOutput "正在进行代码检查..."
    go vet ./...
    
    if ($LASTEXITCODE -ne 0) {
        Write-Error-Output "代码检查发现问题"
        return $false
    }
    
    Write-ColorOutput "代码检查通过"
    return $true
}

# 显示帮助信息
function Show-Help {
    Write-Host ""
    Write-ColorOutput "Go-Tunnel 构建脚本 - Windows PowerShell 版本" "Yellow"
    Write-Host ""
    Write-Host "使用方法:" -ForegroundColor Cyan
    Write-Host "  .\build.ps1 [命令] [选项]" -ForegroundColor White
    Write-Host ""
    Write-Host "可用命令:" -ForegroundColor Cyan
    Write-Host "  install-deps    - 安装依赖" -ForegroundColor White
    Write-Host "  build           - 编译所有程序 (默认)" -ForegroundColor White
    Write-Host "  build-server    - 仅编译服务器" -ForegroundColor White
    Write-Host "  build-client    - 仅编译客户端" -ForegroundColor White
    Write-Host "  run-server      - 运行服务器" -ForegroundColor White
    Write-Host "  run-client      - 运行客户端" -ForegroundColor White
    Write-Host "  clean           - 清理编译文件" -ForegroundColor White
    Write-Host "  test            - 运行测试" -ForegroundColor White
    Write-Host "  init            - 初始化项目目录" -ForegroundColor White
    Write-Host "  build-linux     - 交叉编译 Linux 版本" -ForegroundColor White
    Write-Host "  fmt             - 格式化代码" -ForegroundColor White
    Write-Host "  lint            - 代码检查" -ForegroundColor White
    Write-Host "  help            - 显示此帮助信息" -ForegroundColor White
    Write-Host ""
    Write-Host "选项:" -ForegroundColor Cyan
    Write-Host "  -Config <path>  - 指定配置文件路径 (默认: config.yaml)" -ForegroundColor White
    Write-Host "  -Server <addr>  - 指定服务器地址 (默认: localhost:9000)" -ForegroundColor White
    Write-Host ""
    Write-Host "示例:" -ForegroundColor Cyan
    Write-Host "  .\build.ps1 build" -ForegroundColor Gray
    Write-Host "  .\build.ps1 run-server -Config custom.yaml" -ForegroundColor Gray
    Write-Host "  .\build.ps1 run-client -Server 192.168.1.100:9000" -ForegroundColor Gray
    Write-Host ""
}

# 主函数
function Main {
    # 切换到脚本所在目录
    $scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
    if ($scriptDir) {
        Set-Location $scriptDir
    }
    
    Write-Host ""
    
    # 除了 help 命令外,其他命令都需要检查 Go 环境
    if ($Command -ne "help" -and -not (Test-GoEnvironment)) {
        exit 1
    }
    
    switch ($Command.ToLower()) {
        "install-deps" {
            if (-not (Install-Dependencies)) { exit 1 }
        }
        "build" {
            if (-not (Build-All)) { exit 1 }
        }
        "build-server" {
            if (-not (Build-Server)) { exit 1 }
        }
        "build-client" {
            if (-not (Build-Client)) { exit 1 }
        }
        "run-server" {
            Run-Server
        }
        "run-client" {
            Run-Client
        }
        "clean" {
            Clean-Build
        }
        "test" {
            if (-not (Run-Tests)) { exit 1 }
        }
        "init" {
            Initialize-Directories
        }
        "build-linux" {
            if (-not (Build-Linux)) { exit 1 }
        }
        "fmt" {
            Format-Code
        }
        "lint" {
            if (-not (Lint-Code)) { exit 1 }
        }
        "help" {
            Show-Help
        }
        default {
            Write-Error-Output "未知命令: $Command"
            Write-Host ""
            Show-Help
            exit 1
        }
    }
    
    Write-Host ""
}

# 执行主函数
Main
