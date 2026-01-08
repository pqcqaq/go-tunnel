package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"port-forward/common/service"
	"port-forward/server/api"
	"port-forward/server/config"
	"port-forward/server/db"
	"port-forward/server/forwarder"
	"port-forward/server/tunnel"
	"port-forward/server/utils"
	"strings"
	"syscall"
)

// serverService 服务实例
type serverService struct {
	configPath   string
	cfg          *config.Config
	database     *db.Database
	fwdManager   *forwarder.Manager
	tunnelServer *tunnel.Server
	apiHandler   *api.Handler
	sigChan      chan os.Signal
}

func (s *serverService) Start() error {
	log.Println("服务启动中...")

	// 加载配置
	log.Println("加载配置文件...")
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return fmt.Errorf("加载配置失败: %v", err)
	}
	s.cfg = cfg

	// 初始化数据库
	log.Println("初始化数据库...")
	database, err := db.New(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("初始化数据库失败: %v", err)
	}
	s.database = database

	// 创建转发器管理器
	log.Println("创建转发器管理器...")
	s.fwdManager = forwarder.NewManager()

	// 如果启用隧道，启动隧道服务器
	if cfg.Tunnel.Enabled {
		log.Println("启动隧道服务器...")
		s.tunnelServer = tunnel.NewServer(cfg.Tunnel.ListenPort)
		if err := s.tunnelServer.Start(); err != nil {
			return fmt.Errorf("启动隧道服务器失败: %v", err)
		}
	}

	// 从数据库加载现有映射并启动转发器
	log.Println("加载现有端口映射...")
	mappings, err := database.GetAllMappings()
	if err != nil {
		return fmt.Errorf("加载端口映射失败: %v", err)
	}

	for _, mapping := range mappings {
		used := utils.PortCheck(mapping.SourcePort)
		if used {
			return fmt.Errorf("端口 %d 已被占用", mapping.SourcePort)
		}

		var err error
		if mapping.UseTunnel {
			// 隧道模式：检查隧道服务器是否可用
			if !cfg.Tunnel.Enabled || s.tunnelServer == nil {
				log.Printf("警告: 端口 %d 需要隧道模式但隧道服务未启用，跳过", mapping.SourcePort)
				continue
			}
			err = s.fwdManager.AddTunnel(mapping.SourcePort, mapping.TargetHost, mapping.TargetPort, s.tunnelServer, mapping.BandwidthLimit)
		} else {
			// 直接模式
			err = s.fwdManager.Add(mapping.SourcePort, mapping.TargetHost, mapping.TargetPort, mapping.BandwidthLimit)
		}

		if err != nil {
			log.Printf("警告: 启动端口 %d 的转发失败: %v", mapping.SourcePort, err)
		}
	}

	log.Printf("成功加载 %d 个端口映射", len(mappings))

	// 创建 HTTP API 处理器
	log.Println("初始化 HTTP API...")
	s.apiHandler = api.NewHandler(database, s.fwdManager, s.tunnelServer, cfg.API.APIKey)

	// 启动 HTTP API 服务器
	go func() {
		if err := api.Start(s.apiHandler, cfg.API.ListenPort); err != nil {
			log.Fatalf("启动 HTTP API 服务失败: %v", err)
		}
	}()

	log.Println("===========================================")
	log.Printf("服务器启动成功!")
	log.Printf("HTTP API: http://localhost:%d", cfg.API.ListenPort)
	if cfg.Tunnel.Enabled {
		log.Printf("隧道服务: 端口 %d", cfg.Tunnel.ListenPort)
	}
	log.Println("===========================================")

	// 等待中断信号
	s.sigChan = make(chan os.Signal, 1)
	signal.Notify(s.sigChan, os.Interrupt, syscall.SIGTERM)
	<-s.sigChan

	return nil
}

func (s *serverService) Stop() error {
	log.Println("接收到关闭信号，正在优雅关闭...")

	// 停止所有转发器
	if s.fwdManager != nil {
		log.Println("停止所有端口转发...")
		s.fwdManager.StopAll()
	}

	// 停止隧道服务器
	if s.tunnelServer != nil {
		log.Println("停止隧道服务器...")
		s.tunnelServer.Stop()
	}

	// 关闭数据库
	if s.database != nil {
		log.Println("关闭数据库...")
		s.database.Close()
	}

	log.Println("服务器已关闭")
	return nil
}

func main() {
	// 解析命令行参数
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	regAction := flag.String("reg", "", "服务管理: install(安装), uninstall(卸载), start(启动), stop(停止), status(状态)")
	flag.Parse()

	// 设置日志格式
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// 处理服务注册相关操作
	if *regAction != "" {
		handleServiceAction(*regAction, *configPath)
		return
	}

	// 检查是否以服务模式运行（编译时根据平台自动选择实现）
	if service.IsService() {
		// 以服务模式运行
		runAsService(*configPath)
		return
	}

	// 正常运行模式
	normalRun(*configPath)
}

func normalRun(configPath string) {
	svc := &serverService{
		configPath: configPath,
	}

	if err := svc.Start(); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}

	svc.Stop()
}

func runAsService(configPath string) {
	svc := &serverService{
		configPath: configPath,
	}

	// 创建服务包装器
	wrapper := service.NewWindowsServiceWrapper(svc)

	// 运行服务
	err := service.RunAsService("GoTunnelServer", wrapper)
	if err != nil {
		log.Fatalf("运行服务失败: %v", err)
	}
}

func handleServiceAction(action, configPath string) {
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
		Name:        "GoTunnelServer",
		DisplayName: "Go Tunnel Server",
		Description: "Go Tunnel 端口转发服务器 - 提供端口转发和隧道服务",
		ExecPath:    exePath,
		Args:        buildServiceArgs(configPath),
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

func buildServiceArgs(configPath string) []string {
	args := []string{}

	// 只添加非默认的配置路径
	if configPath != "" && configPath != "config.yaml" {
		args = append(args, "-config", configPath)
	}

	return args
}
