package tunnel

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"port-forward/server/stats"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Tunnel协议定义
// 消息格式: | 版本(1B) | 类型(1B) | 长度(4B) | 数据 |

const (
	// 协议版本
	ProtocolVersion = 0x01
	
	// 消息头大小
	HeaderSize = 6 // 版本(1) + 类型(1) + 长度(4)
	
	// 最大包大小 (1MB)
	MaxPacketSize = 1024 * 1024
	
	// 消息类型
	MsgTypeConnectRequest  = 0x01 // 连接请求
	MsgTypeConnectResponse = 0x02 // 连接响应
	MsgTypeData           = 0x03 // 数据传输
	MsgTypeClose          = 0x04 // 关闭连接
	MsgTypeKeepAlive      = 0x05 // 心跳
	
	// 连接响应状态
	ConnStatusSuccess = 0x00 // 连接成功
	ConnStatusFailed  = 0x01 // 连接失败
	
	// 超时设置
	ConnectTimeout = 10 * time.Second // 连接超时
	ReadTimeout    = 300 * time.Second // 读取超时，统一为60秒
	KeepAliveInterval = 15 * time.Second // 心跳间隔
)

// TunnelMessage 隧道消息
type TunnelMessage struct {
	Version byte
	Type    byte
	Length  uint32
	Data    []byte
}

// ConnectRequestData 连接请求数据
type ConnectRequestData struct {
	ConnID     uint32 // 连接ID
	TargetPort uint16 // 目标端口
	TargetIP   string // 目标IP地址
}

// ConnectResponseData 连接响应数据
type ConnectResponseData struct {
	ConnID uint32 // 连接ID
	Status byte   // 状态码
}

// DataMessage 数据消息
type DataMessage struct {
	ConnID uint32 // 连接ID
	Data   []byte // 数据
}

// CloseMessage 关闭消息
type CloseMessage struct {
	ConnID uint32 // 连接ID
}

// PendingConnection 待处理连接
type PendingConnection struct {
	ID           uint32
	ClientConn   net.Conn
	TargetPort   int
	TargetHost   string // 支持IP或域名
	Created      time.Time
	ResponseChan chan bool // 用于接收连接响应
}

// ActiveConnection 活跃连接
type ActiveConnection struct {
	ID         uint32
	ClientConn net.Conn
	TargetPort int
	TargetHost string // 支持IP或域名
	TargetIP   string
	Created    time.Time
	Closing    int32  // 原子操作标志，表示连接正在关闭
}

// Server 内网穿透服务器
type Server struct {
	listenPort int
	listener   net.Listener
	tunnelConn net.Conn
	cancel     context.CancelFunc
	ctx        context.Context
	wg         sync.WaitGroup
	mu         sync.RWMutex
	
	// 连接管理
	pendingConns map[uint32]*PendingConnection // 待确认连接
	activeConns  map[uint32]*ActiveConnection  // 活跃连接
	closingConns map[uint32]time.Time          // 正在关闭的连接，避免重复处理
	connMu       sync.RWMutex
	nextConnID   uint32
	
	// 消息队列
	sendChan chan *TunnelMessage
	
	// 流量统计（使用原子操作）
	bytesSent     uint64 // 通过隧道发送的总字节数
	bytesReceived uint64 // 通过隧道接收的总字节数
}

// NewServer 创建新的隧道服务器
func NewServer(listenPort int) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	server := &Server{
		listenPort:   listenPort,
		cancel:       cancel,
		ctx:          ctx,
		pendingConns: make(map[uint32]*PendingConnection),
		activeConns:  make(map[uint32]*ActiveConnection),
		closingConns: make(map[uint32]time.Time),
		sendChan:     make(chan *TunnelMessage, 10000), // 增加到10000
	}
	
	// 启动清理器，定期清理过期的关闭连接记录
	go server.cleanupClosingConns()
	
	return server
}

