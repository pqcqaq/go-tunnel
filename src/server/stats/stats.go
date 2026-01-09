package stats

// TrafficStats 流量统计
type TrafficStats struct {
	BytesSent     uint64 `json:"bytes_sent"`     // 发送字节数
	BytesReceived uint64 `json:"bytes_received"` // 接收字节数
	LastUpdate    int64  `json:"last_update"`    // 最后更新时间（Unix时间戳）
}

// PortTrafficStats 端口流量统计
type PortTrafficStats struct {
	Port          int    `json:"port"`
	BytesSent     uint64 `json:"bytes_sent"`
	BytesReceived uint64 `json:"bytes_received"`
	LastUpdate    int64  `json:"last_update"`
}

// AllTrafficStats 所有流量统计
type AllTrafficStats struct {
	Tunnel        TrafficStats       `json:"tunnel"`         // 隧道流量
	Mappings      []PortTrafficStats `json:"mappings"`       // 端口映射流量
	TotalSent     uint64             `json:"total_sent"`     // 总发送
	TotalReceived uint64             `json:"total_received"` // 总接收
	Timestamp     int64              `json:"timestamp"`      // 时间戳
}

// ConnectionInfo 连接信息
type ConnectionInfo struct {
	ClientAddr    string `json:"client_addr"`    // 客户端地址
	TargetAddr    string `json:"target_addr"`    // 目标地址
	BytesSent     uint64 `json:"bytes_sent"`     // 该连接发送的字节数
	BytesReceived uint64 `json:"bytes_received"` // 该连接接收的字节数
	ConnectedAt   int64  `json:"connected_at"`   // 连接建立时间（Unix时间戳）
}

// PortConnectionStats 端口连接统计
type PortConnectionStats struct {
	SourcePort        int              `json:"source_port"`        // 源端口
	TargetHost        string           `json:"target_host"`        // 目标主机
	TargetPort        int              `json:"target_port"`        // 目标端口
	UseTunnel         bool             `json:"use_tunnel"`         // 是否使用隧道
	ActiveConnections []ConnectionInfo `json:"active_connections"` // 活跃连接列表
	TotalConnections  int              `json:"total_connections"`  // 总连接数
}

// AllConnectionsStats 所有连接统计
type AllConnectionsStats struct {
	Mappings  []PortConnectionStats `json:"mappings"`  // 所有映射的连接信息
	Timestamp int64                 `json:"timestamp"` // 时间戳
}
