package main

import (
	"context"
	"flag"
	"log"
	_ "net/http/pprof" // 导入pprof用于性能分析
	"os"
	"os/signal"
	"port-forward/server/api"
	"port-forward/server/config"
	"port-forward/server/db"
	"port-forward/server/forwarder"
	"port-forward/server/tunnel"
	"port-forward/server/utils"
	"syscall"
	"time"
)

func main() {
	// 解析命令行参数
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	// 加载配置
	log.Println("加载配置文件...")
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化数据库
	log.Println("初始化数据库...")
	database, err := db.New(cfg.Database.Path)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	defer database.Close()

	// 创建转发器管理器
	log.Println("创建转发器管理器...")
	fwdManager := forwarder.NewManager()

	// 如果启用隧道，启动隧道服务器
	var tunnelServer *tunnel.Server
	if cfg.Tunnel.Enabled {
		log.Println("启动隧道服务器...")
		tunnelServer = tunnel.NewServer(cfg.Tunnel.ListenPort)
		if err := tunnelServer.Start(); err != nil {
			log.Fatalf("启动隧道服务器失败: %v", err)
		}
		defer tunnelServer.Stop()
	}

	// 从数据库加载现有映射并启动转发器
	log.Println("加载现有端口映射...")
	mappings, err := database.GetAllMappings()
	if err != nil {
		log.Fatalf("加载端口映射失败: %v", err)
	}

	for _, mapping := range mappings {
		// 验证端口在范围内
		// if mapping.SourcePort < cfg.PortRange.From || mapping.SourcePort > cfg.PortRange.End {
		// 	log.Printf("警告: 端口 %d 超出范围，跳过", mapping.SourcePort)
		// 	continue
		// }

		used := utils.PortCheck(mapping.SourcePort)

		if used {
			log.Printf("警告: 端口 %d 已被占!", mapping.SourcePort)
			os.Exit(1)
			continue
		}

		var err error
		if mapping.UseTunnel {
			// 隧道模式：检查隧道服务器是否可用
			if !cfg.Tunnel.Enabled || tunnelServer == nil {
				log.Printf("警告: 端口 %d 需要隧道模式但隧道服务未启用，跳过", mapping.SourcePort)
				continue
			}
			err = fwdManager.AddTunnel(mapping.SourcePort, mapping.TargetHost, mapping.TargetPort, tunnelServer)
		} else {
			// 直接模式
			err = fwdManager.Add(mapping.SourcePort, mapping.TargetHost, mapping.TargetPort)
		}
		
		if err != nil {
			log.Printf("警告: 启动端口 %d 的转发失败: %v", mapping.SourcePort, err)
		}
	}

	log.Printf("成功加载 %d 个端口映射", len(mappings))

	// 创建 HTTP API 处理器
	log.Println("初始化 HTTP API...")
	apiHandler := api.NewHandler(
		database,
		fwdManager,
		tunnelServer,
		// cfg.PortRange.From,
		// cfg.PortRange.End,
	)

	// 启动 HTTP API 服务器
	go func() {
		if err := api.Start(apiHandler, cfg.API.ListenPort); err != nil {
			log.Fatalf("启动 HTTP API 服务失败: %v", err)
		}
	}()

	// // 启动 pprof 调试服务器（用于性能分析和调试）
	// pprofPort := 6060
	// go func() {
	// 	log.Printf("启动 pprof 调试服务器: http://localhost:%d/debug/pprof/", pprofPort)
	// 	if err := http.ListenAndServe(":6060", nil); err != nil {
	// 		log.Printf("pprof 服务器启动失败: %v", err)
	// 	}
	// }()

	// // 启动 goroutine 监控
	// go func() {
	// 	ticker := time.NewTicker(10 * time.Second)
	// 	defer ticker.Stop()
	// 	for range ticker.C {
	// 		numGoroutines := runtime.NumGoroutine()
	// 		log.Printf("[监控] 当前 Goroutine 数量: %d", numGoroutines)
	// 	}
	// }()

	log.Println("===========================================")
	log.Printf("服务器启动成功!")
	// log.Printf("端口范围: %d-%d", cfg.PortRange.From, cfg.PortRange.End)
	log.Printf("HTTP API: http://localhost:%d", cfg.API.ListenPort)
	// log.Printf("调试接口: http://localhost:%d/debug/pprof/", pprofPort)
	if cfg.Tunnel.Enabled {
		log.Printf("隧道服务: 端口 %d", cfg.Tunnel.ListenPort)
	}
	log.Println("===========================================")

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	log.Println("\n接收到关闭信号，正在优雅关闭...")

	// 创建关闭上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 停止所有转发器
	log.Println("停止所有端口转发...")
	fwdManager.StopAll()

	log.Println("服务器已关闭")
	<-ctx.Done()
}