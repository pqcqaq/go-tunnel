package tunnel

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// TestNewClient 测试创建隧道客户端
func TestNewClient(t *testing.T) {
	client := NewClient("127.0.0.1:9000")
	
	if client == nil {
		t.Fatal("创建隧道客户端失败")
	}
	
	if client.serverAddr != "127.0.0.1:9000" {
		t.Errorf("服务器地址不正确，期望 127.0.0.1:9000，得到 %s", client.serverAddr)
	}
	
	if client.connections == nil {
		t.Error("连接映射未初始化")
	}
	
	if client.sendChan == nil {
		t.Error("发送通道未初始化")
	}
	
	if client.ctx == nil {
		t.Error("上下文未初始化")
	}
}

// TestClientStartStop 测试客户端启动和停止
func TestClientStartStop(t *testing.T) {
	// 创建模拟服务器
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建模拟服务器失败: %v", err)
	}
	defer listener.Close()
	
	serverAddr := listener.Addr().String()
	
	// 启动模拟服务器
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			// 保持连接短暂时间然后关闭
			time.Sleep(100 * time.Millisecond)
			conn.Close()
		}
	}()
	
	client := NewClient(serverAddr)
	
	err = client.Start()
	if err != nil {
		t.Fatalf("启动客户端失败: %v", err)
	}
	
	// 等待客户端尝试连接
	time.Sleep(300 * time.Millisecond)
	
	// 停止客户端
	err = client.Stop()
	if err != nil {
		t.Errorf("停止客户端失败: %v", err)
	}
}

// TestClientTunnelMessage 测试客户端隧道消息处理
func TestClientTunnelMessage(t *testing.T) {
	client := NewClient("127.0.0.1:9000")
	
	// 创建测试消息
	data := []byte("test data")
	msg := &TunnelMessage{
		Version: ProtocolVersion,
		Type:    MsgTypeData,
		Length:  uint32(len(data)),
		Data:    data,
	}
	
	// 创建模拟连接
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	
	// 测试写入消息
	go func() {
		err := client.writeTunnelMessage(serverConn, msg)
		if err != nil {
			t.Errorf("写入隧道消息失败: %v", err)
		}
	}()
	
	// 测试读取消息
	receivedMsg, err := client.readTunnelMessage(clientConn)
	if err != nil {
		t.Fatalf("读取隧道消息失败: %v", err)
	}
	
	// 验证消息内容
	if receivedMsg.Version != msg.Version {
		t.Errorf("版本不匹配，期望 %d，得到 %d", msg.Version, receivedMsg.Version)
	}
	
	if receivedMsg.Type != msg.Type {
		t.Errorf("类型不匹配，期望 %d，得到 %d", msg.Type, receivedMsg.Type)
	}
	
	if receivedMsg.Length != msg.Length {
		t.Errorf("长度不匹配，期望 %d，得到 %d", msg.Length, receivedMsg.Length)
	}
	
	if string(receivedMsg.Data) != string(msg.Data) {
		t.Errorf("数据不匹配，期望 %s，得到 %s", string(msg.Data), string(receivedMsg.Data))
	}
}

