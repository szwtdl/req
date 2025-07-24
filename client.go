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
	domain    string
	headers   map[string]string
	jar       http.CookieJar
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
	h.LogInfo("EnableJA3 called", "profile", profile)
	if profile == "" {
		// 禁用 JA3，恢复默认 transport
		h.transport.DialTLSContext = nil
		h.client.Transport = h.transport
		return nil
	}
	h.transport.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		h.LogInfo("DialTLSContext called", "network", network, "addr", addr)
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
		h.LogInfo("Performing TLS handshake with profile", "clientHelloID", clientHelloID, "host", host)
		uConn := utls.UClient(rawConn, &utls.Config{ServerName: host}, clientHelloID)
		if err := uConn.Handshake(); err != nil {
			rawConn.Close()
			h.LogInfo("TLS handshake failed", "error", err)
			return nil, err
		}
		h.LogInfo("TLS handshake success", "host", host)
		return uConn, nil
	}
	h.client.Transport = h.transport
	h.LogInfo("JA3 enabled with profile", "profile", profile)
	return nil
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

	// 获取 header，并统一把 content-type 转为标准写法（首字母大写，其余小写）
	headers := h.GetHeader()
	var contentType string
	for k, v := range headers {
		if strings.ToLower(k) == "content-type" {
			contentType = strings.ToLower(v)
			break
		}
	}
	if contentType == "" {
		contentType = "application/json"
	}

	switch {
	case strings.HasPrefix(contentType, "application/json"):
		data, err = json.Marshal(postData)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal JSON: %w", err)
		}
	case strings.HasPrefix(contentType, "application/x-www-form-urlencoded"):
		postDataValues := make(url.Values)
		for k, v := range postData {
			postDataValues.Set(k, ToString(v))
		}
		data = []byte(postDataValues.Encode())
	default:
		return nil, fmt.Errorf("unsupported Content-Type: %s", contentType)
	}
	var fullUrl string
	if strings.HasPrefix(postUrl, "http://") || strings.HasPrefix(postUrl, "https://") {
		fullUrl = postUrl
	} else {
		fullUrl = fmt.Sprintf("%s/%s", strings.TrimRight(h.GetDomain(), "/"), strings.TrimLeft(postUrl, "/"))
	}
	req, err := http.NewRequest("POST", fullUrl, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	// 统一设置 header，大小写无关
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return h.doRequest(req)
}

