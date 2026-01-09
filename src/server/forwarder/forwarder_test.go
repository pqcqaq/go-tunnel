package forwarder

import (
	"fmt"
	"io"
	"net"
	"port-forward/server/stats"
	"testing"
	"time"
)

// TestNewForwarder 测试创建转发器
func TestNewForwarder(t *testing.T) {
	fwd := NewForwarder(8080, "192.168.1.100", 80, nil, nil, nil)

	if fwd == nil {
		t.Fatal("创建转发器失败")
	}

	if fwd.sourcePort != 8080 {
		t.Errorf("源端口不正确，期望 8080，得到 %d", fwd.sourcePort)
	}

	if fwd.targetHost != "192.168.1.100" {
		t.Errorf("目标主机不正确，期望 192.168.1.100，得到 %s", fwd.targetHost)
	}

	if fwd.targetPort != 80 {
		t.Errorf("目标端口不正确，期望 80，得到 %d", fwd.targetPort)
	}

	if fwd.useTunnel {
		t.Error("普通转发器不应使用隧道")
	}
}

// mockTunnelServer 模拟隧道服务器
type mockTunnelServer struct {
	connected bool
}

func (m *mockTunnelServer) ForwardConnection(clientConn net.Conn, targetIp string, targetPort int) (net.Conn, error) {
	// 简单的模拟实现
	defer clientConn.Close()
	return nil, nil
}

func (m *mockTunnelServer) IsConnected() bool {
	return m.connected
}

func (m *mockTunnelServer) GetTrafficStats() stats.TrafficStats {
	return stats.TrafficStats{}
}

// TestNewTunnelForwarder 测试创建隧道转发器
func TestNewTunnelForwarder(t *testing.T) {
	// 创建模拟隧道服务器
	mockServer := &mockTunnelServer{connected: true}

	fwd := NewTunnelForwarder(8080, "127.0.0.1", 80, mockServer, nil, nil, nil)

	if fwd == nil {
		t.Fatal("创建隧道转发器失败")
	}

	if !fwd.useTunnel {
		t.Error("隧道转发器应使用隧道")
	}

	if fwd.tunnelServer == nil {
		t.Error("隧道服务器未设置")
	}
}

// TestForwarderStartStop 测试转发器启动和停止
func TestForwarderStartStop(t *testing.T) {
	// 创建模拟目标服务器
	targetListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建目标服务器失败: %v", err)
	}
	defer targetListener.Close()

	targetPort := targetListener.Addr().(*net.TCPAddr).Port

	// 启动转发器到一个随机端口
	fwd := NewForwarder(0, "127.0.0.1", targetPort, nil, nil, nil)

	// 创建监听器
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建监听器失败: %v", err)
	}

	fwd.listener = listener
	fwd.sourcePort = listener.Addr().(*net.TCPAddr).Port

	// 启动接受循环
	fwd.wg.Add(1)
	go fwd.acceptLoop()

	// 等待一段时间
	time.Sleep(100 * time.Millisecond)

	// 停止转发器
	err = fwd.Stop()
	if err != nil {
		t.Errorf("停止转发器失败: %v", err)
	}
}

// TestForwarderConnection 测试转发器连接处理
func TestForwarderConnection(t *testing.T) {
	// 创建模拟目标服务器
	targetListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建目标服务器失败: %v", err)
	}
	defer targetListener.Close()

	targetPort := targetListener.Addr().(*net.TCPAddr).Port

	// 在后台处理连接
	go func() {
		conn, err := targetListener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// 回显服务器
		io.Copy(conn, conn)
	}()

	// 创建并启动转发器
	fwd := NewForwarder(0, "127.0.0.1", targetPort, nil, nil, nil)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建监听器失败: %v", err)
	}
	fwd.listener = listener
	fwd.sourcePort = listener.Addr().(*net.TCPAddr).Port

	fwd.wg.Add(1)
	go fwd.acceptLoop()
	defer fwd.Stop()

	time.Sleep(100 * time.Millisecond)

	// 连接到转发器
	client, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", fwd.sourcePort))
	if err != nil {
		t.Fatalf("连接转发器失败: %v", err)
	}
	defer client.Close()

	// 发送数据
	testData := []byte("Hello, World!")
	_, err = client.Write(testData)
	if err != nil {
		t.Fatalf("发送数据失败: %v", err)
	}

	// 读取响应
	buf := make([]byte, len(testData))
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := io.ReadFull(client, buf)
	if err != nil {
		t.Fatalf("读取响应失败: %v", err)
	}

	if n != len(testData) {
		t.Errorf("读取数据长度不正确，期望 %d，得到 %d", len(testData), n)
	}

	if string(buf) != string(testData) {
		t.Errorf("数据不匹配，期望 %s，得到 %s", testData, buf)
	}
}

// TestManagerAdd 测试管理器添加转发器
func TestManagerAdd(t *testing.T) {
	mgr := NewManager()

	// 创建模拟目标服务器
	targetListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建目标服务器失败: %v", err)
	}
	defer targetListener.Close()

	targetPort := targetListener.Addr().(*net.TCPAddr).Port

	// 添加转发器到一个随机可用端口
	fwdListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建转发监听器失败: %v", err)
	}
	sourcePort := fwdListener.Addr().(*net.TCPAddr).Port
	fwdListener.Close() // 关闭以便转发器可以使用这个端口

	err = mgr.Add(sourcePort, "127.0.0.1", targetPort, nil, nil, nil)
	if err != nil {
		t.Fatalf("添加转发器失败: %v", err)
	}

	// 验证转发器已添加
	if !mgr.Exists(sourcePort) {
		t.Error("转发器应该存在")
	}

	// 清理
	mgr.Remove(sourcePort)
}

