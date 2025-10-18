//go:build !windows && !linux
// +build !windows,!linux

package service

import (
	"fmt"
	"runtime"
)

type unsupportedService struct {
	config ServiceConfig
}

// newPlatformService 不支持的平台
func newPlatformService(config ServiceConfig) (Service, error) {
	return nil, fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
}

// IsAdmin 不支持的平台
func IsAdmin() bool {
	return false
}

// IsService 检查是否以服务模式运行（不支持的平台上始终返回 false）
func IsService() bool {
	return false
}

// WindowsServiceHandler 在不支持的平台上的占位接口
type WindowsServiceHandler interface {
	Start() error
	Stop() error
}

// NewWindowsServiceWrapper 在不支持的平台上不可用
func NewWindowsServiceWrapper(handler WindowsServiceHandler) interface{} {
	panic("NewWindowsServiceWrapper is not available on this platform")
}

// RunAsService 在不支持的平台上不可用
func RunAsService(name string, handler interface{}) error {
	return fmt.Errorf("RunAsService is not available on this platform")
}