func (h *HttpClient) DoGet(postUrl string) ([]byte, error) {
	var fullUrl string
	if strings.HasPrefix(postUrl, "http://") || strings.HasPrefix(postUrl, "https://") {
		fullUrl = postUrl
	} else {
		fullUrl = fmt.Sprintf("%s/%s", h.GetDomain(), postUrl)
	}
	req, err := http.NewRequest("GET", fullUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	headers := h.GetHeader()
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return h.doRequest(req)
}

func (h *HttpClient) DoPostAny(postUrl string, postData interface{}) ([]byte, error) {
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
	var fullUrl string
	if strings.HasPrefix(postUrl, "http://") || strings.HasPrefix(postUrl, "https://") {
		fullUrl = postUrl
	} else {
		fullUrl = fmt.Sprintf("%s/%s", h.GetDomain(), postUrl)
	}

	req, err := http.NewRequest("POST", fullUrl, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return h.doRequest(req)
}

func (h *HttpClient) DoPostRaw(postUrl string, rawBody string) ([]byte, error) {
	var fullUrl string
	if strings.HasPrefix(postUrl, "http://") || strings.HasPrefix(postUrl, "https://") {
		fullUrl = postUrl
	} else {
		fullUrl = fmt.Sprintf("%s/%s", h.GetDomain(), postUrl)
	}
	req, err := http.NewRequest("POST", fullUrl, bytes.NewBufferString(rawBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	// 设置请求头
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	h.LogInfo("请求体内容", "raw", rawBody)
	return h.doRequest(req)
}

func (h *HttpClient) UploadFile(postUrl, fieldName, filePath string, extraParams map[string]string) ([]byte, error) {
	var fullUrl string
	if strings.HasPrefix(postUrl, "http://") || strings.HasPrefix(postUrl, "https://") {
		fullUrl = postUrl
	} else {
		fullUrl = fmt.Sprintf("%s/%s", h.GetDomain(), postUrl)
	}
	file, err := os.Open(filePath)
	if err != nil {
		h.LogInfo(fmt.Sprintf("failed to open file: %s", filePath))
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	// 先写入额外表单字段
	for k, v := range extraParams {
		_ = writer.WriteField(k, v)
	}
	// 再写入文件字段
	part, err := writer.CreateFormFile(fieldName, filepath.Base(filePath))
	if err != nil {
		h.LogInfo(fmt.Sprintf("failed to create form file: %s", filePath))
		return nil, fmt.Errorf("创建文件字段失败: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		h.LogInfo(fmt.Sprintf("failed to copy file: %s", filePath))
		return nil, fmt.Errorf("写入文件失败: %w", err)
	}
	// 关闭 writer 以设置 Content-Type 边界
	if err := writer.Close(); err != nil {
		h.LogInfo(fmt.Sprintf("failed to close writer: %s", filePath))
		return nil, fmt.Errorf("关闭 multipart writer 失败: %w", err)
	}
	req, err := http.NewRequest("POST", fullUrl, &requestBody)
	if err != nil {
		h.LogInfo(fmt.Sprintf("failed to create request: %v", err))
		return nil, fmt.Errorf("创建请求失败: %w", err)
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
	// 执行请求
	req.Header.Add("Accept-Encoding", "gzip")
	res, err := h.client.Do(req)
	if err != nil {
		h.LogInfo("请求失败", zap.Error(err))
		return nil, fmt.Errorf("请求失败: %s", err.Error())
	}
	defer res.Body.Close()
	// 判断是否 gzip 压缩
	var reader io.ReadCloser = res.Body
	if res.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(res.Body)
		if err != nil {
			h.LogInfo("解压 gzip 失败", zap.Error(err))
			return nil, fmt.Errorf("解压 gzip 失败: %s", err.Error())
		}
		defer gzReader.Close()
		reader = gzReader
	}
	// 读取响应内容
	body, err := io.ReadAll(reader)
	if err != nil {
		h.LogInfo("读取响应失败", zap.Error(err))
		return nil, fmt.Errorf("读取失败: %s", err.Error())
	}
	h.LogInfo("收到响应", zap.Int("status", res.StatusCode), zap.String("body", string(body)))
	return body, nil
}

func (h *HttpClient) SetDomain(domain string) {
	h.domain = domain
}

func (h *HttpClient) GetDomain() string {
	return h.domain
}

func (h *HttpClient) SetHeader(header map[string]string) {
	if h.headers == nil {
		h.headers = make(map[string]string)
	}
	for k, v := range header {
		canonicalKey := textproto.CanonicalMIMEHeaderKey(k)
		h.headers[canonicalKey] = v
	}
}

func (h *HttpClient) GetHeader() map[string]string {
	return h.headers
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

func (h *HttpClient) GetCookies() []*http.Cookie {
	u, err := url.Parse(h.domain)
	if err != nil {
		return nil
	}
	return h.jar.Cookies(u)
}

func (h *HttpClient) SetCookies(cookies map[string]string) {
	u, err := url.Parse(h.domain)
	if err != nil {
		h.LogError("SetCookiesMap failed", err)
		return
	}
	var cookieList []*http.Cookie
	for k, v := range cookies {
		cookieList = append(cookieList, &http.Cookie{
			Name:   k,
			Value:  v,
			Path:   "/",
			Domain: u.Host,
		})
	}
	h.jar.SetCookies(u, cookieList)
}

func (h *HttpClient) Close() {
	if h.transport != nil {
		h.transport.CloseIdleConnections()
	}
}