// Start 启动隧道服务器
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.listenPort))
	if err != nil {
		return fmt.Errorf("启动隧道服务器失败: %w", err)
	}

	s.listener = listener
	log.Printf("隧道服务器启动: 端口 %d", s.listenPort)

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// acceptLoop 接受客户端连接
func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		s.listener.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second))
		conn, err := s.listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-s.ctx.Done():
				return
			default:
				log.Printf("接受隧道连接失败: %v", err)
				continue
			}
		}

		// 只允许一个客户端连接，如果有旧连接则关闭它
		s.mu.Lock()
		if s.tunnelConn != nil {
			log.Printf("关闭旧的隧道连接，接受新连接: %s -> %s", s.tunnelConn.RemoteAddr(), conn.RemoteAddr())
			s.tunnelConn.Close() // 关闭旧连接，这会让旧的read/write协程退出
		}
		s.tunnelConn = conn
		s.mu.Unlock()

		log.Printf("隧道客户端已连接: %s", conn.RemoteAddr())

		s.wg.Add(2)
		go s.handleTunnelRead(conn)
		go s.handleTunnelWrite(conn)
		// 注释掉服务器端主动心跳，只由客户端发送心跳
		// go s.keepAliveLoop(conn)
	}
}

// handleTunnelRead 处理隧道读取
func (s *Server) handleTunnelRead(conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		conn.Close()
		s.mu.Lock()
		s.tunnelConn = nil
		s.mu.Unlock()
		log.Printf("隧道客户端已断开")
		
		// 关闭所有活动连接
		s.connMu.Lock()
		for _, c := range s.pendingConns {
			c.ClientConn.Close()
			close(c.ResponseChan)
		}
		for _, c := range s.activeConns {
			c.ClientConn.Close()
		}
		s.pendingConns = make(map[uint32]*PendingConnection)
		s.activeConns = make(map[uint32]*ActiveConnection)
		s.closingConns = make(map[uint32]time.Time)
		s.connMu.Unlock()
	}()

	for {
		// 检查是否应该退出
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// 设置读取超时，避免无限阻塞
		conn.SetReadDeadline(time.Now().Add(ReadTimeout))
		msg, err := s.readTunnelMessage(conn)
		if err != nil {
			if err != io.EOF && !isTimeout(err) {
				log.Printf("读取隧道消息失败: %v", err)
			}
			return
		}

		// 重置读取超时
		conn.SetReadDeadline(time.Time{})
		s.handleTunnelMessage(msg)
	}
}

// handleTunnelWrite 处理隧道写入
func (s *Server) handleTunnelWrite(conn net.Conn) {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		case msg := <-s.sendChan:
			// 检查当前连接是否还是活跃的隧道连接
			s.mu.RLock()
			activeConn := s.tunnelConn
			s.mu.RUnlock()

			if activeConn != conn {
				// 如果这不是活跃连接，尝试重新放入队列（非阻塞）
				select {
				case s.sendChan <- msg:
					// 消息重新入队成功
				default:
					// 队列满时丢弃消息，避免阻塞
					log.Printf("发送队列繁忙，丢弃消息（旧连接写入器）")
				}
				// 退出旧的写入器
				return
			}

			if err := s.writeTunnelMessage(conn, msg); err != nil {
				log.Printf("写入隧道消息失败: %v", err)
				return
			}
		}
	}
}

// readTunnelMessage 读取隧道消息
func (s *Server) readTunnelMessage(conn net.Conn) (*TunnelMessage, error) {
	// 读取消息头
	header := make([]byte, HeaderSize)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}
	
	// 统计接收字节数
	s.addBytesReceived(uint64(HeaderSize))

	version := header[0]
	msgType := header[1]
	dataLen := binary.BigEndian.Uint32(header[2:6])

	if version != ProtocolVersion {
		return nil, fmt.Errorf("不支持的协议版本: %d", version)
	}

	if dataLen > MaxPacketSize {
		return nil, fmt.Errorf("数据包过大: %d bytes", dataLen)
	}

	// 读取数据
	var data []byte
	if dataLen > 0 {
		data = make([]byte, dataLen)
		if _, err := io.ReadFull(conn, data); err != nil {
			return nil, err
		}
		// 统计接收字节数
		s.addBytesReceived(uint64(dataLen))
	}

	return &TunnelMessage{
		Version: version,
		Type:    msgType,
		Length:  dataLen,
		Data:    data,
	}, nil
}

