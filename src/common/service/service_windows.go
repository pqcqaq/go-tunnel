//go:build windows
// +build windows

package service

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type windowsService struct {
	config ServiceConfig
}

// newPlatformService 创建 Windows 服务实例
func newPlatformService(config ServiceConfig) (Service, error) {
	return &windowsService{config: config}, nil
}

func (s *windowsService) Install() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("连接服务管理器失败: %v", err)
	}
	defer m.Disconnect()

	// 检查服务是否已存在
	service, err := m.OpenService(s.config.Name)
	if err == nil {
		service.Close()
		return fmt.Errorf("服务 %s 已存在", s.config.Name)
	}

	// 构建完整的启动命令
	exePath := s.config.ExecPath
	args := s.config.Args

	// 创建服务配置
	config := mgr.Config{
		DisplayName:      s.config.DisplayName,
		Description:      s.config.Description,
		StartType:        mgr.StartAutomatic, // 自动启动
		ServiceStartName: "",                 // 使用 LocalSystem 账户
		// 依赖项（可选）
		Dependencies: []string{},
	}

	// 创建服务
	service, err = m.CreateService(s.config.Name, exePath, config, args...)
	if err != nil {
		return fmt.Errorf("创建服务失败: %v", err)
	}
	defer service.Close()

	// 设置服务恢复选项（失败后自动重启）
	err = setRecoveryOptions(service)
	if err != nil {
		// 不阻止安装，仅记录警告
		fmt.Printf("警告: 设置服务恢复选项失败: %v\n", err)
	}

	fmt.Printf("服务 %s 安装成功\n", s.config.DisplayName)
	fmt.Printf("可执行文件: %s\n", exePath)
	fmt.Printf("启动参数: %s\n", strings.Join(args, " "))
	fmt.Printf("工作目录: %s\n", s.config.WorkingDir)
	fmt.Println("\n使用以下命令管理服务:")
	fmt.Printf("  启动服务: sc start %s\n", s.config.Name)
	fmt.Printf("  停止服务: sc stop %s\n", s.config.Name)
	fmt.Printf("  查询状态: sc query %s\n", s.config.Name)
	fmt.Printf("  卸载服务: %s -reg uninstall\n", exePath)

	return nil
}

func (s *windowsService) Uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("连接服务管理器失败: %v", err)
	}
	defer m.Disconnect()

	service, err := m.OpenService(s.config.Name)
	if err != nil {
		return fmt.Errorf("打开服务失败: %v", err)
	}
	defer service.Close()

	// 先停止服务
	status, err := service.Query()
	if err != nil {
		return fmt.Errorf("查询服务状态失败: %v", err)
	}

	if status.State != svc.Stopped {
		fmt.Println("正在停止服务...")
		_, err = service.Control(svc.Stop)
		if err != nil {
			fmt.Printf("警告: 停止服务失败: %v\n", err)
		} else {
			// 等待服务停止
			for i := 0; i < 30; i++ {
				status, err = service.Query()
				if err != nil {
					break
				}
				if status.State == svc.Stopped {
					break
				}
				time.Sleep(time.Second)
			}
		}
	}

	// 删除服务
	err = service.Delete()
	if err != nil {
		return fmt.Errorf("删除服务失败: %v", err)
	}

	fmt.Printf("服务 %s 卸载成功\n", s.config.DisplayName)
	return nil
}

func (s *windowsService) Start() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("连接服务管理器失败: %v", err)
	}
	defer m.Disconnect()

	service, err := m.OpenService(s.config.Name)
	if err != nil {
		return fmt.Errorf("打开服务失败: %v", err)
	}
	defer service.Close()

	err = service.Start()
	if err != nil {
		return fmt.Errorf("启动服务失败: %v", err)
	}

	fmt.Printf("服务 %s 启动成功\n", s.config.DisplayName)
	return nil
}

func (s *windowsService) Stop() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("连接服务管理器失败: %v", err)
	}
	defer m.Disconnect()

	service, err := m.OpenService(s.config.Name)
	if err != nil {
		return fmt.Errorf("打开服务失败: %v", err)
	}
	defer service.Close()

	_, err = service.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("停止服务失败: %v", err)
	}

	fmt.Printf("服务 %s 停止成功\n", s.config.DisplayName)
	return nil
}

func (s *windowsService) Status() (string, error) {
	m, err := mgr.Connect()
	if err != nil {
		return "", fmt.Errorf("连接服务管理器失败: %v", err)
	}
	defer m.Disconnect()

	service, err := m.OpenService(s.config.Name)
	if err != nil {
		return "", fmt.Errorf("打开服务失败: %v", err)
	}
	defer service.Close()

	status, err := service.Query()
	if err != nil {
		return "", fmt.Errorf("查询服务状态失败: %v", err)
	}

	return stateToString(status.State), nil
}

func stateToString(state svc.State) string {
	switch state {
	case svc.Stopped:
		return "已停止"
	case svc.StartPending:
		return "正在启动"
	case svc.StopPending:
		return "正在停止"
	case svc.Running:
		return "运行中"
	case svc.ContinuePending:
		return "继续挂起"
	case svc.PausePending:
		return "暂停挂起"
	case svc.Paused:
		return "已暂停"
	default:
		return "未知"
	}
}

// setRecoveryOptions 设置服务失败恢复选项
func setRecoveryOptions(service *mgr.Service) error {
	// 第一次失败：1分钟后重启
	// 第二次失败：1分钟后重启
	// 后续失败：1分钟后重启
	// 重置失败计数：24小时
	actions := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
	}

	return service.SetRecoveryActions(actions, 24*60*60) // 24小时重置
}

// IsAdmin 检查是否具有管理员权限（Windows）
func IsAdmin() bool {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)

	token := windows.Token(0)
	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}

	return member
}

// RunAsService 以服务模式运行
func RunAsService(name string, handler svc.Handler) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("检查服务模式失败: %v", err)
	}

	if !isService {
		return fmt.Errorf("不是以服务模式运行")
	}

	return svc.Run(name, handler)
}

// IsService 检查是否以服务模式运行（Windows）
func IsService() bool {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return isService
}
