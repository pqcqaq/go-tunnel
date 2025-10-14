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
		connWg.Add(2)
		go func() {
			defer connWg.Done()
			c.handleServerRead(conn)
		}()
		go func() {
			defer connWg.Done()
			c.handleServerWrite(conn)
		}()

		// 等待连接断开
		connWg.Wait()

		c.mu.Lock()
		c.serverConn = nil
		c.mu.Unlock()

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
		time.Sleep(ReconnectDelay)
	}
}

// handleServerRead 处理服务器读取
func (c *Client) handleServerRead(conn net.Conn) {
	defer conn.Close()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		msg, err := c.readTunnelMessage(conn)
		if err != nil {
			if err != io.EOF {
				log.Printf("读取隧道消息失败: %v", err)
			}
			return
		}

		c.handleTunnelMessage(msg)
	}
}

// handleServerWrite 处理服务器写入
func (c *Client) handleServerWrite(conn net.Conn) {
	for {
		select {
		case <-c.ctx.Done():
			return
		case msg := <-c.sendChan:
			if err := c.writeTunnelMessage(conn, msg); err != nil {
				log.Printf("写入隧道消息失败: %v", err)
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
	if len(msg.Data) < 6 {
		log.Printf("连接请求数据太短")
		return
	}

	connID := binary.BigEndian.Uint32(msg.Data[0:4])
	targetPort := binary.BigEndian.Uint16(msg.Data[4:6])
	targetAddr := fmt.Sprintf("127.0.0.1:%d", targetPort)

	log.Printf("收到连接请求: ID=%d, 端口=%d", connID, targetPort)

	// 尝试连接到本地服务
	localConn, err := net.DialTimeout("tcp", targetAddr, ConnectTimeout)
	if err != nil {
		log.Printf("连接本地服务失败 (ID=%d -> %s): %v", connID, targetAddr, err)
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

	log.Printf("建立本地连接: ID=%d -> %s", connID, targetAddr)

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
		log.Printf("收到未知连接的数据: %d", connID)
		return
	}

	// 写入到本地连接
	if _, err := connection.Conn.Write(data); err != nil {
		log.Printf("写入本地连接失败 (ID=%d): %v", connID, err)
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
	// 回应心跳
	response := &TunnelMessage{
		Version: ProtocolVersion,
		Type:    MsgTypeKeepAlive,
		Length:  0,
		Data:    nil,
	}

	select {
	case c.sendChan <- response:
	default:
		log.Printf("发送心跳响应失败: 发送队列已满")
	}
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
		case <-connection.closeChan:
			return
		case <-c.ctx.Done():
			return
		default:
		}

		connection.Conn.SetReadDeadline(time.Now().Add(ReadTimeout))
		n, err := connection.Conn.Read(buffer)
		if err != nil {
			if err != io.EOF && !isTimeout(err) {
				log.Printf("读取本地连接失败 (ID=%d): %v", connection.ID, err)
			}
			return
		}

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
		case <-time.After(5 * time.Second):
			log.Printf("发送数据超时 (ID=%d)", connection.ID)
			return
		case <-c.ctx.Done():
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
		connection.Conn.Close()
	}
	c.connMu.Unlock()

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
	default:
		// 发送队列满，忽略
	}

	if exists {
		log.Printf("连接已关闭: ID=%d", connID)
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