// writeTunnelMessage 写入隧道消息
func (s *Server) writeTunnelMessage(conn net.Conn, msg *TunnelMessage) error {
	// 设置写入超时，防止阻塞
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	defer conn.SetWriteDeadline(time.Time{}) // 重置超时
	
	// 构建消息头
	header := make([]byte, HeaderSize)
	header[0] = msg.Version
	header[1] = msg.Type
	binary.BigEndian.PutUint32(header[2:6], msg.Length)

	// 写入消息头
	if _, err := conn.Write(header); err != nil {
		return fmt.Errorf("写入消息头失败: %w", err)
	}
	
	// 统计发送字节数
	s.addBytesSent(uint64(HeaderSize))

	// 写入数据
	if msg.Length > 0 && msg.Data != nil {
		if _, err := conn.Write(msg.Data); err != nil {
			return fmt.Errorf("写入消息数据失败: %w", err)
		}
		// 统计发送字节数
		s.addBytesSent(uint64(msg.Length))
	}

	return nil
}

// handleTunnelMessage 处理隧道消息
func (s *Server) handleTunnelMessage(msg *TunnelMessage) {
	switch msg.Type {
	case MsgTypeConnectResponse:
		s.handleConnectResponse(msg)
	case MsgTypeData:
		s.handleDataMessage(msg)
	case MsgTypeClose:
		s.handleCloseMessage(msg)
	case MsgTypeKeepAlive:
		s.handleKeepAlive(msg)
	default:
		log.Printf("未知消息类型: %d", msg.Type)
	}
}

// handleConnectResponse 处理连接响应
func (s *Server) handleConnectResponse(msg *TunnelMessage) {
	if len(msg.Data) < 5 {
		log.Printf("连接响应数据太短")
		return
	}

	connID := binary.BigEndian.Uint32(msg.Data[0:4])
	status := msg.Data[4]

	s.connMu.Lock()
	pending, exists := s.pendingConns[connID]
	if !exists {
		s.connMu.Unlock()
		log.Printf("收到未知连接的响应: %d", connID)
		return
	}

	delete(s.pendingConns, connID)
	s.connMu.Unlock()

	if status == ConnStatusSuccess {
		// 连接成功，移到活跃连接
		active := &ActiveConnection{
			ID:         connID,
			ClientConn: pending.ClientConn,
			TargetPort: pending.TargetPort,
			TargetHost: pending.TargetHost,
			Created:    time.Now(),
		}

		s.connMu.Lock()
		s.activeConns[connID] = active
		s.connMu.Unlock()

		log.Printf("连接已建立: ID=%d, 地址=%s:%d", connID, pending.TargetHost, pending.TargetPort)

		// 启动数据转发
		s.wg.Add(1)
		go s.forwardData(active)

		// 通知等待的goroutine
		select {
		case pending.ResponseChan <- true:
		default:
		}
	} else {
		// 连接失败
		log.Printf("连接失败: ID=%d, 状态=%d", connID, status)
		pending.ClientConn.Close()

		// 通知等待的goroutine
		select {
		case pending.ResponseChan <- false:
		default:
		}
	}

	close(pending.ResponseChan)
}

