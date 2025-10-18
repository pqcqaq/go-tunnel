//go:build linux
// +build linux

package service

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

type linuxService struct {
	config ServiceConfig
}

// newPlatformService 创建 Linux 服务实例
func newPlatformService(config ServiceConfig) (Service, error) {
	return &linuxService{config: config}, nil
}

// IsAdmin 检查当前用户是否有管理员权限（Linux）
func IsAdmin() bool {
	return os.Geteuid() == 0
}

// IsService 检查是否以服务模式运行（Linux 上始终返回 false）
// Linux 的 systemd 服务不需要特殊的服务模式，直接以普通模式运行即可
func IsService() bool {
	return false
}

// WindowsServiceHandler 在 Linux 上的占位接口
type WindowsServiceHandler interface {
	Start() error
	Stop() error
}

// NewWindowsServiceWrapper 在 Linux 上不可用
func NewWindowsServiceWrapper(handler WindowsServiceHandler) interface{} {
	panic("NewWindowsServiceWrapper is not available on Linux")
}

// RunAsService 在 Linux 上不可用
func RunAsService(name string, handler interface{}) error {
	return fmt.Errorf("RunAsService is not available on Linux")
}

func (s *linuxService) Install() error {
	// 生成 systemd service 文件
	serviceContent, err := s.generateServiceFile()
	if err != nil {
		return fmt.Errorf("生成服务配置文件失败: %v", err)
	}

	// 服务文件路径
	serviceFilePath := fmt.Sprintf("/etc/systemd/system/%s.service", s.config.Name)

	// 写入服务文件
	err = os.WriteFile(serviceFilePath, []byte(serviceContent), 0644)
	if err != nil {
		return fmt.Errorf("写入服务配置文件失败: %v", err)
	}

	// 重新加载 systemd
	cmd := exec.Command("systemctl", "daemon-reload")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("重新加载 systemd 失败: %v", err)
	}

	// 启用服务（开机自启）
	cmd = exec.Command("systemctl", "enable", s.config.Name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("启用服务失败: %v", err)
	}

	fmt.Printf("服务 %s 安装成功\n", s.config.DisplayName)
	fmt.Printf("服务文件: %s\n", serviceFilePath)
	fmt.Printf("可执行文件: %s\n", s.config.ExecPath)
	fmt.Printf("启动参数: %s\n", strings.Join(s.config.Args, " "))
	fmt.Printf("工作目录: %s\n", s.config.WorkingDir)
	fmt.Println("\n使用以下命令管理服务:")
	fmt.Printf("  启动服务: sudo systemctl start %s\n", s.config.Name)
	fmt.Printf("  停止服务: sudo systemctl stop %s\n", s.config.Name)
	fmt.Printf("  查看状态: sudo systemctl status %s\n", s.config.Name)
	fmt.Printf("  查看日志: sudo journalctl -u %s -f\n", s.config.Name)
	fmt.Printf("  卸载服务: sudo %s -reg uninstall\n", s.config.ExecPath)

	return nil
}

func (s *linuxService) Uninstall() error {
	// 停止服务
	cmd := exec.Command("systemctl", "stop", s.config.Name)
	if err := cmd.Run(); err != nil {
		fmt.Printf("警告: 停止服务失败: %v\n", err)
	}

	// 禁用服务
	cmd = exec.Command("systemctl", "disable", s.config.Name)
	if err := cmd.Run(); err != nil {
		fmt.Printf("警告: 禁用服务失败: %v\n", err)
	}

	// 删除服务文件
	serviceFilePath := fmt.Sprintf("/etc/systemd/system/%s.service", s.config.Name)
	err := os.Remove(serviceFilePath)
	if err != nil {
		return fmt.Errorf("删除服务配置文件失败: %v", err)
	}

	// 重新加载 systemd
	cmd = exec.Command("systemctl", "daemon-reload")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("重新加载 systemd 失败: %v", err)
	}

	// 重置失败状态
	cmd = exec.Command("systemctl", "reset-failed")
	cmd.Run() // 忽略错误

	fmt.Printf("服务 %s 卸载成功\n", s.config.DisplayName)
	return nil
}

func (s *linuxService) Start() error {
	cmd := exec.Command("systemctl", "start", s.config.Name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("启动服务失败: %v", err)
	}

	fmt.Printf("服务 %s 启动成功\n", s.config.DisplayName)
	return nil
}

func (s *linuxService) Stop() error {
	cmd := exec.Command("systemctl", "stop", s.config.Name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("停止服务失败: %v", err)
	}

	fmt.Printf("服务 %s 停止成功\n", s.config.DisplayName)
	return nil
}

func (s *linuxService) Status() (string, error) {
	cmd := exec.Command("systemctl", "is-active", s.config.Name)
	output, err := cmd.Output()
	if err != nil {
		// 即使服务不活跃也会返回错误，所以检查输出
		status := strings.TrimSpace(string(output))
		if status != "" {
			return status, nil
		}
		return "", fmt.Errorf("查询服务状态失败: %v", err)
	}

	return strings.TrimSpace(string(output)), nil
}

func (s *linuxService) generateServiceFile() (string, error) {
	// systemd service 文件模板
	tmpl := `[Unit]
Description={{.Description}}
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory={{.WorkingDir}}
ExecStart={{.ExecStart}}
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

# 安全设置
LimitNOFILE=65536
LimitNPROC=65536

# 性能和资源限制
TimeoutStartSec=0
TimeoutStopSec=30

# 失败处理
StartLimitInterval=60
StartLimitBurst=5

[Install]
WantedBy=multi-user.target
`

	// 构建完整的启动命令
	execStart := s.config.ExecPath
	if len(s.config.Args) > 0 {
		execStart += " " + strings.Join(s.config.Args, " ")
	}

	// 准备模板数据
	data := map[string]string{
		"Description": s.config.Description,
		"WorkingDir":  s.config.WorkingDir,
		"ExecStart":   execStart,
	}

	// 解析并执行模板
	t, err := template.New("service").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	err = t.Execute(&buf, data)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
