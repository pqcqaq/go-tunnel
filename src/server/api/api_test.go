package api

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"port-forward/server/db"
	"port-forward/server/forwarder"
	"port-forward/server/tunnel"
	"testing"
)

const testAPIKey = "test-api-key-12345"

// setupTestHandler 创建测试用的 Handler
func setupTestHandler(t *testing.T, useTunnel bool) (*Handler, *db.Database, func()) {
	// 创建临时数据库
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("创建数据库失败: %v", err)
	}

	// 创建转发器管理器
	fwdMgr := forwarder.NewManager()

	// 创建隧道服务器（如果启用）
	var tunnelServer *tunnel.Server
	if useTunnel {
		// 使用随机端口
		listener, _ := net.Listen("tcp", "127.0.0.1:0")
		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()

		tunnelServer = tunnel.NewServer(port)
		tunnelServer.Start()
	}

	handler := NewHandler(database, fwdMgr, tunnelServer, testAPIKey)

	cleanup := func() {
		fwdMgr.StopAll()
		if tunnelServer != nil {
			tunnelServer.Stop()
		}
		database.Close()
		os.RemoveAll(tmpDir)
	}

	return handler, database, cleanup
}

// addAuthHeader 添加认证头到请求
func addAuthHeader(req *http.Request) {
	req.Header.Set("X-API-Key", testAPIKey)
}

// TestNewHandler 测试创建处理器
func TestNewHandler(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t, false)
	defer cleanup()

	if handler == nil {
		t.Fatal("创建处理器失败")
	}

	// if handler.portRangeFrom != 10000 {
	// 	t.Errorf("起始端口不正确，期望 10000，得到 %d", handler.portRangeFrom)
	// }

	// if handler.portRangeEnd != 20000 {
	// 	t.Errorf("结束端口不正确，期望 20000，得到 %d", handler.portRangeEnd)
	// }
}

// TestHandleHealth 测试健康检查
func TestHandleHealth(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t, false)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("状态码不正确，期望 200，得到 %d", w.Code)
	}

	var result map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&result)
	if err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}

	if result["status"] != "ok" {
		t.Errorf("健康状态不正确，期望 ok，得到 %v", result["status"])
	}
}

// TestHandleHealthWithTunnel 测试带隧道的健康检查
func TestHandleHealthWithTunnel(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t, true)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("状态码不正确，期望 200，得到 %d", w.Code)
	}

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)

	if result["tunnel_enabled"] != true {
		t.Error("隧道应该启用")
	}

	// 隧道未连接客户端时应该为 false
	if result["tunnel_connected"] != false {
		t.Error("隧道应该未连接")
	}
}

// TestHandleCreateMapping 测试创建映射
func TestHandleCreateMapping(t *testing.T) {
	handler, database, cleanup := setupTestHandler(t, false)
	defer cleanup()

	reqBody := CreateMappingRequest{
		// Port:     15000,
		SourcePort: 15000,
		TargetPort: 15000,
		TargetHost: "192.168.1.100",
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/mapping/create", bytes.NewReader(body))
	addAuthHeader(req)
	w := httptest.NewRecorder()

	handler.handleCreateMapping(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("状态码不正确，期望 200，得到 %d", w.Code)
	}

	var result Response
	json.NewDecoder(w.Body).Decode(&result)

	if !result.Success {
		t.Errorf("创建映射失败: %s", result.Message)
	}

	// 验证数据库中存在映射
	mapping, err := database.GetMapping(15000)
	if err != nil {
		t.Fatalf("获取映射失败: %v", err)
	}

	if mapping == nil {
		t.Fatal("映射不存在")
	}

	if mapping.TargetHost != "192.168.1.100" {
		t.Errorf("目标 IP 不正确，期望 192.168.1.100，得到 %s", mapping.TargetHost)
	}
}

// TestHandleCreateMappingInvalidPort 测试创建映射时端口无效
func TestHandleCreateMappingInvalidPort(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t, false)
	defer cleanup()

	tests := []struct {
		name string
		port int
	}{
		{"端口太小", 5000},
		{"端口太大", 25000},
		{"端口为0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := CreateMappingRequest{
				SourcePort: tt.port,
				TargetPort: tt.port,
				TargetHost: "192.168.1.100",
			}

			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest(http.MethodPost, "/api/mapping/create", bytes.NewReader(body))
			addAuthHeader(req)
			w := httptest.NewRecorder()

			handler.handleCreateMapping(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("状态码不正确，期望 400，得到 %d", w.Code)
			}
		})
	}
}

// TestHandleCreateMappingDuplicate 测试创建重复映射
func TestHandleCreateMappingDuplicate(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t, false)
	defer cleanup()

	reqBody := CreateMappingRequest{
		// Port:     15000,
		SourcePort: 15000,
		TargetPort: 15000,
		TargetHost: "192.168.1.100",
	}

	// 第一次创建
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/mapping/create", bytes.NewReader(body))
	addAuthHeader(req)
	w := httptest.NewRecorder()
	handler.handleCreateMapping(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("第一次创建失败")
	}

	// 第二次创建（应该失败）
	body, _ = json.Marshal(reqBody)
	req = httptest.NewRequest(http.MethodPost, "/api/mapping/create", bytes.NewReader(body))
	addAuthHeader(req)
	w = httptest.NewRecorder()
	handler.handleCreateMapping(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("状态码不正确，期望 409，得到 %d", w.Code)
	}
}