// handleDataMessage 处理数据消息
func (s *Server) handleDataMessage(msg *TunnelMessage) {
	if len(msg.Data) < 4 {
		log.Printf("数据消息太短")
		return
	}

	connID := binary.BigEndian.Uint32(msg.Data[0:4])
	data := msg.Data[4:]

	s.connMu.RLock()
	active, exists := s.activeConns[connID]
	isClosing := false
	if _, found := s.closingConns[connID]; found {
		isClosing = true
	}
	s.connMu.RUnlock()

	if !exists {
		// 检查是否是正在关闭的连接，避免重复发送关闭消息
		if !isClosing {
			log.Printf("收到未知连接的数据: %d，发送关闭消息", connID)
			// 标记为正在关闭，避免重复处理
			s.connMu.Lock()
			s.closingConns[connID] = time.Now()
			s.connMu.Unlock()
			
			// 连接不存在，发送关闭消息通知对端
			closeData := make([]byte, 4)
			binary.BigEndian.PutUint32(closeData, connID)
			closeMsg := &TunnelMessage{
				Version: ProtocolVersion,
				Type:    MsgTypeClose,
				Length:  4,
				Data:    closeData,
			}
			select {
			case s.sendChan <- closeMsg:
			default:
			}
		}
		return
	}

	// 检查连接是否正在关闭
	if atomic.LoadInt32(&active.Closing) == 1 {
		return
	}

	// 写入到客户端连接
	if _, err := active.ClientConn.Write(data); err != nil {
		log.Printf("写入客户端连接失败 (ID=%d): %v", connID, err)
		s.closeConnection(connID)
	}
}

// handleCloseMessage 处理关闭消息
func (s *Server) handleCloseMessage(msg *TunnelMessage) {
	if len(msg.Data) < 4 {
		log.Printf("关闭消息数据太短")
		return
	}

	connID := binary.BigEndian.Uint32(msg.Data[0:4])
	s.closeConnection(connID)
}

// handleKeepAlive 处理心跳消息
func (s *Server) handleKeepAlive(msg *TunnelMessage) {
	// 服务器收到客户端的心跳请求，回应一次即可
	// 不要形成心跳循环
	response := &TunnelMessage{
		Version: ProtocolVersion,
		Type:    MsgTypeKeepAlive,
		Length:  0,
		Data:    nil,
	}

	select {
	case s.sendChan <- response:
		// log.Printf("回应心跳消息") // 降低日志频率
	case <-time.After(1 * time.Second):
		// 心跳不是关键消息，超时就跳过
		log.Printf("发送心跳响应超时，跳过")
	default:
		// 队列满时跳过心跳，避免阻塞数据传输
		log.Printf("发送队列忙碌，跳过心跳响应")
	}
}

// forwardData 转发数据
func (s *Server) forwardData(active *ActiveConnection) {
	defer s.wg.Done()
	defer s.closeConnection(active.ID)

	buffer := make([]byte, 32*1024)
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// 设置读取超时
		active.ClientConn.SetReadDeadline(time.Now().Add(ReadTimeout))
		n, err := active.ClientConn.Read(buffer)
		
		if err != nil {
			// 检查是否是正在关闭的连接，避免记录无关错误
			if atomic.LoadInt32(&active.Closing) == 1 {
				return // 连接正在关闭，正常退出
			}
			
			// 任何错误都应该终止转发，包括超时
			if err == io.EOF {
				log.Printf("客户端连接正常关闭 (ID=%d)", active.ID)
			} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Printf("客户端连接超时 (ID=%d)", active.ID)
			} else {
				// 只记录非关闭相关的错误
				if !isConnectionClosed(err) {
					log.Printf("读取客户端连接失败 (ID=%d): %v", active.ID, err)
				}
			}
			return
		}

		// 读取到0字节，连接已关闭
		if n == 0 {
			log.Printf("客户端连接已关闭 (ID=%d, 读取0字节)", active.ID)
			return
		}

		// 重置读取超时
		active.ClientConn.SetReadDeadline(time.Time{})

		// 检查连接是否正在关闭
		if atomic.LoadInt32(&active.Closing) == 1 {
			return
		}

		// 发送数据到隧道
		dataMsg := make([]byte, 4+n)
		binary.BigEndian.PutUint32(dataMsg[0:4], active.ID)
		copy(dataMsg[4:], buffer[:n])

		msg := &TunnelMessage{
			Version: ProtocolVersion,
			Type:    MsgTypeData,
			Length:  uint32(len(dataMsg)),
			Data:    dataMsg,
		}

		select {
		case s.sendChan <- msg:
			// 数据已发送
		case <-time.After(2 * time.Second): // 减少超时时间
			queueLen := len(s.sendChan)
			log.Printf("发送数据超时 (ID=%d), 队列长度: %d/10000", active.ID, queueLen)
			return
		case <-s.ctx.Done():
			return
		}
	}
}

