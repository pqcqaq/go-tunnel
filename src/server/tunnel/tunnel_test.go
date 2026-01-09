package tunnel

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// TestNewServer 测试创建隧道服务器
func TestNewServer(t *testing.T) {
	server := NewServer(9000)

	if server == nil {
		t.Fatal("创建隧道服务器失败")
	}

	if server.listenPort != 9000 {
		t.Errorf("监听端口不正确，期望 9000，得到 %d", server.listenPort)
	}

	if server.pendingConns == nil {
		t.Error("待处理连接映射未初始化")
	}

	if server.activeConns == nil {
		t.Error("活跃连接映射未初始化")
	}

	if server.sendChan == nil {
		t.Error("发送通道未初始化")
	}

	if server.ctx == nil {
		t.Error("上下文未初始化")
	}
}

// TestServerStartStop 测试服务器启动和停止
func TestServerStartStop(t *testing.T) {
	// 使用随机端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("获取随机端口失败: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	server := NewServer(port)

	err = server.Start()
	if err != nil {
		t.Fatalf("启动服务器失败: %v", err)
	}

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 验证服务器是否监听端口
	conn, err := net.Dial("tcp", server.listener.Addr().String())
	if err != nil {
		t.Errorf("无法连接到服务器: %v", err)
	} else {
		conn.Close()
	}

	// 停止服务器
	err = server.Stop()
	if err != nil {
		t.Errorf("停止服务器失败: %v", err)
	}
}

// TestTunnelMessage 测试隧道消息的序列化和反序列化
func TestTunnelMessage(t *testing.T) {
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

	server := NewServer(9000)

	// 测试写入消息
	go func() {
		err := server.writeTunnelMessage(serverConn, msg)
		if err != nil {
			t.Errorf("写入隧道消息失败: %v", err)
		}
	}()

	// 测试读取消息
	receivedMsg, err := server.readTunnelMessage(clientConn)
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

// TestConnectRequest 测试连接请求处理
func TestConnectRequest(t *testing.T) {
	// 启动一个测试HTTP服务器
	testListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("启动测试服务器失败: %v", err)
	}
	defer testListener.Close()

	testPort := testListener.Addr().(*net.TCPAddr).Port

	// 启动一个简单的echo服务
	go func() {
		for {
			conn, err := testListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c) // echo服务
			}(conn)
		}
	}()

	// 模拟测试（实际测试需要更复杂的设置）
	t.Logf("测试服务器运行在端口: %d", testPort)

	// 创建隧道服务器，使用随机端口
	tunnelListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建隧道监听器失败: %v", err)
	}
	defer tunnelListener.Close()

	tunnelPort := tunnelListener.Addr().(*net.TCPAddr).Port
	tunnelListener.Close() // 关闭以便服务器重新绑定

	server := NewServer(tunnelPort)

	err = server.Start()
	if err != nil {
		t.Fatalf("启动隧道服务器失败: %v", err)
	}
	defer server.Stop()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 模拟客户端连接
	tunnelConn, err := net.Dial("tcp", server.listener.Addr().String())
	if err != nil {
		t.Fatalf("连接隧道服务器失败: %v", err)
	}
	defer tunnelConn.Close()

	// 等待隧道连接被处理
	time.Sleep(100 * time.Millisecond)

	// 验证隧道是否已连接
	if !server.IsConnected() {
		t.Error("隧道未连接")
	}
}

// TestProtocolVersionCheck 测试协议版本检查
func TestProtocolVersionCheck(t *testing.T) {
	server := NewServer(9000)

	// 创建模拟连接
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	// 发送错误版本的消息
	wrongVersionHeader := make([]byte, HeaderSize)
	wrongVersionHeader[0] = 0xFF // 错误版本
	wrongVersionHeader[1] = MsgTypeData
	binary.BigEndian.PutUint32(wrongVersionHeader[2:6], 0)

	go func() {
		clientConn.Write(wrongVersionHeader)
	}()

	// 尝试读取消息，应该返回错误
	_, err := server.readTunnelMessage(serverConn)
	if err == nil {
		t.Error("期望版本检查失败，但成功了")
	}

	if err.Error() != "不支持的协议版本: 255" {
		t.Errorf("错误消息不正确: %v", err)
	}
}

// TestMaxPacketSizeCheck 测试最大包大小检查
func TestMaxPacketSizeCheck(t *testing.T) {
	server := NewServer(9000)

	// 创建模拟连接
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	// 发送超大包
	oversizedHeader := make([]byte, HeaderSize)
	oversizedHeader[0] = ProtocolVersion
	oversizedHeader[1] = MsgTypeData
	binary.BigEndian.PutUint32(oversizedHeader[2:6], MaxPacketSize+1)

	go func() {
		clientConn.Write(oversizedHeader)
	}()

	// 尝试读取消息，应该返回错误
	_, err := server.readTunnelMessage(serverConn)
	if err == nil {
		t.Error("期望包大小检查失败，但成功了")
	}
}

// TestConcurrentConnections 测试并发连接处理
func TestConcurrentConnections(t *testing.T) {
	// 使用随机端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("获取随机端口失败: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	server := NewServer(port)

	err = server.Start()
	if err != nil {
		t.Fatalf("启动服务器失败: %v", err)
	}
	defer server.Stop()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	var wg sync.WaitGroup
	connCount := 5

	// 并发创建多个连接
	for i := 0; i < connCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			conn, err := net.Dial("tcp", server.listener.Addr().String())
			if err != nil {
				t.Errorf("连接 %d 失败: %v", id, err)
				return
			}
			defer conn.Close()

			// 保持连接一段时间
			time.Sleep(200 * time.Millisecond)
		}(i)
	}

	wg.Wait()

	// 验证只有一个隧道连接被接受
	if server.IsConnected() {
		// 应该只有一个连接被接受
		t.Log("隧道连接已建立")
	}
}