// TestHandleCreateMappingInvalidJSON 测试无效的 JSON
func TestHandleCreateMappingInvalidJSON(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t, false)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/mapping/create", bytes.NewReader([]byte("invalid json")))
	addAuthHeader(req)
	w := httptest.NewRecorder()

	handler.handleCreateMapping(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("状态码不正确，期望 400，得到 %d", w.Code)
	}
}

// TestHandleCreateMappingInvalidIP 测试无效的 IP 地址
func TestHandleCreateMappingInvalidIP(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t, false)
	defer cleanup()

	reqBody := CreateMappingRequest{
		// Port:     15000,
		SourcePort: 15000,
		TargetPort: 15000,
		TargetHost: "", // 使用空字符串而不是无效域名，避免DNS查询超时
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/mapping/create", bytes.NewReader(body))
	addAuthHeader(req)
	w := httptest.NewRecorder()

	handler.handleCreateMapping(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("状态码不正确，期望 400，得到 %d", w.Code)
	}
}

// TestHandleCreateMappingEmptyIP 测试空 IP 地址
func TestHandleCreateMappingEmptyIP(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t, false)
	defer cleanup()

	reqBody := CreateMappingRequest{
		// Port:     15000,
		SourcePort: 15000,
		TargetPort: 15000,
		TargetHost: "",
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/mapping/create", bytes.NewReader(body))
	addAuthHeader(req)
	w := httptest.NewRecorder()

	handler.handleCreateMapping(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("状态码不正确，期望 400，得到 %d", w.Code)
	}
}

// TestHandleCreateMappingTunnelNotConnected 测试隧道未连接时创建映射
func TestHandleCreateMappingTunnelNotConnected(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t, true)
	defer cleanup()

	reqBody := CreateMappingRequest{
		// Port:      15000,
		SourcePort: 15000,
		TargetPort: 15000,
		UseTunnel:  true, // 明确指定使用隧道模式
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/mapping/create", bytes.NewReader(body))
	addAuthHeader(req)
	w := httptest.NewRecorder()

	handler.handleCreateMapping(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("状态码不正确，期望 503，得到 %d", w.Code)
	}
}

// TestHandleRemoveMapping 测试删除映射
func TestHandleRemoveMapping(t *testing.T) {
	handler, database, cleanup := setupTestHandler(t, false)
	defer cleanup()

	// 先创建一个映射
	database.AddMapping(15000, "192.168.1.100", 15000, false, nil)
	handler.forwarderMgr.Add(15000, "192.168.1.100", 15000, nil)

	reqBody := RemoveMappingRequest{
		Port: 15000,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/mapping/remove", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.handleRemoveMapping(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("状态码不正确，期望 200，得到 %d", w.Code)
	}

	// 验证映射已删除
	mapping, _ := database.GetMapping(15000)
	if mapping != nil {
		t.Error("映射应该已被删除")
	}
}

// TestHandleRemoveMappingNotExist 测试删除不存在的映射
func TestHandleRemoveMappingNotExist(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t, false)
	defer cleanup()

	reqBody := RemoveMappingRequest{
		Port: 15000,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/mapping/remove", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.handleRemoveMapping(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("状态码不正确，期望 404，得到 %d", w.Code)
	}
}

// TestHandleListMappings 测试列出映射
func TestHandleListMappings(t *testing.T) {
	handler, database, cleanup := setupTestHandler(t, false)
	defer cleanup()

	// 添加一些映射
	database.AddMapping(15000, "192.168.1.100", 15000, false, nil)
	database.AddMapping(15001, "192.168.1.101", 15001, true, nil)
	database.AddMapping(15002, "192.168.1.102", 15002, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/mapping/list", nil)
	addAuthHeader(req)
	w := httptest.NewRecorder()

	handler.handleListMappings(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("状态码不正确，期望 200，得到 %d", w.Code)
	}

	var result Response
	json.NewDecoder(w.Body).Decode(&result)

	if !result.Success {
		t.Errorf("列出映射失败: %s", result.Message)
	}

	data := result.Data.(map[string]interface{})
	count := int(data["count"].(float64))

	if count != 3 {
		t.Errorf("映射数量不正确，期望 3，得到 %d", count)
	}
}

// TestHandleListMappingsEmpty 测试列出空映射列表
func TestHandleListMappingsEmpty(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t, false)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/mapping/list", nil)
	addAuthHeader(req)
	w := httptest.NewRecorder()

	handler.handleListMappings(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("状态码不正确，期望 200，得到 %d", w.Code)
	}

	var result Response
	json.NewDecoder(w.Body).Decode(&result)

	data := result.Data.(map[string]interface{})
	count := int(data["count"].(float64))

	if count != 0 {
		t.Errorf("映射数量不正确，期望 0，得到 %d", count)
	}
}

