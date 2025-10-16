package main

import (
	"crypto/rand"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// TestConfig 测试配置
type TestConfig struct {
	ListenPort   int           // 监听端口（接收端）
	ConnectPort  int           // 连接端口（发送端）
	TestDuration time.Duration // 测试持续时间
	PacketSize   int           // 数据包大小
	Concurrent   int           // 并发连接数
	Mode         string        // 测试模式: speed, reliability, all
}

// TestResult 测试结果
type TestResult struct {
	TotalBytes      uint64        // 总传输字节数
	TotalPackets    uint64        // 总数据包数
	SuccessPackets  uint64        // 成功的数据包数
	FailedPackets   uint64        // 失败的数据包数
	Duration        time.Duration // 实际测试时长
	AvgSpeed        float64       // 平均速度 (MB/s)
	MinLatency      time.Duration // 最小延迟
	MaxLatency      time.Duration // 最大延迟
	AvgLatency      time.Duration // 平均延迟
	SuccessRate     float64       // 成功率
	ConnectionFails uint64        // 连接失败次数
}

var (
	totalBytes      uint64
	totalPackets    uint64
	successPackets  uint64
	failedPackets   uint64
	connectionFails uint64
	latencySum      int64
	minLatency      int64 = 1<<63 - 1
	maxLatency      int64
	latencyCount    uint64
)

func main() {
	config := parseFlags()

	log.Printf("=" + "=================================")
	log.Printf("TCP 转发性能测试工具")
	log.Printf("=" + "=================================")
	log.Printf("")
	log.Printf("测试配置:")
	log.Printf("  监听端口:     %d", config.ListenPort)
	log.Printf("  连接端口:     %d", config.ConnectPort)
	log.Printf("  测试时长:     %v", config.TestDuration)
	log.Printf("  数据包大小:   %d bytes", config.PacketSize)
	log.Printf("  并发连接数:   %d", config.Concurrent)
	log.Printf("  测试模式:     %s", config.Mode)
	log.Println()

	// 启动监听服务器
	listener, err := startListener(config.ListenPort)
	if err != nil {
		log.Fatalf("启动监听服务失败: %v", err)
	}
	defer listener.Close()

	log.Printf("✓ 监听服务已启动在端口 %d", config.ListenPort)
	log.Println()

	// 等待一秒确保监听服务已准备好
	time.Sleep(time.Second)

	// 运行测试
	var result *TestResult
	switch config.Mode {
	case "speed":
		result = runSpeedTest(config)
	case "reliability":
		result = runReliabilityTest(config)
	case "sizes":
		runPacketSizeTests(config)
		return
	case "concurrent":
		runConcurrentTests(config)
		return
	case "bandwidth":
		runBandwidthTest(config)
		return
	case "upload":
		result = runUploadTest(config)
	case "download":
		result = runDownloadTest(config)
	case "all":
		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Println("▶ 运行速度测试")
		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		speedResult := runSpeedTest(config)
		printResult(speedResult, "速度测试")
		log.Println()

		// 重置计数器
		resetCounters()
		time.Sleep(2 * time.Second)

		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Println("▶ 运行可靠性测试")
		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		reliabilityResult := runReliabilityTest(config)
		printResult(reliabilityResult, "可靠性测试")
		log.Println()

		// 重置计数器
		resetCounters()
		time.Sleep(2 * time.Second)

		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Println("▶ 运行并发测试")
		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		runConcurrentTests(config)
		log.Println()

		// 重置计数器
		resetCounters()
		time.Sleep(2 * time.Second)

		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Println("▶ 运行最大带宽测试")
		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		runBandwidthTest(config)
		log.Println()

		// 重置计数器
		resetCounters()
		time.Sleep(2 * time.Second)

		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Println("▶ 运行不同数据包大小测试")
		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		runPacketSizeTests(config)
		return
	default:
		log.Fatalf("未知的测试模式: %s", config.Mode)
	}

	// 打印结果
	printResult(result, config.Mode)
}

func parseFlags() *TestConfig {
	config := &TestConfig{}

	flag.IntVar(&config.ListenPort, "listen", 9900, "监听端口")
	flag.IntVar(&config.ConnectPort, "connect", 9901, "连接端口")
	flag.DurationVar(&config.TestDuration, "duration", 10*time.Second, "测试持续时间")
	flag.IntVar(&config.PacketSize, "size", 1024, "数据包大小(bytes)")
	flag.IntVar(&config.Concurrent, "concurrent", 1, "并发连接数")
	flag.StringVar(&config.Mode, "mode", "all", "测试模式: speed, reliability, sizes, all, concurrent, bandwidth, upload, download")

	flag.Parse()

	return config
}

// startListener 启动TCP监听服务
func startListener(port int) (net.Listener, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleConnection(conn)
		}
	}()

	return listener, nil
}

