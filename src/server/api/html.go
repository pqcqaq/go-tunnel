package api

const html = `
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>流量监控 - Go Tunnel</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"></script>
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
            max-width: 1400px;
            margin: 0 auto;
        }
        
        h1 {
            color: white;
            text-align: center;
            margin-bottom: 30px;
            font-size: 2.5em;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.3);
        }
        
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }
        
        .stat-card {
            background: rgba(255, 255, 255, 0.95);
            border-radius: 15px;
            padding: 25px;
            box-shadow: 0 8px 32px rgba(0,0,0,0.1);
            transition: transform 0.3s;
        }
        
        .stat-card:hover {
            transform: translateY(-5px);
        }
        
        .stat-card h3 {
            color: #667eea;
            margin-bottom: 15px;
            font-size: 1.3em;
            border-bottom: 2px solid #667eea;
            padding-bottom: 10px;
        }
        
        .stat-value {
            font-size: 2em;
            font-weight: bold;
            color: #333;
            margin: 10px 0;
        }
        
        .stat-label {
            color: #666;
            font-size: 0.9em;
            margin-top: 5px;
        }
        
        .chart-container {
            background: rgba(255, 255, 255, 0.95);
            border-radius: 15px;
            padding: 25px;
            margin-bottom: 20px;
            box-shadow: 0 8px 32px rgba(0,0,0,0.1);
        }
        
        .chart-container h2 {
            color: #667eea;
            margin-bottom: 20px;
            font-size: 1.5em;
        }
        
        .mapping-list {
            background: rgba(255, 255, 255, 0.95);
            border-radius: 15px;
            padding: 25px;
            box-shadow: 0 8px 32px rgba(0,0,0,0.1);
        }
        
        .mapping-item {
            background: #f8f9fa;
            border-radius: 10px;
            padding: 15px;
            margin-bottom: 15px;
            border-left: 4px solid #667eea;
        }
        
        .mapping-item h4 {
            color: #667eea;
            margin-bottom: 10px;
        }
        
        .mapping-stats {
            display: flex;
            justify-content: space-between;
            color: #666;
            font-size: 0.9em;
        }
        
        .status-indicator {
            display: inline-block;
            width: 12px;
            height: 12px;
            border-radius: 50%;
            background: #4CAF50;
            animation: pulse 2s infinite;
        }
        
        @keyframes pulse {
            0%, 100% {
                opacity: 1;
            }
            50% {
                opacity: 0.5;
            }
        }
        
        .update-time {
            text-align: center;
            color: white;
            margin-top: 20px;
            font-size: 0.9em;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1><span class="status-indicator"></span> 流量监控面板</h1>
        
        <div class="stats-grid">
            <div class="stat-card">
                <h3>总发送流量</h3>
                <div class="stat-value" id="total-sent">0 B</div>
                <div class="stat-label">Total Sent</div>
            </div>
            
            <div class="stat-card">
                <h3>总接收流量</h3>
                <div class="stat-value" id="total-received">0 B</div>
                <div class="stat-label">Total Received</div>
            </div>
            
            <div class="stat-card">
                <h3>隧道发送</h3>
                <div class="stat-value" id="tunnel-sent">0 B</div>
                <div class="stat-label">Tunnel Sent</div>
            </div>
            
            <div class="stat-card">
                <h3>隧道接收</h3>
                <div class="stat-value" id="tunnel-received">0 B</div>
                <div class="stat-label">Tunnel Received</div>
            </div>
        </div>
        
        <div class="chart-container">
            <h2>实时流量趋势</h2>
            <canvas id="trafficChart"></canvas>
        </div>
        
        <div class="mapping-list">
            <h2 style="color: #667eea; margin-bottom: 20px;">端口映射流量</h2>
            <div id="mapping-list"></div>
        </div>
        
        <div class="update-time">
            最后更新: <span id="update-time">-</span> | 每 3 秒自动刷新
        </div>
    </div>

    <script>
        // 初始化图表
        const ctx = document.getElementById('trafficChart');
        const chart = new Chart(ctx, {
            type: 'line',
            data: {
                labels: [],
                datasets: [
                    {
                        label: '发送 (KB/s)',
                        data: [],
                        borderColor: '#667eea',
                        backgroundColor: 'rgba(102, 126, 234, 0.1)',
                        tension: 0.4,
                        fill: true
                    },
                    {
                        label: '接收 (KB/s)',
                        data: [],
                        borderColor: '#764ba2',
                        backgroundColor: 'rgba(118, 75, 162, 0.1)',
                        tension: 0.4,
                        fill: true
                    }
                ]
            },
            options: {
                responsive: true,
                maintainAspectRatio: true,
                aspectRatio: 2.5,
                plugins: {
                    legend: {
                        display: true,
                        position: 'top',
                    }
                },
                scales: {
                    y: {
                        beginAtZero: true,
                        ticks: {
                            callback: function(value) {
                                return value.toFixed(2) + ' KB/s';
                            }
                        }
                    },
                    x: {
                        display: true
                    }
                }
            }
        });

        let lastData = null;
        const maxDataPoints = 20;

        // 格式化字节数
        function formatBytes(bytes) {
            if (bytes === 0) return '0 B';
            const k = 1024;
            const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
            const i = Math.floor(Math.log(bytes) / Math.log(k));
            return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
        }

        // 更新统计数据
        function updateStats(data) {
            document.getElementById('total-sent').textContent = formatBytes(data.total_sent);
            document.getElementById('total-received').textContent = formatBytes(data.total_received);
            document.getElementById('tunnel-sent').textContent = formatBytes(data.tunnel.bytes_sent);
            document.getElementById('tunnel-received').textContent = formatBytes(data.tunnel.bytes_received);
            
            // 更新时间
            const now = new Date();
            document.getElementById('update-time').textContent = now.toLocaleTimeString('zh-CN');
        }

        // 更新图表
        function updateChart(data) {
            const now = new Date().toLocaleTimeString('zh-CN');
            
            // 计算速率 (如果有上次数据)
            let sendRate = 0;
            let recvRate = 0;
            
            if (lastData) {
                const timeDiff = 3; // 3秒间隔
                sendRate = (data.total_sent - lastData.total_sent) / timeDiff / 1024; // KB/s
                recvRate = (data.total_received - lastData.total_received) / timeDiff / 1024; // KB/s
            }
            
            lastData = data;
            
            // 添加新数据点
            chart.data.labels.push(now);
            chart.data.datasets[0].data.push(sendRate);
            chart.data.datasets[1].data.push(recvRate);
            
            // 限制数据点数量
            if (chart.data.labels.length > maxDataPoints) {
                chart.data.labels.shift();
                chart.data.datasets[0].data.shift();
                chart.data.datasets[1].data.shift();
            }
            
            chart.update('none'); // 无动画更新，更流畅
        }

        // 更新端口映射列表
        function updateMappings(mappings) {
            const container = document.getElementById('mapping-list');
            
            if (mappings.length === 0) {
                container.innerHTML = '<p style="color: #999; text-align: center;">暂无端口映射</p>';
                return;
            }
            
            container.innerHTML = mappings.map(m => 
                '<div class="mapping-item">' +
                    '<h4>端口 ' + m.port + '</h4>' +
                    '<div class="mapping-stats">' +
                        '<span>发送: ' + formatBytes(m.bytes_sent) + '</span>' +
                        '<span>接收: ' + formatBytes(m.bytes_received) + '</span>' +
                    '</div>' +
                '</div>'
            ).join('');
        }

        // 获取流量数据
        async function fetchTrafficData() {
            try {
                const response = await fetch('/api/stats/traffic');
                const result = await response.json();
                
                if (result.success) {
                    updateStats(result.data);
                    updateChart(result.data);
                    updateMappings(result.data.mappings || []);
                }
            } catch (error) {
                console.error('获取流量数据失败:', error);
            }
        }

        // 初始加载
        fetchTrafficData();

        // 定时刷新 (每3秒)
        setInterval(fetchTrafficData, 3000);
    </script>
</body>
</html>
	`