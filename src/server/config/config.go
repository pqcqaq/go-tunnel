package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 应用配置结构
type Config struct {
	PortRange PortRangeConfig `yaml:"port_range"`
	Tunnel    TunnelConfig    `yaml:"tunnel"`
	API       APIConfig       `yaml:"api"`
	Database  DatabaseConfig  `yaml:"database"`
}

// PortRangeConfig 端口范围配置
type PortRangeConfig struct {
	From int `yaml:"from"`
	End  int `yaml:"end"`
}

// TunnelConfig 内网穿透配置
type TunnelConfig struct {
	Enabled    bool `yaml:"enabled"`
	ListenPort int  `yaml:"listen_port"`
}

// APIConfig HTTP API 配置
type APIConfig struct {
	ListenPort int `yaml:"listen_port"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// Load 从文件加载配置
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	return &config, nil
}

// Validate 验证配置的有效性
func (c *Config) Validate() error {
	if c.PortRange.From <= 0 || c.PortRange.End <= 0 {
		return fmt.Errorf("端口范围必须大于 0")
	}
	if c.PortRange.From > c.PortRange.End {
		return fmt.Errorf("起始端口不能大于结束端口")
	}
	if c.PortRange.End-c.PortRange.From > 10000 {
		return fmt.Errorf("端口范围过大，最多支持 10000 个端口")
	}
	if c.Tunnel.Enabled && c.Tunnel.ListenPort <= 0 {
		return fmt.Errorf("内网穿透端口必须大于 0")
	}
	if c.API.ListenPort <= 0 {
		return fmt.Errorf("API 端口必须大于 0")
	}
	if c.Database.Path == "" {
		return fmt.Errorf("数据库路径不能为空")
	}
	return nil
}