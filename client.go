package client

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	utls "github.com/refraction-networking/utls"
	"go.uber.org/zap"
	"golang.org/x/net/proxy"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ProxyConfig struct {
	Type     string
	Address  string
	Username string
	Password string
}

type TransportConfig struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration
	MaxConcurrency      int // 最大并发请求数，0 表示不限制
}

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

// Session 代表一个独立的 HTTP 会话，拥有独立的 CookieJar。
// 适用于多账号/多用户并发场景，各 goroutine 持有各自的 Session。
type Session struct {
	jar     http.CookieJar
	headers map[string]string
	mu      sync.RWMutex
}

// NewSession 创建一个新的独立 Session。
func NewSession() *Session {
	jar, _ := cookiejar.New(nil)
	return &Session{jar: jar, headers: make(map[string]string)}
}

// SetCookies 设置指定 URL 域名下的 Cookie。
func (s *Session) SetCookies(rawURL string, cookies map[string]string, reset ...bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	if len(reset) > 0 && reset[0] {
		s.jar.SetCookies(u, []*http.Cookie{})
	}
	secure := u.Scheme == "https"
	var list []*http.Cookie
	for k, v := range cookies {
		list = append(list, &http.Cookie{
			Name: k, Value: v, Path: "/", Domain: u.Hostname(), Secure: secure,
		})
	}
	s.jar.SetCookies(u, list)
}

// GetCookies 获取指定 URL 域名下的所有 Cookie。
func (s *Session) GetCookies(rawURL string) []*http.Cookie {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	return s.jar.Cookies(u)
}

// GetCookieValue 获取指定 URL 域名下某个 Cookie 的值。
func (s *Session) GetCookieValue(rawURL, name string) string {
	for _, c := range s.GetCookies(rawURL) {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

// SetHeader 为本 Session 设置请求头（会覆盖同名 client 级别的 header）。
func (s *Session) SetHeader(name, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.headers[textproto.CanonicalMIMEHeaderKey(name)] = value
}

// getHeaders 返回 Session headers 的副本（线程安全）。
func (s *Session) getHeaders() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make(map[string]string, len(s.headers))
	for k, v := range s.headers {
		cp[k] = v
	}
	return cp
}

func NewHttpClient(domain string, timeout ...time.Duration) *HttpClient {
	return NewHttpClientWithTransport(domain, nil, timeout...)
}

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

func (h *HttpClient) EnableJA3(profile string) error {
	if profile == "" {
		// 禁用 JA3，恢复默认 transport
		h.transport.DialTLSContext = nil
		h.client.Transport = h.transport
		return nil
	}
	h.transport.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		// 优先使用已配置的代理 Dialer（如 SOCKS5），避免绕过代理直连
		var rawConn net.Conn
		var err error
		if h.transport.DialContext != nil {
			rawConn, err = h.transport.DialContext(ctx, network, addr)
		} else {
			d := &net.Dialer{Timeout: h.client.Timeout}
			rawConn, err = d.DialContext(ctx, network, addr)
		}
		if err != nil {
			h.LogInfo("DialContext failed", "error", err)
			return nil, err
		}
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			rawConn.Close()
			h.LogInfo("SplitHostPort failed", "error", err)
			return nil, fmt.Errorf("invalid addr %s: %v", addr, err)
		}
		clientHelloID := getClientHelloID(profile)
		uConn := utls.UClient(rawConn, &utls.Config{ServerName: host}, clientHelloID)
		// 提前构建 ClientHello，再直接修改 ALPNExtension，
		// 确保只声明 http/1.1，阻止服务端协商 h2。
		// 说明：Config.NextProtos 无法覆盖预设指纹的 ALPN，因为
		// ALPNExtension.writeToUConn 会反向覆盖 config.NextProtos。
		// 正确做法：BuildHandshakeState() 之后找到 ALPNExtension 并修改。
		if err := uConn.BuildHandshakeState(); err != nil {
			rawConn.Close()
			h.LogInfo("BuildHandshakeState failed", "error", err)
			return nil, err
		}
		for _, ext := range uConn.Extensions {
			if alpnExt, ok := ext.(*utls.ALPNExtension); ok {
				alpnExt.AlpnProtocols = []string{"http/1.1"}
				break
			}
		}
		if err := uConn.Handshake(); err != nil {
			rawConn.Close()
			h.LogInfo("TLS handshake failed", "error", err)
			return nil, err
		}
		return uConn, nil
	}
	h.client.Transport = h.transport
	return nil
}

