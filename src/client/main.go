package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"port-forward/client/tunnel"
	"port-forward/common/service"
	"strings"
	"syscall"
)

// clientService 客户端服务实例
type clientService struct {
	serverAddr string
	client     *tunnel.Client
	sigChan    chan os.Signal
}

func (s *clientService) Start() error {
	log.Println("客户端服务启动中...")

	// 创建隧道客户端
	log.Printf("隧道客户端启动...")
	log.Printf("服务器地址: %s", s.serverAddr)

	s.client = tunnel.NewClient(s.serverAddr)

	// 启动客户端
	if err := s.client.Start(); err != nil {
		return fmt.Errorf("启动隧道客户端失败: %v", err)
	}

	log.Println("===========================================")
	log.Println("隧道客户端运行中...")
	log.Println("===========================================")

	// 等待中断信号
	s.sigChan = make(chan os.Signal, 1)
	signal.Notify(s.sigChan, os.Interrupt, syscall.SIGTERM)
	<-s.sigChan

	return nil
}

func (s *clientService) Stop() error {
	log.Println("接收到关闭信号，正在关闭...")

	// 停止客户端
	if s.client != nil {
		if err := s.client.Stop(); err != nil {
			log.Printf("停止客户端失败: %v", err)
			return err
		}
	}

	log.Println("客户端已关闭")
	return nil
}

func main() {
	// 解析命令行参数
	serverAddr := flag.String("server", "localhost:9000", "隧道服务器地址 (host:port)")
	regAction := flag.String("reg", "", "服务管理: install(安装), uninstall(卸载), start(启动), stop(停止), status(状态)")
	flag.Parse()

	// 设置日志格式
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// 处理服务注册相关操作
	if *regAction != "" {
		handleServiceAction(*regAction, *serverAddr)
		return
	}

	// 检查是否以服务模式运行（编译时根据平台自动选择实现）
	if service.IsService() {
		// 以服务模式运行
		runAsService(*serverAddr)
		return
	}

	// 正常运行模式
	normalRun(*serverAddr)
}

func normalRun(serverAddr string) {
	svc := &clientService{
		serverAddr: serverAddr,
	}

	if err := svc.Start(); err != nil {
		log.Fatalf("客户端启动失败: %v", err)
	}

	svc.Stop()
}

func runAsService(serverAddr string) {
	svc := &clientService{
		serverAddr: serverAddr,
	}

	// 创建服务包装器
	wrapper := service.NewWindowsServiceWrapper(svc)

	// 运行服务
	err := service.RunAsService("GoTunnelClient", wrapper)
	if err != nil {
		log.Fatalf("运行服务失败: %v", err)
	}
}

func handleServiceAction(action, serverAddr string) {
	// 检查管理员权限
	if !service.IsAdmin() {
		log.Fatal("需要管理员/root权限来执行此操作")
	}

	// 获取可执行文件路径
	exePath, err := os.Executable()
	if err != nil {
		log.Fatalf("获取可执行文件路径失败: %v", err)
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		log.Fatalf("获取绝对路径失败: %v", err)
	}

	// 准备服务配置
	cfg := service.ServiceConfig{
		Name:        "GoTunnelClient",
		DisplayName: "Go Tunnel Client",
		Description: "Go Tunnel 隧道客户端 - 连接到隧道服务器并提供内网穿透服务",
		ExecPath:    exePath,
		Args:        buildServiceArgs(serverAddr),
		WorkingDir:  filepath.Dir(exePath),
	}

	// 创建服务实例
	svcInstance, err := service.NewService(cfg)
	if err != nil {
		log.Fatalf("创建服务实例失败: %v", err)
	}

	// 执行操作
	switch strings.ToLower(action) {
	case "install":
		err = svcInstance.Install()
	case "uninstall":
		err = svcInstance.Uninstall()
	case "start":
		err = svcInstance.Start()
	case "stop":
		err = svcInstance.Stop()
	case "status":
		status, err := svcInstance.Status()
		if err != nil {
			log.Fatalf("查询服务状态失败: %v", err)
		}
		fmt.Printf("服务状态: %s\n", status)
		return
	default:
		log.Fatalf("未知操作: %s。支持的操作: install, uninstall, start, stop, status", action)
	}

	if err != nil {
		log.Fatalf("执行操作 %s 失败: %v", action, err)
	}
}

func buildServiceArgs(serverAddr string) []string {
	args := []string{}

	// 只添加非默认的服务器地址
	if serverAddr != "" && serverAddr != "localhost:9000" {
		args = append(args, "-server", serverAddr)
	}

	return args
}
