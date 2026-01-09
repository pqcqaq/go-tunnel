package forwarder

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"port-forward/server/stats"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// TunnelServer 隧道服务器接口
type TunnelServer interface {
	ForwardConnection(clientConn net.Conn, targetIP string, targetPort int) (net.Conn, error)
	IsConnected() bool
	GetTrafficStats() stats.TrafficStats
}

// Forwarder 端口转发器
type Forwarder struct {
	sourcePort   int
	targetPort   int
	targetHost   string // 支持IP或域名
	listener     net.Listener
	cancel       context.CancelFunc
	ctx          context.Context
	wg           sync.WaitGroup
	tunnelServer TunnelServer
	useTunnel    bool
	limit        *int64

	// 流量统计（使用原子操作）
	bytesSent     uint64        // 发送字节数
	bytesReceived uint64        // 接收字节数
	limiterOut    *rate.Limiter // 限速器（出方向）
	limiterIn     *rate.Limiter // 限速器（入方向）
}

// NewForwarder 创建新的端口转发器
func NewForwarder(sourcePort int, targetHost string, targetPort int, limit *int64) *Forwarder {
	ctx, cancel := context.WithCancel(context.Background())
	var limiterOut, limiterIn *rate.Limiter
	if limit != nil {
		// burst设置为1秒的流量，这样可以平滑处理突发
		// 同时不会一次性消耗太多令牌
		burst := int(*limit) / 100
		if burst < 10240 {
			burst = 10240 // 最小burst为10KB
		}
		log.Printf("设置限速: %d bytes/sec, burst: %d bytes", *limit, burst)
		limiterOut = rate.NewLimiter(rate.Limit(*limit), burst)
		limiterIn = rate.NewLimiter(rate.Limit(*limit), burst)
	}
	return &Forwarder{
		sourcePort: sourcePort,
		targetPort: targetPort,
		targetHost: targetHost,
		cancel:     cancel,
		ctx:        ctx,
		useTunnel:  false,
		limit:      limit,
		limiterOut: limiterOut,
		limiterIn:  limiterIn,
	}
}

// NewTunnelForwarder 创建使用隧道的端口转发器
func NewTunnelForwarder(sourcePort int, targetHost string, targetPort int, tunnelServer TunnelServer, limit *int64) *Forwarder {
	ctx, cancel := context.WithCancel(context.Background())
	var limiterOut, limiterIn *rate.Limiter
	if limit != nil {
		// burst设置为1秒的流量，这样可以平滑处理突发
		// 同时不会一次性消耗太多令牌
		burst := int(*limit) / 100
		if burst < 10240 {
			burst = 10240 // 最小burst为10KB
		}
		log.Printf("设置限速: %d bytes/sec, burst: %d bytes", *limit, burst)
		limiterOut = rate.NewLimiter(rate.Limit(*limit), burst)
		limiterIn = rate.NewLimiter(rate.Limit(*limit), burst)
	}
	return &Forwarder{
		sourcePort:   sourcePort,
		targetPort:   targetPort,
		targetHost:   targetHost,
		tunnelServer: tunnelServer,
		useTunnel:    true,
		cancel:       cancel,
		ctx:          ctx,
		limit:        limit,
		limiterOut:   limiterOut,
		limiterIn:    limiterIn,
	}
}

// Start 启动端口转发
func (f *Forwarder) Start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", f.sourcePort))
	if err != nil {
		return fmt.Errorf("监听端口 %d 失败: %w", f.sourcePort, err)
	}

	f.listener = listener
	log.Printf("端口转发启动: %d -> %s:%d (tunnel: %v)", f.sourcePort, f.targetHost, f.targetPort, f.useTunnel)

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

type rateLimitedReader struct {
	r       io.Reader
	limiter *rate.Limiter
	ctx     context.Context
}

func (rlr *rateLimitedReader) Read(p []byte) (int, error) {
	if rlr.limiter == nil {
		return rlr.r.Read(p)
	}

	// 使用更小的块大小以实现更平滑的限流
	// 2KB是一个合理的值，既不会太频繁调用，也能保持流量平滑
	chunkSize := 2048

	// 不超过缓冲区大小
	if len(p) < chunkSize {
		chunkSize = len(p)
	}

	// 预先申请令牌
	if err := rlr.limiter.WaitN(rlr.ctx, chunkSize); err != nil {
		return 0, err
	}

	// 限制实际读取大小
	if len(p) > chunkSize {
		p = p[:chunkSize]
	}

	// 执行读取
	n, err := rlr.r.Read(p)

	// 如果实际读取少于申请的令牌，归还多余的令牌
	// 注意：rate.Limiter 不支持归还令牌，所以这里只能接受这个损耗
	// 或者改用先读后限的方式

	return n, err
}

// countingWriter 带统计的 Writer
type countingWriter struct {
	w       io.Writer
	counter *uint64
	port    int
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	if n > 0 {
		atomic.AddUint64(cw.counter, uint64(n))
	}
	return n, err
}

