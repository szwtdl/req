package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type HttpClient struct {
	client  *http.Client
	domain  string
	header  map[string]string
	cookies map[string]string
}

func NewHttpClient(domain string, header map[string]string) *HttpClient {
	return &HttpClient{
		client: &http.Client{
			Timeout: 10 * http.DefaultClient.Timeout,
		},
		domain:  domain,
		header:  header,
		cookies: make(map[string]string),
	}
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

func (h *HttpClient) doRequest(req *http.Request) ([]byte, error) {
	res, err := h.client.Do(req)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("请求失败: %s", err.Error()))
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(res.Body)
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("读取失败: %s", err.Error()))
	}
	if res.StatusCode != http.StatusOK {
		return nil, errors.New(fmt.Sprintf("请求失败: %s", body))
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
	h.header = header
}

func (h *HttpClient) GetHeader() map[string]string {
	return h.header
}

func (h *HttpClient) SetCookies(cookies map[string]string) {
	h.cookies = cookies
}

func (h *HttpClient) GetCookies() map[string]string {
	return h.cookies
}
