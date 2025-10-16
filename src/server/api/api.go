package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"port-forward/server/db"
	"port-forward/server/forwarder"
	"port-forward/server/stats"
	"port-forward/server/tunnel"
	"port-forward/server/utils"
	"strconv"
	"time"
)

// Handler HTTP API 处理器
type Handler struct {
	db             *db.Database
	forwarderMgr   *forwarder.Manager
	tunnelServer   *tunnel.Server
	// portRangeFrom  int
	// portRangeEnd   int
}

// NewHandler 创建新的 API 处理器
func NewHandler(database *db.Database, fwdMgr *forwarder.Manager, ts *tunnel.Server) *Handler {
	return &Handler{
		db:            database,
		forwarderMgr:  fwdMgr,
		tunnelServer:  ts,
		// portRangeFrom: portFrom,
		// portRangeEnd:  portEnd,
	}
}

// CreateMappingRequest 创建映射请求
type CreateMappingRequest struct {
	SourcePort int    `json:"source_port"` // 源端口（本地监听端口）
	TargetPort int    `json:"target_port"` // 目标端口（远程服务端口）
	TargetHost string `json:"target_host"` // 目标主机（支持IP或域名）
	UseTunnel  bool   `json:"use_tunnel"`  // 是否使用隧道模式
}

// RemoveMappingRequest 删除映射请求
type RemoveMappingRequest struct {
	Port int `json:"port"`
}

// Response 统一响应格式
type Response struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/mapping/create", h.handleCreateMapping)
	mux.HandleFunc("/api/mapping/remove", h.handleRemoveMapping)
	mux.HandleFunc("/api/mapping/list", h.handleListMappings)
	mux.HandleFunc("/api/stats/traffic", h.handleGetTrafficStats)
	mux.HandleFunc("/api/stats/monitor", h.handleTrafficMonitor)
	mux.HandleFunc("/health", h.handleHealth)
}

// validateHostOrIP 验证主机名或IP地址
func (h *Handler) validateHostOrIP(hostOrIP string) error {
	if hostOrIP == "" {
		return fmt.Errorf("主机名或IP地址不能为空")
	}

	// 检查是否为有效的IP地址
	if net.ParseIP(hostOrIP) != nil {
		return nil // 是有效的IP地址
	}

	// 尝试解析域名以验证其有效性
	_, err := net.LookupHost(hostOrIP)
	if err != nil {
		return fmt.Errorf("域名解析失败: %w", err)
	}

	return nil
}

// handleCreateMapping 处理创建映射请求
func (h *Handler) handleCreateMapping(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "只支持 POST 方法")
		return
	}

	var req CreateMappingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "请求格式错误: "+err.Error())
		return
	}

	// 验证端口范围
	// if req.SourcePort < h.portRangeFrom || req.SourcePort > h.portRangeEnd {
	// 	h.writeError(w, http.StatusBadRequest, fmt.Sprintf("端口必须在 %d-%d 范围内", h.portRangeFrom, h.portRangeEnd))
	// 	return
	// }

	// 检查端口是否已被使用
	if h.forwarderMgr.Exists(req.SourcePort) {
		h.writeError(w, http.StatusConflict, "端口已被占用, 已经存在该映射")
		return
	}

	used := utils.PortCheck(req.SourcePort)
	if used {
		h.writeError(w, http.StatusConflict, "系统内端口已被占用")
		return
	}

	// 根据请求决定使用哪种模式
	if req.UseTunnel {
		// 隧道模式，检查隧道服务器是否可用
		if h.tunnelServer == nil {
			h.writeError(w, http.StatusServiceUnavailable, "隧道服务未启用")
			return
		}
		if !h.tunnelServer.IsConnected() {
			h.writeError(w, http.StatusServiceUnavailable, "隧道未连接")
			return
		}
		// 隧道模式也需要目标主机（客户端会连接到该主机）
		if err := h.validateHostOrIP(req.TargetHost); err != nil {
			h.writeError(w, http.StatusBadRequest, "目标主机格式无效: "+err.Error())
			return
		}
	} else {
		// 直接模式需要验证主机名或IP
		if err := h.validateHostOrIP(req.TargetHost); err != nil {
			h.writeError(w, http.StatusBadRequest, "目标主机格式无效: "+err.Error())
			return
		}
	}

	// 添加到数据库
	if err := h.db.AddMapping(req.SourcePort, req.TargetHost, req.TargetPort, req.UseTunnel); err != nil {
		h.writeError(w, http.StatusInternalServerError, "保存映射失败: "+err.Error())
		return
	}

	// 启动转发器
	var err error
	if req.UseTunnel {
		// 隧道模式：使用隧道转发
		err = h.forwarderMgr.AddTunnel(req.SourcePort, req.TargetHost, req.TargetPort, h.tunnelServer)
	} else {
		// 直接模式：直接TCP转发
		err = h.forwarderMgr.Add(req.SourcePort, req.TargetHost, req.TargetPort)
	}
	
	if err != nil {
		// 回滚数据库操作
		h.db.RemoveMapping(req.SourcePort)
		h.writeError(w, http.StatusInternalServerError, "启动转发失败: "+err.Error())
		return
	}

	log.Printf("创建端口映射: %d -> %s:%d (tunnel: %v)", req.SourcePort, req.TargetHost, req.TargetPort, req.UseTunnel)

	h.writeSuccess(w, "端口映射创建成功", map[string]interface{}{
		"source_port": req.SourcePort,
		"target_host": req.TargetHost,
		"target_port": req.TargetPort,
		"use_tunnel":  req.UseTunnel,
	})
}

