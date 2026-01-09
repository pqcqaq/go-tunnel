package forwarder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"port-forward/server/stats"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/google/uuid"
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

	// 活跃连接管理
	connections map[string]*activeConnection // 活跃连接映射
	connMutex   sync.RWMutex                 // 连接映射锁

	// 访问控制
	accessRule string   // 访问控制规则: "disabled", "whitelist", "blacklist"
	accessIPs  []string // IP列表（CIDR或单个IP）
}

func (f *Forwarder) UpdateBandwidthLimit(limit *int64) {
	if limit == nil || *limit == 0 {
		f.limiterOut = nil
		f.limiterIn = nil
		f.limit = nil
		log.Printf("取消端口 %d 的带宽限制", f.sourcePort)
		return
	}
	f.limit = limit
	// burst设置为1秒的流量，这样可以平滑处理突发
	burst := int(*limit) / 100
	if burst < 10240 {
		burst = 10240 // 最小burst为10KB
	}
	log.Printf("更新端口 %d 的带宽限制: %d bytes/sec, burst: %d bytes", f.sourcePort, *limit, burst)
	f.limiterOut = rate.NewLimiter(rate.Limit(*limit), burst)
	f.limiterIn = rate.NewLimiter(rate.Limit(*limit), burst)
}

func (f *Forwarder) UpdateAccessControl(rule *string, ps *string) {
	if rule == nil || *rule == "disabled" {
		f.accessRule = "disabled"
		f.accessIPs = nil
		log.Printf("禁用端口 %d 的访问控制", f.sourcePort)
		return
	}
	f.accessRule = *rule
	f.accessIPs = parseIPList(ps)
}

// activeConnection 活跃连接信息
type activeConnection struct {
	clientAddr    string
	targetAddr    string
	bytesSent     uint64
	bytesReceived uint64
	connectedAt   int64
}

// parseIPList 解析IP列表JSON字符串为切片
func parseIPList(ipListJSON *string) []string {
	if ipListJSON == nil || *ipListJSON == "" {
		return nil
	}

	var ips []string
	if err := json.Unmarshal([]byte(*ipListJSON), &ips); err != nil {
		log.Printf("解析IP列表失败: %v", err)
		return nil
	}

	return ips
}

// checkIPAllowed 检查IP是否允许访问
func (f *Forwarder) checkIPAllowed(remoteAddr string) bool {
	// 如果规则被禁用，允许所有连接
	if f.accessRule == "" || f.accessRule == "disabled" {
		return true
	}

	// 提取IP地址（去除端口）
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		log.Printf("解析远程地址失败: %v", err)
		return false
	}

	clientIP := net.ParseIP(host)
	if clientIP == nil {
		log.Printf("无效的IP地址: %s", host)
		return false
	}

	// 检查IP是否在列表中
	inList := false
	for _, ipStr := range f.accessIPs {
		// 检查是否为CIDR网段
		if strings.Contains(ipStr, "/") {
			_, ipNet, err := net.ParseCIDR(ipStr)
			if err != nil {
				log.Printf("无效的CIDR: %s", ipStr)
				continue
			}
			if ipNet.Contains(clientIP) {
				inList = true
				break
			}
		} else {
			// 单个IP地址
			ip := net.ParseIP(ipStr)
			if ip != nil && ip.Equal(clientIP) {
				inList = true
				break
			}
		}
	}

	// 根据规则类型返回结果
	if f.accessRule == "whitelist" {
		return inList // 白名单：只允许列表中的IP
	} else if f.accessRule == "blacklist" {
		return !inList // 黑名单：拒绝列表中的IP
	}

	return true
}

