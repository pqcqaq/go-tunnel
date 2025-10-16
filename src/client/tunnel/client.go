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

const (
	// 协议版本
	ProtocolVersion = 0x01
	
	// 消息头大小
	HeaderSize = 6 // 版本(1) + 类型(1) + 长度(4)
	
	// 最大包大小
	MaxPacketSize = 1024 * 1024
	
	// 重连延迟
	ReconnectDelay = 5 * time.Second
	
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

// LocalConnection 本地连接
type LocalConnection struct {
	ID         uint32
	TargetAddr string
	Conn       net.Conn
	closeChan  chan struct{}
	closeOnce  sync.Once
}

// Client 内网穿透客户端
type Client struct {
	serverAddr string
	serverConn net.Conn
	cancel     context.CancelFunc
	ctx        context.Context
	wg         sync.WaitGroup
	mu         sync.RWMutex
	
	// 连接管理
	connections map[uint32]*LocalConnection
	connMu      sync.RWMutex
	
	// 消息队列
	sendChan chan *TunnelMessage
}

// NewClient 创建新的隧道客户端
func NewClient(serverAddr string) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		serverAddr:  serverAddr,
		cancel:      cancel,
		ctx:         ctx,
		connections: make(map[uint32]*LocalConnection),
		sendChan:    make(chan *TunnelMessage, 1000),
	}
}

// Start 启动隧道客户端
func (c *Client) Start() error {
	log.Printf("正在连接到隧道服务器: %s", c.serverAddr)
	
	c.wg.Add(1)
	go c.connectLoop()
	
	return nil
}

// connectLoop 连接循环（支持自动重连）
func (c *Client) connectLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		conn, err := net.DialTimeout("tcp", c.serverAddr, 10*time.Second)
		if err != nil {
			log.Printf("连接隧道服务器失败: %v，%v 后重试", err, ReconnectDelay)
			time.Sleep(ReconnectDelay)
			continue
		}

		log.Printf("已连接到隧道服务器: %s", c.serverAddr)

		c.mu.Lock()
		c.serverConn = conn
		c.mu.Unlock()

		// 处理连接
		var connWg sync.WaitGroup
		connCtx, connCancel := context.WithCancel(context.Background())
		
		connWg.Add(3)
		go func() {
			defer connWg.Done()
			c.handleServerRead(conn, connCtx, connCancel)
		}()
		go func() {
			defer connWg.Done()
			c.handleServerWrite(conn, connCtx, connCancel)
		}()
		go func() {
			defer connWg.Done()
			c.keepAliveLoop(conn, connCtx, connCancel)
		}()

		// 等待任一协程出错
		connWg.Wait()
		
		// 确保所有协程都退出
		connCancel()
		
		// 短暂等待确保资源清理
		time.Sleep(100 * time.Millisecond)

		// 连接断开后立即清理资源
		c.mu.Lock()
		c.serverConn = nil
		c.mu.Unlock()

		// 清空发送队列
		c.drainSendChan()

		// 关闭所有本地连接
		c.connMu.Lock()
		for _, conn := range c.connections {
			conn.closeOnce.Do(func() {
				close(conn.closeChan)
			})
			if conn.Conn != nil {
				conn.Conn.Close()
			}
		}
		c.connections = make(map[uint32]*LocalConnection)
		c.connMu.Unlock()

		log.Printf("与服务器断开连接，%v 后重连", ReconnectDelay)
		
		// 使用带取消的sleep，确保可以及时响应关闭信号
		select {
		case <-time.After(ReconnectDelay):
		case <-c.ctx.Done():
			return
		}
	}
}

// handleServerRead 处理服务器读取
func (c *Client) handleServerRead(conn net.Conn, connCtx context.Context, connCancel context.CancelFunc) {
	defer conn.Close()

	for {
		// 检查是否应该退出
		select {
		case <-c.ctx.Done():
			return
		case <-connCtx.Done():
			return
		default:
		}

		// 设置读取超时，避免无限阻塞
		conn.SetReadDeadline(time.Now().Add(ReadTimeout))
		msg, err := c.readTunnelMessage(conn)
		if err != nil {
			if err != io.EOF && !isTimeout(err) {
				log.Printf("读取隧道消息失败: %v", err)
			}
			connCancel() // 通知其他协程退出
			return
		}

		// 重置读取超时
		conn.SetReadDeadline(time.Time{})
		c.handleTunnelMessage(msg)
	}
}

