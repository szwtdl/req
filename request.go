package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
)

// DoGet 发送 GET 请求。
func (h *HttpClient) DoGet(path string) ([]byte, error) {
	req, err := http.NewRequest("GET", h.buildFullURL(path), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	return h.doRequest(req)
}

// DoGetRaw 发送 GET 请求（带详细日志）。
func (h *HttpClient) DoGetRaw(path string) ([]byte, error) {
	req, err := http.NewRequest("GET", h.buildFullURL(path), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create GET request: %w", err)
	}
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	h.LogInfo("GET 请求准备发送", "url", req.URL.String(), "headers", req.Header)
	return h.doRequest(req)
}

// DoGetRawWithHeader 发送 GET 请求并返回响应头（带详细日志）。
// 用于下载类接口读取响应头（如 Content-Disposition 的 fileName）。
func (h *HttpClient) DoGetRawWithHeader(path string) ([]byte, http.Header, error) {
	req, err := http.NewRequest("GET", h.buildFullURL(path), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GET request: %w", err)
	}
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	h.LogInfo("GET 请求准备发送", "url", req.URL.String(), "headers", req.Header)
	return h.doRequestWithHeader(req, h.defaultClient())
}

// DoPost 发送 POST 请求，根据 Content-Type 自动序列化（JSON / form）。
func (h *HttpClient) DoPost(path string, postData map[string]string) ([]byte, error) {
	headers := h.GetHeader()
	data, err := encodeBody(headers, postData)
	if err != nil {
		return nil, err
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

func (h *HttpClient) GetHeaderValue(name string) string {
	headers := h.GetHeader()
	for k, v := range headers {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

// DoPostAny 发送 POST 请求，body 支持任意结构体（仅 JSON）。
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

// DoPostRaw 发送带原始字符串 body 的 POST 请求。
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

// DoPostMultipart 发送 multipart/form-data 格式的 POST 请求（纯字段，无文件）。
func (h *HttpClient) DoPostMultipart(path string, fields map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = writer.WriteField(k, v)
	}
	_ = writer.Close()

	req, err := http.NewRequest("POST", h.buildFullURL(path), &buf)
	if err != nil {
		return nil, err
	}
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return h.doRequest(req)
}

// DoPut 发送 PUT 请求，根据 Content-Type 自动序列化（JSON / form）。
func (h *HttpClient) DoPut(path string, putData map[string]string) ([]byte, error) {
	headers := h.GetHeader()
	data, err := encodeBody(headers, putData)
	if err != nil {
		return nil, err
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

// DoPutRaw 发送原始二进制数据（适用于 OSS 上传等场景）。
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
	headers := h.GetHeader()
	var reqBody []byte
	if len(body) > 0 && body[0] != nil {
		var err error
		reqBody, err = encodeBody(headers, body[0])
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
	data, err := encodeBody(headers, patchData)
	if err != nil {
		return nil, err
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
	resp, err := h.defaultClient().Do(req)
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
	resp, err := h.defaultClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return resp.Header.Clone(), nil
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

// DoGetRedirect 发送 GET 请求并阻止跟随重定向，返回 3xx 响应的 Location 跳转链接。
// 若响应非 3xx 或没有 Location 头，返回空字符串。
func (h *HttpClient) DoGetRedirect(path string) (string, error) {
	return h.doGetRedirect(path, nil)
}

// DoGetRedirectWithSession 使用独立 Session 发送 GET 请求并阻止跟随重定向，返回 Location 跳转链接。
func (h *HttpClient) DoGetRedirectWithSession(s *Session, path string) (string, error) {
	return h.doGetRedirect(path, s)
}

// doGetRedirect 执行不跟随重定向的 GET 请求，返回 Location 头。
func (h *HttpClient) doGetRedirect(path string, s *Session) (string, error) {
	req, err := http.NewRequest("GET", h.buildFullURL(path), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}
	headers := h.GetHeader()
	if s != nil {
		for k, v := range s.getHeaders() {
			headers[k] = v
		}
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// 构建不跟随重定向的 client
	c := &http.Client{
		Transport: h.transport,
		Timeout:   h.client.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // 阻止跟随重定向
		},
	}
	if s != nil {
		c.Jar = s.getJar()
	} else {
		c.Jar = h.defaultJar()
	}

	// 并发限速
	if h.semaphore != nil {
		h.semaphore <- struct{}{}
		defer func() { <-h.semaphore }()
	}

	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	location := resp.Header.Get("Location")
	h.LogInfo("获取重定向链接",
		"status", resp.StatusCode,
		"url", req.URL.String(),
		"location", location,
	)
	return location, nil
}

// DoPostWithSession 使用独立 Session（独立 CookieJar）发送 POST 请求。
func (h *HttpClient) DoPostWithSession(s *Session, path string, postData map[string]string) ([]byte, error) {
	headers := h.GetHeader()
	for k, v := range s.getHeaders() {
		headers[k] = v
	}
	data, err := encodeBody(headers, postData)
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

// encodeBody 根据 Content-Type header 将 map 序列化为 JSON 或 form-urlencoded。
func encodeBody(headers map[string]string, data map[string]string) ([]byte, error) {
	contentType := "application/json"
	for k, v := range headers {
		if strings.ToLower(k) == "content-type" {
			contentType = strings.ToLower(v)
			break
		}
	}
	switch {
	case strings.HasPrefix(contentType, "application/json"):
		b, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal JSON: %w", err)
		}
		return b, nil
	case strings.HasPrefix(contentType, "application/x-www-form-urlencoded"):
		values := make(url.Values)
		for k, v := range data {
			values.Set(k, ToString(v))
		}
		return []byte(values.Encode()), nil
	default:
		return nil, fmt.Errorf("unsupported Content-Type: %s", contentType)
	}
}