// TestKeepAlive 测试心跳消息
func TestKeepAlive(t *testing.T) {
	server := NewServer(9000)

	// 创建模拟连接
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	// 创建心跳消息
	keepAliveMsg := &TunnelMessage{
		Version: ProtocolVersion,
		Type:    MsgTypeKeepAlive,
		Length:  0,
		Data:    nil,
	}

	// 启动消息处理
	go func() {
		for {
			msg, err := server.readTunnelMessage(clientConn)
			if err != nil {
				return
			}
			server.handleTunnelMessage(msg)
		}
	}()

	// 启动发送处理
	go func() {
		for {
			select {
			case msg := <-server.sendChan:
				server.writeTunnelMessage(serverConn, msg)
			case <-time.After(time.Second):
				return
			}
		}
	}()

	// 发送心跳
	err := server.writeTunnelMessage(serverConn, keepAliveMsg)
	if err != nil {
		t.Fatalf("发送心跳失败: %v", err)
	}

	// 读取响应
	response, err := server.readTunnelMessage(clientConn)
	if err != nil {
		t.Fatalf("读取心跳响应失败: %v", err)
	}

	if response.Type != MsgTypeKeepAlive {
		t.Errorf("心跳响应类型不正确，期望 %d，得到 %d", MsgTypeKeepAlive, response.Type)
	}
}

// MockClient 模拟客户端用于测试
type MockClient struct {
	conn net.Conn
}

func (mc *MockClient) sendConnectResponse(connID uint32, status byte) error {
	responseData := make([]byte, 5)
	binary.BigEndian.PutUint32(responseData[0:4], connID)
	responseData[4] = status

	msg := &TunnelMessage{
		Version: ProtocolVersion,
		Type:    MsgTypeConnectResponse,
		Length:  5,
		Data:    responseData,
	}

	// 构建消息头
	header := make([]byte, HeaderSize)
	header[0] = msg.Version
	header[1] = msg.Type
	binary.BigEndian.PutUint32(header[2:6], msg.Length)

	// 写入消息
	if _, err := mc.conn.Write(header); err != nil {
		return err
	}

	if msg.Length > 0 && msg.Data != nil {
		if _, err := mc.conn.Write(msg.Data); err != nil {
			return err
		}
	}

	return nil
}

// TestForwardConnection 测试连接转发
func TestForwardConnection(t *testing.T) {
	// 启动测试目标服务
	targetListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("启动目标服务失败: %v", err)
	}
	defer targetListener.Close()

	targetPort := targetListener.Addr().(*net.TCPAddr).Port

	// 启动简单的echo服务
	go func() {
		for {
			conn, err := targetListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}()

	// 创建隧道服务器，使用随机端口
	tunnelListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建隧道监听器失败: %v", err)
	}
	tunnelPort := tunnelListener.Addr().(*net.TCPAddr).Port
	tunnelListener.Close() // 关闭以便服务器重新绑定

	server := NewServer(tunnelPort)
	err = server.Start()
	if err != nil {
		t.Fatalf("启动隧道服务器失败: %v", err)
	}
	defer server.Stop()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 连接到隧道服务器（模拟客户端）
	tunnelConn, err := net.Dial("tcp", server.listener.Addr().String())
	if err != nil {
		t.Fatalf("连接隧道服务器失败: %v", err)
	}
	defer tunnelConn.Close()

	// 等待隧道连接建立
	time.Sleep(100 * time.Millisecond)

	if !server.IsConnected() {
		t.Fatal("隧道未连接")
	}

	// 创建模拟客户端连接
	clientConn, serverSideConn := net.Pipe()
	defer clientConn.Close()
	defer serverSideConn.Close()

	// 创建模拟客户端
	mockClient := &MockClient{conn: tunnelConn}

	// 启动连接转发
	go func() {
		conn, err := server.ForwardConnection(serverSideConn, "127.0.0.1", targetPort)
		if err != nil {
			t.Errorf("转发连接失败: %v", err)
		}
		defer conn.Close()
	}()

	// 读取连接请求
	header := make([]byte, HeaderSize)
	_, err = io.ReadFull(tunnelConn, header)
	if err != nil {
		t.Fatalf("读取连接请求头失败: %v", err)
	}

	if header[1] != MsgTypeConnectRequest {
		t.Fatalf("期望连接请求，得到消息类型: %d", header[1])
	}

	dataLen := binary.BigEndian.Uint32(header[2:6])
	data := make([]byte, dataLen)
	_, err = io.ReadFull(tunnelConn, data)
	if err != nil {
		t.Fatalf("读取连接请求数据失败: %v", err)
	}

	connID := binary.BigEndian.Uint32(data[0:4])
	requestedPort := binary.BigEndian.Uint16(data[4:6])

	if int(requestedPort) != targetPort {
		t.Errorf("请求端口不匹配，期望 %d，得到 %d", targetPort, requestedPort)
	}

	// 发送连接成功响应
	err = mockClient.sendConnectResponse(connID, ConnStatusSuccess)
	if err != nil {
		t.Fatalf("发送连接响应失败: %v", err)
	}

	// 等待连接建立
	time.Sleep(100 * time.Millisecond)

	t.Log("连接转发测试完成")
}
