package api

import "port-forward/server/html"

func GetTraffticMonitorHTML() string {
	file, err := html.AssetsFS.Open("traffic_monitor.html")
	if err != nil {
		return "<h1>Failed to load HTML</h1>"
	}
	defer file.Close()
	buf := make([]byte, 40960) // 40KB buffer
	n, err := file.Read(buf)
	if err != nil {
		return "<h1>Failed to read HTML</h1>"
	}
	return string(buf[:n])
}

func GetManagementHTML() string {
	file, err := html.AssetsFS.Open("management.html")
	if err != nil {
		return "<h1>Failed to load HTML</h1>"
	}
	defer file.Close()
	buf := make([]byte, 40960) // 40KB buffer
	n, err := file.Read(buf)
	if err != nil {
		return "<h1>Failed to read HTML</h1>"
	}
	return string(buf[:n])
}

func GetConnectionsHTML() string {
	file, err := html.AssetsFS.Open("connections.html")
	if err != nil {
		return "<h1>Failed to load HTML</h1>"
	}
	defer file.Close()
	buf := make([]byte, 40960) // 40KB buffer
	n, err := file.Read(buf)
	if err != nil {
		return "<h1>Failed to read HTML</h1>"
	}
	return string(buf[:n])
}

func GetLoginHTML() string {
	file, err := html.AssetsFS.Open("login.html")
	if err != nil {
		return "<h1>Failed to load HTML</h1>"
	}
	defer file.Close()
	buf := make([]byte, 40960) // 40KB buffer
	n, err := file.Read(buf)
	if err != nil {
		return "<h1>Failed to read HTML</h1>"
	}
	return string(buf[:n])
}

func GetDashboardHTML() string {
	file, err := html.AssetsFS.Open("dashboard.html")
	if err != nil {
		return "<h1>Failed to load HTML</h1>"
	}
	defer file.Close()
	buf := make([]byte, 81920) // 80KB buffer for dashboard
	n, err := file.Read(buf)
	if err != nil {
		return "<h1>Failed to read HTML</h1>"
	}
	return string(buf[:n])
}
