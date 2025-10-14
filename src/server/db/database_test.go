package db

import (
	"os"
	"testing"
)

func TestDatabase(t *testing.T) {
	// 使用临时数据库
	dbPath := "/tmp/test_mappings.db"
	defer os.Remove(dbPath)

	// 创建数据库
	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("创建数据库失败: %v", err)
	}
	defer db.Close()

	t.Run("添加映射", func(t *testing.T) {
		err := db.AddMapping(10001, "192.168.1.100", 22)
		if err != nil {
			t.Errorf("添加映射失败: %v", err)
		}
	})

	t.Run("获取映射", func(t *testing.T) {
		mapping, err := db.GetMapping(10001)
		if err != nil {
			t.Errorf("获取映射失败: %v", err)
		}
		if mapping == nil {
			t.Error("映射不应该为空")
		}
		if mapping.SourcePort != 10001 {
			t.Errorf("期望源端口为 10001，得到 %d", mapping.SourcePort)
		}
		if mapping.TargetIP != "192.168.1.100" {
			t.Errorf("期望目标 IP 为 192.168.1.100，得到 %s", mapping.TargetIP)
		}
		if mapping.TargetPort != 22 {
			t.Errorf("期望目标端口为 22，得到 %d", mapping.TargetPort)
		}
	})

	t.Run("添加重复映射应该失败", func(t *testing.T) {
		err := db.AddMapping(10001, "192.168.1.101", 22)
		if err == nil {
			t.Error("添加重复映射应该失败")
		}
	})

	t.Run("获取所有映射", func(t *testing.T) {
		// 添加更多映射
		db.AddMapping(10002, "192.168.1.101", 22)
		db.AddMapping(10003, "192.168.1.102", 22)

		mappings, err := db.GetAllMappings()
		if err != nil {
			t.Errorf("获取所有映射失败: %v", err)
		}
		if len(mappings) != 3 {
			t.Errorf("期望 3 个映射，得到 %d", len(mappings))
		}
	})

	t.Run("删除映射", func(t *testing.T) {
		err := db.RemoveMapping(10001)
		if err != nil {
			t.Errorf("删除映射失败: %v", err)
		}

		mapping, err := db.GetMapping(10001)
		if err != nil {
			t.Errorf("查询映射失败: %v", err)
		}
		if mapping != nil {
			t.Error("映射应该已被删除")
		}
	})

	t.Run("删除不存在的映射应该失败", func(t *testing.T) {
		err := db.RemoveMapping(99999)
		if err == nil {
			t.Error("删除不存在的映射应该失败")
		}
	})
}

func TestDatabaseConcurrency(t *testing.T) {
	dbPath := "/tmp/test_concurrent.db"
	defer os.Remove(dbPath)

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("创建数据库失败: %v", err)
	}
	defer db.Close()

	// 并发添加映射
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(port int) {
			err := db.AddMapping(10000+port, "192.168.1.100", port)
			if err != nil {
				t.Logf("添加映射失败 (端口 %d): %v", 10000+port, err)
			}
			done <- true
		}(i)
	}

	// 等待所有操作完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证映射数量
	mappings, err := db.GetAllMappings()
	if err != nil {
		t.Errorf("获取所有映射失败: %v", err)
	}
	if len(mappings) == 0 {
		t.Error("应该至少有一些映射")
	}
}