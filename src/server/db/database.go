package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

// Mapping 端口映射结构
type Mapping struct {
	ID             int64  `json:"id"`
	SourcePort     int    `json:"source_port"`
	TargetHost     string `json:"target_host"` // 支持IP或域名
	TargetPort     int    `json:"target_port"`
	UseTunnel      bool   `json:"use_tunnel"`
	BandwidthLimit *int64 `json:"bandwidth_limit,omitempty"` // 带宽限制，字节/秒，可为空
	CreatedAt      string `json:"created_at"`
	// 规则，白名单还是黑名单
	AccessRule *string `json:"access_rule,omitempty"` // 可选，访问控制规则，JSON格式
	AccessIPs  *string `json:"access_ips,omitempty"`  // 可选，访问控制的IP列表，JSON格式
}

// TrafficRecord 流量统计记录
type TrafficRecord struct {
	ID            int64  `json:"id"`
	Port          int    `json:"port"`           // 端口号
	IsTunnel      bool   `json:"is_tunnel"`      // 是否为隧道整体流量
	BytesSent     uint64 `json:"bytes_sent"`     // 发送字节数
	BytesReceived uint64 `json:"bytes_received"` // 接收字节数
	RecordedAt    string `json:"recorded_at"`    // 记录时间
}

// Database 数据库管理器
type Database struct {
	db *sql.DB
	mu sync.RWMutex
}

// New 创建新的数据库管理器
func New(dbPath string) (*Database, error) {
	// 确保数据库目录存在
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据库目录失败: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	database := &Database{db: db}

	// 初始化表结构
	if err := database.initTables(); err != nil {
		db.Close()
		return nil, err
	}

	return database, nil
}

// initTables 初始化数据库表
func (d *Database) initTables() error {
	// 创建表结构
	query := `
	CREATE TABLE IF NOT EXISTS mappings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_port INTEGER NOT NULL UNIQUE,
		target_host TEXT NOT NULL,
		target_port INTEGER NOT NULL,
		use_tunnel BOOLEAN NOT NULL DEFAULT 0,
		bandwidth_limit INTEGER,
		access_rule TEXT,
		access_ips TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_source_port ON mappings(source_port);
	
	CREATE TABLE IF NOT EXISTS traffic_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		port INTEGER NOT NULL,
		is_tunnel BOOLEAN NOT NULL DEFAULT 0,
		bytes_sent INTEGER NOT NULL DEFAULT 0,
		bytes_received INTEGER NOT NULL DEFAULT 0,
		recorded_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_port ON traffic_records(port);
	CREATE INDEX IF NOT EXISTS idx_is_tunnel ON traffic_records(is_tunnel);
	CREATE INDEX IF NOT EXISTS idx_recorded_at ON traffic_records(recorded_at);
	`

	_, err := d.db.Exec(query)
	if err != nil {
		return fmt.Errorf("初始化数据库表失败: %w", err)
	}

	// 检查是否需要迁移现有数据
	if err := d.migrateDatabase(); err != nil {
		return fmt.Errorf("数据库迁移失败: %w", err)
	}

	return nil
}

