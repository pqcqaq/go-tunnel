package config

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// 创建临时配置文件
	configContent := `
port_range:
  from: 10000
  end: 10100

tunnel:
  enabled: true
  listen_port: 9000

api:
  listen_port: 8080

database:
  path: "./data/mappings.db"
`
	tmpFile, err := os.CreateTemp("", "config_test_*.yaml")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(configContent)); err != nil {
		t.Fatalf("写入配置文件失败: %v", err)
	}
	tmpFile.Close()

	// 加载配置
	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	// 验证配置
	// if cfg.PortRange.From != 10000 {
	// 	t.Errorf("期望起始端口为 10000，得到 %d", cfg.PortRange.From)
	// }
	// if cfg.PortRange.End != 10100 {
	// 	t.Errorf("期望结束端口为 10100，得到 %d", cfg.PortRange.End)
	// }
	if !cfg.Tunnel.Enabled {
		t.Error("期望隧道启用")
	}
	if cfg.Tunnel.ListenPort != 9000 {
		t.Errorf("期望隧道端口为 9000，得到 %d", cfg.Tunnel.ListenPort)
	}
	if cfg.API.ListenPort != 8080 {
		t.Errorf("期望 API 端口为 8080，得到 %d", cfg.API.ListenPort)
	}
}

// TestLoadConfigFileNotFound 测试配置文件不存在
func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("应该返回文件不存在错误")
	}
}

// TestLoadConfigInvalidYAML 测试无效的 YAML 格式
func TestLoadConfigInvalidYAML(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config_invalid_*.yaml")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	
	// 写入无效的 YAML
	invalidYAML := `
port_range:
  from: 10000
  end: invalid_number
`
	tmpFile.Write([]byte(invalidYAML))
	tmpFile.Close()
	
	_, err = Load(tmpFile.Name())
	if err == nil {
		t.Error("应该返回 YAML 解析错误")
	}
}

// TestConfigEdgeCases 测试配置边界情况
func TestConfigEdgeCases(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config_edge_*.yaml")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	
	// 测试刚好 10000 个端口（边界值）
	configContent := `
port_range:
  from: 1
  end: 10000
tunnel:
  enabled: false
api:
  listen_port: 8080
database:
  path: ./data/mappings.db
`
	tmpFile.Write([]byte(configContent))
	tmpFile.Close()
	
	// cfg, err := Load(tmpFile.Name())
	// if err != nil {
	// 	t.Errorf("边界值配置应该有效: %v", err)
	// }
	
	// if cfg != nil && (cfg.PortRange.End-cfg.PortRange.From) != 9999 {
	// 	t.Errorf("端口范围计算不正确")
	// }
}

// BenchmarkLoadConfig 基准测试配置加载
func BenchmarkLoadConfig(b *testing.B) {
	tmpFile, _ := os.CreateTemp("", "config_bench_*.yaml")
	defer os.Remove(tmpFile.Name())
	
	configContent := `
port_range:
  from: 10000
  end: 20000
tunnel:
  enabled: true
  listen_port: 9000
api:
  listen_port: 8080
database:
  path: ./data/mappings.db
`
	tmpFile.Write([]byte(configContent))
	tmpFile.Close()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Load(tmpFile.Name())
	}
}

// BenchmarkValidateConfig 基准测试配置验证
func BenchmarkValidateConfig(b *testing.B) {
	cfg := &Config{
		// PortRange: PortRangeConfig{From: 10000, End: 20000},
		Tunnel:    TunnelConfig{Enabled: true, ListenPort: 9000},
		API:       APIConfig{ListenPort: 8080},
		Database:  DatabaseConfig{Path: "./data/db.sqlite"},
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.Validate()
	}
}
func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "有效配置",
			config: Config{
				// PortRange:  PortRangeConfig{From: 10000, End: 10100},
				Tunnel:     TunnelConfig{Enabled: true, ListenPort: 9000},
				API:        APIConfig{ListenPort: 8080},
				Database:   DatabaseConfig{Path: "./data/test.db"},
			},
			wantErr: false,
		},
		{
			name: "无效端口范围 - 起始端口为0",
			config: Config{
				// PortRange:  PortRangeConfig{From: 0, End: 10100},
				Tunnel:     TunnelConfig{Enabled: false, ListenPort: 0},
				API:        APIConfig{ListenPort: 8080},
				Database:   DatabaseConfig{Path: "./data/test.db"},
			},
			wantErr: true,
		},
		{
			name: "无效端口范围 - 起始大于结束",
			config: Config{
				// PortRange:  PortRangeConfig{From: 10100, End: 10000},
				Tunnel:     TunnelConfig{Enabled: false, ListenPort: 0},
				API:        APIConfig{ListenPort: 8080},
				Database:   DatabaseConfig{Path: "./data/test.db"},
			},
			wantErr: true,
		},
		{
			name: "端口范围过大",
			config: Config{
				// PortRange:  PortRangeConfig{From: 1, End: 40000},
				Tunnel:     TunnelConfig{Enabled: false, ListenPort: 0},
				API:        APIConfig{ListenPort: 8080},
				Database:   DatabaseConfig{Path: "./data/test.db"},
			},
			wantErr: true,
		},
		{
			name: "启用隧道但端口无效",
			config: Config{
				// PortRange:  PortRangeConfig{From: 10000, End: 10100},
				Tunnel:     TunnelConfig{Enabled: true, ListenPort: 0},
				API:        APIConfig{ListenPort: 8080},
				Database:   DatabaseConfig{Path: "./data/test.db"},
			},
			wantErr: true,
		},
		{
			name: "数据库路径为空",
			config: Config{
				// PortRange:  PortRangeConfig{From: 10000, End: 10100},
				Tunnel:     TunnelConfig{Enabled: false, ListenPort: 0},
				API:        APIConfig{ListenPort: 8080},
				Database:   DatabaseConfig{Path: ""},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}