// closeConnection 关闭连接
func (s *Server) closeConnection(connID uint32) {
	s.connMu.Lock()
	active, exists := s.activeConns[connID]
	if exists {
		// 使用原子操作标记连接正在关闭
		if !atomic.CompareAndSwapInt32(&active.Closing, 0, 1) {
			// 连接已经在关闭中，避免重复处理
			s.connMu.Unlock()
			return
		}
		
		delete(s.activeConns, connID)
		// 记录关闭时间，避免重复发送关闭消息
		s.closingConns[connID] = time.Now()
		
		// 确保连接被关闭
		if active.ClientConn != nil {
			active.ClientConn.Close()
		}
	}
	s.connMu.Unlock()

	if !exists {
		// 连接不存在，检查是否已经在关闭列表中
		s.connMu.RLock()
		_, isClosing := s.closingConns[connID]
		s.connMu.RUnlock()
		
		if isClosing {
			return // 已经处理过了
		}
		
		// 标记为正在关闭
		s.connMu.Lock()
		s.closingConns[connID] = time.Now()
		s.connMu.Unlock()
	}

	// 发送关闭消息
	closeData := make([]byte, 4)
	binary.BigEndian.PutUint32(closeData, connID)

	msg := &TunnelMessage{
		Version: ProtocolVersion,
		Type:    MsgTypeClose,
		Length:  4,
		Data:    closeData,
	}

	select {
	case s.sendChan <- msg:
		log.Printf("连接已关闭: ID=%d", connID)
	case <-time.After(1 * time.Second):
		log.Printf("发送关闭消息超时: ID=%d", connID)
	case <-s.ctx.Done():
		log.Printf("服务器关闭，跳过发送关闭消息: ID=%d", connID)
	}
}

// ForwardConnection 转发连接到隧道（新的透明代理实现）
func (s *Server) ForwardConnection(clientConn net.Conn, targetHost string, targetPort int) error {
	s.mu.RLock()
	tunnelConnected := s.tunnelConn != nil
	s.mu.RUnlock()

	if !tunnelConnected {
		return fmt.Errorf("隧道连接不可用")
	}

	// 创建待处理连接
	s.connMu.Lock()
	connID := s.nextConnID
	s.nextConnID++
	
	pending := &PendingConnection{
		ID:           connID,
		ClientConn:   clientConn,
		TargetPort:   targetPort,
		TargetHost:   targetHost, // 现在支持域名
		Created:      time.Now(),
		ResponseChan: make(chan bool, 1),
	}
	s.pendingConns[connID] = pending
	s.connMu.Unlock()

	// 发送连接请求
	// 格式: connID(4) + targetPort(2) + targetHostLen(1) + targetHost(变长)
	targetHostBytes := []byte(targetHost)
	reqData := make([]byte, 7+len(targetHostBytes))
	binary.BigEndian.PutUint32(reqData[0:4], connID)
	binary.BigEndian.PutUint16(reqData[4:6], uint16(targetPort))
	reqData[6] = byte(len(targetHostBytes))
	copy(reqData[7:], targetHostBytes)

	msg := &TunnelMessage{
		Version: ProtocolVersion,
		Type:    MsgTypeConnectRequest,
		Length:  uint32(len(reqData)),
		Data:    reqData,
	}

	select {
	case s.sendChan <- msg:
	case <-time.After(5 * time.Second):
		s.connMu.Lock()
		delete(s.pendingConns, connID)
		s.connMu.Unlock()
		return fmt.Errorf("发送连接请求超时")
	}

	log.Printf("发送连接请求: ID=%d, 地址=%s:%d", connID, targetHost, targetPort)

	// 等待连接响应
	select {
	case success := <-pending.ResponseChan:
		if success {
			return nil // 连接建立成功，forwardData会处理后续的数据转发
		} else {
			return fmt.Errorf("远程连接失败")
		}
	case <-time.After(ConnectTimeout):
		s.connMu.Lock()
		delete(s.pendingConns, connID)
		s.connMu.Unlock()
		clientConn.Close()
		return fmt.Errorf("连接超时")
	case <-s.ctx.Done():
		return fmt.Errorf("服务器关闭")
	}
}