// handleConnection 处理接收到的连接（Echo服务器）
func handleConnection(conn net.Conn) {
	defer conn.Close()

	buffer := make([]byte, 8192)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err != io.EOF {
				// log.Printf("读取错误: %v", err)
			}
			return
		}

		// 回显数据
		_, err = conn.Write(buffer[:n])
		if err != nil {
			// log.Printf("写入错误: %v", err)
			return
		}
	}
}

// runSpeedTest 运行速度测试
func runSpeedTest(config *TestConfig) *TestResult {
	log.Println("开始速度测试...")

	startTime := time.Now()
	stopChan := make(chan bool)
	var wg sync.WaitGroup

	// 启动多个并发连接
	for i := 0; i < config.Concurrent; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runSpeedWorker(config, stopChan, id)
		}(i)
	}

	// 等待测试时间结束
	time.Sleep(config.TestDuration)
	close(stopChan)

	wg.Wait()
	duration := time.Since(startTime)

	// 计算结果
	result := &TestResult{
		TotalBytes:     atomic.LoadUint64(&totalBytes),
		TotalPackets:   atomic.LoadUint64(&totalPackets),
		SuccessPackets: atomic.LoadUint64(&successPackets),
		FailedPackets:  atomic.LoadUint64(&failedPackets),
		Duration:       duration,
	}

	if result.TotalPackets > 0 {
		result.SuccessRate = float64(result.SuccessPackets) / float64(result.TotalPackets) * 100
	}

	if duration.Seconds() > 0 {
		result.AvgSpeed = float64(result.TotalBytes) / 1024 / 1024 / duration.Seconds()
	}

	if latencyCount := atomic.LoadUint64(&latencyCount); latencyCount > 0 {
		result.AvgLatency = time.Duration(atomic.LoadInt64(&latencySum) / int64(latencyCount))
		result.MinLatency = time.Duration(atomic.LoadInt64(&minLatency))
		result.MaxLatency = time.Duration(atomic.LoadInt64(&maxLatency))
	}

	result.ConnectionFails = atomic.LoadUint64(&connectionFails)

	return result
}

// runSpeedWorker 速度测试工作协程
func runSpeedWorker(config *TestConfig, stopChan chan bool, id int) {
	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", config.ConnectPort))
	if err != nil {
		log.Printf("✗ [Worker %d] 连接失败: %v", id, err)
		atomic.AddUint64(&connectionFails, 1)
		return
	}
	defer conn.Close()

	data := make([]byte, config.PacketSize)
	rand.Read(data)

	buffer := make([]byte, config.PacketSize)

	for {
		select {
		case <-stopChan:
			return
		default:
			sendTime := time.Now()

			// 发送数据
			_, err := conn.Write(data)
			if err != nil {
				atomic.AddUint64(&failedPackets, 1)
				atomic.AddUint64(&totalPackets, 1)
				continue
			}

			// 接收响应
			n, err := io.ReadFull(conn, buffer)
			if err != nil {
				atomic.AddUint64(&failedPackets, 1)
				atomic.AddUint64(&totalPackets, 1)
				continue
			}

			latency := time.Since(sendTime)

			// 更新统计
			atomic.AddUint64(&totalBytes, uint64(n))
			atomic.AddUint64(&totalPackets, 1)
			atomic.AddUint64(&successPackets, 1)

			// 更新延迟统计
			atomic.AddInt64(&latencySum, int64(latency))
			atomic.AddUint64(&latencyCount, 1)

			// 更新最小延迟
			for {
				old := atomic.LoadInt64(&minLatency)
				if int64(latency) >= old {
					break
				}
				if atomic.CompareAndSwapInt64(&minLatency, old, int64(latency)) {
					break
				}
			}

			// 更新最大延迟
			for {
				old := atomic.LoadInt64(&maxLatency)
				if int64(latency) <= old {
					break
				}
				if atomic.CompareAndSwapInt64(&maxLatency, old, int64(latency)) {
					break
				}
			}
		}
	}
}