// handleServerWrite 处理服务器写入
func (c *Client) handleServerWrite(conn net.Conn, connCtx context.Context, connCancel context.CancelFunc) {
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-connCtx.Done():
			return
		case msg := <-c.sendChan:
			if err := c.writeTunnelMessage(conn, msg); err != nil {
				log.Printf("写入隧道消息失败: %v", err)
				// 清空发送队列，避免阻塞
				go c.drainSendChan()
				connCancel() // 通知其他协程退出
				return
			}
		}
	}
}

// readTunnelMessage 读取隧道消息
func (c *Client) readTunnelMessage(conn net.Conn) (*TunnelMessage, error) {
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
func (c *Client) writeTunnelMessage(conn net.Conn, msg *TunnelMessage) error {
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

	// 写入数据
	if msg.Length > 0 && msg.Data != nil {
		if _, err := conn.Write(msg.Data); err != nil {
			return fmt.Errorf("写入消息数据失败: %w", err)
		}
	}

	return nil
}

// handleTunnelMessage 处理隧道消息
func (c *Client) handleTunnelMessage(msg *TunnelMessage) {
	switch msg.Type {
	case MsgTypeConnectRequest:
		c.handleConnectRequest(msg)
	case MsgTypeData:
		c.handleDataMessage(msg)
	case MsgTypeClose:
		c.handleCloseMessage(msg)
	case MsgTypeKeepAlive:
		c.handleKeepAlive(msg)
	default:
		log.Printf("未知消息类型: %d", msg.Type)
	}
}

// handleConnectRequest 处理连接请求
func (c *Client) handleConnectRequest(msg *TunnelMessage) {
	// 解析: connID(4) + targetPort(2) + targetHostLen(1) + targetHost(变长)
	if len(msg.Data) < 7 {
		log.Printf("连接请求数据太短")
		return
	}

	connID := binary.BigEndian.Uint32(msg.Data[0:4])
	targetPort := binary.BigEndian.Uint16(msg.Data[4:6])
	targetHostLen := int(msg.Data[6])
	
	if len(msg.Data) < 7+targetHostLen {
		log.Printf("连接请求数据不完整")
		return
	}
	
	targetHost := string(msg.Data[7 : 7+targetHostLen])
	targetAddr := net.JoinHostPort(targetHost, fmt.Sprintf("%d", targetPort))

	log.Printf("收到连接请求: ID=%d, 地址=%s", connID, targetAddr)

	// 尝试连接到目标服务（支持域名解析）
	localConn, err := net.DialTimeout("tcp", targetAddr, ConnectTimeout)
	if err != nil {
		log.Printf("连接目标服务失败 (ID=%d -> %s): %v", connID, targetAddr, err)
		c.sendConnectResponse(connID, ConnStatusFailed)
		return
	}

	// 创建本地连接对象
	connection := &LocalConnection{
		ID:         connID,
		TargetAddr: targetAddr,
		Conn:       localConn,
		closeChan:  make(chan struct{}),
	}

	c.connMu.Lock()
	c.connections[connID] = connection
	c.connMu.Unlock()

	log.Printf("建立目标连接: ID=%d -> %s", connID, targetAddr)

	// 发送连接成功响应
	c.sendConnectResponse(connID, ConnStatusSuccess)

	// 启动数据转发
	go c.forwardData(connection)
}

// handleDataMessage 处理数据消息
func (c *Client) handleDataMessage(msg *TunnelMessage) {
	if len(msg.Data) < 4 {
		log.Printf("数据消息太短")
		return
	}

	connID := binary.BigEndian.Uint32(msg.Data[0:4])
	data := msg.Data[4:]

	c.connMu.RLock()
	connection, exists := c.connections[connID]
	c.connMu.RUnlock()

	if !exists {
		log.Printf("收到未知连接的数据: %d，发送关闭消息", connID)
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
		case c.sendChan <- closeMsg:
		default:
		}
		return
	}

	if _, err := connection.Conn.Write(data); err != nil {
		log.Printf("写入目标连接失败 (ID=%d): %v", connID, err)
		c.closeConnection(connID)
	}
}

// handleCloseMessage 处理关闭消息
func (c *Client) handleCloseMessage(msg *TunnelMessage) {
	if len(msg.Data) < 4 {
		log.Printf("关闭消息数据太短")
		return
	}

	connID := binary.BigEndian.Uint32(msg.Data[0:4])
	c.closeConnection(connID)
}

