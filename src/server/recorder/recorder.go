package recorder

import (
	"context"
	"log"
	"port-forward/server/db"
	"port-forward/server/forwarder"
	"port-forward/server/tunnel"
	"sync"
	"time"
)

// Recorder 流量统计记录器
type Recorder struct {
	db           *db.Database
	forwarderMgr *forwarder.Manager
	tunnelServer *tunnel.Server
	interval     time.Duration
	retentionDay int
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// New 创建新的记录器
func New(database *db.Database, fwdMgr *forwarder.Manager, ts *tunnel.Server, recordInterval, retentionDays int) *Recorder {
	ctx, cancel := context.WithCancel(context.Background())
	return &Recorder{
		db:           database,
		forwarderMgr: fwdMgr,
		tunnelServer: ts,
		interval:     time.Duration(recordInterval) * time.Second,
		retentionDay: retentionDays,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start 启动记录器
func (r *Recorder) Start() {
	log.Printf("流量统计记录器启动: 记录间隔=%v, 数据保留=%d天", r.interval, r.retentionDay)

	// 启动定时记录任务
	r.wg.Add(1)
	go r.recordLoop()

	// 启动定时清理任务（每小时执行一次）
	r.wg.Add(1)
	go r.cleanupLoop()
}

// Stop 停止记录器
func (r *Recorder) Stop() {
	log.Println("正在停止流量统计记录器...")
	r.cancel()
	r.wg.Wait()
	log.Println("流量统计记录器已停止")
}

// recordLoop 定时记录循环
func (r *Recorder) recordLoop() {
	defer r.wg.Done()

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.recordStats()
		case <-r.ctx.Done():
			return
		}
	}
}

// cleanupLoop 定时清理循环
func (r *Recorder) cleanupLoop() {
	defer r.wg.Done()

	// 每小时执行一次清理
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// 启动时先执行一次清理
	r.cleanup()

	for {
		select {
		case <-ticker.C:
			r.cleanup()
		case <-r.ctx.Done():
			return
		}
	}
}

// recordStats 记录当前统计信息
func (r *Recorder) recordStats() {
	// 记录隧道流量（is_tunnel=true, port可以是0或任意标识）
	if r.tunnelServer != nil {
		tunnelStats := r.tunnelServer.GetTrafficStats()
		if err := r.db.AddTrafficRecord(0, true, tunnelStats.BytesSent, tunnelStats.BytesReceived); err != nil {
			log.Printf("记录隧道流量失败: %v", err)
		} else {
			log.Printf("记录隧道流量: 发送=%d, 接收=%d", tunnelStats.BytesSent, tunnelStats.BytesReceived)
		}
	}

	// 记录各端口映射的流量（is_tunnel=false）
	forwarderStats := r.forwarderMgr.GetAllTrafficStats()
	recordCount := 0
	for port, stats := range forwarderStats {
		if err := r.db.AddTrafficRecord(port, false, stats.BytesSent, stats.BytesReceived); err != nil {
			log.Printf("记录端口 %d 流量失败: %v", port, err)
		} else {
			recordCount++
		}
	}

	if recordCount > 0 {
		log.Printf("已记录 %d 个端口的流量统计", recordCount)
	}
}

// cleanup 清理旧数据
func (r *Recorder) cleanup() {
	if err := r.db.CleanOldTrafficRecords(r.retentionDay); err != nil {
		log.Printf("清理旧流量记录失败: %v", err)
	}
}