func (h *HttpClient) DisableJA3() {
	h.LogInfo("DisableJA3 called")
	if h.transport != nil {
		h.transport.DialTLSContext = nil
		h.client.Transport = h.transport
	}
	h.LogInfo("JA3 disabled, using default TLS")
}

func getClientHelloID(profile string) utls.ClientHelloID {
	switch profile {
	case "chrome":
		return utls.HelloChrome_120
	case "firefox":
		return utls.HelloFirefox_102
	case "safari":
		return utls.HelloSafari_16_0
	case "edge":
		return utls.HelloEdge_106
	case "ios":
		return utls.HelloIOS_14
	default:
		return utls.HelloChrome_120
	}
}

func (h *HttpClient) SetLogger(logger *zap.SugaredLogger) {
	h.logger = logger
}

func (h *HttpClient) LogInfo(msg string, fields ...interface{}) {
	if h.logger != nil {
		h.logger.Infow(msg, fields...)
	}
}

func (h *HttpClient) LogError(msg string, err error) {
	if h.logger != nil {
		h.logger.Errorw(msg, "error", err)
	}
}

func (h *HttpClient) SetProxy(cfg *ProxyConfig) error {
	if cfg == nil {
		h.transport.Proxy = nil
		h.transport.DialContext = nil
		return nil
	}

	switch cfg.Type {
	case "http":
		proxyURL := &url.URL{
			Scheme: "http",
			Host:   cfg.Address,
		}
		if cfg.Username != "" && cfg.Password != "" {
			proxyURL.User = url.UserPassword(cfg.Username, cfg.Password)
		}
		h.transport.Proxy = http.ProxyURL(proxyURL)
		h.transport.DialContext = nil // 确保不再使用 SOCKS5 Dialer
	case "socks5":
		var auth *proxy.Auth
		if cfg.Username != "" && cfg.Password != "" {
			auth = &proxy.Auth{
				User:     cfg.Username,
				Password: cfg.Password,
			}
		}
		dialer, err := proxy.SOCKS5("tcp", cfg.Address, auth, proxy.Direct)
		if err != nil {
			return err
		}
		h.transport.Proxy = nil // 确保不再使用 HTTP Proxy
		// 优先使用 ContextDialer，使 SOCKS5 握手阶段能被 context 超时/取消，
		// 避免高并发时 SOCKS5 握手卡死占用并发槽。
		if cd, ok := dialer.(proxy.ContextDialer); ok {
			h.transport.DialContext = cd.DialContext
		} else {
			h.transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			}
		}
	default:
		return fmt.Errorf("unsupported proxy type: %s", cfg.Type)
	}
	return nil
}

func (h *HttpClient) SetTimeout(timeout time.Duration) {
	h.client.Timeout = timeout
	h.transport.IdleConnTimeout = timeout
	h.LogInfo("Timeout set", "duration", timeout)
}

func (h *HttpClient) GetTimeout() time.Duration {
	return h.client.Timeout
}

func (h *HttpClient) DoPost(path string, postData map[string]string) ([]byte, error) {
	var data []byte
	var err error

	headers := h.GetHeader()
	contentType := "application/json"
	for k, v := range headers {
		if strings.ToLower(k) == "content-type" {
			contentType = strings.ToLower(v)
			break
		}
	}

	switch {
	case strings.HasPrefix(contentType, "application/json"):
		data, err = json.Marshal(postData)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal JSON: %w", err)
		}
	case strings.HasPrefix(contentType, "application/x-www-form-urlencoded"):
		values := make(url.Values)
		for k, v := range postData {
			values.Set(k, ToString(v))
		}
		data = []byte(values.Encode())
	default:
		return nil, fmt.Errorf("unsupported Content-Type: %s", contentType)
	}

	req, err := http.NewRequest("POST", h.buildFullURL(path), bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return h.doRequest(req)
}