// handleKeepAlive 处理心跳消息
func (c *Client) handleKeepAlive(msg *TunnelMessage) {
	// 客户端收到服务器的心跳响应，不需要再回应
	// 这样避免心跳消息的无限循环
	// log.Printf("收到服务器心跳响应")
}

// sendConnectResponse 发送连接响应
func (c *Client) sendConnectResponse(connID uint32, status byte) {
	responseData := make([]byte, 5)
	binary.BigEndian.PutUint32(responseData[0:4], connID)
	responseData[4] = status

	msg := &TunnelMessage{
		Version: ProtocolVersion,
		Type:    MsgTypeConnectResponse,
		Length:  5,
		Data:    responseData,
	}

	select {
	case c.sendChan <- msg:
	default:
		log.Printf("发送连接响应失败: 发送队列已满")
	}
}

// forwardData 转发数据
func (c *Client) forwardData(connection *LocalConnection) {
	defer c.closeConnection(connection.ID)

	buffer := make([]byte, 32*1024)
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-connection.closeChan:
			return
		default:
		}

		// 设置读取超时
		connection.Conn.SetReadDeadline(time.Now().Add(ReadTimeout))
		n, err := connection.Conn.Read(buffer)
		
		if err != nil {
			// 任何错误都应该终止转发
			if err == io.EOF {
				log.Printf("目标连接正常关闭 (ID=%d)", connection.ID)
			} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Printf("目标连接超时 (ID=%d)", connection.ID)
			} else {
				log.Printf("读取目标连接失败 (ID=%d): %v", connection.ID, err)
			}
			return
		}

		// 读取到0字节，连接已关闭
		if n == 0 {
			log.Printf("目标连接已关闭 (ID=%d, 读取0字节)", connection.ID)
			return
		}

		// 重置读取超时
		connection.Conn.SetReadDeadline(time.Time{})

		// 发送数据到隧道
		dataMsg := make([]byte, 4+n)
		binary.BigEndian.PutUint32(dataMsg[0:4], connection.ID)
		copy(dataMsg[4:], buffer[:n])

		msg := &TunnelMessage{
			Version: ProtocolVersion,
			Type:    MsgTypeData,
			Length:  uint32(len(dataMsg)),
			Data:    dataMsg,
		}

		select {
		case c.sendChan <- msg:
			// 数据已发送
		case <-time.After(5 * time.Second):
			log.Printf("发送数据超时 (ID=%d)", connection.ID)
			return
		case <-c.ctx.Done():
			return
		case <-connection.closeChan:
			return
		}
	}
}

// closeConnection 关闭连接
func (c *Client) closeConnection(connID uint32) {
	c.connMu.Lock()
	connection, exists := c.connections[connID]
	if exists {
		delete(c.connections, connID)
		connection.closeOnce.Do(func() {
			close(connection.closeChan)
		})
		// 确保连接被关闭
		if connection.Conn != nil {
			connection.Conn.Close()
		}
	}
	c.connMu.Unlock()

	if !exists {
		// 连接不存在，无需发送关闭消息
		return
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
	case c.sendChan <- msg:
		log.Printf("连接已关闭: ID=%d", connID)
	case <-time.After(1 * time.Second):
		log.Printf("发送关闭消息超时: ID=%d", connID)
	case <-c.ctx.Done():
		log.Printf("客户端关闭，跳过发送关闭消息: ID=%d", connID)
	}
}

// Stop 停止隧道客户端
func (c *Client) Stop() error {
	log.Println("正在停止隧道客户端...")
	c.cancel()

	c.mu.Lock()
	if c.serverConn != nil {
		c.serverConn.Close()
	}
	c.mu.Unlock()

	// 等待所有协程结束
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("隧道客户端已停止")
	case <-time.After(5 * time.Second):
		log.Println("隧道客户端停止超时")
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
func (c *Client) keepAliveLoop(conn net.Conn, connCtx context.Context, connCancel context.CancelFunc) {
	ticker := time.NewTicker(KeepAliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-connCtx.Done():
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
			case c.sendChan <- keepAliveMsg:
				// 降低心跳日志频率，避免刷屏
				// log.Printf("发送心跳消息")
			case <-time.After(5 * time.Second):
				log.Printf("发送心跳消息超时")
				connCancel() // 通知其他协程退出
				return
			case <-c.ctx.Done():
				return
			case <-connCtx.Done():
				return
			}
		}
	}
}

// drainSendChan 清空发送队列，避免阻塞
func (c *Client) drainSendChan() {
	for {
		select {
		case <-c.sendChan:
			// 丢弃消息
		default:
			return
		}
	}
}