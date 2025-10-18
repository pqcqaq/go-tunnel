//go:build windows
// +build windows

package service

import (
	"log"

	"golang.org/x/sys/windows/svc"
)

// WindowsServiceHandler Windows 服务处理器接口
type WindowsServiceHandler interface {
	// Start 启动服务逻辑
	Start() error
	// Stop 停止服务逻辑
	Stop() error
}

// windowsServiceWrapper Windows 服务包装器
type windowsServiceWrapper struct {
	handler WindowsServiceHandler
}

// NewWindowsServiceWrapper 创建 Windows 服务包装器
func NewWindowsServiceWrapper(handler WindowsServiceHandler) svc.Handler {
	return &windowsServiceWrapper{handler: handler}
}

// Execute 实现 svc.Handler 接口
func (w *windowsServiceWrapper) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	// 通知服务控制管理器服务正在启动
	changes <- svc.Status{State: svc.StartPending}

	// 启动服务
	err := w.handler.Start()
	if err != nil {
		log.Printf("启动服务失败: %v", err)
		return true, 1
	}

	// 通知服务控制管理器服务已启动
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	log.Println("服务已启动，等待控制命令...")

loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				// 服务状态查询
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				// 收到停止命令
				log.Println("收到停止命令，正在关闭服务...")
				changes <- svc.Status{State: svc.StopPending}

				// 停止服务
				err := w.handler.Stop()
				if err != nil {
					log.Printf("停止服务失败: %v", err)
				}

				// 通知服务已停止
				break loop
			default:
				log.Printf("收到未知控制命令: %v", c.Cmd)
			}
		}
	}

	return false, 0
}