// IsConnected 检查隧道是否已连接
func (s *Server) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tunnelConn != nil
}

// Stop 停止隧道服务器
func (s *Server) Stop() error {
	s.cancel()

	if s.listener != nil {
		s.listener.Close()
	}

	s.mu.Lock()
	if s.tunnelConn != nil {
		s.tunnelConn.Close()
	}
	s.mu.Unlock()

	// 等待所有协程结束
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("隧道服务器已停止")
	case <-time.After(5 * time.Second):
		log.Printf("隧道服务器停止超时")
	}

	return nil
}

// isTimeout 检查是否为超时错误
func isTimeout(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}

// keepAliveLoop 心跳循环
func (s *Server) keepAliveLoop(conn net.Conn) {
	defer s.wg.Done()
	
	ticker := time.NewTicker(KeepAliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// 发送心跳消息
			keepAliveMsg := &TunnelMessage{
				Version: ProtocolVersion,
				Type:    MsgTypeKeepAlive,
				Length:  0,
				Data:    nil,
			}

			select {
			case s.sendChan <- keepAliveMsg:
				log.Printf("发送心跳消息")
			case <-time.After(5 * time.Second):
				log.Printf("发送心跳消息超时")
				return
			case <-s.ctx.Done():
				return
			}
		}
	}
}

// GetTrafficStats 获取流量统计信息
func (s *Server) GetTrafficStats() stats.TrafficStats {
	return stats.TrafficStats{
		BytesSent:     atomic.LoadUint64(&s.bytesSent),
		BytesReceived: atomic.LoadUint64(&s.bytesReceived),
		LastUpdate:    time.Now().Unix(),
	}
}

// addBytesSent 增加发送字节数
func (s *Server) addBytesSent(bytes uint64) {
	atomic.AddUint64(&s.bytesSent, bytes)
}

// addBytesReceived 增加接收字节数
func (s *Server) addBytesReceived(bytes uint64) {
	atomic.AddUint64(&s.bytesReceived, bytes)
}

// cleanupClosingConns 定期清理过期的关闭连接记录
func (s *Server) cleanupClosingConns() {
	ticker := time.NewTicker(30 * time.Second) // 每30秒清理一次
	defer ticker.Stop()

	const maxClosingRecords = 10000 // 最大保留记录数
	const maxAge = 2 * time.Minute  // 最大保留时间，从5分钟减少到2分钟

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			s.connMu.Lock()
			
			// 按时间清理过期记录
			for connID, closeTime := range s.closingConns {
				if now.Sub(closeTime) > maxAge {
					delete(s.closingConns, connID)
				}
			}
			
			// 如果记录数量仍然过多，删除最旧的记录
			if len(s.closingConns) > maxClosingRecords {
				// 删除一半的最旧记录，避免频繁清理
				deleteCount := len(s.closingConns) - maxClosingRecords/2
				deletedCount := 0
				
				for connID, closeTime := range s.closingConns {
					if deletedCount >= deleteCount {
						break
					}
					if closeTime.Before(now.Add(-maxAge/2)) {
						delete(s.closingConns, connID)
						deletedCount++
					}
				}
			}
			
			s.connMu.Unlock()
		}
	}
}

// isConnectionClosed 检查错误是否是连接关闭相关的
func isConnectionClosed(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "use of closed network connection") ||
		   strings.Contains(errStr, "connection reset by peer") ||
		   strings.Contains(errStr, "broken pipe")
}