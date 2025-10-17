# Go-Tunnel PowerShell �����ű�
# ������ Windows �����¹�����Ŀ

param(
    [Parameter(Position=0)]
    [string]$Command = "help",
    [string]$Config = "config.yaml",
    [string]$Server = "localhost:9000"
)

# ��ɫ�������
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

# ��� Go ����
function Test-GoEnvironment {
    try {
        $goVersion = go version 2>$null
        if ($LASTEXITCODE -ne 0) {
            Write-Error-Output "����: δ�ҵ� Go ����,���Ȱ�װ Go"
            return $false
        }
        Write-Info-Output "Go ����: $goVersion"
        return $true
    }
    catch {
        Write-Error-Output "����: �޷���� Go ����"
        return $false
    }
}

# ��װ����
function Install-Dependencies {
    Write-ColorOutput "���ڰ�װ����..."
    go mod download
    if ($LASTEXITCODE -ne 0) {
        Write-Error-Output "��������ʧ��"
        return $false
    }
    go mod tidy
    if ($LASTEXITCODE -ne 0) {
        Write-Error-Output "��������ʧ��"
        return $false
    }
    Write-ColorOutput "������װ���"
    return $true
}

# ��ʼ��Ŀ¼
function Initialize-Directories {
    Write-ColorOutput "���ڳ�ʼ����ĿĿ¼..."
    $binDir = "..\bin"
    $dataDir = "..\bin\data"
    
    if (-not (Test-Path $binDir)) {
        New-Item -ItemType Directory -Path $binDir -Force | Out-Null
        Write-Info-Output "�Ѵ���Ŀ¼: $binDir"
    }
    
    if (-not (Test-Path $dataDir)) {
        New-Item -ItemType Directory -Path $dataDir -Force | Out-Null
        Write-Info-Output "�Ѵ���Ŀ¼: $dataDir"
    }
    
    Write-ColorOutput "Ŀ¼��ʼ�����"
}

# ���������
function Build-Server {
    Write-ColorOutput "���ڱ��������..."
    Initialize-Directories
    
    $serverExe = "..\bin\server.exe"
    go build -ldflags "-s -w" -o $serverExe .\server
    
    if ($LASTEXITCODE -ne 0) {
        Write-Error-Output "����������ʧ��"
        return $false
    }
    
    Write-ColorOutput "�������������: bin\server.exe"
    return $true
}