// TestClientHandleConnectRequest 测试客户端处理连接请求
func TestClientHandleConnectRequest(t *testing.T) {
	// 启动本地测试服务
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("启动本地测试服务失败: %v", err)
	}
	defer listener.Close()
	
	localPort := listener.Addr().(*net.TCPAddr).Port
	
	// 启动echo服务
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c) // echo服务
			}(conn)
		}
	}()
	
	client := NewClient("127.0.0.1:9000")
	
	// 创建连接请求消息
	connID := uint32(12345)
	reqData := make([]byte, 6)
	binary.BigEndian.PutUint32(reqData[0:4], connID)
	binary.BigEndian.PutUint16(reqData[4:6], uint16(localPort))
	
	msg := &TunnelMessage{
		Version: ProtocolVersion,
		Type:    MsgTypeConnectRequest,
		Length:  6,
		Data:    reqData,
	}
	
	// 创建模拟服务器连接
	serverConn, clientServerConn := net.Pipe()
	defer serverConn.Close()
	defer clientServerConn.Close()
	
	// 启动发送处理
	go func() {
		for {
			select {
			case msg := <-client.sendChan:
				client.writeTunnelMessage(clientServerConn, msg)
			case <-time.After(time.Second):
				return
			}
		}
	}()
	
	// 处理连接请求
	client.handleTunnelMessage(msg)
	
	// 等待连接建立
	time.Sleep(100 * time.Millisecond)
	
	// 验证连接是否建立
	client.connMu.RLock()
	_, exists := client.connections[connID]
	client.connMu.RUnlock()
	
	if !exists {
		t.Error("连接未建立")
	}
	
	// 读取连接响应
	response, err := client.readTunnelMessage(serverConn)
	if err != nil {
		t.Fatalf("读取连接响应失败: %v", err)
	}
	
	if response.Type != MsgTypeConnectResponse {
		t.Errorf("响应类型不正确，期望 %d，得到 %d", MsgTypeConnectResponse, response.Type)
	}
	
	if len(response.Data) < 5 {
		t.Fatal("响应数据太短")
	}
	
	responseConnID := binary.BigEndian.Uint32(response.Data[0:4])
	status := response.Data[4]
	
	if responseConnID != connID {
		t.Errorf("响应连接ID不匹配，期望 %d，得到 %d", connID, responseConnID)
	}
	
	if status != ConnStatusSuccess {
		t.Errorf("连接状态不正确，期望 %d，得到 %d", ConnStatusSuccess, status)
	}
	
	// 清理连接
	client.closeConnection(connID)
}

// TestClientHandleDataMessage 测试客户端处理数据消息
func TestClientHandleDataMessage(t *testing.T) {
	client := NewClient("127.0.0.1:9000")
	
	// 创建模拟本地连接
	localConn, remoteConn := net.Pipe()
	defer localConn.Close()
	defer remoteConn.Close()
	
	connID := uint32(12345)
	connection := &LocalConnection{
		ID:         connID,
		TargetAddr: "127.0.0.1:8080",
		Conn:       localConn,
		closeChan:  make(chan struct{}),
	}
	
	client.connMu.Lock()
	client.connections[connID] = connection
	client.connMu.Unlock()
	
	// 创建数据消息
	testData := []byte("Hello, World!")
	dataMsg := make([]byte, 4+len(testData))
	binary.BigEndian.PutUint32(dataMsg[0:4], connID)
	copy(dataMsg[4:], testData)
	
	msg := &TunnelMessage{
		Version: ProtocolVersion,
		Type:    MsgTypeData,
		Length:  uint32(len(dataMsg)),
		Data:    dataMsg,
	}
	
	// 处理数据消息
	go client.handleTunnelMessage(msg)
	
	// 从远程连接读取数据
	buffer := make([]byte, len(testData))
	remoteConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := remoteConn.Read(buffer)
	if err != nil {
		t.Fatalf("读取数据失败: %v", err)
	}
	
	if n != len(testData) {
		t.Errorf("数据长度不匹配，期望 %d，得到 %d", len(testData), n)
	}
	
	if string(buffer[:n]) != string(testData) {
		t.Errorf("数据内容不匹配，期望 %s，得到 %s", string(testData), string(buffer[:n]))
	}
	
	// 清理连接
	client.closeConnection(connID)
}

// TestClientHandleCloseMessage 测试客户端处理关闭消息
func TestClientHandleCloseMessage(t *testing.T) {
	client := NewClient("127.0.0.1:9000")
	
	// 创建模拟本地连接
	localConn, _ := net.Pipe()
	defer localConn.Close()
	
	connID := uint32(12345)
	connection := &LocalConnection{
		ID:         connID,
		TargetAddr: "127.0.0.1:8080",
		Conn:       localConn,
		closeChan:  make(chan struct{}),
	}
	
	client.connMu.Lock()
	client.connections[connID] = connection
	client.connMu.Unlock()
	
	// 创建关闭消息
	closeData := make([]byte, 4)
	binary.BigEndian.PutUint32(closeData, connID)
	
	msg := &TunnelMessage{
		Version: ProtocolVersion,
		Type:    MsgTypeClose,
		Length:  4,
		Data:    closeData,
	}
	
	// 处理关闭消息
	client.handleTunnelMessage(msg)
	
	// 等待连接关闭
	time.Sleep(100 * time.Millisecond)
	
	// 验证连接是否被移除
	client.connMu.RLock()
	_, exists := client.connections[connID]
	client.connMu.RUnlock()
	
	if exists {
		t.Error("连接未被移除")
	}
}

