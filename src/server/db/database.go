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
	ID          int64  `json:"id"`
	SourcePort  int    `json:"source_port"`
	TargetIP    string `json:"target_ip"`
	TargetPort  int    `json:"target_port"`
	UseTunnel   bool   `json:"use_tunnel"`
	CreatedAt   string `json:"created_at"`
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
		target_ip TEXT NOT NULL,
		target_port INTEGER NOT NULL,
		use_tunnel BOOLEAN NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_source_port ON mappings(source_port);
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
			break
		}
	}

	// 如果不存在 use_tunnel 列，则添加它
	if !hasUseTunnel {
		_, err := d.db.Exec("ALTER TABLE mappings ADD COLUMN use_tunnel BOOLEAN NOT NULL DEFAULT 0")
		if err != nil {
			return fmt.Errorf("添加 use_tunnel 列失败: %w", err)
		}
	}

	return nil
}

// AddMapping 添加端口映射
func (d *Database) AddMapping(sourcePort int, targetIP string, targetPort int, useTunnel bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `INSERT INTO mappings (source_port, target_ip, target_port, use_tunnel) VALUES (?, ?, ?, ?)`
	_, err := d.db.Exec(query, sourcePort, targetIP, targetPort, useTunnel)
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

// GetMapping 获取指定端口的映射
func (d *Database) GetMapping(sourcePort int) (*Mapping, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	query := `SELECT id, source_port, target_ip, target_port, use_tunnel, created_at FROM mappings WHERE source_port = ?`
	
	var mapping Mapping
	err := d.db.QueryRow(query, sourcePort).Scan(
		&mapping.ID,
		&mapping.SourcePort,
		&mapping.TargetIP,
		&mapping.TargetPort,
		&mapping.UseTunnel,
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

	query := `SELECT id, source_port, target_ip, target_port, use_tunnel, created_at FROM mappings ORDER BY source_port`
	
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
			&mapping.TargetIP,
			&mapping.TargetPort,
			&mapping.UseTunnel,
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