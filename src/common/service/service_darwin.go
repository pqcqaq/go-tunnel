//go:build darwin
// +build darwin

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

type darwinService struct {
	config ServiceConfig
}

// newPlatformService 创建 macOS 服务实例
func newPlatformService(config ServiceConfig) (Service, error) {
	return &darwinService{config: config}, nil
}

// IsAdmin 检查当前用户是否有管理员权限（macOS）
func IsAdmin() bool {
	return os.Geteuid() == 0
}

// IsService 检查是否以服务模式运行（macOS 上始终返回 false）
// macOS 的 launchd 服务不需要特殊的服务模式，直接以普通模式运行即可
func IsService() bool {
	return false
}

// WindowsServiceHandler 在 macOS 上的占位接口
type WindowsServiceHandler interface {
	Start() error
	Stop() error
}

// NewWindowsServiceWrapper 在 macOS 上不可用
func NewWindowsServiceWrapper(handler WindowsServiceHandler) interface{} {
	panic("NewWindowsServiceWrapper is not available on macOS")
}

// RunAsService 在 macOS 上不可用
func RunAsService(name string, handler interface{}) error {
	return fmt.Errorf("RunAsService is not available on macOS")
}

func (s *darwinService) Install() error {
	// 生成 launchd plist 文件
	plistContent, err := s.generatePlistFile()
	if err != nil {
		return fmt.Errorf("生成服务配置文件失败: %v", err)
	}

	// plist 文件路径
	plistFilePath := fmt.Sprintf("/Library/LaunchDaemons/%s.plist", s.config.Name)

	// 写入 plist 文件
	err = os.WriteFile(plistFilePath, []byte(plistContent), 0644)
	if err != nil {
		return fmt.Errorf("写入服务配置文件失败: %v", err)
	}

	// 设置文件所有者为 root
	cmd := exec.Command("chown", "root:wheel", plistFilePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("设置文件所有者失败: %v", err)
	}

	// 加载服务
	cmd = exec.Command("launchctl", "load", plistFilePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("加载服务失败: %v", err)
	}

	fmt.Printf("服务 %s 安装成功\n", s.config.DisplayName)
	fmt.Printf("服务文件: %s\n", plistFilePath)
	fmt.Printf("可执行文件: %s\n", s.config.ExecPath)
	fmt.Printf("启动参数: %s\n", strings.Join(s.config.Args, " "))
	fmt.Printf("工作目录: %s\n", s.config.WorkingDir)
	fmt.Println("\n使用以下命令管理服务:")
	fmt.Printf("  启动服务: sudo launchctl start %s\n", s.config.Name)
	fmt.Printf("  停止服务: sudo launchctl stop %s\n", s.config.Name)
	fmt.Printf("  查看服务: sudo launchctl list | grep %s\n", s.config.Name)
	fmt.Printf("  查看日志: sudo tail -f /var/log/%s.log\n", s.config.Name)
	fmt.Printf("  卸载服务: sudo %s -reg uninstall\n", s.config.ExecPath)

	return nil
}

func (s *darwinService) Uninstall() error {
	plistFilePath := fmt.Sprintf("/Library/LaunchDaemons/%s.plist", s.config.Name)

	// 停止服务
	cmd := exec.Command("launchctl", "stop", s.config.Name)
	if err := cmd.Run(); err != nil {
		fmt.Printf("警告: 停止服务失败: %v\n", err)
	}

	// 卸载服务
	cmd = exec.Command("launchctl", "unload", plistFilePath)
	if err := cmd.Run(); err != nil {
		fmt.Printf("警告: 卸载服务失败: %v\n", err)
	}

	// 删除 plist 文件
	err := os.Remove(plistFilePath)
	if err != nil {
		return fmt.Errorf("删除服务配置文件失败: %v", err)
	}

	fmt.Printf("服务 %s 已卸载\n", s.config.DisplayName)
	return nil
}

func (s *darwinService) Start() error {
	cmd := exec.Command("launchctl", "start", s.config.Name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("启动服务失败: %v, 输出: %s", err, string(output))
	}

	fmt.Printf("服务 %s 已启动\n", s.config.DisplayName)
	return nil
}

func (s *darwinService) Stop() error {
	cmd := exec.Command("launchctl", "stop", s.config.Name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("停止服务失败: %v, 输出: %s", err, string(output))
	}

	fmt.Printf("服务 %s 已停止\n", s.config.DisplayName)
	return nil
}

func (s *darwinService) Status() (string, error) {
	cmd := exec.Command("launchctl", "list", s.config.Name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 如果服务不存在或未加载
		if strings.Contains(string(output), "Could not find") {
			return "未安装", nil
		}
		return "", fmt.Errorf("查询服务状态失败: %v", err)
	}

	// 解析输出
	outputStr := string(output)
	if strings.Contains(outputStr, "\"PID\"") {
		// 提取 PID
		lines := strings.Split(outputStr, "\n")
		for _, line := range lines {
			if strings.Contains(line, "\"PID\"") {
				if strings.Contains(line, "= 0") || strings.Contains(line, "= -") {
					return "已停止", nil
				}
				return "运行中", nil
			}
		}
	}

	return "未知", nil
}

func (s *darwinService) generatePlistFile() (string, error) {
	// 构建命令行参数
	programArguments := []string{s.config.ExecPath}
	programArguments = append(programArguments, s.config.Args...)

	// 转换为 XML 格式的数组
	var argsXML strings.Builder
	for _, arg := range programArguments {
		argsXML.WriteString(fmt.Sprintf("\t\t<string>%s</string>\n", arg))
	}

	// plist 模板
	const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>{{.Name}}</string>
	<key>ProgramArguments</key>
	<array>
{{.ProgramArguments}}	</array>
	<key>WorkingDirectory</key>
	<string>{{.WorkingDir}}</string>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>/var/log/{{.Name}}.log</string>
	<key>StandardErrorPath</key>
	<string>/var/log/{{.Name}}.error.log</string>
</dict>
</plist>
`

	// 准备模板数据
	data := struct {
		Name             string
		ProgramArguments string
		WorkingDir       string
	}{
		Name:             s.config.Name,
		ProgramArguments: argsXML.String(),
		WorkingDir:       s.config.WorkingDir,
	}

	// 渲染模板
	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		return "", err
	}

	var result strings.Builder
	err = tmpl.Execute(&result, data)
	if err != nil {
		return "", err
	}

	return result.String(), nil
}

// GetLogPath 获取服务日志文件路径
func GetLogPath(serviceName string) string {
	return filepath.Join("/var/log", serviceName+".log")
}

// GetErrorLogPath 获取服务错误日志文件路径
func GetErrorLogPath(serviceName string) string {
	return filepath.Join("/var/log", serviceName+".error.log")
}