func (h *HttpClient) DoGet(path string) ([]byte, error) {
	fullUrl := h.buildFullURL(path)
	req, err := http.NewRequest("GET", fullUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	return h.doRequest(req)
}

func (h *HttpClient) DoPut(path string, putData map[string]string) ([]byte, error) {
	var data []byte
	var err error

	headers := h.GetHeader()
	contentType := "application/json"
	for k, v := range headers {
		if strings.ToLower(k) == "content-type" {
			contentType = strings.ToLower(v)
			break
		}
	}

	switch {
	case strings.HasPrefix(contentType, "application/json"):
		data, err = json.Marshal(putData)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal JSON: %w", err)
		}
	case strings.HasPrefix(contentType, "application/x-www-form-urlencoded"):
		values := make(url.Values)
		for k, v := range putData {
			values.Set(k, ToString(v))
		}
		data = []byte(values.Encode())
	default:
		return nil, fmt.Errorf("unsupported Content-Type: %s", contentType)
	}

	req, err := http.NewRequest("PUT", h.buildFullURL(path), bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return h.doRequest(req)
}

// DoPutRaw 发送原始二进制数据（适用于 OSS 上传）
func (h *HttpClient) DoPutRaw(path string, raw []byte) ([]byte, error) {
	req, err := http.NewRequest("PUT", h.buildFullURL(path), bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	return h.doRequest(req)
}

// DoDelete 发送 DELETE 请求，body 可选（传 nil 表示无 body）。
func (h *HttpClient) DoDelete(path string, body ...map[string]string) ([]byte, error) {
	var reqBody []byte
	headers := h.GetHeader()
	if len(body) > 0 && body[0] != nil {
		contentType := "application/json"
		for k, v := range headers {
			if strings.ToLower(k) == "content-type" {
				contentType = strings.ToLower(v)
				break
			}
		}
		var err error
		switch {
		case strings.HasPrefix(contentType, "application/json"):
			reqBody, err = json.Marshal(body[0])
		case strings.HasPrefix(contentType, "application/x-www-form-urlencoded"):
			values := make(url.Values)
			for k, v := range body[0] {
				values.Set(k, v)
			}
			reqBody = []byte(values.Encode())
		default:
			return nil, fmt.Errorf("unsupported Content-Type: %s", contentType)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
	}
	var req *http.Request
	var err error
	if len(reqBody) > 0 {
		req, err = http.NewRequest("DELETE", h.buildFullURL(path), bytes.NewReader(reqBody))
	} else {
		req, err = http.NewRequest("DELETE", h.buildFullURL(path), nil)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return h.doRequest(req)
}

// DoDeleteRaw 发送带原始字符串 body 的 DELETE 请求。
func (h *HttpClient) DoDeleteRaw(path, rawBody string) ([]byte, error) {
	req, err := http.NewRequest("DELETE", h.buildFullURL(path), bytes.NewBufferString(rawBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	return h.doRequest(req)
}

// DoPatch 发送 PATCH 请求，根据 Content-Type 自动序列化（JSON / form）。
func (h *HttpClient) DoPatch(path string, patchData map[string]string) ([]byte, error) {
	headers := h.GetHeader()
	contentType := "application/json"
	for k, v := range headers {
		if strings.ToLower(k) == "content-type" {
			contentType = strings.ToLower(v)
			break
		}
	}
	var data []byte
	var err error
	switch {
	case strings.HasPrefix(contentType, "application/json"):
		data, err = json.Marshal(patchData)
	case strings.HasPrefix(contentType, "application/x-www-form-urlencoded"):
		values := make(url.Values)
		for k, v := range patchData {
			values.Set(k, ToString(v))
		}
		data = []byte(values.Encode())
	default:
		return nil, fmt.Errorf("unsupported Content-Type: %s", contentType)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	req, err := http.NewRequest("PATCH", h.buildFullURL(path), bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return h.doRequest(req)
}

// DoPatchAny 发送 PATCH 请求，body 支持任意结构体（仅 JSON）。
func (h *HttpClient) DoPatchAny(path string, patchData interface{}) ([]byte, error) {
	data, err := json.Marshal(patchData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	req, err := http.NewRequest("PATCH", h.buildFullURL(path), bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	return h.doRequest(req)
}

// DoPatchRaw 发送带原始字符串 body 的 PATCH 请求。
func (h *HttpClient) DoPatchRaw(path, rawBody string) ([]byte, error) {
	req, err := http.NewRequest("PATCH", h.buildFullURL(path), bytes.NewBufferString(rawBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	return h.doRequest(req)
}

// DoHead 发送 HEAD 请求，返回响应 Headers（body 始终为空）。
func (h *HttpClient) DoHead(path string) (http.Header, error) {
	req, err := http.NewRequest("HEAD", h.buildFullURL(path), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	if h.semaphore != nil {
		h.semaphore <- struct{}{}
		defer func() { <-h.semaphore }()
	}
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return resp.Header.Clone(), nil
}

// DoOptions 发送 OPTIONS 请求，返回响应 Headers（含 Allow 等协商字段）。
func (h *HttpClient) DoOptions(path string) (http.Header, error) {
	req, err := http.NewRequest("OPTIONS", h.buildFullURL(path), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	if h.semaphore != nil {
		h.semaphore <- struct{}{}
		defer func() { <-h.semaphore }()
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return resp.Header.Clone(), nil
}

func (h *HttpClient) DoPostAny(path string, postData interface{}) ([]byte, error) {
	headers := h.GetHeader()
	contentType, exists := headers["Content-Type"]
	if !exists {
		contentType = "application/json"
	}
	if contentType != "application/json" {
		return nil, fmt.Errorf("DoPostAny only supports application/json")
	}
	data, err := json.Marshal(postData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	req, err := http.NewRequest("POST", h.buildFullURL(path), bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return h.doRequest(req)
}

func (h *HttpClient) DoPostRaw(path, rawBody string) ([]byte, error) {
	req, err := http.NewRequest("POST", h.buildFullURL(path), bytes.NewBufferString(rawBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	h.LogInfo("请求体内容", "raw", rawBody)
	return h.doRequest(req)
}

func (h *HttpClient) DoGetRaw(path string) ([]byte, error) {
	req, err := http.NewRequest("GET", h.buildFullURL(path), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create GET request: %w", err)
	}
	// 设置 headers
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	h.LogInfo("GET 请求准备发送", "url", req.URL.String(), "headers", req.Header)
	// 调用内部请求方法
	return h.doRequest(req)
}

func (h *HttpClient) DoPostMultipart(path string, fields map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// 写普通字段
	for k, v := range fields {
		_ = writer.WriteField(k, v)
	}

	_ = writer.Close()

	req, err := http.NewRequest("POST", h.buildFullURL(path), &buf)
	if err != nil {
		return nil, err
	}

	// 设置 header
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return h.doRequest(req)
}

func (h *HttpClient) UploadFile(path, fieldName, filePath string, extraParams map[string]string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		h.LogInfo(fmt.Sprintf("failed to open file: %s", filePath))
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	for k, v := range extraParams {
		_ = writer.WriteField(k, v)
	}
	part, err := writer.CreateFormFile(fieldName, filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("创建文件字段失败: %w", err)
	}
	if _, err = io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("写入文件失败: %w", err)
	}
	_ = writer.Close()
	req, err := http.NewRequest("POST", h.buildFullURL(path), &requestBody)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return h.doRequest(req)
}

func (h *HttpClient) DownloadFile(path, savePath string) error {
	h.LogInfo("DownloadFile called", "url", path, "savePath", savePath)
	req, err := http.NewRequest("GET", h.buildFullURL(path), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	// 设置 header
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed, status code: %d", resp.StatusCode)
	}

	out, err := os.Create(savePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	h.LogInfo("File downloaded successfully", "path", savePath)
	return nil
}

func (h *HttpClient) doRequest(req *http.Request) ([]byte, error) {
	return h.doRequestWith(req, h.client)
}

func (h *HttpClient) doRequestWith(req *http.Request, c *http.Client) ([]byte, error) {
	// 并发限速
	if h.semaphore != nil {
		h.semaphore <- struct{}{}
		defer func() { <-h.semaphore }()
	}

	// 一次性读取请求体，供日志和重试使用
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			h.LogInfo("读取请求体失败", zap.Error(err))
		}
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}
	requestBody := string(bodyBytes)

	// 用 Set 避免重试时 Accept-Encoding 重复追加
	req.Header.Set("Accept-Encoding", "gzip")

	h.LogInfo("请求准备发送",
		"method", req.Method,
		"url", req.URL.String(),
		"headers", req.Header,
		"body", requestBody,
	)

	const maxRetries = 3
	var (
		res *http.Response
		err error
	)
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// 重置 body 供重试使用
			if len(bodyBytes) > 0 {
				req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			}
			wait := time.Duration(attempt) * 200 * time.Millisecond
			h.LogInfo("请求重试",
				"attempt", attempt,
				"wait", wait.String(),
				"url", req.URL.String(),
			)
			time.Sleep(wait)
		}

		res, err = c.Do(req)
		if err == nil {
			break
		}
		// 只对瞬态连接错误重试（EOF、连接重置等）
		if !IsRetryableError(err) || attempt == maxRetries {
			break
		}
	}

	if err != nil {
		h.LogInfo("请求失败",
			"error", err,
			"method", req.Method,
			"url", req.URL.String(),
			"headers", req.Header,
			"body", requestBody,
		)
		switch {
		case IsTimeoutError(err):
			return nil, errors.New("请求超时")
		case IsDNSError(err):
			return nil, errors.New("地址错误")
		case IsConnectionRefused(err):
			return nil, errors.New("连接被拒绝")
		case IsNetworkUnreachable(err):
			return nil, errors.New("网络不可达")
		case IsInvalidAddressError(err):
			return nil, errors.New("无效的 URL 或地址")
		default:
			return nil, err
		}
	}
	defer res.Body.Close()

	var reader io.ReadCloser = res.Body
	if res.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(res.Body)
		if err != nil {
			h.LogInfo("解压 gzip 失败", zap.Error(err))
			return nil, err
		}
		defer gzReader.Close()
		reader = gzReader
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		h.LogInfo("读取响应失败",
			zap.Error(err),
			"method", req.Method,
			"url", req.URL.String(),
		)
		return nil, err
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		h.LogInfo("请求返回非成功状态",
			"status", res.StatusCode,
			"method", req.Method,
			"url", req.URL.String(),
			"request_headers", req.Header,
			"request_body", requestBody,
			"response_headers", fmt.Sprintf("%v", res.Header),
			"response_body", string(body),
		)
		return body, nil
	}

	h.LogInfo("请求成功",
		"status", res.StatusCode,
		"method", req.Method,
		"url", req.URL.String(),
		"request_headers", req.Header,
		"request_body", requestBody,
		"response_headers", fmt.Sprintf("%v", res.Header),
		"response_body", string(body),
	)

	return body, nil
}

// clientWithSession 使用 Session 的 jar 创建一个临时 http.Client（共享 transport）。
func (h *HttpClient) clientWithSession(s *Session) *http.Client {
	return &http.Client{
		Transport: h.transport,
		Timeout:   h.client.Timeout,
		Jar:       s.jar,
	}
}

func (h *HttpClient) SetDomain(domain string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.domain = domain
}

func (h *HttpClient) GetDomain() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.domain
}

func (h *HttpClient) SetHeader(headers map[string]string) {
	for k, v := range headers {
		h.setHeaderInternal(k, v)
	}
}

func (h *HttpClient) AddHeader(name, value string) {
	h.setHeaderInternal(name, value)
}

func (h *HttpClient) setHeaderInternal(name, value string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.headers == nil {
		h.headers = make(map[string]string)
	}
	canonicalKey := textproto.CanonicalMIMEHeaderKey(name)
	h.headers[canonicalKey] = value
}

// GetHeader 返回当前 headers 的副本（线程安全）。
func (h *HttpClient) GetHeader() map[string]string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	cp := make(map[string]string, len(h.headers))
	for k, v := range h.headers {
		cp[k] = v
	}
	return cp
}

func (h *HttpClient) GetCookies() []*http.Cookie {
	u, err := url.Parse(h.domain)
	if err != nil {
		return nil
	}
	return h.jar.Cookies(u)
}

func (h *HttpClient) GetCookieValue(name string) string {
	u, err := url.Parse(h.domain)
	if err != nil {
		h.LogError("GetCookieValue failed", err)
		return ""
	}
	for _, c := range h.jar.Cookies(u) {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func (h *HttpClient) SetCookies(cookies map[string]string, opts ...bool) {
	u, err := url.Parse(h.domain)
	if err != nil {
		h.LogError("SetCookies failed", err)
		return
	}
	reset := false
	if len(opts) > 0 {
		reset = opts[0]
	}
	if reset {
		// 清空当前 URL 下的所有 Cookie
		h.jar.SetCookies(u, []*http.Cookie{})
	}
	var cookieList []*http.Cookie
	secure := false
	if u.Scheme == "https" {
		secure = true
	}
	for k, v := range cookies {
		cookieList = append(cookieList, &http.Cookie{
			Name:   k,
			Value:  v,
			Path:   "/",
			Domain: u.Hostname(),
			Secure: secure,
		})
	}
	h.jar.SetCookies(u, cookieList)
}

// GetCookiesFor 获取指定 URL 域名下的所有 Cookie（多域名场景使用）。
func (h *HttpClient) GetCookiesFor(rawURL string) []*http.Cookie {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	return h.jar.Cookies(u)
}

// GetCookieValueFor 获取指定 URL 域名下某个 Cookie 的值（多域名场景使用）。
func (h *HttpClient) GetCookieValueFor(rawURL, name string) string {
	for _, c := range h.GetCookiesFor(rawURL) {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

// SetCookiesFor 设置指定 URL 域名下的 Cookie（多域名场景使用）。
func (h *HttpClient) SetCookiesFor(rawURL string, cookies map[string]string, opts ...bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		h.LogError("SetCookiesFor failed", err)
		return
	}
	if len(opts) > 0 && opts[0] {
		h.jar.SetCookies(u, []*http.Cookie{})
	}
	secure := u.Scheme == "https"
	var list []*http.Cookie
	for k, v := range cookies {
		list = append(list, &http.Cookie{
			Name: k, Value: v, Path: "/", Domain: u.Hostname(), Secure: secure,
		})
	}
	h.jar.SetCookies(u, list)
}

// DoGetWithSession 使用独立 Session（独立 CookieJar）发送 GET 请求。
func (h *HttpClient) DoGetWithSession(s *Session, path string) ([]byte, error) {
	req, err := http.NewRequest("GET", h.buildFullURL(path), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	headers := h.GetHeader()
	for k, v := range s.getHeaders() {
		headers[k] = v // session header 优先级更高
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return h.doRequestWith(req, h.clientWithSession(s))
}

// DoPostWithSession 使用独立 Session（独立 CookieJar）发送 POST 请求。
func (h *HttpClient) DoPostWithSession(s *Session, path string, postData map[string]string) ([]byte, error) {
	headers := h.GetHeader()
	for k, v := range s.getHeaders() {
		headers[k] = v
	}
	contentType := "application/json"
	for k, v := range headers {
		if strings.ToLower(k) == "content-type" {
			contentType = strings.ToLower(v)
			break
		}
	}
	var data []byte
	var err error
	switch {
	case strings.HasPrefix(contentType, "application/json"):
		data, err = json.Marshal(postData)
	case strings.HasPrefix(contentType, "application/x-www-form-urlencoded"):
		values := make(url.Values)
		for k, v := range postData {
			values.Set(k, v)
		}
		data = []byte(values.Encode())
	default:
		return nil, fmt.Errorf("unsupported Content-Type: %s", contentType)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: %w", err)
	}
	req, err := http.NewRequest("POST", h.buildFullURL(path), bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return h.doRequestWith(req, h.clientWithSession(s))
}

func (h *HttpClient) Close() {
	if h.transport != nil {
		h.transport.CloseIdleConnections()
	}
}

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