// runReliabilityTest 运行可靠性测试
func runReliabilityTest(config *TestConfig) *TestResult {
	log.Println("开始可靠性测试...")

	startTime := time.Now()
	stopChan := make(chan bool)
	var wg sync.WaitGroup

	// 启动多个并发连接
	for i := 0; i < config.Concurrent; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runReliabilityWorker(config, stopChan, id)
		}(i)
	}

	// 等待测试时间结束
	time.Sleep(config.TestDuration)
	close(stopChan)

	wg.Wait()
	duration := time.Since(startTime)

	// 计算结果
	result := &TestResult{
		TotalBytes:     atomic.LoadUint64(&totalBytes),
		TotalPackets:   atomic.LoadUint64(&totalPackets),
		SuccessPackets: atomic.LoadUint64(&successPackets),
		FailedPackets:  atomic.LoadUint64(&failedPackets),
		Duration:       duration,
	}

	if result.TotalPackets > 0 {
		result.SuccessRate = float64(result.SuccessPackets) / float64(result.TotalPackets) * 100
	}

	if duration.Seconds() > 0 {
		result.AvgSpeed = float64(result.TotalBytes) / 1024 / 1024 / duration.Seconds()
	}

	if latencyCount := atomic.LoadUint64(&latencyCount); latencyCount > 0 {
		result.AvgLatency = time.Duration(atomic.LoadInt64(&latencySum) / int64(latencyCount))
		result.MinLatency = time.Duration(atomic.LoadInt64(&minLatency))
		result.MaxLatency = time.Duration(atomic.LoadInt64(&maxLatency))
	}

	result.ConnectionFails = atomic.LoadUint64(&connectionFails)

	return result
}

