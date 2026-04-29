package client

import "time"

// ProxyConfig 代理配置。
type ProxyConfig struct {
	Type     string
	Address  string
	Username string
	Password string
}

// TransportConfig HTTP 传输层配置。
type TransportConfig struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration
	MaxConcurrency      int // 最大并发请求数，0 表示不限制
}

