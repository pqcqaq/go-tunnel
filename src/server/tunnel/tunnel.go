package tunnel

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
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
	ReadTimeout    = 30 * time.Second // 读取超时
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
	TargetIP     string
	Created      time.Time
	ResponseChan chan bool // 用于接收连接响应
}

// ActiveConnection 活跃连接
type ActiveConnection struct {
	ID         uint32
	ClientConn net.Conn
	TargetPort int
	TargetIP   string
	Created    time.Time
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
	connMu       sync.RWMutex
	nextConnID   uint32
	
	// 消息队列
	sendChan chan *TunnelMessage
}

// NewServer 创建新的隧道服务器
func NewServer(listenPort int) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		listenPort:   listenPort,
		cancel:       cancel,
		ctx:          ctx,
		pendingConns: make(map[uint32]*PendingConnection),
		activeConns:  make(map[uint32]*ActiveConnection),
		sendChan:     make(chan *TunnelMessage, 1000),
	}
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

		// 只允许一个客户端连接
		s.mu.Lock()
		if s.tunnelConn != nil {
			log.Printf("拒绝额外的隧道连接: %s", conn.RemoteAddr())
			conn.Close()
			s.mu.Unlock()
			continue
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
		s.connMu.Unlock()
	}()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		msg, err := s.readTunnelMessage(conn)
		if err != nil {
			if err != io.EOF {
				log.Printf("读取隧道消息失败: %v", err)
			}
			return
		}

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
	// 构建消息头
	header := make([]byte, HeaderSize)
	header[0] = msg.Version
	header[1] = msg.Type
	binary.BigEndian.PutUint32(header[2:6], msg.Length)

	// 写入消息头
	if _, err := conn.Write(header); err != nil {
		return err
	}

	// 写入数据
	if msg.Length > 0 && msg.Data != nil {
		if _, err := conn.Write(msg.Data); err != nil {
			return err
		}
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
			TargetIP:   pending.TargetIP,
			Created:    time.Now(),
		}

		s.connMu.Lock()
		s.activeConns[connID] = active
		s.connMu.Unlock()

		log.Printf("连接已建立: ID=%d, 地址=%s:%d", connID, pending.TargetIP, pending.TargetPort)

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
	s.connMu.RUnlock()

	if !exists {
		log.Printf("收到未知连接的数据: %d", connID)
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
	// 回应心跳
	response := &TunnelMessage{
		Version: ProtocolVersion,
		Type:    MsgTypeKeepAlive,
		Length:  0,
		Data:    nil,
	}

	select {
	case s.sendChan <- response:
		// log.Printf("回应心跳消息") // 降低日志频率
	default:
		log.Printf("发送心跳响应失败: 发送队列已满")
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

		active.ClientConn.SetReadDeadline(time.Now().Add(ReadTimeout))
		n, err := active.ClientConn.Read(buffer)
		if err != nil {
			if err != io.EOF && !isTimeout(err) {
				log.Printf("读取客户端连接失败 (ID=%d): %v", active.ID, err)
			}
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
		case <-time.After(5 * time.Second):
			log.Printf("发送数据超时 (ID=%d)", active.ID)
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
		delete(s.activeConns, connID)
		active.ClientConn.Close()
	}
	s.connMu.Unlock()

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
	default:
		// 发送队列满，忽略
	}

	if exists {
		log.Printf("连接已关闭: ID=%d", connID)
	}
}

// ForwardConnection 转发连接到隧道（新的透明代理实现）
func (s *Server) ForwardConnection(clientConn net.Conn, targetIP string, targetPort int) error {
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
		TargetIP:     targetIP,
		Created:      time.Now(),
		ResponseChan: make(chan bool, 1),
	}
	s.pendingConns[connID] = pending
	s.connMu.Unlock()

	// 发送连接请求
	// 格式: connID(4) + targetPort(2) + targetIPLen(1) + targetIP(变长)
	targetIPBytes := []byte(targetIP)
	reqData := make([]byte, 7+len(targetIPBytes))
	binary.BigEndian.PutUint32(reqData[0:4], connID)
	binary.BigEndian.PutUint16(reqData[4:6], uint16(targetPort))
	reqData[6] = byte(len(targetIPBytes))
	copy(reqData[7:], targetIPBytes)

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

	log.Printf("发送连接请求: ID=%d, 地址=%s:%d", connID, targetIP, targetPort)

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