// runReliabilityWorker 可靠性测试工作协程
func runReliabilityWorker(config *TestConfig, stopChan chan bool, id int) {
	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", config.ConnectPort))
	if err != nil {
		log.Printf("✗ [Worker %d] 连接失败: %v", id, err)
		atomic.AddUint64(&connectionFails, 1)
		return
	}
	defer conn.Close()

	packetNum := uint64(0)

	for {
		select {
		case <-stopChan:
			return
		default:
			packetNum++

			// 创建带序列号的数据包
			data := make([]byte, config.PacketSize)
			binary.BigEndian.PutUint64(data[0:8], packetNum)
			rand.Read(data[8:])

			sendTime := time.Now()

			// 发送数据
			_, err := conn.Write(data)
			if err != nil {
				atomic.AddUint64(&failedPackets, 1)
				atomic.AddUint64(&totalPackets, 1)
				log.Printf("✗ [Worker %d] 发送失败 (包 #%d): %v", id, packetNum, err)
				continue
			}

			// 接收响应
			buffer := make([]byte, config.PacketSize)
			n, err := io.ReadFull(conn, buffer)
			if err != nil {
				atomic.AddUint64(&failedPackets, 1)
				atomic.AddUint64(&totalPackets, 1)
				log.Printf("✗ [Worker %d] 接收失败 (包 #%d): %v", id, packetNum, err)
				continue
			}

			latency := time.Since(sendTime)

			// 验证数据完整性
			receivedNum := binary.BigEndian.Uint64(buffer[0:8])
			if receivedNum != packetNum {
				atomic.AddUint64(&failedPackets, 1)
				atomic.AddUint64(&totalPackets, 1)
				log.Printf("✗ [Worker %d] 数据不匹配 (期望 #%d, 收到 #%d)", id, packetNum, receivedNum)
				continue
			}

			// 验证数据内容
			if !bytesEqual(data, buffer) {
				atomic.AddUint64(&failedPackets, 1)
				atomic.AddUint64(&totalPackets, 1)
				log.Printf("✗ [Worker %d] 数据损坏 (包 #%d)", id, packetNum)
				continue
			}

			// 更新统计
			atomic.AddUint64(&totalBytes, uint64(n))
			atomic.AddUint64(&totalPackets, 1)
			atomic.AddUint64(&successPackets, 1)

			// 更新延迟统计
			atomic.AddInt64(&latencySum, int64(latency))
			atomic.AddUint64(&latencyCount, 1)

			// 更新最小延迟
			for {
				old := atomic.LoadInt64(&minLatency)
				if int64(latency) >= old {
					break
				}
				if atomic.CompareAndSwapInt64(&minLatency, old, int64(latency)) {
					break
				}
			}

			// 更新最大延迟
			for {
				old := atomic.LoadInt64(&maxLatency)
				if int64(latency) <= old {
					break
				}
				if atomic.CompareAndSwapInt64(&maxLatency, old, int64(latency)) {
					break
				}
			}

			// 在可靠性测试中添加小延迟，确保每个包都能被验证
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// bytesEqual 比较两个字节切片是否相等
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// runPacketSizeTests 运行不同数据包大小的测试
func runPacketSizeTests(config *TestConfig) {
	// 定义要测试的数据包大小（bytes）
	packetSizes := []int{
		64,      // 最小包 - 适用于控制消息
		256,     // 小包
		512,     // 小包
		1024,    // 1KB - 标准小包
		4096,    // 4KB
		8192,    // 8KB
		16384,   // 16KB
		32768,   // 32KB
		65536,   // 64KB - MTU友好大小
		131072,  // 128KB
		262144,  // 256KB
		524288,  // 512KB
		1048576, // 1MB - 大包
	}

	results := make([]*PacketSizeTestResult, 0, len(packetSizes))

	log.Println()
	log.Println("开始测试不同数据包大小...")
	log.Printf("将测试 %d 种不同的数据包大小", len(packetSizes))
	log.Println()

	for i, size := range packetSizes {
		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Printf("▶ 测试 %d/%d: 数据包大小 = %s", i+1, len(packetSizes), formatBytes(size))
		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// 创建新的配置
		testConfig := *config
		testConfig.PacketSize = size

		// 重置计数器
		resetCounters()

		// 运行速度测试
		result := runSpeedTest(&testConfig)

		// 保存结果
		sizeResult := &PacketSizeTestResult{
			PacketSize: size,
			TestResult: *result,
		}
		results = append(results, sizeResult)

		// 打印简要结果
		log.Printf("✓ 完成: 速度=%.2f MB/s, 成功率=%.2f%%, 延迟=%v",
			result.AvgSpeed, result.SuccessRate, result.AvgLatency)
		log.Println()

		// 在测试之间稍作延迟
		if i < len(packetSizes)-1 {
			time.Sleep(1 * time.Second)
		}
	}

	// 打印汇总结果
	printPacketSizeComparison(results)
}

// PacketSizeTestResult 数据包大小测试结果
type PacketSizeTestResult struct {
	PacketSize int
	TestResult
}

// formatBytes 格式化字节大小
func formatBytes(bytes int) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%d %cB", bytes/int(div), "KMGTPE"[exp])
}

// printPacketSizeComparison 打印数据包大小对比结果
func printPacketSizeComparison(results []*PacketSizeTestResult) {
	log.Println()
	log.Println("┏" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓")
	log.Printf("┃  数据包大小测试 - 对比结果")
	log.Println("┣" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┫")
	log.Printf("┃  %-10s │ %-12s │ %-10s │ %-12s │ %-12s │ %-10s ┃",
		"包大小", "速度(MB/s)", "成功率", "包数/秒", "平均延迟", "总传输")
	log.Println("┣" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┫")

	var bestSpeed *PacketSizeTestResult
	var bestRate *PacketSizeTestResult
	var bestLatency *PacketSizeTestResult
	var bestThroughput *PacketSizeTestResult

	for _, result := range results {
		packetsPerSec := float64(result.TotalPackets) / result.Duration.Seconds()
		totalMB := float64(result.TotalBytes) / 1024 / 1024

		// 找出最佳结果
		if bestSpeed == nil || result.AvgSpeed > bestSpeed.AvgSpeed {
			bestSpeed = result
		}
		if bestRate == nil || result.SuccessRate > bestRate.SuccessRate {
			bestRate = result
		}
		if bestLatency == nil || (result.AvgLatency > 0 && result.AvgLatency < bestLatency.AvgLatency) {
			bestLatency = result
		}
		if bestThroughput == nil || packetsPerSec > float64(bestThroughput.TotalPackets)/bestThroughput.Duration.Seconds() {
			bestThroughput = result
		}

		// 打印每行结果
		marker := " "
		if result == bestSpeed {
			marker = "🚀"
		}

		log.Printf("┃ %s%-9s │ %12.2f │ %9.2f%% │ %12.0f │ %12v │ %8.2f MB ┃",
			marker,
			formatBytes(result.PacketSize),
			result.AvgSpeed,
			result.SuccessRate,
			packetsPerSec,
			result.AvgLatency,
			totalMB)
	}

	log.Println("┣" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┫")
	log.Printf("┃  🚀 = 最快速度")
	log.Println("┣" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┫")

	if bestSpeed != nil {
		log.Printf("┃  最佳速度:         %s @ %.2f MB/s",
			formatBytes(bestSpeed.PacketSize), bestSpeed.AvgSpeed)
	}
	if bestRate != nil {
		log.Printf("┃  最佳成功率:       %s @ %.2f%%",
			formatBytes(bestRate.PacketSize), bestRate.SuccessRate)
	}
	if bestLatency != nil && bestLatency.AvgLatency > 0 {
		log.Printf("┃  最低延迟:         %s @ %v",
			formatBytes(bestLatency.PacketSize), bestLatency.AvgLatency)
	}
	if bestThroughput != nil {
		packetsPerSec := float64(bestThroughput.TotalPackets) / bestThroughput.Duration.Seconds()
		log.Printf("┃  最高吞吐量:       %s @ %.0f packets/s",
			formatBytes(bestThroughput.PacketSize), packetsPerSec)
	}
}

// printResult 打印测试结果
func printResult(result *TestResult, testName string) {
	log.Println()
	log.Println("┏" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓")
	log.Printf("┃  %s - 测试结果", testName)
	log.Println("┣" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┫")
	log.Printf("┃  测试时长:       %v", result.Duration)
	log.Printf("┃  总字节数:       %d (%.2f MB)", result.TotalBytes, float64(result.TotalBytes)/1024/1024)
	log.Printf("┃  总数据包数:     %d", result.TotalPackets)
	log.Printf("┃  成功数据包:     %d", result.SuccessPackets)
	log.Printf("┃  失败数据包:     %d", result.FailedPackets)
	log.Printf("┃  连接失败次数:   %d", result.ConnectionFails)
	log.Println("┣" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┫")

	// 成功率着色
	successRateStr := fmt.Sprintf("%.2f%%", result.SuccessRate)
	if result.SuccessRate >= 99.9 {
		successRateStr = "✓ " + successRateStr
	} else if result.SuccessRate >= 95 {
		successRateStr = "⚠ " + successRateStr
	} else {
		successRateStr = "✗ " + successRateStr
	}
	log.Printf("┃  成功率:         %s", successRateStr)
	log.Printf("┃  平均速度:       %.2f MB/s", result.AvgSpeed)

	if result.AvgLatency > 0 {
		log.Println("┣" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┫")
		log.Printf("┃  最小延迟:       %v", result.MinLatency)
		log.Printf("┃  最大延迟:       %v", result.MaxLatency)
		log.Printf("┃  平均延迟:       %v", result.AvgLatency)
	}

	if result.TotalPackets > 0 {
		packetsPerSec := float64(result.TotalPackets) / result.Duration.Seconds()
		log.Println("┣" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┫")
		log.Printf("┃  吞吐量:         %.2f packets/s", packetsPerSec)
	}

	log.Println("┗" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛")
}

// resetCounters 重置所有计数器
func resetCounters() {
	atomic.StoreUint64(&totalBytes, 0)
	atomic.StoreUint64(&totalPackets, 0)
	atomic.StoreUint64(&successPackets, 0)
	atomic.StoreUint64(&failedPackets, 0)
	atomic.StoreUint64(&connectionFails, 0)
	atomic.StoreInt64(&latencySum, 0)
	atomic.StoreInt64(&minLatency, 1<<63-1)
	atomic.StoreInt64(&maxLatency, 0)
	atomic.StoreUint64(&latencyCount, 0)
}

// runConcurrentTests 运行不同并发数的测试
func runConcurrentTests(config *TestConfig) {
	// 定义要测试的并发数
	concurrentLevels := []int{1, 2, 5, 10, 20, 50, 100}

	results := make([]*ConcurrentTestResult, 0, len(concurrentLevels))

	log.Println()
	log.Println("开始测试不同并发连接数...")
	log.Printf("将测试 %d 种不同的并发级别", len(concurrentLevels))
	log.Println()

	for i, concurrent := range concurrentLevels {
		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Printf("▶ 测试 %d/%d: 并发数 = %d", i+1, len(concurrentLevels), concurrent)
		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// 创建新的配置
		testConfig := *config
		testConfig.Concurrent = concurrent

		// 重置计数器
		resetCounters()

		// 运行速度测试
		result := runSpeedTest(&testConfig)

		// 保存结果
		concurrentResult := &ConcurrentTestResult{
			Concurrent: concurrent,
			TestResult: *result,
		}
		results = append(results, concurrentResult)

		// 打印简要结果
		log.Printf("✓ 完成: 速度=%.2f MB/s, 成功率=%.2f%%, 延迟=%v, 连接失败=%d",
			result.AvgSpeed, result.SuccessRate, result.AvgLatency, result.ConnectionFails)
		log.Println()

		// 在测试之间稍作延迟
		if i < len(concurrentLevels)-1 {
			time.Sleep(2 * time.Second)
		}
	}

	// 打印汇总结果
	printConcurrentComparison(results)
}

// ConcurrentTestResult 并发测试结果
type ConcurrentTestResult struct {
	Concurrent int
	TestResult
}

// printConcurrentComparison 打印并发测试对比结果
func printConcurrentComparison(results []*ConcurrentTestResult) {
	log.Println()
	log.Println("┏" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓")
	log.Printf("┃  并发连接数测试 - 对比结果")
	log.Println("┣" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┫")
	log.Printf("┃  %-8s │ %-12s │ %-10s │ %-12s │ %-12s │ %-10s │ %-10s ┃",
		"并发数", "速度(MB/s)", "成功率", "包数/秒", "平均延迟", "连接失败", "总传输")
	log.Println("┣" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┫")

	var bestSpeed *ConcurrentTestResult
	var bestRate *ConcurrentTestResult
	var bestLatency *ConcurrentTestResult

	for _, result := range results {
		packetsPerSec := float64(result.TotalPackets) / result.Duration.Seconds()
		totalMB := float64(result.TotalBytes) / 1024 / 1024

		// 找出最佳结果
		if bestSpeed == nil || result.AvgSpeed > bestSpeed.AvgSpeed {
			bestSpeed = result
		}
		if bestRate == nil || result.SuccessRate > bestRate.SuccessRate {
			bestRate = result
		}
		if bestLatency == nil || (result.AvgLatency > 0 && result.AvgLatency < bestLatency.AvgLatency) {
			bestLatency = result
		}

		// 打印每行结果
		marker := " "
		if result == bestSpeed {
			marker = "🚀"
		}

		log.Printf("┃ %s%-7d │ %12.2f │ %9.2f%% │ %12.0f │ %12v │ %10d │ %8.2f MB ┃",
			marker,
			result.Concurrent,
			result.AvgSpeed,
			result.SuccessRate,
			packetsPerSec,
			result.AvgLatency,
			result.ConnectionFails,
			totalMB)
	}

	log.Println("┣" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┫")
	log.Printf("┃  🚀 = 最快速度")
	log.Println("┣" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┫")

	if bestSpeed != nil {
		log.Printf("┃  最佳速度:         并发数=%d @ %.2f MB/s",
			bestSpeed.Concurrent, bestSpeed.AvgSpeed)
	}
	if bestRate != nil {
		log.Printf("┃  最佳成功率:       并发数=%d @ %.2f%%",
			bestRate.Concurrent, bestRate.SuccessRate)
	}
	if bestLatency != nil && bestLatency.AvgLatency > 0 {
		log.Printf("┃  最低延迟:         并发数=%d @ %v",
			bestLatency.Concurrent, bestLatency.AvgLatency)
	}

	log.Println("┗" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛")
}

// runBandwidthTest 运行最大带宽测试
func runBandwidthTest(config *TestConfig) {
	log.Println()
	log.Println("开始最大带宽测试...")
	log.Printf("将通过逐步增加并发数和数据包大小来测试最大带宽")
	log.Println()

	// 测试不同的并发数和包大小组合
	testCases := []struct {
		concurrent int
		packetSize int
		name       string
	}{
		{1, 65536, "单连接-64KB"},
		{5, 65536, "5连接-64KB"},
		{10, 65536, "10连接-64KB"},
		{20, 65536, "20连接-64KB"},
		{50, 65536, "50连接-64KB"},
		{10, 131072, "10连接-128KB"},
		{20, 131072, "20连接-128KB"},
		{50, 131072, "50连接-128KB"},
		{10, 262144, "10连接-256KB"},
		{20, 262144, "20连接-256KB"},
	}

	results := make([]*BandwidthTestResult, 0, len(testCases))
	var maxBandwidth *BandwidthTestResult

	for i, tc := range testCases {
		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Printf("▶ 测试 %d/%d: %s", i+1, len(testCases), tc.name)
		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// 创建新的配置
		testConfig := *config
		testConfig.Concurrent = tc.concurrent
		testConfig.PacketSize = tc.packetSize

		// 重置计数器
		resetCounters()

		// 运行速度测试
		result := runSpeedTest(&testConfig)

		// 保存结果
		bwResult := &BandwidthTestResult{
			Name:       tc.name,
			Concurrent: tc.concurrent,
			PacketSize: tc.packetSize,
			TestResult: *result,
		}
		results = append(results, bwResult)

		// 更新最大带宽
		if maxBandwidth == nil || result.AvgSpeed > maxBandwidth.AvgSpeed {
			maxBandwidth = bwResult
		}

		// 打印简要结果
		log.Printf("✓ 完成: 速度=%.2f MB/s, 成功率=%.2f%%",
			result.AvgSpeed, result.SuccessRate)
		log.Println()

		// 在测试之间稍作延迟
		if i < len(testCases)-1 {
			time.Sleep(2 * time.Second)
		}
	}

	// 打印汇总结果
	printBandwidthComparison(results, maxBandwidth)
}

// BandwidthTestResult 带宽测试结果
type BandwidthTestResult struct {
	Name       string
	Concurrent int
	PacketSize int
	TestResult
}

// printBandwidthComparison 打印带宽测试对比结果
func printBandwidthComparison(results []*BandwidthTestResult, maxBandwidth *BandwidthTestResult) {
	log.Println()
	log.Println("┏" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓")
	log.Printf("┃  最大带宽测试 - 对比结果")
	log.Println("┣" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┫")
	log.Printf("┃  %-20s │ %-12s │ %-10s │ %-12s │ %-10s ┃",
		"配置", "速度(MB/s)", "成功率", "平均延迟", "总传输")
	log.Println("┣" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┫")

	for _, result := range results {
		totalMB := float64(result.TotalBytes) / 1024 / 1024

		// 打印每行结果
		marker := " "
		if result == maxBandwidth {
			marker = "🏆"
		}

		log.Printf("┃ %s%-19s │ %12.2f │ %9.2f%% │ %12v │ %8.2f MB ┃",
			marker,
			result.Name,
			result.AvgSpeed,
			result.SuccessRate,
			result.AvgLatency,
			totalMB)
	}

	log.Println("┣" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┫")
	log.Printf("┃  🏆 = 最大带宽")

	if maxBandwidth != nil {
		log.Println("┣" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┫")
		log.Printf("┃  最大带宽配置:")
		log.Printf("┃    配置:           %s", maxBandwidth.Name)
		log.Printf("┃    速度:           %.2f MB/s (%.2f Mbps)", maxBandwidth.AvgSpeed, maxBandwidth.AvgSpeed*8)
		log.Printf("┃    并发数:         %d", maxBandwidth.Concurrent)
		log.Printf("┃    数据包大小:     %s", formatBytes(maxBandwidth.PacketSize))
		log.Printf("┃    成功率:         %.2f%%", maxBandwidth.SuccessRate)
	}

	log.Println("┗" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛")
}

// runUploadTest 运行模拟文件上传测试
func runUploadTest(config *TestConfig) *TestResult {
	log.Println("开始文件上传模拟测试...")
	log.Printf("模拟上传一个大文件（单向传输，客户端->服务器）")

	startTime := time.Now()
	stopChan := make(chan bool)
	var wg sync.WaitGroup

	// 使用较大的包和多个并发连接模拟文件上传
	uploadConfig := *config
	uploadConfig.PacketSize = 65536 // 64KB 包
	if uploadConfig.Concurrent < 5 {
		uploadConfig.Concurrent = 5 // 至少5个并发连接
	}

	// 启动上传工作协程
	for i := 0; i < uploadConfig.Concurrent; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runUploadWorker(&uploadConfig, stopChan, id)
		}(i)
	}

	// 等待测试时间结束
	time.Sleep(uploadConfig.TestDuration)
	close(stopChan)

	wg.Wait()
	duration := time.Since(startTime)

	// 计算结果
	result := &TestResult{
		TotalBytes:     atomic.LoadUint64(&totalBytes),
		TotalPackets:   atomic.LoadUint64(&totalPackets),
		SuccessPackets: atomic.LoadUint64(&successPackets),
		FailedPackets:  atomic.LoadUint64(&failedPackets),
		Duration:       duration,
	}

	if result.TotalPackets > 0 {
		result.SuccessRate = float64(result.SuccessPackets) / float64(result.TotalPackets) * 100
	}

	if duration.Seconds() > 0 {
		result.AvgSpeed = float64(result.TotalBytes) / 1024 / 1024 / duration.Seconds()
	}

	if latencyCount := atomic.LoadUint64(&latencyCount); latencyCount > 0 {
		result.AvgLatency = time.Duration(atomic.LoadInt64(&latencySum) / int64(latencyCount))
		result.MinLatency = time.Duration(atomic.LoadInt64(&minLatency))
		result.MaxLatency = time.Duration(atomic.LoadInt64(&maxLatency))
	}

	result.ConnectionFails = atomic.LoadUint64(&connectionFails)

	log.Printf("\n✓ 模拟上传完成: 传输了 %.2f MB 数据", float64(result.TotalBytes)/1024/1024)

	return result
}

// runUploadWorker 上传测试工作协程
func runUploadWorker(config *TestConfig, stopChan chan bool, id int) {
	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", config.ConnectPort))
	if err != nil {
		log.Printf("✗ [Upload Worker %d] 连接失败: %v", id, err)
		atomic.AddUint64(&connectionFails, 1)
		return
	}
	defer conn.Close()

	// 生成随机数据模拟文件内容
	data := make([]byte, config.PacketSize)
	rand.Read(data)

	buffer := make([]byte, config.PacketSize)

	for {
		select {
		case <-stopChan:
			return
		default:
			sendTime := time.Now()

			// 发送数据
			_, err := conn.Write(data)
			if err != nil {
				atomic.AddUint64(&failedPackets, 1)
				atomic.AddUint64(&totalPackets, 1)
				continue
			}

			// 接收确认（echo back）
			n, err := io.ReadFull(conn, buffer)
			if err != nil {
				atomic.AddUint64(&failedPackets, 1)
				atomic.AddUint64(&totalPackets, 1)
				continue
			}

			latency := time.Since(sendTime)

			// 更新统计
			atomic.AddUint64(&totalBytes, uint64(n))
			atomic.AddUint64(&totalPackets, 1)
			atomic.AddUint64(&successPackets, 1)

			// 更新延迟统计
			atomic.AddInt64(&latencySum, int64(latency))
			atomic.AddUint64(&latencyCount, 1)

			// 更新最小延迟
			for {
				old := atomic.LoadInt64(&minLatency)
				if int64(latency) >= old {
					break
				}
				if atomic.CompareAndSwapInt64(&minLatency, old, int64(latency)) {
					break
				}
			}

			// 更新最大延迟
			for {
				old := atomic.LoadInt64(&maxLatency)
				if int64(latency) <= old {
					break
				}
				if atomic.CompareAndSwapInt64(&maxLatency, old, int64(latency)) {
					break
				}
			}
		}
	}
}

// runDownloadTest 运行模拟文件下载测试
func runDownloadTest(config *TestConfig) *TestResult {
	log.Println("开始文件下载模拟测试...")
	log.Printf("模拟下载一个大文件（单向传输，服务器->客户端）")

	startTime := time.Now()
	stopChan := make(chan bool)
	var wg sync.WaitGroup

	// 使用较大的包和多个并发连接模拟文件下载
	downloadConfig := *config
	downloadConfig.PacketSize = 65536 // 64KB 包
	if downloadConfig.Concurrent < 5 {
		downloadConfig.Concurrent = 5 // 至少5个并发连接
	}

	// 启动下载工作协程
	for i := 0; i < downloadConfig.Concurrent; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runDownloadWorker(&downloadConfig, stopChan, id)
		}(i)
	}

	// 等待测试时间结束
	time.Sleep(downloadConfig.TestDuration)
	close(stopChan)

	wg.Wait()
	duration := time.Since(startTime)

	// 计算结果
	result := &TestResult{
		TotalBytes:     atomic.LoadUint64(&totalBytes),
		TotalPackets:   atomic.LoadUint64(&totalPackets),
		SuccessPackets: atomic.LoadUint64(&successPackets),
		FailedPackets:  atomic.LoadUint64(&failedPackets),
		Duration:       duration,
	}

	if result.TotalPackets > 0 {
		result.SuccessRate = float64(result.SuccessPackets) / float64(result.TotalPackets) * 100
	}

	if duration.Seconds() > 0 {
		result.AvgSpeed = float64(result.TotalBytes) / 1024 / 1024 / duration.Seconds()
	}

	if latencyCount := atomic.LoadUint64(&latencyCount); latencyCount > 0 {
		result.AvgLatency = time.Duration(atomic.LoadInt64(&latencySum) / int64(latencyCount))
		result.MinLatency = time.Duration(atomic.LoadInt64(&minLatency))
		result.MaxLatency = time.Duration(atomic.LoadInt64(&maxLatency))
	}

	result.ConnectionFails = atomic.LoadUint64(&connectionFails)

	log.Printf("\n✓ 模拟下载完成: 传输了 %.2f MB 数据", float64(result.TotalBytes)/1024/1024)

	return result
}

// runDownloadWorker 下载测试工作协程（与上传类似，但可以添加不同的逻辑）
func runDownloadWorker(config *TestConfig, stopChan chan bool, id int) {
	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", config.ConnectPort))
	if err != nil {
		log.Printf("✗ [Download Worker %d] 连接失败: %v", id, err)
		atomic.AddUint64(&connectionFails, 1)
		return
	}
	defer conn.Close()

	// 生成随机数据
	data := make([]byte, config.PacketSize)
	rand.Read(data)

	buffer := make([]byte, config.PacketSize)

	for {
		select {
		case <-stopChan:
			return
		default:
			sendTime := time.Now()

			// 发送请求
			_, err := conn.Write(data)
			if err != nil {
				atomic.AddUint64(&failedPackets, 1)
				atomic.AddUint64(&totalPackets, 1)
				continue
			}

			// 接收数据（模拟下载）
			n, err := io.ReadFull(conn, buffer)
			if err != nil {
				atomic.AddUint64(&failedPackets, 1)
				atomic.AddUint64(&totalPackets, 1)
				continue
			}

			latency := time.Since(sendTime)

			// 更新统计
			atomic.AddUint64(&totalBytes, uint64(n))
			atomic.AddUint64(&totalPackets, 1)
			atomic.AddUint64(&successPackets, 1)

			// 更新延迟统计
			atomic.AddInt64(&latencySum, int64(latency))
			atomic.AddUint64(&latencyCount, 1)

			// 更新最小延迟
			for {
				old := atomic.LoadInt64(&minLatency)
				if int64(latency) >= old {
					break
				}
				if atomic.CompareAndSwapInt64(&minLatency, old, int64(latency)) {
					break
				}
			}

			// 更新最大延迟
			for {
				old := atomic.LoadInt64(&maxLatency)
				if int64(latency) <= old {
					break
				}
				if atomic.CompareAndSwapInt64(&maxLatency, old, int64(latency)) {
					break
				}
			}
		}
	}
}