// migrateDatabase 迁移现有数据库结构
func (d *Database) migrateDatabase() error {
	// 检查 use_tunnel 列是否存在
	rows, err := d.db.Query("PRAGMA table_info(mappings)")
	if err != nil {
		return fmt.Errorf("检查表结构失败: %w", err)
	}
	defer rows.Close()

	hasUseTunnel := false
	hasTargetHost := false
	hasBandwidthLimit := false
	hasAccessRule := false
	hasAccessIPs := false
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, hasDefault int
		var defaultValue interface{}

		err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &hasDefault)
		if err != nil {
			return fmt.Errorf("扫描表结构失败: %w", err)
		}

		if name == "use_tunnel" {
			hasUseTunnel = true
		}
		if name == "target_host" {
			hasTargetHost = true
		}
		if name == "bandwidth_limit" {
			hasBandwidthLimit = true
		}
		if name == "access_rule" {
			hasAccessRule = true
		}
		if name == "access_ips" {
			hasAccessIPs = true
		}
	}

	// 如果不存在 use_tunnel 列，则添加它
	if !hasUseTunnel {
		_, err := d.db.Exec("ALTER TABLE mappings ADD COLUMN use_tunnel BOOLEAN NOT NULL DEFAULT 0")
		if err != nil {
			return fmt.Errorf("添加 use_tunnel 列失败: %w", err)
		}
	}

	// 如果还使用旧的 target_ip 列，则重命名为 target_host
	if !hasTargetHost {
		// 检查是否存在 target_ip 列
		rows2, err := d.db.Query("PRAGMA table_info(mappings)")
		if err != nil {
			return fmt.Errorf("检查表结构失败: %w", err)
		}
		defer rows2.Close()

		hasTargetIP := false
		for rows2.Next() {
			var cid int
			var name, dataType string
			var notNull, hasDefault int
			var defaultValue interface{}

			err := rows2.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &hasDefault)
			if err != nil {
				return fmt.Errorf("扫描表结构失败: %w", err)
			}

			if name == "target_ip" {
				hasTargetIP = true
				break
			}
		}

		if hasTargetIP {
			// 重命名 target_ip 为 target_host
			_, err := d.db.Exec("ALTER TABLE mappings RENAME COLUMN target_ip TO target_host")
			if err != nil {
				return fmt.Errorf("重命名 target_ip 列失败: %w", err)
			}
		} else {
			// 如果既没有 target_ip 也没有 target_host，添加 target_host 列
			_, err := d.db.Exec("ALTER TABLE mappings ADD COLUMN target_host TEXT NOT NULL DEFAULT ''")
			if err != nil {
				return fmt.Errorf("添加 target_host 列失败: %w", err)
			}
		}
	}

	// 如果不存在 bandwidth_limit 列，则添加它
	if !hasBandwidthLimit {
		_, err := d.db.Exec("ALTER TABLE mappings ADD COLUMN bandwidth_limit INTEGER")
		if err != nil {
			return fmt.Errorf("添加 bandwidth_limit 列失败: %w", err)
		}
	}

	// 如果不存在 access_rule 列，则添加它
	if !hasAccessRule {
		_, err := d.db.Exec("ALTER TABLE mappings ADD COLUMN access_rule TEXT")
		if err != nil {
			return fmt.Errorf("添加 access_rule 列失败: %w", err)
		}
	}

	// 如果不存在 access_ips 列，则添加它
	if !hasAccessIPs {
		_, err := d.db.Exec("ALTER TABLE mappings ADD COLUMN access_ips TEXT")
		if err != nil {
			return fmt.Errorf("添加 access_ips 列失败: %w", err)
		}
	}

	// 检查 traffic_records 表的 is_tunnel 列
	rows3, err := d.db.Query("PRAGMA table_info(traffic_records)")
	if err == nil {
		defer rows3.Close()
		hasIsTunnel := false
		for rows3.Next() {
			var cid int
			var name, dataType string
			var notNull, hasDefault int
			var defaultValue interface{}

			err := rows3.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &hasDefault)
			if err != nil {
				return fmt.Errorf("扫描 traffic_records 表结构失败: %w", err)
			}

			if name == "is_tunnel" {
				hasIsTunnel = true
				break
			}
		}

		// 如果不存在 is_tunnel 列，则添加它
		if !hasIsTunnel {
			_, err := d.db.Exec("ALTER TABLE traffic_records ADD COLUMN is_tunnel BOOLEAN NOT NULL DEFAULT 0")
			if err != nil {
				return fmt.Errorf("添加 is_tunnel 列失败: %w", err)
			}
		}
	}

	return nil
}