// NewForwarder 创建新的端口转发器
func NewForwarder(sourcePort int, targetHost string, targetPort int, limit *int64, accessRule *string, accessIPs *string) *Forwarder {
	ctx, cancel := context.WithCancel(context.Background())
	var limiterOut, limiterIn *rate.Limiter
	if limit != nil && *limit != 0 {
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

	// 解析访问规则
	rule := "disabled"
	if accessRule != nil {
		rule = *accessRule
	}

	return &Forwarder{
		sourcePort:  sourcePort,
		targetPort:  targetPort,
		targetHost:  targetHost,
		cancel:      cancel,
		ctx:         ctx,
		useTunnel:   false,
		limit:       limit,
		limiterOut:  limiterOut,
		limiterIn:   limiterIn,
		connections: make(map[string]*activeConnection),
		accessRule:  rule,
		accessIPs:   parseIPList(accessIPs),
	}
}

// NewTunnelForwarder 创建使用隧道的端口转发器
func NewTunnelForwarder(sourcePort int, targetHost string, targetPort int, tunnelServer TunnelServer, limit *int64, accessRule *string, accessIPs *string) *Forwarder {
	ctx, cancel := context.WithCancel(context.Background())
	var limiterOut, limiterIn *rate.Limiter
	if limit != nil && *limit != 0 {
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

	// 解析访问规则
	rule := "disabled"
	if accessRule != nil {
		rule = *accessRule
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
		connections:  make(map[string]*activeConnection),
		accessRule:   rule,
		accessIPs:    parseIPList(accessIPs),
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
	w           io.Writer
	counter     *uint64
	port        int
	connCounter *uint64 // 连接级别的计数器
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	if n > 0 {
		atomic.AddUint64(cw.counter, uint64(n))
		if cw.connCounter != nil {
			atomic.AddUint64(cw.connCounter, uint64(n))
		}
	}
	return n, err
}

// handleConnection 处理单个连接
func (f *Forwarder) handleConnection(clientConn net.Conn) {
	defer f.wg.Done()
	defer clientConn.Close()

	// 生成连接ID
	connID := uuid.New().String()
	clientAddr := clientConn.RemoteAddr().String()

	// 检查IP访问控制
	if !f.checkIPAllowed(clientAddr) {
		log.Printf("端口 %d 拒绝连接: %s (访问规则: %s, 连接ID: %s)", f.sourcePort, clientAddr, f.accessRule, connID)

		// 白名单模式：返回简单提示（便于调试配置错误）
		// 黑名单模式：静默拒绝（不暴露信息给恶意访问者）
		if f.accessRule == "whitelist" {
			clientConn.Write([]byte("Access denied: IP not in whitelist\n"))
		}

		return
	}

	log.Printf("端口 %d 收到新连接: %s (连接ID: %s)", f.sourcePort, clientAddr, connID)

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

	// 记录活跃连接
	targetAddr := fmt.Sprintf("%s:%d", f.targetHost, f.targetPort)
	conn := &activeConnection{
		clientAddr:    clientAddr,
		targetAddr:    targetAddr,
		bytesSent:     0,
		bytesReceived: 0,
		connectedAt:   time.Now().Unix(),
	}
	f.connMutex.Lock()
	f.connections[connID] = conn
	f.connMutex.Unlock()

	// 连接关闭时移除记录
	defer func() {
		f.connMutex.Lock()
		delete(f.connections, connID)
		f.connMutex.Unlock()
		log.Printf("端口 %d 连接关闭: %s (连接ID: %s, 发送: %d, 接收: %d)",
			f.sourcePort, clientAddr, connID,
			atomic.LoadUint64(&conn.bytesSent),
			atomic.LoadUint64(&conn.bytesReceived))
	}()

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
			w:           targetConn,
			counter:     &f.bytesSent,
			port:        f.sourcePort,
			connCounter: &conn.bytesSent,
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
			w:           clientConn,
			counter:     &f.bytesReceived,
			port:        f.sourcePort,
			connCounter: &conn.bytesReceived,
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

func (m *Manager) GetForwarder(port int) *Forwarder {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.forwarders[port]
}

// NewManager 创建新的转发器管理器
func NewManager() *Manager {
	return &Manager{
		forwarders: make(map[int]*Forwarder),
	}
}

// Add 添加并启动转发器
func (m *Manager) Add(sourcePort int, targetHost string, targetPort int, limit *int64, accessRule *string, accessIPs *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.forwarders[sourcePort]; exists {
		return fmt.Errorf("端口 %d 已被占用", sourcePort)
	}

	forwarder := NewForwarder(sourcePort, targetHost, targetPort, limit, accessRule, accessIPs)
	if err := forwarder.Start(); err != nil {
		return err
	}

	m.forwarders[sourcePort] = forwarder
	return nil
}

// AddTunnel 添加使用隧道的转发器
func (m *Manager) AddTunnel(sourcePort int, targetHost string, targetPort int, tunnelServer TunnelServer, limit *int64, accessRule *string, accessIPs *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.forwarders[sourcePort]; exists {
		return fmt.Errorf("端口 %d 已被占用", sourcePort)
	}

	forwarder := NewTunnelForwarder(sourcePort, targetHost, targetPort, tunnelServer, limit, accessRule, accessIPs)
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

// GetActiveConnections 获取某个转发器的活跃连接信息
func (f *Forwarder) GetActiveConnections() []stats.ConnectionInfo {
	f.connMutex.RLock()
	defer f.connMutex.RUnlock()

	connections := make([]stats.ConnectionInfo, 0, len(f.connections))
	for _, conn := range f.connections {
		connections = append(connections, stats.ConnectionInfo{
			ClientAddr:    conn.clientAddr,
			TargetAddr:    conn.targetAddr,
			BytesSent:     atomic.LoadUint64(&conn.bytesSent),
			BytesReceived: atomic.LoadUint64(&conn.bytesReceived),
			ConnectedAt:   conn.connectedAt,
		})
	}

	return connections
}

// GetAllActiveConnections 获取所有转发器的活跃连接信息
func (m *Manager) GetAllActiveConnections() []stats.PortConnectionStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	allStats := make([]stats.PortConnectionStats, 0, len(m.forwarders))
	for port, forwarder := range m.forwarders {
		connections := forwarder.GetActiveConnections()
		allStats = append(allStats, stats.PortConnectionStats{
			SourcePort:        port,
			TargetHost:        forwarder.targetHost,
			TargetPort:        forwarder.targetPort,
			UseTunnel:         forwarder.useTunnel,
			ActiveConnections: connections,
			TotalConnections:  len(connections),
		})
	}

	return allStats
}