// handleConnection 处理单个连接
func (f *Forwarder) handleConnection(clientConn net.Conn) {
	defer f.wg.Done()
	defer clientConn.Close()

	log.Printf("端口 %d 收到新连接: %s", f.sourcePort, clientConn.RemoteAddr())

	var targetConn net.Conn
	var err error

	if f.useTunnel {
		// 使用隧道转发，获取 TunnelConn
		if f.tunnelServer == nil || !f.tunnelServer.IsConnected() {
			log.Printf("隧道服务器不可用 (端口 %d)", f.sourcePort)
			return
		}

		// 获取隧道连接
		targetConn, err = f.tunnelServer.ForwardConnection(clientConn, f.targetHost, f.targetPort)
		if err != nil {
			log.Printf("隧道转发失败 (端口 %d -> %s:%d): %v", f.sourcePort, f.targetHost, f.targetPort, err)
			return
		}
	} else {
		// 直接连接目标
		dialer := &net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}

		// 动态解析域名并连接
		targetAddr := fmt.Sprintf("%s:%d", f.targetHost, f.targetPort)
		targetConn, err = dialer.DialContext(f.ctx, "tcp", targetAddr)
		if err != nil {
			log.Printf("连接目标失败 (端口 %d -> %s): %v", f.sourcePort, targetAddr, err)
			return
		}
	}

	defer targetConn.Close()

	// 双向转发
	var wg sync.WaitGroup
	wg.Add(2)

	// 客户端 -> 目标
	go func() {
		defer wg.Done()
		reader := &rateLimitedReader{
			r:       clientConn,
			limiter: f.limiterOut,
			ctx:     f.ctx,
		}
		writer := &countingWriter{
			w:       targetConn,
			counter: &f.bytesSent,
			port:    f.sourcePort,
		}
		n, _ := io.Copy(writer, reader)
		log.Printf("端口 %d: 客户端->目标传输完成，本次发送 %d 字节 (总发送: %d)", f.sourcePort, n, atomic.LoadUint64(&f.bytesSent))
		// 关闭目标连接的写入端，通知对方不会再发送数据
		if tcpConn, ok := targetConn.(*net.TCPConn); ok {
			tcpConn.CloseWrite()
		}
	}()

	// 目标 -> 客户端
	go func() {
		defer wg.Done()
		reader := &rateLimitedReader{
			r:       targetConn,
			limiter: f.limiterIn,
			ctx:     f.ctx,
		}
		writer := &countingWriter{
			w:       clientConn,
			counter: &f.bytesReceived,
			port:    f.sourcePort,
		}
		n, _ := io.Copy(writer, reader)
		log.Printf("端口 %d: 目标->客户端传输完成，本次接收 %d 字节 (总接收: %d)", f.sourcePort, n, atomic.LoadUint64(&f.bytesReceived))
		// 关闭客户端连接的写入端
		if tcpConn, ok := clientConn.(*net.TCPConn); ok {
			tcpConn.CloseWrite()
		}
	}()

	// 创建一个 channel 来等待完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// 等待两个方向都完成或上下文取消
	select {
	case <-done:
		// 两个方向都已完成
	case <-f.ctx.Done():
		// 转发器被停止，连接会在 defer 中关闭
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
func (m *Manager) Add(sourcePort int, targetHost string, targetPort int, limit *int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.forwarders[sourcePort]; exists {
		return fmt.Errorf("端口 %d 已被占用", sourcePort)
	}

	forwarder := NewForwarder(sourcePort, targetHost, targetPort, limit)
	if err := forwarder.Start(); err != nil {
		return err
	}

	m.forwarders[sourcePort] = forwarder
	return nil
}

// AddTunnel 添加使用隧道的转发器
func (m *Manager) AddTunnel(sourcePort int, targetHost string, targetPort int, tunnelServer TunnelServer, limit *int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.forwarders[sourcePort]; exists {
		return fmt.Errorf("端口 %d 已被占用", sourcePort)
	}

	forwarder := NewTunnelForwarder(sourcePort, targetHost, targetPort, tunnelServer, limit)
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

// GetTrafficStats 获取流量统计信息
func (f *Forwarder) GetTrafficStats() stats.TrafficStats {
	return stats.TrafficStats{
		BytesSent:     atomic.LoadUint64(&f.bytesSent),
		BytesReceived: atomic.LoadUint64(&f.bytesReceived),
		LastUpdate:    time.Now().Unix(),
	}
}

// GetAllTrafficStats 获取所有转发器的流量统计
func (m *Manager) GetAllTrafficStats() map[int]stats.TrafficStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statsMap := make(map[int]stats.TrafficStats)
	for port, forwarder := range m.forwarders {
		statsMap[port] = forwarder.GetTrafficStats()
	}

	return statsMap
}