// TestClientKeepAlive 测试客户端心跳处理
func TestClientKeepAlive(t *testing.T) {
	client := NewClient("127.0.0.1:9000")
	
	// 创建模拟连接
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	
	// 启动发送处理
	go func() {
		for {
			select {
			case msg := <-client.sendChan:
				client.writeTunnelMessage(clientConn, msg)
			case <-time.After(time.Second):
				return
			}
		}
	}()
	
	// 创建心跳消息
	keepAliveMsg := &TunnelMessage{
		Version: ProtocolVersion,
		Type:    MsgTypeKeepAlive,
		Length:  0,
		Data:    nil,
	}
	
	// 处理心跳消息
	client.handleTunnelMessage(keepAliveMsg)
	
	// 读取心跳响应
	response, err := client.readTunnelMessage(serverConn)
	if err != nil {
		t.Fatalf("读取心跳响应失败: %v", err)
	}
	
	if response.Type != MsgTypeKeepAlive {
		t.Errorf("心跳响应类型不正确，期望 %d，得到 %d", MsgTypeKeepAlive, response.Type)
	}
}

// TestClientReconnect 测试客户端重连功能
func TestClientReconnect(t *testing.T) {
	// 这个测试比较复杂，需要模拟服务器关闭和重启
	// 简化版本只测试客户端的创建和基本功能
	client := NewClient("127.0.0.1:19999") // 使用不存在的端口
	
	err := client.Start()
	if err != nil {
		t.Fatalf("启动客户端失败: %v", err)
	}
	
	// 等待客户端尝试连接
	time.Sleep(200 * time.Millisecond)
	
	err = client.Stop()
	if err != nil {
		t.Errorf("停止客户端失败: %v", err)
	}
}

// TestClientConcurrentConnections 测试客户端并发连接处理
func TestClientConcurrentConnections(t *testing.T) {
	client := NewClient("127.0.0.1:9000")
	
	var wg sync.WaitGroup
	connCount := 5
	
	// 创建多个模拟连接
	for i := 0; i < connCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			// 创建模拟本地连接
			localConn, _ := net.Pipe()
			defer localConn.Close()
			
			connID := uint32(id + 1)
			connection := &LocalConnection{
				ID:         connID,
				TargetAddr: "127.0.0.1:8080",
				Conn:       localConn,
				closeChan:  make(chan struct{}),
			}
			
			client.connMu.Lock()
			client.connections[connID] = connection
			client.connMu.Unlock()
			
			// 保持连接一段时间
			time.Sleep(100 * time.Millisecond)
			
			// 清理连接
			client.closeConnection(connID)
		}(i)
	}
	
	wg.Wait()
	
	// 验证所有连接都被清理
	client.connMu.RLock()
	remainingConns := len(client.connections)
	client.connMu.RUnlock()
	
	if remainingConns != 0 {
		t.Errorf("还有 %d 个连接未清理", remainingConns)
	}
}

// TestProtocolConstants 测试协议常量
func TestProtocolConstants(t *testing.T) {
	if ProtocolVersion != 0x01 {
		t.Errorf("协议版本不正确，期望 0x01，得到 0x%02x", ProtocolVersion)
	}
	
	if HeaderSize != 6 {
		t.Errorf("消息头大小不正确，期望 6，得到 %d", HeaderSize)
	}
	
	if MaxPacketSize != 1024*1024 {
		t.Errorf("最大包大小不正确，期望 %d，得到 %d", 1024*1024, MaxPacketSize)
	}
	
	// 验证消息类型常量
	expectedTypes := map[string]byte{
		"MsgTypeConnectRequest":  MsgTypeConnectRequest,
		"MsgTypeConnectResponse": MsgTypeConnectResponse,
		"MsgTypeData":           MsgTypeData,
		"MsgTypeClose":          MsgTypeClose,
		"MsgTypeKeepAlive":      MsgTypeKeepAlive,
	}
	
	for name, value := range expectedTypes {
		if value == 0 {
			t.Errorf("消息类型 %s 未定义", name)
		}
	}
	
	// 验证连接状态常量
	if ConnStatusSuccess != 0x00 {
		t.Errorf("连接成功状态不正确，期望 0x00，得到 0x%02x", ConnStatusSuccess)
	}
	
	if ConnStatusFailed != 0x01 {
		t.Errorf("连接失败状态不正确，期望 0x01，得到 0x%02x", ConnStatusFailed)
	}
}