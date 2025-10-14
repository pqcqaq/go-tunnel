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
	// HeaderSize 消息头大小
	HeaderSize = 8
	// MaxPacketSize 最大包大小
	MaxPacketSize = 1024 * 1024
	// ReconnectDelay 重连延迟
	ReconnectDelay = 5 * time.Second
)

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
}

// LocalConnection 本地连接
type LocalConnection struct {
	ID         uint32
	TargetAddr string
	Conn       net.Conn
	closeChan  chan struct{}
}

// NewClient 创建新的隧道客户端
func NewClient(serverAddr string) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		serverAddr:  serverAddr,
		cancel:      cancel,
		ctx:         ctx,
		connections: make(map[uint32]*LocalConnection),
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
		if err := c.handleServerConnection(conn); err != nil {
			if err != io.EOF {
				log.Printf("处理服务器连接出错: %v", err)
			}
		}

		c.mu.Lock()
		c.serverConn = nil
		c.mu.Unlock()

		// 关闭所有本地连接
		c.connMu.Lock()
		for _, conn := range c.connections {
			close(conn.closeChan)
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

// handleServerConnection 处理服务器连接
func (c *Client) handleServerConnection(conn net.Conn) error {
	for {
		select {
		case <-c.ctx.Done():
			return nil
		default:
		}

		// 读取消息头
		header := make([]byte, HeaderSize)
		if _, err := io.ReadFull(conn, header); err != nil {
			return err
		}

		dataLen := binary.BigEndian.Uint32(header[0:4])
		connID := binary.BigEndian.Uint32(header[4:8])

		if dataLen > MaxPacketSize {
			return fmt.Errorf("数据包过大: %d bytes", dataLen)
		}

		// 读取数据
		data := make([]byte, dataLen)
		if _, err := io.ReadFull(conn, data); err != nil {
			return err
		}

		// 处理数据
		c.handleData(connID, data)
	}
}

// handleData 处理接收到的数据
func (c *Client) handleData(connID uint32, data []byte) {
	c.connMu.Lock()
	localConn, exists := c.connections[connID]
	
	if !exists {
		// 新连接，需要建立到本地服务的连接
		// 从数据中解析目标端口（这里简化处理，实际应该从协议中获取）
		localConn = &LocalConnection{
			ID:        connID,
			closeChan: make(chan struct{}),
		}
		c.connections[connID] = localConn
		c.connMu.Unlock()
		
		// 启动本地连接处理
		c.wg.Add(1)
		go c.handleLocalConnection(localConn)
		
		// 重新获取锁并发送数据
		c.connMu.Lock()
	}
	c.connMu.Unlock()

	// 发送数据到本地连接
	if localConn.Conn != nil {
		localConn.Conn.Write(data)
	}
}

// handleLocalConnection 处理本地连接
func (c *Client) handleLocalConnection(localConn *LocalConnection) {
	defer c.wg.Done()
	defer func() {
		c.connMu.Lock()
		delete(c.connections, localConn.ID)
		c.connMu.Unlock()
		
		if localConn.Conn != nil {
			localConn.Conn.Close()
		}
	}()

	// 连接到本地目标服务
	// 这里使用固定的本地地址，实际应该根据映射配置
	targetAddr := localConn.TargetAddr
	if targetAddr == "" {
		targetAddr = "127.0.0.1:22" // 默认 SSH
	}

	conn, err := net.DialTimeout("tcp", targetAddr, 5*time.Second)
	if err != nil {
		log.Printf("连接本地服务失败 (连接 %d -> %s): %v", localConn.ID, targetAddr, err)
		return
	}
	defer conn.Close()

	localConn.Conn = conn
	log.Printf("建立本地连接: %d -> %s", localConn.ID, targetAddr)

	// 从本地服务读取数据并发送到服务器
	buffer := make([]byte, 32*1024)
	for {
		select {
		case <-localConn.closeChan:
			return
		case <-c.ctx.Done():
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, err := conn.Read(buffer)
		if err != nil {
			if err != io.EOF && !isTimeout(err) {
				log.Printf("读取本地连接失败 (连接 %d): %v", localConn.ID, err)
			}
			return
		}

		// 发送到服务器
		c.mu.RLock()
		serverConn := c.serverConn
		c.mu.RUnlock()

		if serverConn == nil {
			return
		}

		data := make([]byte, HeaderSize+n)
		binary.BigEndian.PutUint32(data[0:4], uint32(n))
		binary.BigEndian.PutUint32(data[4:8], localConn.ID)
		copy(data[HeaderSize:], buffer[:n])

		if _, err := serverConn.Write(data); err != nil {
			log.Printf("发送数据到服务器失败 (连接 %d): %v", localConn.ID, err)
			return
		}
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