package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestConfig 测试配置
type TestConfig struct {
	ServerAPIAddr    string // 服务器 API 地址
	ServerTunnelPort int    // 服务器隧道端口
	TestPort         int    // 测试用的端口
	LocalServicePort int    // 本地服务端口（客户端监听）
}

// 默认测试配置
var defaultConfig = TestConfig{
	ServerAPIAddr:    "http://localhost:8080",
	ServerTunnelPort: 9000,
	TestPort:         30001,
	LocalServicePort: 8888,
}

// CreateMappingRequest 创建映射请求
type CreateMappingRequest struct {
	Port     int    `json:"port"`
	TargetIP string `json:"target_ip,omitempty"`
}

// RemoveMappingRequest 删除映射请求
type RemoveMappingRequest struct {
	Port int `json:"port"`
}

// Response API 响应
type Response struct {
	Success bool                   `json:"success"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// TestForwardingBasic 测试基本转发功能
// 前置条件：服务器和客户端已启动,客户端在本地 8888 端口运行了一个 echo 服务
func TestForwardingBasic(t *testing.T) {
	config := defaultConfig

	// 1. 启动本地测试服务（模拟客户端本地服务）
	t.Log("启动本地测试服务...")
	stopChan := make(chan struct{})
	go startEchoServer(config.LocalServicePort, stopChan, t)
	defer close(stopChan)

	// 等待服务启动
	time.Sleep(1 * time.Second)

	// 2. 创建端口映射
	t.Logf("创建端口映射: %d -> localhost:%d", config.TestPort, config.LocalServicePort)
	err := createMapping(config.ServerAPIAddr, config.TestPort, "")
	if err != nil {
		t.Fatalf("创建端口映射失败: %v", err)
	}
	t.Log("端口映射创建成功")

	// 清理：测试结束后删除映射
	defer func() {
		t.Log("清理端口映射...")
		err := removeMapping(config.ServerAPIAddr, config.TestPort)
		if err != nil {
			t.Logf("清理端口映射失败: %v", err)
		}
		// 等待清理完成
		time.Sleep(2 * time.Second)
	}()

	// 等待映射生效
	time.Sleep(2 * time.Second)

	// 3. 通过服务器端口发送请求
	testMessage := "Hello, Tunnel!"
	t.Logf("发送测试消息: %s", testMessage)

	response, err := sendTCPRequest(fmt.Sprintf("localhost:%d", config.TestPort), testMessage)
	if err != nil {
		t.Fatalf("发送请求失败: %v", err)
	}

	// 4. 验证响应
	expectedResponse := "ECHO: " + testMessage
	if response != expectedResponse {
		t.Fatalf("响应不匹配.\n期望: %s\n实际: %s", expectedResponse, response)
	}

	t.Logf("✓ 转发成功，收到响应: %s", response)
}

// TestMultipleForwards 测试多个端口同时转发
func TestMultipleForwards(t *testing.T) {
	config := defaultConfig

	// 启动本地测试服务
	t.Log("启动本地测试服务...")
	stopChan := make(chan struct{})
	go startEchoServer(config.LocalServicePort, stopChan, t)
	defer close(stopChan)

	time.Sleep(1 * time.Second)

	// 测试多个端口
	testPorts := []int{30002, 30003, 30004}

	// 创建多个映射
	for _, port := range testPorts {
		t.Logf("创建端口映射: %d", port)
		err := createMapping(config.ServerAPIAddr, port, "")
		if err != nil {
			t.Fatalf("创建端口 %d 映射失败: %v", port, err)
		}
		defer func(p int) {
			err := removeMapping(config.ServerAPIAddr, p)
			if err != nil {
				t.Logf("删除端口 %d 映射失败: %v", p, err)
			}
		}(port)
	}

	time.Sleep(2 * time.Second)

	// 并发测试所有端口
	for _, port := range testPorts {
		t.Run(fmt.Sprintf("Port_%d", port), func(t *testing.T) {
			testMessage := fmt.Sprintf("Test message for port %d", port)
			response, err := sendTCPRequest(fmt.Sprintf("localhost:%d", port), testMessage)
			if err != nil {
				t.Errorf("端口 %d 请求失败: %v", port, err)
				return
			}

			expectedResponse := "ECHO: " + testMessage
			if response != expectedResponse {
				t.Errorf("端口 %d 响应不匹配.\n期望: %s\n实际: %s", port, expectedResponse, response)
				return
			}

			t.Logf("✓ 端口 %d 转发成功", port)
		})
	}
	
	// 等待清理完成
	time.Sleep(2 * time.Second)
}

// TestAddAndRemoveMapping 测试动态添加和删除映射
func TestAddAndRemoveMapping(t *testing.T) {
	config := defaultConfig

	// 启动本地测试服务
	t.Log("启动本地测试服务...")
	stopChan := make(chan struct{})
	go startEchoServer(config.LocalServicePort, stopChan, t)
	defer close(stopChan)

	time.Sleep(1 * time.Second)

	port := 30005

	// 1. 创建映射
	t.Logf("创建端口映射: %d", port)
	err := createMapping(config.ServerAPIAddr, port, "")
	if err != nil {
		t.Fatalf("创建端口映射失败: %v", err)
	}

	time.Sleep(2 * time.Second)

	// 2. 验证映射工作
	t.Log("验证映射工作...")
	response, err := sendTCPRequest(fmt.Sprintf("localhost:%d", port), "test1")
	if err != nil {
		t.Fatalf("映射创建后请求失败: %v", err)
	}
	if response != "ECHO: test1" {
		t.Fatalf("响应不匹配: %s", response)
	}
	t.Log("✓ 映射工作正常")

	// 3. 删除映射
	t.Log("删除端口映射...")
	err = removeMapping(config.ServerAPIAddr, port)
	if err != nil {
		t.Fatalf("删除端口映射失败: %v", err)
	}

	time.Sleep(2 * time.Second)

	// 4. 验证映射已删除（连接应该失败）
	t.Log("验证映射已删除...")
	_, err = sendTCPRequest(fmt.Sprintf("localhost:%d", port), "test2")
	if err == nil {
		t.Fatal("映射删除后请求应该失败，但成功了")
	}
	t.Logf("✓ 映射已删除，连接失败（符合预期）: %v", err)

	// 5. 重新创建映射
	t.Log("重新创建端口映射...")
	err = createMapping(config.ServerAPIAddr, port, "")
	if err != nil {
		t.Fatalf("重新创建端口映射失败: %v", err)
	}
	defer func() {
		err := removeMapping(config.ServerAPIAddr, port)
		if err != nil {
			t.Logf("清理端口映射失败: %v", err)
		}
		time.Sleep(2 * time.Second)
	}()

	time.Sleep(2 * time.Second)

	// 6. 验证映射恢复工作
	t.Log("验证映射恢复工作...")
	response, err = sendTCPRequest(fmt.Sprintf("localhost:%d", port), "test3")
	if err != nil {
		t.Fatalf("映射重新创建后请求失败: %v", err)
	}
	if response != "ECHO: test3" {
		t.Fatalf("响应不匹配: %s", response)
	}
	t.Log("✓ 映射恢复工作正常")
}

// TestConcurrentRequests 测试并发请求
func TestConcurrentRequests(t *testing.T) {
	config := defaultConfig

	// 启动本地测试服务
	t.Log("启动本地测试服务...")
	stopChan := make(chan struct{})
	go startEchoServer(config.LocalServicePort, stopChan, t)
	defer close(stopChan)

	time.Sleep(1 * time.Second)

	port := 30006

	// 创建映射
	t.Logf("创建端口映射: %d", port)
	err := createMapping(config.ServerAPIAddr, port, "")
	if err != nil {
		t.Fatalf("创建端口映射失败: %v", err)
	}
	defer func() {
		err := removeMapping(config.ServerAPIAddr, port)
		if err != nil {
			t.Logf("清理端口映射失败: %v", err)
		}
		time.Sleep(2 * time.Second)
	}()

	time.Sleep(2 * time.Second)

	// 并发发送请求
	concurrency := 10
	results := make(chan error, concurrency)

	t.Logf("并发发送 %d 个请求...", concurrency)
	for i := 0; i < concurrency; i++ {
		go func(index int) {
			message := fmt.Sprintf("concurrent_%d", index)
			response, err := sendTCPRequest(fmt.Sprintf("localhost:%d", port), message)
			if err != nil {
				results <- fmt.Errorf("请求 %d 失败: %w", index, err)
				return
			}

			expectedResponse := "ECHO: " + message
			if response != expectedResponse {
				results <- fmt.Errorf("请求 %d 响应不匹配: 期望=%s, 实际=%s", index, expectedResponse, response)
				return
			}

			results <- nil
		}(i)
	}

	// 收集结果
	successCount := 0
	for i := 0; i < concurrency; i++ {
		err := <-results
		if err != nil {
			t.Errorf("%v", err)
		} else {
			successCount++
		}
	}

	t.Logf("✓ 并发测试完成: %d/%d 成功", successCount, concurrency)

	if successCount < concurrency {
		t.Fatalf("部分并发请求失败")
	}
}

// TestListMappings 测试列出所有映射
func TestListMappings(t *testing.T) {
	config := defaultConfig

	// 创建几个映射
	testPorts := []int{30010, 30011, 30012}

	for _, port := range testPorts {
		t.Logf("创建端口映射: %d", port)
		err := createMapping(config.ServerAPIAddr, port, "")
		if err != nil {
			t.Fatalf("创建端口 %d 映射失败: %v", port, err)
		}
		defer removeMapping(config.ServerAPIAddr, port)
	}

	time.Sleep(500 * time.Millisecond)

	// 列出所有映射
	t.Log("列出所有映射...")
	mappings, err := listMappings(config.ServerAPIAddr)
	if err != nil {
		t.Fatalf("列出映射失败: %v", err)
	}

	t.Logf("当前映射数量: %d", len(mappings))

	// 验证创建的映射都在列表中
	for _, port := range testPorts {
		found := false
		for _, mapping := range mappings {
			if sourcePort, ok := mapping["source_port"].(float64); ok && int(sourcePort) == port {
				found = true
				t.Logf("✓ 找到映射: 端口 %d", port)
				break
			}
		}
		if !found {
			t.Errorf("映射端口 %d 未在列表中找到", port)
		}
	}
}

// TestHealthCheck 测试健康检查
func TestHealthCheck(t *testing.T) {
	config := defaultConfig

	url := config.ServerAPIAddr + "/health"
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("健康检查请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("健康检查返回非 200 状态码: %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	t.Logf("✓ 健康检查成功: %s", string(body))
}

// ============ 辅助函数 ============

// createMapping 创建端口映射
func createMapping(apiAddr string, port int, targetIP string) error {
	url := apiAddr + "/api/mapping/create"

	reqBody := CreateMappingRequest{
		Port:     port,
		TargetIP: targetIP,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result Response
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("API 返回失败: %s", result.Message)
	}

	return nil
}

// removeMapping 删除端口映射
func removeMapping(apiAddr string, port int) error {
	url := apiAddr + "/api/mapping/remove"

	reqBody := RemoveMappingRequest{
		Port: port,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result Response
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("API 返回失败: %s", result.Message)
	}

	return nil
}

// listMappings 列出所有映射
func listMappings(apiAddr string) ([]map[string]interface{}, error) {
	url := apiAddr + "/api/mapping/list"

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result Response
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("API 返回失败: %s", result.Message)
	}

	// 提取映射列表
	if mappingsData, ok := result.Data["mappings"].([]interface{}); ok {
		mappings := make([]map[string]interface{}, len(mappingsData))
		for i, item := range mappingsData {
			if mapping, ok := item.(map[string]interface{}); ok {
				mappings[i] = mapping
			}
		}
		return mappings, nil
	}

	return []map[string]interface{}{}, nil
}

// sendTCPRequest 发送 TCP 请求并接收响应
func sendTCPRequest(addr string, message string) (string, error) {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return "", fmt.Errorf("连接失败: %w", err)
	}
	defer conn.Close()

	// 设置超时 - 增加到10秒
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	// 发送消息
	_, err = conn.Write([]byte(message + "\n"))
	if err != nil {
		return "", fmt.Errorf("发送失败: %w", err)
	}

	// 接收响应
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		return "", fmt.Errorf("接收失败: %w", err)
	}

	return string(buffer[:n]), nil
}

// startEchoServer 启动简单的 echo 服务器（用于测试）
func startEchoServer(port int, stopChan chan struct{}, t *testing.T) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		t.Logf("启动 echo 服务器失败: %v", err)
		return
	}
	defer listener.Close()

	t.Logf("Echo 服务器启动在端口 %d", port)

	go func() {
		<-stopChan
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-stopChan:
				return
			default:
				continue
			}
		}

		go handleEchoConnection(conn, t)
	}
}

// handleEchoConnection 处理 echo 连接
func handleEchoConnection(conn net.Conn, t *testing.T) {
	defer conn.Close()

	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		if err != io.EOF {
			t.Logf("读取数据失败: %v", err)
		}
		return
	}

	message := string(buffer[:n])
	message = string(bytes.TrimSpace([]byte(message)))

	response := fmt.Sprintf("ECHO: %s", message)
	_, err = conn.Write([]byte(response))
	if err != nil {
		t.Logf("发送响应失败: %v", err)
	}
}
