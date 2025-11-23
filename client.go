package client

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
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
	"time"
)

type ProxyConfig struct {
	Type     string
	Address  string
	Username string
	Password string
}

type HttpClient struct {
	client    *http.Client
	transport *http.Transport
	jar       http.CookieJar
	logger    *zap.SugaredLogger
	domain    string
	headers   map[string]string
}

func NewHttpClient(domain string, timeout ...time.Duration) *HttpClient {
	defaultTimeout := 30 * time.Second
	if len(timeout) > 0 && timeout[0] > 0 {
		defaultTimeout = timeout[0]
	}
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		DisableKeepAlives:   false,
		IdleConnTimeout:     defaultTimeout,
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(fmt.Errorf("failed to create cookie jar: %v", err))
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
		jar: jar,
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
		dialer := &net.Dialer{Timeout: h.client.Timeout}
		rawConn, err := dialer.DialContext(ctx, network, addr)
		if err != nil {
			h.LogInfo("DialContext failed", "error", err)
			return nil, err
		}
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			rawConn.Close()
			h.LogInfo("DialContext failed", "error", err)
			return nil, fmt.Errorf("invalid addr %s: %v", addr, err)
		}
		clientHelloID := getClientHelloID(profile)
		uConn := utls.UClient(rawConn, &utls.Config{ServerName: host}, clientHelloID)
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
		h.transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
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
	// 记录请求体内容（如果有）
	var requestBody string
	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			h.LogInfo("读取请求体失败", zap.Error(err))
		} else {
			requestBody = string(bodyBytes)
			// 重新生成 req.Body 供 http.Client 使用
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
	}

	h.LogInfo("请求准备发送",
		"method", req.Method,
		"url", req.URL.String(),
		"headers", req.Header,
		"body", requestBody,
	)

	// 执行请求
	req.Header.Add("Accept-Encoding", "gzip")
	res, err := h.client.Do(req)
	if err != nil {
		h.LogInfo("请求失败",
			"error", err,
			"method", req.Method,
			"url", req.URL.String(),
			"headers", req.Header,
			"body", requestBody,
		)
		return nil, fmt.Errorf("网络请求失败，请稍后重试")
	}
	defer res.Body.Close()

	// 判断是否 gzip 压缩
	var reader io.ReadCloser = res.Body
	if res.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(res.Body)
		if err != nil {
			h.LogInfo("解压 gzip 失败", zap.Error(err))
			return nil, fmt.Errorf("响应解压失败")
		}
		defer gzReader.Close()
		reader = gzReader
	}

	// 读取响应内容
	body, err := io.ReadAll(reader)
	if err != nil {
		h.LogInfo("读取响应失败",
			zap.Error(err),
			"method", req.Method,
			"url", req.URL.String(),
			"headers", req.Header,
			"body", requestBody,
		)
		return nil, fmt.Errorf("读取服务响应失败")
	}

	// 如果状态码非 2xx，也记录完整信息
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
		return body, fmt.Errorf("请求失败：HTTP %d", res.StatusCode)
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

func (h *HttpClient) SetDomain(domain string) {
	h.domain = domain
}

func (h *HttpClient) GetDomain() string {
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
	if h.headers == nil {
		h.headers = make(map[string]string)
	}
	canonicalKey := textproto.CanonicalMIMEHeaderKey(name)
	h.headers[canonicalKey] = value
}

func (h *HttpClient) GetHeader() map[string]string {
	if h.headers == nil {
		h.headers = make(map[string]string)
	}
	return h.headers
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

func (h *HttpClient) Close() {
	if h.transport != nil {
		h.transport.CloseIdleConnections()
	}
}

func (h *HttpClient) buildFullURL(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	domain := strings.TrimRight(h.GetDomain(), "/")
	path = strings.TrimLeft(path, "/")
	return fmt.Sprintf("%s/%s", domain, path)
}
