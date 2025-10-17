package api

const managementHTML = `
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>端口映射管理 - Go Tunnel</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
        }
        
        .container {
            max-width: 1200px;
            margin: 0 auto;
        }
        
        h1 {
            color: white;
            text-align: center;
            margin-bottom: 30px;
            font-size: 2.5em;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.3);
        }
        
        .nav-tabs {
            display: flex;
            background: rgba(255, 255, 255, 0.1);
            border-radius: 15px 15px 0 0;
            overflow: hidden;
            margin-bottom: 0;
        }
        
        .nav-tab {
            flex: 1;
            padding: 15px 20px;
            background: transparent;
            border: none;
            color: rgba(255, 255, 255, 0.7);
            cursor: pointer;
            transition: all 0.3s;
            font-size: 1.1em;
        }
        
        .nav-tab.active {
            background: rgba(255, 255, 255, 0.95);
            color: #667eea;
            font-weight: bold;
        }
        
        .nav-tab:hover:not(.active) {
            background: rgba(255, 255, 255, 0.2);
            color: white;
        }
        
        .tab-content {
            background: rgba(255, 255, 255, 0.95);
            border-radius: 0 0 15px 15px;
            padding: 30px;
            box-shadow: 0 8px 32px rgba(0,0,0,0.1);
            min-height: 600px;
        }
        
        .tab-pane {
            display: none;
        }
        
        .tab-pane.active {
            display: block;
        }
        
        .form-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }
        
        .form-group {
            margin-bottom: 20px;
        }
        
        .form-group label {
            display: block;
            margin-bottom: 8px;
            color: #333;
            font-weight: bold;
            font-size: 1.1em;
        }
        
        .form-group input, .form-group select {
            width: 100%;
            padding: 12px 15px;
            border: 2px solid #ddd;
            border-radius: 8px;
            font-size: 1em;
            transition: border-color 0.3s;
        }
        
        .form-group input:focus, .form-group select:focus {
            outline: none;
            border-color: #667eea;
            box-shadow: 0 0 10px rgba(102, 126, 234, 0.2);
        }
        
        .btn {
            padding: 12px 25px;
            border: none;
            border-radius: 8px;
            font-size: 1em;
            cursor: pointer;
            transition: all 0.3s;
            font-weight: bold;
            text-transform: uppercase;
            letter-spacing: 1px;
        }
        
        .btn-primary {
            background: linear-gradient(45deg, #667eea, #764ba2);
            color: white;
        }
        
        .btn-primary:hover {
            transform: translateY(-2px);
            box-shadow: 0 5px 15px rgba(102, 126, 234, 0.4);
        }
        
        .btn-danger {
            background: linear-gradient(45deg, #ff6b6b, #ee5a52);
            color: white;
        }
        
        .btn-danger:hover {
            transform: translateY(-2px);
            box-shadow: 0 5px 15px rgba(255, 107, 107, 0.4);
        }
        
        .btn-secondary {
            background: #6c757d;
            color: white;
        }
        
        .btn-secondary:hover {
            background: #5a6268;
            transform: translateY(-2px);
        }
        
        .mapping-list {
            max-height: 500px;
            overflow-y: auto;
        }
        
        .mapping-item {
            background: #f8f9fa;
            border-radius: 10px;
            padding: 20px;
            margin-bottom: 15px;
            border-left: 4px solid #667eea;
            display: flex;
            justify-content: space-between;
            align-items: center;
            transition: all 0.3s;
        }
        
        .mapping-item:hover {
            transform: translateX(5px);
            box-shadow: 0 5px 15px rgba(0,0,0,0.1);
        }
        
        .mapping-info {
            flex: 1;
        }
        
        .mapping-info h4 {
            color: #667eea;
            margin-bottom: 8px;
            font-size: 1.2em;
        }
        
        .mapping-details {
            color: #666;
            font-size: 0.95em;
            line-height: 1.4;
        }
        
        .mapping-actions {
            display: flex;
            gap: 10px;
        }
        
        .status-badge {
            display: inline-block;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 0.8em;
            font-weight: bold;
            text-transform: uppercase;
        }
        
        .status-tunnel {
            background: #e3f2fd;
            color: #1976d2;
        }
        
        .status-direct {
            background: #f3e5f5;
            color: #7b1fa2;
        }
        
        .message {
            padding: 15px;
            border-radius: 8px;
            margin-bottom: 20px;
            font-weight: bold;
        }
        
        .message.success {
            background: #d4edda;
            color: #155724;
            border: 1px solid #c3e6cb;
        }
        
        .message.error {
            background: #f8d7da;
            color: #721c24;
            border: 1px solid #f5c6cb;
        }
        
        .message.info {
            background: #cce7ff;
            color: #004085;
            border: 1px solid #b3d7ff;
        }
        
        .loading {
            text-align: center;
            padding: 40px;
            color: #666;
        }
        
        .spinner {
            display: inline-block;
            width: 40px;
            height: 40px;
            border: 4px solid #f3f3f3;
            border-top: 4px solid #667eea;
            border-radius: 50%;
            animation: spin 1s linear infinite;
        }
        
        @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
        }
        
        .checkbox-group {
            display: flex;
            align-items: center;
            gap: 10px;
            margin-top: 10px;
        }
        
        .checkbox-group input[type="checkbox"] {
            width: auto;
            transform: scale(1.2);
        }
        
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 15px;
            margin-bottom: 20px;
        }
        
        .stat-card {
            background: linear-gradient(45deg, #667eea, #764ba2);
            color: white;
            padding: 20px;
            border-radius: 10px;
            text-align: center;
        }
        
        .stat-value {
            font-size: 2em;
            font-weight: bold;
            margin-bottom: 5px;
        }
        
        .stat-label {
            font-size: 0.9em;
            opacity: 0.9;
        }
        
        .quick-actions {
            display: flex;
            gap: 15px;
            margin-bottom: 20px;
            flex-wrap: wrap;
        }
        
        @media (max-width: 768px) {
            .container {
                padding: 10px;
            }
            
            .form-grid {
                grid-template-columns: 1fr;
            }
            
            .mapping-item {
                flex-direction: column;
                align-items: flex-start;
                gap: 15px;
            }
            
            .mapping-actions {
                width: 100%;
                justify-content: flex-end;
            }
            
            .quick-actions {
                flex-direction: column;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🚀 端口映射管理中心</h1>
        
        <div class="nav-tabs">
            <button class="nav-tab active" onclick="switchTab('overview')">概览</button>
            <button class="nav-tab" onclick="switchTab('create')">创建映射</button>
            <button class="nav-tab" onclick="switchTab('manage')">管理映射</button>
            <button class="nav-tab" onclick="switchTab('monitor')">流量监控</button>
        </div>
        
        <div class="tab-content">
            <!-- 概览页面 -->
            <div id="overview" class="tab-pane active">
                <h2 style="color: #667eea; margin-bottom: 20px;">系统概览</h2>
                
                <div class="stats-grid">
                    <div class="stat-card">
                        <div class="stat-value" id="total-mappings">-</div>
                        <div class="stat-label">总映射数</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-value" id="tunnel-mappings">-</div>
                        <div class="stat-label">隧道映射</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-value" id="direct-mappings">-</div>
                        <div class="stat-label">直连映射</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-value" id="tunnel-status">-</div>
                        <div class="stat-label">隧道状态</div>
                    </div>
                </div>
                
                <div class="quick-actions">
                    <button class="btn btn-primary" onclick="switchTab('create')">
                        ➕ 快速创建映射
                    </button>
                    <button class="btn btn-secondary" onclick="refreshOverview()">
                        🔄 刷新状态
                    </button>
                    <button class="btn btn-secondary" onclick="switchTab('monitor')">
                        📊 查看监控
                    </button>
                </div>
                
                <div id="recent-mappings">
                    <h3 style="color: #667eea; margin-bottom: 15px;">最近的映射</h3>
                    <div id="recent-mappings-list" class="loading">
                        <div class="spinner"></div>
                        <p>加载中...</p>
                    </div>
                </div>
            </div>
            
            <!-- 创建映射页面 -->
            <div id="create" class="tab-pane">
                <h2 style="color: #667eea; margin-bottom: 20px;">创建新的端口映射</h2>
                
                <div id="create-message"></div>
                
                <form id="create-form">
                    <div class="form-grid">
                        <div class="form-group">
                            <label for="source-port">源端口 (本地监听端口) *</label>
                            <input type="number" id="source-port" min="1" max="65535" required 
                                   placeholder="例如: 8080">
                        </div>
                        
                        <div class="form-group">
                            <label for="target-host">目标主机 *</label>
                            <input type="text" id="target-host" required 
                                   placeholder="例如: localhost 或 192.168.1.100">
                        </div>
                        
                        <div class="form-group">
                            <label for="target-port">目标端口 *</label>
                            <input type="number" id="target-port" min="1" max="65535" required 
                                   placeholder="例如: 3000">
                        </div>
                        
                        <div class="form-group">
                            <label>连接模式</label>
                            <div class="checkbox-group">
                                <input type="checkbox" id="use-tunnel">
                                <label for="use-tunnel">使用隧道模式</label>
                            </div>
                            <small style="color: #666; margin-top: 5px; display: block;">
                                隧道模式：通过加密隧道转发流量，适合跨网络访问<br>
                                直连模式：直接TCP转发，适合本地网络访问
                            </small>
                        </div>
                    </div>
                    
                    <div style="text-align: center; margin-top: 30px;">
                        <button type="submit" class="btn btn-primary" style="margin-right: 15px;">
                            🚀 创建映射
                        </button>
                        <button type="button" class="btn btn-secondary" onclick="resetCreateForm()">
                            🔄 重置表单
                        </button>
                    </div>
                </form>
            </div>
            
            <!-- 管理映射页面 -->
            <div id="manage" class="tab-pane">
                <h2 style="color: #667eea; margin-bottom: 20px;">管理端口映射</h2>
                
                <div id="manage-message"></div>
                
                <div style="text-align: right; margin-bottom: 20px;">
                    <button class="btn btn-secondary" onclick="loadMappings()">
                        🔄 刷新列表
                    </button>
                </div>
                
                <div id="mappings-list" class="loading">
                    <div class="spinner"></div>
                    <p>加载映射列表中...</p>
                </div>
            </div>
            
            <!-- 流量监控页面 -->
            <div id="monitor" class="tab-pane">
                <h2 style="color: #667eea; margin-bottom: 20px;">流量监控</h2>
                
                <div style="text-align: center; padding: 40px;">
                    <p style="color: #666; margin-bottom: 20px;">
                        点击下方按钮打开详细的流量监控页面
                    </p>
                    <button class="btn btn-primary" onclick="openTrafficMonitor()">
                        📊 打开流量监控
                    </button>
                </div>
            </div>
        </div>
    </div>

    <script>
        // 全局变量
        let currentMappings = [];
        
        // 切换标签页
        function switchTab(tabName) {
            // 隐藏所有标签页内容
            document.querySelectorAll('.tab-pane').forEach(pane => {
                pane.classList.remove('active');
            });
            
            // 移除所有标签页按钮的激活状态
            document.querySelectorAll('.nav-tab').forEach(tab => {
                tab.classList.remove('active');
            });
            
            // 显示选中的标签页
            document.getElementById(tabName).classList.add('active');
            event.target.classList.add('active');
            
            // 根据标签页执行相应的初始化
            switch(tabName) {
                case 'overview':
                    refreshOverview();
                    break;
                case 'manage':
                    loadMappings();
                    break;
            }
        }
        
        // 显示消息
        function showMessage(containerId, type, message) {
            const container = document.getElementById(containerId);
            container.innerHTML = '<div class="message ' + type + '">' + message + '</div>';
            
            // 3秒后自动隐藏成功消息
            if (type === 'success') {
                setTimeout(() => {
                    container.innerHTML = '';
                }, 3000);
            }
        }
        
        // 创建映射表单提交
        document.getElementById('create-form').addEventListener('submit', async function(e) {
            e.preventDefault();
            
            const formData = {
                source_port: parseInt(document.getElementById('source-port').value),
                target_host: document.getElementById('target-host').value,
                target_port: parseInt(document.getElementById('target-port').value),
                use_tunnel: document.getElementById('use-tunnel').checked
            };
            
            try {
                const response = await fetch('/api/mapping/create', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify(formData)
                });
                
                const result = await response.json();
                
                if (result.success) {
                    showMessage('create-message', 'success', '✅ ' + result.message);
                    resetCreateForm();
                } else {
                    showMessage('create-message', 'error', '❌ ' + result.message);
                }
            } catch (error) {
                showMessage('create-message', 'error', '❌ 网络错误: ' + error.message);
            }
        });
        
        // 重置创建表单
        function resetCreateForm() {
            document.getElementById('create-form').reset();
        }
        
        // 删除映射
        async function deleteMapping(port) {
            if (!confirm('确定要删除端口 ' + port + ' 的映射吗？')) {
                return;
            }
            
            try {
                const response = await fetch('/api/mapping/remove', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ port: port })
                });
                
                const result = await response.json();
                
                if (result.success) {
                    showMessage('manage-message', 'success', '✅ ' + result.message);
                    loadMappings(); // 重新加载列表
                } else {
                    showMessage('manage-message', 'error', '❌ ' + result.message);
                }
            } catch (error) {
                showMessage('manage-message', 'error', '❌ 网络错误: ' + error.message);
            }
        }
        
        // 加载映射列表
        async function loadMappings() {
            const container = document.getElementById('mappings-list');
            container.innerHTML = '<div class="loading"><div class="spinner"></div><p>加载中...</p></div>';
            
            try {
                const response = await fetch('/api/mapping/list');
                const result = await response.json();
                
                if (result.success) {
                    currentMappings = result.data.mappings || [];
                    renderMappings(currentMappings);
                } else {
                    container.innerHTML = '<div class="message error">❌ ' + result.message + '</div>';
                }
            } catch (error) {
                container.innerHTML = '<div class="message error">❌ 网络错误: ' + error.message + '</div>';
            }
        }
        
        // 渲染映射列表
        function renderMappings(mappings) {
            const container = document.getElementById('mappings-list');
            
            if (mappings.length === 0) {
                container.innerHTML = '<div class="message info">📝 暂无端口映射，点击"创建映射"标签页开始创建。</div>';
                return;
            }
            
            const html = mappings.map(mapping => 
                '<div class="mapping-item">' +
                    '<div class="mapping-info">' +
                        '<h4>端口 ' + mapping.source_port + '</h4>' +
                        '<div class="mapping-details">' +
                            '<strong>目标:</strong> ' + mapping.target_host + ':' + mapping.target_port + '<br>' +
                            '<strong>模式:</strong> ' +
                            '<span class="status-badge ' + (mapping.use_tunnel ? 'status-tunnel' : 'status-direct') + '">' +
                                (mapping.use_tunnel ? '隧道模式' : '直连模式') +
                            '</span><br>' +
                            '<strong>创建时间:</strong> ' + new Date(mapping.created_at).toLocaleString('zh-CN') +
                        '</div>' +
                    '</div>' +
                    '<div class="mapping-actions">' +
                        '<button class="btn btn-danger" onclick="deleteMapping(' + mapping.source_port + ')">' +
                            '🗑️ 删除' +
                        '</button>' +
                    '</div>' +
                '</div>'
            ).join('');
            
            container.innerHTML = html;
        }
        
        // 刷新概览页面
        async function refreshOverview() {
            try {
                // 加载映射列表
                const mappingsResponse = await fetch('/api/mapping/list');
                const mappingsResult = await mappingsResponse.json();
                
                // 加载健康状态
                const healthResponse = await fetch('/health');
                const healthResult = await healthResponse.json();
                
                if (mappingsResult.success) {
                    const mappings = mappingsResult.data.mappings || [];
                    const tunnelMappings = mappings.filter(m => m.use_tunnel).length;
                    const directMappings = mappings.filter(m => !m.use_tunnel).length;
                    
                    document.getElementById('total-mappings').textContent = mappings.length;
                    document.getElementById('tunnel-mappings').textContent = tunnelMappings;
                    document.getElementById('direct-mappings').textContent = directMappings;
                    
                    // 渲染最近的映射（最多5个）
                    const recentMappings = mappings.slice(-5).reverse();
                    renderRecentMappings(recentMappings);
                }
                
                if (healthResult) {
                    const tunnelStatus = healthResult.tunnel_enabled ? 
                        (healthResult.tunnel_connected ? '🟢 已连接' : '🟡 未连接') : 
                        '🔴 未启用';
                    document.getElementById('tunnel-status').textContent = tunnelStatus;
                }
            } catch (error) {
                console.error('刷新概览失败:', error);
            }
        }
        
        // 渲染最近的映射
        function renderRecentMappings(mappings) {
            const container = document.getElementById('recent-mappings-list');
            
            if (mappings.length === 0) {
                container.innerHTML = '<div class="message info">暂无映射记录</div>';
                return;
            }
            
            const html = mappings.map(mapping => 
                '<div class="mapping-item">' +
                    '<div class="mapping-info">' +
                        '<h4>端口 ' + mapping.source_port + '</h4>' +
                        '<div class="mapping-details">' +
                            '<strong>目标:</strong> ' + mapping.target_host + ':' + mapping.target_port + ' | ' +
                            '<span class="status-badge ' + (mapping.use_tunnel ? 'status-tunnel' : 'status-direct') + '">' +
                                (mapping.use_tunnel ? '隧道' : '直连') +
                            '</span>' +
                        '</div>' +
                    '</div>' +
                '</div>'
            ).join('');
            
            container.innerHTML = html;
        }
        
        // 打开流量监控页面
        function openTrafficMonitor() {
            window.open('/api/stats/monitor', '_blank');
        }
        
        // 页面加载完成后初始化
        document.addEventListener('DOMContentLoaded', function() {
            refreshOverview();
        });
    </script>
</body>
</html>
`