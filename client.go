package client

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// HttpClient 封装了 http.Client，提供连接池、代理、JA3 指纹、并发限制等功能。
type HttpClient struct {
	client    *http.Client
	transport *http.Transport
	jar       http.CookieJar
	logger    *zap.SugaredLogger
	domain    string
	headers   map[string]string
	mu        sync.RWMutex // 保护 headers 和 domain
	semaphore chan struct{} // 并发限速，nil 表示不限
}

// NewHttpClient 使用默认传输配置创建 HttpClient。
func NewHttpClient(domain string, timeout ...time.Duration) *HttpClient {
	return NewHttpClientWithTransport(domain, nil, timeout...)
}

// NewHttpClientWithTransport 使用自定义传输配置创建 HttpClient。
func NewHttpClientWithTransport(domain string, tc *TransportConfig, timeout ...time.Duration) *HttpClient {
	defaultTimeout := 30 * time.Second
	if len(timeout) > 0 && timeout[0] > 0 {
		defaultTimeout = timeout[0]
	}

	maxIdleConns := 10000
	maxIdleConnsPerHost := 10000
	maxConnsPerHost := 10000
	idleConnTimeout := defaultTimeout

	if tc != nil {
		if tc.MaxIdleConns > 0 {
			maxIdleConns = tc.MaxIdleConns
		}
		if tc.MaxIdleConnsPerHost > 0 {
			maxIdleConnsPerHost = tc.MaxIdleConnsPerHost
		}
		if tc.MaxConnsPerHost > 0 {
			maxConnsPerHost = tc.MaxConnsPerHost
		}
		if tc.IdleConnTimeout > 0 {
			idleConnTimeout = tc.IdleConnTimeout
		}
	}

	transport := &http.Transport{
		MaxIdleConns:        maxIdleConns,
		MaxIdleConnsPerHost: maxIdleConnsPerHost,
		MaxConnsPerHost:     maxConnsPerHost,
		DisableKeepAlives:   false,
		IdleConnTimeout:     idleConnTimeout,
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(fmt.Errorf("failed to create cookie jar: %v", err))
	}

	var semaphore chan struct{}
	if tc != nil && tc.MaxConcurrency > 0 {
		semaphore = make(chan struct{}, tc.MaxConcurrency)
	}

	return &HttpClient{
		client: &http.Client{
			Transport: transport,
			Timeout:   defaultTimeout,
			Jar:       jar,
		},
		transport: transport,
		domain:    domain,
		headers: map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
			"User-Agent":   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3",
		},
		jar:       jar,
		semaphore: semaphore,
	}
}

// SetDomain 设置默认域名。
func (h *HttpClient) SetDomain(domain string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.domain = domain
}

// GetDomain 返回当前默认域名。
func (h *HttpClient) GetDomain() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.domain
}

// SetLogger 设置日志记录器。
func (h *HttpClient) SetLogger(logger *zap.SugaredLogger) {
	h.logger = logger
}

// LogInfo 输出 Info 级别日志。
func (h *HttpClient) LogInfo(msg string, fields ...interface{}) {
	if h.logger != nil {
		h.logger.Infow(msg, fields...)
	}
}

// LogError 输出 Error 级别日志。
func (h *HttpClient) LogError(msg string, err error) {
	if h.logger != nil {
		h.logger.Errorw(msg, "error", err)
	}
}

// Close 关闭所有空闲连接。
func (h *HttpClient) Close() {
	if h.transport != nil {
		h.transport.CloseIdleConnections()
	}
}

// buildFullURL 将相对路径拼接为完整 URL；若已是绝对 URL 则直接返回。
func (h *HttpClient) buildFullURL(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	h.mu.RLock()
	domain := strings.TrimRight(h.domain, "/")
	h.mu.RUnlock()
	path = strings.TrimLeft(path, "/")
	return fmt.Sprintf("%s/%s", domain, path)
}