# ����ͻ���
function Build-Client {
    Write-ColorOutput "���ڱ���ͻ���..."
    Initialize-Directories
    
    $clientExe = "..\bin\client.exe"
    $env:CGO_ENABLED = "0"
    go build -a -ldflags "-s -w -extldflags `"-static`"" -o $clientExe .\client
    
    if ($LASTEXITCODE -ne 0) {
        Write-Error-Output "�ͻ��˱���ʧ��"
        return $false
    }
    
    Write-ColorOutput "�ͻ��˱������: bin\client.exe"
    return $true
}

# �������г���
function Build-All {
    Write-ColorOutput "���ڱ������г���..."
    
    if (-not (Build-Server)) {
        return $false
    }
    
    if (-not (Build-Client)) {
        return $false
    }
    
    Write-ColorOutput "���г���������" "Green"
    return $true
}

# ���з�����
function Run-Server {
    Write-ColorOutput "��������������..."
    
    if (-not (Build-Server)) {
        return
    }
    
    $serverExe = "..\bin\server.exe"
    if (Test-Path $serverExe) {
        Write-Info-Output "������������,�����ļ�: $Config"
        & $serverExe -config $Config
    }
    else {
        Write-Error-Output "��������ִ���ļ�������: $serverExe"
    }
}

# ���пͻ���
function Run-Client {
    Write-ColorOutput "���������ͻ���..."
    
    if (-not (Build-Client)) {
        return
    }
    
    $clientExe = "..\bin\client.exe"
    if (Test-Path $clientExe) {
        Write-Info-Output "�ͻ���������,��������ַ: $Server"
        & $clientExe -server $Server
    }
    else {
        Write-Error-Output "�ͻ��˿�ִ���ļ�������: $clientExe"
    }
}

# ��������ļ�
function Clean-Build {
    Write-ColorOutput "������������ļ�..."
    
    $binDir = "..\bin"
    $dataDir = "data"
    
    if (Test-Path $binDir) {
        Remove-Item -Path $binDir -Recurse -Force
        Write-Info-Output "��ɾ��: $binDir"
    }
    
    if (Test-Path $dataDir) {
        Remove-Item -Path $dataDir -Recurse -Force
        Write-Info-Output "��ɾ��: $dataDir"
    }
    
    Write-ColorOutput "�������"
}

# ���в���
function Run-Tests {
    Write-ColorOutput "�������в���..."
    go test -v ./...
    
    if ($LASTEXITCODE -ne 0) {
        Write-Error-Output "����ʧ��"
        return $false
    }
    
    Write-ColorOutput "����ͨ��"
    return $true
}

# ������� Linux �汾
function Build-Linux {
    Write-ColorOutput "���ڽ������ Linux �汾..."
    Initialize-Directories
    
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    
    Write-Info-Output "���� Linux ������..."
    go build -o ..\bin\server-linux .\server
    if ($LASTEXITCODE -ne 0) {
        Write-Error-Output "Linux ����������ʧ��"
        return $false
    }
    
    Write-Info-Output "���� Linux �ͻ���..."
    go build -o ..\bin\client-linux .\client
    if ($LASTEXITCODE -ne 0) {
        Write-Error-Output "Linux �ͻ��˱���ʧ��"
        return $false
    }
    
    # �ָ���������
    Remove-Item Env:\GOOS
    Remove-Item Env:\GOARCH
    
    Write-ColorOutput "Linux �汾�������"
    return $true
}

# ��ʽ������
function Format-Code {
    Write-ColorOutput "���ڸ�ʽ������..."
    go fmt ./...
    Write-ColorOutput "�����ʽ�����"
}

# ������
function Lint-Code {
    Write-ColorOutput "���ڽ��д�����..."
    go vet ./...
    
    if ($LASTEXITCODE -ne 0) {
        Write-Error-Output "�����鷢������"
        return $false
    }
    
    Write-ColorOutput "������ͨ��"
    return $true
}

# ��ʾ������Ϣ
function Show-Help {
    Write-Host ""
    Write-ColorOutput "Go-Tunnel �����ű� - Windows PowerShell �汾" "Yellow"
    Write-Host ""
    Write-Host "ʹ�÷���:" -ForegroundColor Cyan
    Write-Host "  .\build.ps1 [����] [ѡ��]" -ForegroundColor White
    Write-Host ""
    Write-Host "��������:" -ForegroundColor Cyan
    Write-Host "  install-deps    - ��װ����" -ForegroundColor White
    Write-Host "  build           - �������г��� (Ĭ��)" -ForegroundColor White
    Write-Host "  build-server    - �����������" -ForegroundColor White
    Write-Host "  build-client    - ������ͻ���" -ForegroundColor White
    Write-Host "  run-server      - ���з�����" -ForegroundColor White
    Write-Host "  run-client      - ���пͻ���" -ForegroundColor White
    Write-Host "  clean           - ��������ļ�" -ForegroundColor White
    Write-Host "  test            - ���в���" -ForegroundColor White
    Write-Host "  init            - ��ʼ����ĿĿ¼" -ForegroundColor White
    Write-Host "  build-linux     - ������� Linux �汾" -ForegroundColor White
    Write-Host "  fmt             - ��ʽ������" -ForegroundColor White
    Write-Host "  lint            - ������" -ForegroundColor White
    Write-Host "  help            - ��ʾ�˰�����Ϣ" -ForegroundColor White
    Write-Host ""
    Write-Host "ѡ��:" -ForegroundColor Cyan
    Write-Host "  -Config <path>  - ָ�������ļ�·�� (Ĭ��: config.yaml)" -ForegroundColor White
    Write-Host "  -Server <addr>  - ָ����������ַ (Ĭ��: localhost:9000)" -ForegroundColor White
    Write-Host ""
    Write-Host "ʾ��:" -ForegroundColor Cyan
    Write-Host "  .\build.ps1 build" -ForegroundColor Gray
    Write-Host "  .\build.ps1 run-server -Config custom.yaml" -ForegroundColor Gray
    Write-Host "  .\build.ps1 run-client -Server 192.168.1.100:9000" -ForegroundColor Gray
    Write-Host ""
}

# ������
function Main {
    # �л����ű�����Ŀ¼
    $scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
    if ($scriptDir) {
        Set-Location $scriptDir
    }
    
    Write-Host ""
    
    # ���� help ������,���������Ҫ��� Go ����
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
            Write-Error-Output "δ֪����: $Command"
            Write-Host ""
            Show-Help
            exit 1
        }
    }
    
    Write-Host ""
}

# ִ��������
Main
