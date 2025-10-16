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
	Tunnel      TrafficStats        `json:"tunnel"`         // 隧道流量
	Mappings    []PortTrafficStats  `json:"mappings"`       // 端口映射流量
	TotalSent   uint64              `json:"total_sent"`     // 总发送
	TotalReceived uint64            `json:"total_received"` // 总接收
	Timestamp   int64               `json:"timestamp"`      // 时间戳
}