// TestHandleMethodNotAllowed 测试不允许的 HTTP 方法
func TestHandleMethodNotAllowed(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t, false)
	defer cleanup()

	tests := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
		method  string
	}{
		{"创建映射 GET", handler.handleCreateMapping, http.MethodGet},
		{"删除映射 GET", handler.handleRemoveMapping, http.MethodGet},
		{"列出映射 POST", handler.handleListMappings, http.MethodPost},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/test", nil)
			addAuthHeader(req)
			w := httptest.NewRecorder()

			tt.handler(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("状态码不正确，期望 405，得到 %d", w.Code)
			}
		})
	}
}

// TestRegisterRoutes 测试路由注册
func TestRegisterRoutes(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t, false)
	defer cleanup()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 测试路由是否注册
	routes := []string{
		"/api/mapping/create",
		"/api/mapping/remove",
		"/api/mapping/list",
		"/health",
	}

	for _, route := range routes {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		// 如果路由不存在，应该返回 404
		if w.Code == http.StatusNotFound {
			t.Errorf("路由 %s 未注册", route)
		}
	}
}

// TestWriteSuccess 测试成功响应
func TestWriteSuccess(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t, false)
	defer cleanup()

	w := httptest.NewRecorder()
	handler.writeSuccess(w, "测试成功", map[string]string{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("状态码不正确，期望 200，得到 %d", w.Code)
	}

	var result Response
	json.NewDecoder(w.Body).Decode(&result)

	if !result.Success {
		t.Error("Success 应该为 true")
	}

	if result.Message != "测试成功" {
		t.Errorf("消息不正确，期望 '测试成功'，得到 '%s'", result.Message)
	}
}

// TestWriteError 测试错误响应
func TestWriteError(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t, false)
	defer cleanup()

	w := httptest.NewRecorder()
	handler.writeError(w, http.StatusBadRequest, "测试错误")

	if w.Code != http.StatusBadRequest {
		t.Errorf("状态码不正确，期望 400，得到 %d", w.Code)
	}

	var result Response
	json.NewDecoder(w.Body).Decode(&result)

	if result.Success {
		t.Error("Success 应该为 false")
	}

	if result.Message != "测试错误" {
		t.Errorf("消息不正确，期望 '测试错误'，得到 '%s'", result.Message)
	}
}

// BenchmarkHandleHealth 基准测试健康检查
func BenchmarkHandleHealth(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")
	database, _ := db.New(dbPath)
	defer database.Close()

	fwdMgr := forwarder.NewManager()
	handler := NewHandler(database, fwdMgr, nil, "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.handleHealth(w, req)
	}
}

// BenchmarkHandleListMappings 基准测试列出映射
func BenchmarkHandleListMappings(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")
	database, _ := db.New(dbPath)
	defer database.Close()

	// 添加一些映射
	for i := 0; i < 100; i++ {
		useTunnel := i%2 == 0 // 偶数使用隧道模式
		database.AddMapping(10000+i, "192.168.1.1", 10000+i, useTunnel, nil)
	}

	fwdMgr := forwarder.NewManager()
	handler := NewHandler(database, fwdMgr, nil, "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/api/mapping/list", nil)
	req.Header.Set("X-API-Key", "test-api-key")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.handleListMappings(w, req)
	}
}

// TestAuthMiddleware 测试认证中间件
func TestAuthMiddleware(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t, false)
	defer cleanup()

	tests := []struct {
		name           string
		apiKey         string
		useHeader      bool
		useQueryParam  bool
		expectedStatus int
		expectedMsg    string
	}{
		{
			name:           "有效的API密钥(请求头)",
			apiKey:         testAPIKey,
			useHeader:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "有效的API密钥(查询参数)",
			apiKey:         testAPIKey,
			useQueryParam:  true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "无效的API密钥",
			apiKey:         "invalid-key",
			useHeader:      true,
			expectedStatus: http.StatusUnauthorized,
			expectedMsg:    "无效的 API 密钥",
		},
		{
			name:           "缺少API密钥",
			apiKey:         "",
			expectedStatus: http.StatusUnauthorized,
			expectedMsg:    "无效的 API 密钥",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/mapping/list"
			if tt.useQueryParam {
				url += "?api_key=" + tt.apiKey
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			if tt.useHeader && tt.apiKey != "" {
				req.Header.Set("X-API-Key", tt.apiKey)
			}

			w := httptest.NewRecorder()
			handler.handleListMappings(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("状态码不正确，期望 %d，得到 %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedMsg != "" {
				var result Response
				json.NewDecoder(w.Body).Decode(&result)
				if result.Message != tt.expectedMsg {
					t.Errorf("错误消息不正确，期望 '%s'，得到 '%s'", tt.expectedMsg, result.Message)
				}
			}
		})
	}
}
