package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"port-forward/client/tunnel"
	"syscall"
)

func main() {
	// 解析命令行参数
	serverAddr := flag.String("server", "localhost:9000", "隧道服务器地址 (host:port)")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// 创建隧道客户端
	log.Printf("隧道客户端启动...")
	log.Printf("服务器地址: %s", *serverAddr)
	
	client := tunnel.NewClient(*serverAddr)
	
	// 启动客户端
	if err := client.Start(); err != nil {
		log.Fatalf("启动隧道客户端失败: %v", err)
	}

	log.Println("===========================================")
	log.Println("隧道客户端运行中...")
	log.Println("按 Ctrl+C 退出")
	log.Println("===========================================")

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	log.Println("\n接收到关闭信号，正在关闭...")

	// 停止客户端
	if err := client.Stop(); err != nil {
		log.Printf("停止客户端失败: %v", err)
	}

	log.Println("客户端已关闭")
}