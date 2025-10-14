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

// Protocol 定义隧道协议
// 消息格式: [4字节长度][4字节端口][数据]

const (
	// HeaderSize 消息头大小（长度+端口）
	HeaderSize = 8
	// MaxPacketSize 最大包大小 (1MB)
	MaxPacketSize = 1024 * 1024
)

// Server 内网穿透服务器
type Server struct {
	listenPort int
	listener   net.Listener
	client     net.Conn
	cancel     context.CancelFunc
	ctx        context.Context
	wg         sync.WaitGroup
	mu         sync.RWMutex
	
	// 连接管理
	connections map[uint32]*Connection
	connMu      sync.RWMutex
	nextConnID  uint32
}

// Connection 表示一个客户端连接
type Connection struct {
	ID         uint32
	TargetPort int
	ClientConn net.Conn
	readChan   chan []byte
	writeChan  chan []byte
	closeChan  chan struct{}
}

// NewServer 创建新的隧道服务器
func NewServer(listenPort int) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		listenPort:  listenPort,
		cancel:      cancel,
		ctx:         ctx,
		connections: make(map[uint32]*Connection),
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
		if s.client != nil {
			log.Printf("拒绝额外的隧道连接: %s", conn.RemoteAddr())
			conn.Close()
			s.mu.Unlock()
			continue
		}
		s.client = conn
		s.mu.Unlock()

		log.Printf("隧道客户端已连接: %s", conn.RemoteAddr())

		s.wg.Add(1)
		go s.handleClient(conn)
	}
}

// handleClient 处理客户端连接
func (s *Server) handleClient(conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		conn.Close()
		s.mu.Lock()
		s.client = nil
		s.mu.Unlock()
		log.Printf("隧道客户端已断开")
		
		// 关闭所有活动连接
		s.connMu.Lock()
		for _, c := range s.connections {
			close(c.closeChan)
		}
		s.connections = make(map[uint32]*Connection)
		s.connMu.Unlock()
	}()

	// 读取来自客户端的数据
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// 读取消息头
		header := make([]byte, HeaderSize)
		if _, err := io.ReadFull(conn, header); err != nil {
			if err != io.EOF {
				log.Printf("读取隧道消息头失败: %v", err)
			}
			return
		}

		dataLen := binary.BigEndian.Uint32(header[0:4])
		connID := binary.BigEndian.Uint32(header[4:8])

		if dataLen > MaxPacketSize {
			log.Printf("数据包过大: %d bytes", dataLen)
			return
		}

		// 读取数据
		data := make([]byte, dataLen)
		if _, err := io.ReadFull(conn, data); err != nil {
			log.Printf("读取隧道数据失败: %v", err)
			return
		}

		// 将数据发送到对应的连接
		s.connMu.RLock()
		connection, exists := s.connections[connID]
		s.connMu.RUnlock()

		if exists {
			select {
			case connection.readChan <- data:
			case <-connection.closeChan:
			case <-time.After(5 * time.Second):
				log.Printf("向连接 %d 发送数据超时", connID)
			}
		}
	}
}

// ForwardConnection 转发连接到隧道
func (s *Server) ForwardConnection(clientConn net.Conn, targetPort int) error {
	s.mu.RLock()
	tunnelConn := s.client
	s.mu.RUnlock()

	if tunnelConn == nil {
		return fmt.Errorf("隧道连接不可用")
	}

	// 创建连接对象
	s.connMu.Lock()
	connID := s.nextConnID
	s.nextConnID++
	
	connection := &Connection{
		ID:         connID,
		TargetPort: targetPort,
		ClientConn: clientConn,
		readChan:   make(chan []byte, 100),
		writeChan:  make(chan []byte, 100),
		closeChan:  make(chan struct{}),
	}
	s.connections[connID] = connection
	s.connMu.Unlock()

	defer func() {
		s.connMu.Lock()
		delete(s.connections, connID)
		s.connMu.Unlock()
		close(connection.closeChan)
		clientConn.Close()
	}()

	// 启动读写协程
	errChan := make(chan error, 2)

	// 从客户端读取并发送到隧道
	go func() {
		buffer := make([]byte, 32*1024)
		for {
			select {
			case <-connection.closeChan:
				return
			default:
			}

			clientConn.SetReadDeadline(time.Now().Add(30 * time.Second))
			n, err := clientConn.Read(buffer)
			if err != nil {
				errChan <- err
				return
			}

			// 发送到隧道
			data := make([]byte, HeaderSize+n)
			binary.BigEndian.PutUint32(data[0:4], uint32(n))
			binary.BigEndian.PutUint32(data[4:8], connID)
			copy(data[HeaderSize:], buffer[:n])

			s.mu.RLock()
			_, err = tunnelConn.Write(data)
			s.mu.RUnlock()
			
			if err != nil {
				errChan <- err
				return
			}
		}
	}()

	// 从隧道读取并发送到客户端
	go func() {
		for {
			select {
			case data := <-connection.readChan:
				if _, err := clientConn.Write(data); err != nil {
					errChan <- err
					return
				}
			case <-connection.closeChan:
				return
			}
		}
	}()

	// 等待错误或关闭
	select {
	case <-errChan:
	case <-connection.closeChan:
	case <-s.ctx.Done():
	}

	return nil
}

// IsConnected 检查隧道是否已连接
func (s *Server) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client != nil
}

// Stop 停止隧道服务器
func (s *Server) Stop() error {
	s.cancel()

	if s.listener != nil {
		s.listener.Close()
	}

	s.mu.Lock()
	if s.client != nil {
		s.client.Close()
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