// handleRemoveMapping 处理删除映射请求
func (h *Handler) handleRemoveMapping(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "只支持 POST 方法")
		return
	}

	var req RemoveMappingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "请求格式错误: "+err.Error())
		return
	}

	// 验证端口范围
	// if req.Port < h.portRangeFrom || req.Port > h.portRangeEnd {
	// 	h.writeError(w, http.StatusBadRequest, fmt.Sprintf("端口必须在 %d-%d 范围内", h.portRangeFrom, h.portRangeEnd))
	// 	return
	// }

	// 检查映射是否存在
	if !h.forwarderMgr.Exists(req.Port) {
		h.writeError(w, http.StatusNotFound, "端口映射不存在")
		return
	}

	// 停止转发器
	if err := h.forwarderMgr.Remove(req.Port); err != nil {
		h.writeError(w, http.StatusInternalServerError, "停止转发失败: "+err.Error())
		return
	}

	// 从数据库删除
	if err := h.db.RemoveMapping(req.Port); err != nil {
		log.Printf("从数据库删除映射失败 (端口 %d): %v", req.Port, err)
		// 即使数据库删除失败，转发器已经停止，仍然返回成功
	}

	log.Printf("删除端口映射: %d", req.Port)

	h.writeSuccess(w, "端口映射删除成功", map[string]interface{}{
		"port": req.Port,
	})
}

// handleListMappings 处理列出所有映射请求
func (h *Handler) handleListMappings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "只支持 GET 方法")
		return
	}

	mappings, err := h.db.GetAllMappings()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "获取映射列表失败: "+err.Error())
		return
	}

	h.writeSuccess(w, "获取映射列表成功", map[string]interface{}{
		"mappings": mappings,
		"count":    len(mappings),
	})
}

// handleHealth 健康检查
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":           "ok",
		"tunnel_enabled":   h.tunnelServer != nil,
		"tunnel_connected": false,
	}

	if h.tunnelServer != nil {
		status["tunnel_connected"] = h.tunnelServer.IsConnected()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// writeSuccess 写入成功响应
func (h *Handler) writeSuccess(w http.ResponseWriter, message string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(Response{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// writeError 写入错误响应
func (h *Handler) writeError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(Response{
		Success: false,
		Message: message,
	})
}

// Start 启动 HTTP 服务器
func Start(handler *Handler, port int) error {
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	server := &http.Server{
		Addr:         ":" + strconv.Itoa(port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("HTTP API 服务启动: 端口 %d", port)
	return server.ListenAndServe()
}

// handleGetTrafficStats 获取流量统计
func (h *Handler) handleGetTrafficStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "只支持 GET 方法")
		return
	}

	// 获取隧道流量统计
	var tunnelStats stats.TrafficStats
	if h.tunnelServer != nil {
		tunnelStats = h.tunnelServer.GetTrafficStats()
	}

	// 获取所有端口映射的流量统计
	forwarderStats := h.forwarderMgr.GetAllTrafficStats()
	
	// 构建响应
	mappings := make([]stats.PortTrafficStats, 0, len(forwarderStats))
	var totalSent, totalReceived uint64
	
	for port, stat := range forwarderStats {
		mappings = append(mappings, stats.PortTrafficStats{
			Port:          port,
			BytesSent:     stat.BytesSent,
			BytesReceived: stat.BytesReceived,
			LastUpdate:    stat.LastUpdate,
		})
		totalSent += stat.BytesSent
		totalReceived += stat.BytesReceived
	}
	
	// 加上隧道的流量
	totalSent += tunnelStats.BytesSent
	totalReceived += tunnelStats.BytesReceived
	
	response := stats.AllTrafficStats{
		Tunnel:        tunnelStats,
		Mappings:      mappings,
		TotalSent:     totalSent,
		TotalReceived: totalReceived,
		Timestamp:     time.Now().Unix(),
	}

	h.writeSuccess(w, "获取流量统计成功", response)
}

// handleTrafficMonitor 流量监控页面
func (h *Handler) handleTrafficMonitor(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, html)
}