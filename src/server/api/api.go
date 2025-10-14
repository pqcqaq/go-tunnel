package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"port-forward/server/db"
	"port-forward/server/forwarder"
	"port-forward/server/tunnel"
	"strconv"
	"time"
)

// Handler HTTP API 处理器
type Handler struct {
	db             *db.Database
	forwarderMgr   *forwarder.Manager
	tunnelServer   *tunnel.Server
	portRangeFrom  int
	portRangeEnd   int
	useTunnel      bool
}

// NewHandler 创建新的 API 处理器
func NewHandler(database *db.Database, fwdMgr *forwarder.Manager, ts *tunnel.Server, portFrom, portEnd int, useTunnel bool) *Handler {
	return &Handler{
		db:            database,
		forwarderMgr:  fwdMgr,
		tunnelServer:  ts,
		portRangeFrom: portFrom,
		portRangeEnd:  portEnd,
		useTunnel:     useTunnel,
	}
}

// CreateMappingRequest 创建映射请求
type CreateMappingRequest struct {
	Port     int    `json:"port"`      // 源端口和目标端口（相同）
	TargetIP string `json:"target_ip"` // 目标 IP（非隧道模式使用）
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
	mux.HandleFunc("/health", h.handleHealth)
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
	if req.Port < h.portRangeFrom || req.Port > h.portRangeEnd {
		h.writeError(w, http.StatusBadRequest, fmt.Sprintf("端口必须在 %d-%d 范围内", h.portRangeFrom, h.portRangeEnd))
		return
	}

	// 检查端口是否已被使用
	if h.forwarderMgr.Exists(req.Port) {
		h.writeError(w, http.StatusConflict, "端口已被占用")
		return
	}

	// 非隧道模式需要验证 IP
	if !h.useTunnel {
		if req.TargetIP == "" {
			h.writeError(w, http.StatusBadRequest, "target_ip 不能为空")
			return
		}
		if net.ParseIP(req.TargetIP) == nil {
			h.writeError(w, http.StatusBadRequest, "target_ip 格式无效")
			return
		}
	} else {
		// 隧道模式，检查隧道是否连接
		if !h.tunnelServer.IsConnected() {
			h.writeError(w, http.StatusServiceUnavailable, "隧道未连接")
			return
		}
		// 隧道模式使用本地地址
		req.TargetIP = "127.0.0.1"
	}

	// 添加到数据库
	if err := h.db.AddMapping(req.Port, req.TargetIP, req.Port); err != nil {
		h.writeError(w, http.StatusInternalServerError, "保存映射失败: "+err.Error())
		return
	}

	// 启动转发器
	if err := h.forwarderMgr.Add(req.Port, req.TargetIP, req.Port); err != nil {
		// 回滚数据库操作
		h.db.RemoveMapping(req.Port)
		h.writeError(w, http.StatusInternalServerError, "启动转发失败: "+err.Error())
		return
	}

	log.Printf("创建端口映射: %d -> %s:%d", req.Port, req.TargetIP, req.Port)

	h.writeSuccess(w, "端口映射创建成功", map[string]interface{}{
		"port":       req.Port,
		"target_ip":  req.TargetIP,
		"use_tunnel": h.useTunnel,
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
	if req.Port < h.portRangeFrom || req.Port > h.portRangeEnd {
		h.writeError(w, http.StatusBadRequest, fmt.Sprintf("端口必须在 %d-%d 范围内", h.portRangeFrom, h.portRangeEnd))
		return
	}

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
		"mappings":   mappings,
		"count":      len(mappings),
		"use_tunnel": h.useTunnel,
	})
}

// handleHealth 健康检查
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":           "ok",
		"tunnel_enabled":   h.useTunnel,
		"tunnel_connected": false,
	}

	if h.useTunnel {
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