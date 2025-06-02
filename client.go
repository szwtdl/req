package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"golang.org/x/net/proxy"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	domain    string
	header    map[string]string
	cookies   map[string]string
	logger    *zap.SugaredLogger
}

func NewHttpClient(domain string, timeout ...time.Duration) *HttpClient {
	defaultTimeout := 30 * time.Second
	if len(timeout) > 0 && timeout[0] > 0 {
		defaultTimeout = timeout[0]
	}
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		DisableKeepAlives:   true,
		IdleConnTimeout:     defaultTimeout,
	}
	return &HttpClient{
		client: &http.Client{
			Transport: transport,
			Timeout:   defaultTimeout,
		},
		transport: transport,
		domain:    domain,
		header: map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
			"User-Agent":   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3",
		},
		cookies: make(map[string]string),
		logger:  nil,
	}
	// Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3
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

func (h *HttpClient) SetTimeout(timeout time.Duration) {
	h.client.Timeout = timeout
	h.transport.IdleConnTimeout = timeout
	h.LogInfo("Timeout set", "duration", timeout)
}

func (h *HttpClient) GetTimeout() time.Duration {
	return h.client.Timeout
}

func (h *HttpClient) DoPost(postUrl string, postData map[string]string) ([]byte, error) {
	var data []byte
	var err error
	headers := h.GetHeader()
	contentType, exists := headers["Content-Type"]
	if !exists {
		contentType = "application/json"
	}
	switch contentType {
	case "application/json":
		data, err = json.Marshal(postData)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal JSON: %w", err)
		}
	case "application/x-www-form-urlencoded":
		postDataValues := make(url.Values)
		for k, v := range postData {
			postDataValues.Set(k, ToString(v))
		}
		data = []byte(postDataValues.Encode())
	default:
		return nil, fmt.Errorf("unsupported Content-Type: %s", contentType)
	}
	domain := h.GetDomain()
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/%s", domain, postUrl), bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return h.doRequest(req)
}

func (h *HttpClient) DoGet(url string) ([]byte, error) {
	domain := h.GetDomain()
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/%s", domain, url), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	headers := h.GetHeader()
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return h.doRequest(req)
}

func (h *HttpClient) UploadFile(uploadUrl, fieldName, filePath string, extraParams map[string]string) ([]byte, error) {
	domain := h.GetDomain()
	file, err := os.Open(filePath)
	if err != nil {
		h.LogInfo(fmt.Sprintf("failed to open file: %s", filePath))
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	// 写入文件字段
	part, err := writer.CreateFormFile(fieldName, filepath.Base(filePath))
	if err != nil {
		h.LogInfo(fmt.Sprintf("failed to create form file: %s", filePath))
		return nil, fmt.Errorf("创建文件字段失败: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		h.LogInfo(fmt.Sprintf("failed to copy file: %s", filePath))
		return nil, fmt.Errorf("写入文件失败: %w", err)
	}
	// 写入额外表单字段
	for k, v := range extraParams {
		_ = writer.WriteField(k, v)
	}
	// 关闭 writer 以设置 Content-Type 边界
	if err := writer.Close(); err != nil {
		h.LogInfo(fmt.Sprintf("failed to close writer: %s", filePath))
		return nil, fmt.Errorf("关闭 multipart writer 失败: %w", err)
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/%s", domain, uploadUrl), &requestBody)
	if err != nil {
		h.LogInfo(fmt.Sprintf("failed to create request: %v", err))
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	if len(h.cookies) > 0 {
		cookieHeader := ""
		for k, v := range h.cookies {
			cookieHeader += fmt.Sprintf("%s=%s; ", k, v)
		}
		req.Header.Set("Cookie", cookieHeader)
	}
	// 设置头部
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return h.doRequest(req)
}

func (h *HttpClient) doRequest(req *http.Request) ([]byte, error) {
	h.LogInfo("请求准备发送", "method", req.Method, "url", req.URL.String(), "headers", req.Header)
	// 添加已有 cookie 到请求头
	if len(h.cookies) > 0 {
		cookieHeader := ""
		for k, v := range h.cookies {
			cookieHeader += fmt.Sprintf("%s=%s; ", k, v)
		}
		req.Header.Set("Cookie", cookieHeader)
	}
	// 执行请求
	res, err := h.client.Do(req)
	if err != nil {
		h.LogInfo("请求失败", zap.Error(err))
		return nil, fmt.Errorf("请求失败: %s", err.Error())
	}
	defer res.Body.Close()
	// 解析响应 Cookie
	for _, c := range res.Cookies() {
		h.cookies[c.Name] = c.Value
	}
	// 读取响应内容
	body, err := io.ReadAll(res.Body)
	if err != nil {
		h.LogInfo("读取响应失败", zap.Error(err))
		return nil, fmt.Errorf("读取失败: %s", err.Error())
	}
	h.LogInfo("收到响应", zap.Int("status", res.StatusCode), zap.String("body", string(body)))
	if res.StatusCode != http.StatusOK {
		h.LogInfo("请求失败", zap.Int("status", res.StatusCode), zap.String("body", string(body)))
		return nil, fmt.Errorf("请求失败: %s", string(body))
	}
	return body, nil
}

func (h *HttpClient) SetDomain(domain string) {
	h.domain = domain
}

func (h *HttpClient) GetDomain() string {
	return h.domain
}

func (h *HttpClient) SetHeader(header map[string]string) {
	if h.header == nil {
		h.header = make(map[string]string)
	}
	for k, v := range header {
		h.header[k] = v
	}
}

func (h *HttpClient) GetHeader() map[string]string {
	return h.header
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

func (h *HttpClient) SetCookies(cookies map[string]string) {
	cookie := h.GetCookies()
	for k, v := range cookies {
		cookie[k] = v
	}
	h.cookies = cookie
}

func (h *HttpClient) GetCookies() map[string]string {
	return h.cookies
}
