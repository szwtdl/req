package client

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// doRequest 使用默认 client 执行请求。
func (h *HttpClient) doRequest(req *http.Request) ([]byte, error) {
	return h.doRequestWith(req, h.client)
}

// clientWithSession 使用 Session 的 jar 创建一个临时 http.Client（共享 transport）。
func (h *HttpClient) clientWithSession(s *Session) *http.Client {
	return &http.Client{
		Transport: h.transport,
		Timeout:   h.client.Timeout,
		Jar:       s.jar,
	}
}

// doRequestWith 执行实际 HTTP 请求，包含并发限速、自动解压、错误重试及详细日志。
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

