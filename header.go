package client

import "net/textproto"

// SetHeader 批量设置请求头（已存在的 key 会被覆盖）。
func (h *HttpClient) SetHeader(headers map[string]string) {
	for k, v := range headers {
		h.setHeaderInternal(k, v)
	}
}

// AddHeader 添加或覆盖单个请求头。
func (h *HttpClient) AddHeader(name, value string) {
	h.setHeaderInternal(name, value)
}

// setHeaderInternal 线程安全地写入单个请求头（key 规范化为 Canonical-MIME 格式）。
func (h *HttpClient) setHeaderInternal(name, value string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.headers == nil {
		h.headers = make(map[string]string)
	}
	h.headers[textproto.CanonicalMIMEHeaderKey(name)] = value
}

// GetHeader 返回当前所有请求头的副本（线程安全）。
func (h *HttpClient) GetHeader() map[string]string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	cp := make(map[string]string, len(h.headers))
	for k, v := range h.headers {
		cp[k] = v
	}
	return cp
}

