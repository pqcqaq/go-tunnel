package service

import (
	"fmt"
	"os"
	"path/filepath"
)

// ServiceConfig 服务配置
type ServiceConfig struct {
	Name        string   // 服务名称
	DisplayName string   // 服务显示名称
	Description string   // 服务描述
	ExecPath    string   // 可执行文件路径
	Args        []string // 启动参数
	WorkingDir  string   // 工作目录
}

// Service 服务接口
type Service interface {
	// Install 安装服务
	Install() error
	// Uninstall 卸载服务
	Uninstall() error
	// Start 启动服务
	Start() error
	// Stop 停止服务
	Stop() error
	// Status 查询服务状态
	Status() (string, error)
}

// NewService 创建服务实例
func NewService(config ServiceConfig) (Service, error) {
	// 如果没有指定可执行文件路径，使用当前程序路径
	if config.ExecPath == "" {
		execPath, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("获取可执行文件路径失败: %v", err)
		}
		config.ExecPath, err = filepath.Abs(execPath)
		if err != nil {
			return nil, fmt.Errorf("获取绝对路径失败: %v", err)
		}
	}

	// 如果没有指定工作目录，使用可执行文件所在目录
	if config.WorkingDir == "" {
		config.WorkingDir = filepath.Dir(config.ExecPath)
	}

	// 调用平台特定的实现（编译时自动选择）
	return newPlatformService(config)
}

// newPlatformService 创建平台特定的服务实例
// 该函数在 service_windows.go、service_linux.go、service_unsupported.go 中实现
// 编译时会根据 build tags 自动选择对应平台的实现

// IsAdmin 检查当前用户是否有管理员权限
// 该函数在 service_windows.go、service_linux.go、service_unsupported.go 中实现
// 编译时会根据 build tags 自动选择对应平台的实现