// AddMapping 添加带宽限制的端口映射
func (d *Database) AddMapping(sourcePort int, targetHost string, targetPort int, useTunnel bool, bandwidthLimit *int64, accessRule *string, accessIPs *string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `INSERT INTO mappings (source_port, target_host, target_port, use_tunnel, bandwidth_limit, access_rule, access_ips) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := d.db.Exec(query, sourcePort, targetHost, targetPort, useTunnel, bandwidthLimit, accessRule, accessIPs)
	if err != nil {
		return fmt.Errorf("添加端口映射失败: %w", err)
	}

	return nil
}

// RemoveMapping 删除端口映射
func (d *Database) RemoveMapping(sourcePort int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `DELETE FROM mappings WHERE source_port = ?`
	result, err := d.db.Exec(query, sourcePort)
	if err != nil {
		return fmt.Errorf("删除端口映射失败: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("端口映射不存在")
	}

	return nil
}

// UpdateMapping 更新端口映射的访问控制规则
func (d *Database) UpdateMapping(sourcePort int, limit *int64, accessRule *string, accessIPs *string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 先把老的查出来，如果是nil的话就赋值上老的
	selectQuery := `SELECT bandwidth_limit, access_rule, access_ips FROM mappings WHERE source_port = ?`
	var oldLimit sql.NullInt64
	var oldAccessRule sql.NullString
	var oldAccessIPs sql.NullString

	err := d.db.QueryRow(selectQuery, sourcePort).Scan(&oldLimit, &oldAccessRule, &oldAccessIPs)
	if err != nil {
		return fmt.Errorf("查询现有映射失败: %w", err)
	}

	if limit == nil && oldLimit.Valid {
		limit = &oldLimit.Int64
	}
	if accessRule == nil && oldAccessRule.Valid {
		accessRule = &oldAccessRule.String
	}
	if accessIPs == nil && oldAccessIPs.Valid {
		accessIPs = &oldAccessIPs.String
	}

	// 然后更新
	query := `UPDATE mappings SET bandwidth_limit = ?, access_rule = ?, access_ips = ? WHERE source_port = ?`
	result, err := d.db.Exec(query, limit, accessRule, accessIPs, sourcePort)
	if err != nil {
		return fmt.Errorf("更新访问规则失败: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("端口映射不存在")
	}

	return nil
}

// GetMapping 获取指定端口的映射
func (d *Database) GetMapping(sourcePort int) (*Mapping, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	query := `SELECT id, source_port, target_host, target_port, use_tunnel, bandwidth_limit, access_rule, access_ips, created_at FROM mappings WHERE source_port = ?`

	var mapping Mapping
	err := d.db.QueryRow(query, sourcePort).Scan(
		&mapping.ID,
		&mapping.SourcePort,
		&mapping.TargetHost,
		&mapping.TargetPort,
		&mapping.UseTunnel,
		&mapping.BandwidthLimit,
		&mapping.AccessRule,
		&mapping.AccessIPs,
		&mapping.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询端口映射失败: %w", err)
	}

	return &mapping, nil
}

// GetAllMappings 获取所有端口映射
func (d *Database) GetAllMappings() ([]*Mapping, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	query := `SELECT id, source_port, target_host, target_port, use_tunnel, bandwidth_limit, access_rule, access_ips, created_at FROM mappings ORDER BY source_port`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("查询所有映射失败: %w", err)
	}
	defer rows.Close()

	var mappings []*Mapping
	for rows.Next() {
		var mapping Mapping
		if err := rows.Scan(
			&mapping.ID,
			&mapping.SourcePort,
			&mapping.TargetHost,
			&mapping.TargetPort,
			&mapping.UseTunnel,
			&mapping.BandwidthLimit,
			&mapping.AccessRule,
			&mapping.AccessIPs,
			&mapping.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("扫描映射记录失败: %w", err)
		}
		mappings = append(mappings, &mapping)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历映射记录失败: %w", err)
	}

	return mappings, nil
}

// Close 关闭数据库连接
func (d *Database) Close() error {
	return d.db.Close()
}

// AddTrafficRecord 添加流量统计记录
func (d *Database) AddTrafficRecord(port int, isTunnel bool, bytesSent, bytesReceived uint64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `INSERT INTO traffic_records (port, is_tunnel, bytes_sent, bytes_received) VALUES (?, ?, ?, ?)`
	_, err := d.db.Exec(query, port, isTunnel, bytesSent, bytesReceived)
	if err != nil {
		return fmt.Errorf("添加流量记录失败: %w", err)
	}

	return nil
}

// CleanOldTrafficRecords 清理旧的流量记录
func (d *Database) CleanOldTrafficRecords(retentionDays int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `DELETE FROM traffic_records WHERE recorded_at < datetime('now', '-' || ? || ' days')`
	result, err := d.db.Exec(query, retentionDays)
	if err != nil {
		return fmt.Errorf("清理旧流量记录失败: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		fmt.Printf("已清理 %d 条旧流量记录（保留 %d 天）\n", rows, retentionDays)
	}

	return nil
}

// GetTrafficRecords 获取流量记录
func (d *Database) GetTrafficRecords(port int, limit int) ([]*TrafficRecord, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var query string
	var rows *sql.Rows
	var err error

	if port == -1 {
		// 获取所有记录
		query = `SELECT id, port, is_tunnel, bytes_sent, bytes_received, recorded_at 
				 FROM traffic_records 
				 ORDER BY recorded_at DESC 
				 LIMIT ?`
		rows, err = d.db.Query(query, limit)
	} else {
		// 获取指定端口的记录
		query = `SELECT id, port, is_tunnel, bytes_sent, bytes_received, recorded_at 
				 FROM traffic_records 
				 WHERE port = ? 
				 ORDER BY recorded_at DESC 
				 LIMIT ?`
		rows, err = d.db.Query(query, port, limit)
	}

	if err != nil {
		return nil, fmt.Errorf("查询流量记录失败: %w", err)
	}
	defer rows.Close()

	var records []*TrafficRecord
	for rows.Next() {
		var record TrafficRecord
		if err := rows.Scan(
			&record.ID,
			&record.Port,
			&record.IsTunnel,
			&record.BytesSent,
			&record.BytesReceived,
			&record.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf("扫描流量记录失败: %w", err)
		}
		records = append(records, &record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历流量记录失败: %w", err)
	}

	return records, nil
}