// TestManagerAddDuplicate 测试添加重复转发器
func TestManagerAddDuplicate(t *testing.T) {
	mgr := NewManager()

	// 获取一个随机端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建监听器失败: %v", err)
	}
	sourcePort := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// 添加第一个转发器
	err = mgr.Add(sourcePort, "127.0.0.1", 80, nil, nil, nil)
	if err != nil {
		t.Fatalf("添加第一个转发器失败: %v", err)
	}
	defer mgr.Remove(sourcePort)

	// 尝试添加重复端口
	err = mgr.Add(sourcePort, "127.0.0.1", 81, nil, nil, nil)
	if err == nil {
		t.Error("应该返回端口已占用错误")
	}
}

// TestManagerRemove 测试移除转发器
func TestManagerRemove(t *testing.T) {
	mgr := NewManager()

	// 获取一个随机端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建监听器失败: %v", err)
	}
	sourcePort := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// 添加转发器
	err = mgr.Add(sourcePort, "127.0.0.1", 80, nil, nil, nil)
	if err != nil {
		t.Fatalf("添加转发器失败: %v", err)
	}

	// 移除转发器
	err = mgr.Remove(sourcePort)
	if err != nil {
		t.Errorf("移除转发器失败: %v", err)
	}

	// 验证转发器已移除
	if mgr.Exists(sourcePort) {
		t.Error("转发器应该已被移除")
	}
}

// TestManagerRemoveNonExistent 测试移除不存在的转发器
func TestManagerRemoveNonExistent(t *testing.T) {
	mgr := NewManager()

	err := mgr.Remove(9999)
	if err == nil {
		t.Error("应该返回转发器不存在错误")
	}
}

// TestManagerExists 测试检查转发器是否存在
func TestManagerExists(t *testing.T) {
	mgr := NewManager()

	// 检查不存在的转发器
	if mgr.Exists(8080) {
		t.Error("转发器不应该存在")
	}

	// 获取一个随机端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建监听器失败: %v", err)
	}
	sourcePort := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// 添加转发器
	err = mgr.Add(sourcePort, "127.0.0.1", 80, nil, nil, nil)
	if err != nil {
		t.Fatalf("添加转发器失败: %v", err)
	}
	defer mgr.Remove(sourcePort)

	// 检查存在的转发器
	if !mgr.Exists(sourcePort) {
		t.Error("转发器应该存在")
	}
}

// TestManagerStopAll 测试停止所有转发器
func TestManagerStopAll(t *testing.T) {
	mgr := NewManager()

	// 添加多个转发器
	ports := make([]int, 0)
	for i := 0; i < 3; i++ {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("创建监听器失败: %v", err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()

		err = mgr.Add(port, "127.0.0.1", 80+i, nil, nil, nil)
		if err != nil {
			t.Fatalf("添加转发器 %d 失败: %v", i, err)
		}
		ports = append(ports, port)
	}

	// 停止所有转发器
	mgr.StopAll()

	// 验证所有转发器已停止
	for _, port := range ports {
		if mgr.Exists(port) {
			t.Errorf("端口 %d 的转发器应该已停止", port)
		}
	}
}

// TestForwarderContextCancellation 测试上下文取消
func TestForwarderContextCancellation(t *testing.T) {
	fwd := NewForwarder(0, "127.0.0.1", 80, nil, nil, nil)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建监听器失败: %v", err)
	}
	fwd.listener = listener

	fwd.wg.Add(1)
	go fwd.acceptLoop()

	// 取消上下文
	fwd.cancel()

	// 等待 goroutine 退出
	done := make(chan struct{})
	go func() {
		fwd.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 成功退出
	case <-time.After(2 * time.Second):
		t.Error("上下文取消后 goroutine 未退出")
	}
}

// BenchmarkForwarderConnection 基准测试转发器连接
func BenchmarkForwarderConnection(b *testing.B) {
	// 创建模拟目标服务器
	targetListener, _ := net.Listen("tcp", "127.0.0.1:0")
	defer targetListener.Close()

	targetPort := targetListener.Addr().(*net.TCPAddr).Port

	// 后台处理连接
	go func() {
		for {
			conn, err := targetListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(io.Discard, c)
			}(conn)
		}
	}()

	// 创建转发器
	fwd := NewForwarder(0, "127.0.0.1", targetPort, nil, nil, nil)
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	fwd.listener = listener
	fwd.sourcePort = listener.Addr().(*net.TCPAddr).Port

	fwd.wg.Add(1)
	go fwd.acceptLoop()
	defer fwd.Stop()

	time.Sleep(100 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", fwd.sourcePort))
		if err != nil {
			b.Fatalf("连接失败: %v", err)
		}
		conn.Write([]byte("test"))
		conn.Close()
	}
}

// BenchmarkManagerOperations 基准测试管理器操作
func BenchmarkManagerOperations(b *testing.B) {
	mgr := NewManager()

	b.Run("Add", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			listener, _ := net.Listen("tcp", "127.0.0.1:0")
			port := listener.Addr().(*net.TCPAddr).Port
			listener.Close()

			mgr.Add(port, "127.0.0.1", 80, nil, nil, nil)
		}
	})

	b.Run("Exists", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			mgr.Exists(8080)
		}
	})
}
