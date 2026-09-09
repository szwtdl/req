package client

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// RequestOptions 是单次请求配置；调用期间不要修改 Headers。
// Session 为空时不自动携带或保存 Cookie。NoRedirect 禁止自动跟随跳转。
type RequestOptions struct {
	Headers    http.Header
	Session    *Session
	NoRedirect bool
}

// Response 保留 HTTP 状态、响应头和响应体；非 2xx 状态由调用方判断。
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// Do 使用完整 HTTP(S) URL 执行请求，不修改默认域名、Header 或 Cookie。
// 不自动重试，避免重复提交；ctx 覆盖排队、连接及响应读取阶段。
// Transport、代理、JA3 和默认超时必须在并发请求开始前配置完成。
func (h *HttpClient) Do(ctx context.Context, method, rawURL string, body io.Reader, options RequestOptions) (*Response, error) {
	if ctx == nil {
		return nil, fmt.Errorf("request context is required")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse request URL: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return nil, fmt.Errorf("request requires an absolute HTTP(S) URL without credentials")
	}
	request, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	for name, value := range h.GetHeader() {
		request.Header.Set(name, value)
	}
	c := &http.Client{Transport: h.transport, Timeout: h.client.Timeout}
	if options.Session != nil {
		for name, value := range options.Session.getHeaders() {
			request.Header.Set(name, value)
		}
		c.Jar = options.Session.getJar()
	}
	for name, values := range options.Headers {
		request.Header.Del(name)
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	if options.NoRedirect {
		c.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	}
	if h.semaphore != nil {
		select {
		case h.semaphore <- struct{}{}:
			defer func() { <-h.semaphore }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	response, err := c.Do(request)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer response.Body.Close()
	var reader io.Reader = response.Body
	if response.Header.Get("Content-Encoding") == "gzip" && method != http.MethodHead {
		decoded, err := gzip.NewReader(response.Body)
		if err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}
		defer decoded.Close()
		reader = decoded
	}
	data, err := io.ReadAll(reader)
	result := &Response{StatusCode: response.StatusCode, Header: response.Header.Clone(), Body: data}
	if err != nil {
		return result, fmt.Errorf("read response: %w", err)
	}
	return result, nil
}
