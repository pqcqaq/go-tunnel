package forwarder

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// Forwarder 端口转发器
type Forwarder struct {
	sourcePort  int
	targetAddr  string
	listener    net.Listener
	cancel      context.CancelFunc
	ctx         context.Context
	wg          sync.WaitGroup
	tunnelConn  net.Conn
	useTunnel   bool
	mu          sync.RWMutex
}

// NewForwarder 创建新的端口转发器
func NewForwarder(sourcePort int, targetIP string, targetPort int) *Forwarder {
	ctx, cancel := context.WithCancel(context.Background())
	return &Forwarder{
		sourcePort: sourcePort,
		targetAddr: fmt.Sprintf("%s:%d", targetIP, targetPort),
		cancel:     cancel,
		ctx:        ctx,
		useTunnel:  false,
	}
}

// NewTunnelForwarder 创建使用隧道的端口转发器
func NewTunnelForwarder(sourcePort int, targetPort int, tunnelConn net.Conn) *Forwarder {
	ctx, cancel := context.WithCancel(context.Background())
	return &Forwarder{
		sourcePort: sourcePort,
		targetAddr: fmt.Sprintf("127.0.0.1:%d", targetPort),
		tunnelConn: tunnelConn,
		useTunnel:  true,
		cancel:     cancel,
		ctx:        ctx,
	}
}

// Start 启动端口转发
func (f *Forwarder) Start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", f.sourcePort))
	if err != nil {
		return fmt.Errorf("监听端口 %d 失败: %w", f.sourcePort, err)
	}

	f.listener = listener
	log.Printf("端口转发启动: %d -> %s (tunnel: %v)", f.sourcePort, f.targetAddr, f.useTunnel)

	f.wg.Add(1)
	go f.acceptLoop()

	return nil
}

// acceptLoop 接受连接循环
func (f *Forwarder) acceptLoop() {
	defer f.wg.Done()

	for {
		select {
		case <-f.ctx.Done():
			return
		default:
		}

		// 设置接受超时，避免阻塞关闭
		f.listener.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second))
		
		conn, err := f.listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-f.ctx.Done():
				return
			default:
				log.Printf("接受连接失败 (端口 %d): %v", f.sourcePort, err)
				continue
			}
		}

		f.wg.Add(1)
		go f.handleConnection(conn)
	}
}

// handleConnection 处理单个连接
func (f *Forwarder) handleConnection(clientConn net.Conn) {
	defer f.wg.Done()
	defer clientConn.Close()

	var targetConn net.Conn
	var err error

	if f.useTunnel {
		// 使用隧道连接
		f.mu.RLock()
		targetConn = f.tunnelConn
		f.mu.RUnlock()

		if targetConn == nil {
			log.Printf("隧道连接不可用 (端口 %d)", f.sourcePort)
			return
		}
	} else {
		// 直接连接目标
		dialer := &net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		
		targetConn, err = dialer.DialContext(f.ctx, "tcp", f.targetAddr)
		if err != nil {
			log.Printf("连接目标失败 (端口 %d -> %s): %v", f.sourcePort, f.targetAddr, err)
			return
		}
		defer targetConn.Close()
	}

	// 双向转发
	errChan := make(chan error, 2)

	// 客户端 -> 目标
	go func() {
		_, err := io.Copy(targetConn, clientConn)
		errChan <- err
	}()

	// 目标 -> 客户端
	go func() {
		_, err := io.Copy(clientConn, targetConn)
		errChan <- err
	}()

	// 等待任一方向完成或出错
	select {
	case <-errChan:
		// 连接已关闭或出错
	case <-f.ctx.Done():
		// 转发器被停止
	}
}

// Stop 停止端口转发
func (f *Forwarder) Stop() error {
	f.cancel()
	
	if f.listener != nil {
		if err := f.listener.Close(); err != nil {
			log.Printf("关闭监听器失败 (端口 %d): %v", f.sourcePort, err)
		}
	}

	// 等待所有连接处理完成（最多等待5秒）
	done := make(chan struct{})
	go func() {
		f.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("端口转发已停止: %d", f.sourcePort)
	case <-time.After(5 * time.Second):
		log.Printf("端口转发停止超时: %d", f.sourcePort)
	}

	return nil
}

// SetTunnelConn 设置隧道连接
func (f *Forwarder) SetTunnelConn(conn net.Conn) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tunnelConn = conn
}

// Manager 转发器管理器
type Manager struct {
	forwarders map[int]*Forwarder
	mu         sync.RWMutex
}

// NewManager 创建新的转发器管理器
func NewManager() *Manager {
	return &Manager{
		forwarders: make(map[int]*Forwarder),
	}
}

// Add 添加并启动转发器
func (m *Manager) Add(sourcePort int, targetIP string, targetPort int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.forwarders[sourcePort]; exists {
		return fmt.Errorf("端口 %d 已被占用", sourcePort)
	}

	forwarder := NewForwarder(sourcePort, targetIP, targetPort)
	if err := forwarder.Start(); err != nil {
		return err
	}

	m.forwarders[sourcePort] = forwarder
	return nil
}

// AddTunnel 添加使用隧道的转发器
func (m *Manager) AddTunnel(sourcePort int, targetPort int, tunnelConn net.Conn) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.forwarders[sourcePort]; exists {
		return fmt.Errorf("端口 %d 已被占用", sourcePort)
	}

	forwarder := NewTunnelForwarder(sourcePort, targetPort, tunnelConn)
	if err := forwarder.Start(); err != nil {
		return err
	}

	m.forwarders[sourcePort] = forwarder
	return nil
}

// Remove 移除并停止转发器
func (m *Manager) Remove(sourcePort int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	forwarder, exists := m.forwarders[sourcePort]
	if !exists {
		return fmt.Errorf("端口 %d 的转发器不存在", sourcePort)
	}

	if err := forwarder.Stop(); err != nil {
		return err
	}

	delete(m.forwarders, sourcePort)
	return nil
}

// Exists 检查转发器是否存在
func (m *Manager) Exists(sourcePort int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.forwarders[sourcePort]
	return exists
}

// StopAll 停止所有转发器
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for port, forwarder := range m.forwarders {
		if err := forwarder.Stop(); err != nil {
			log.Printf("停止端口 %d 的转发器失败: %v", port, err)
		}
	}

	m.forwarders = make(map[int]*Forwarder)
}