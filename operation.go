package client

import "net/http"

// NewOperation 创建独立操作状态，复用连接池、限流器和启动阶段配置。
// 不复制默认 Cookie；调用方可传入已有 Session 延续登录会话。
// 同一个操作内允许顺序设置域名和 Header，不应并发修改操作状态。
func (h *HttpClient) NewOperation(domain string, session *Session) *HttpClient {
	if session == nil {
		session = NewSession()
	}
	h.mu.RLock()
	headers := make(map[string]string, len(h.headers))
	for name, value := range h.headers {
		headers[name] = value
	}
	logger := h.logger
	h.mu.RUnlock()
	jar := session.getJar()
	return &HttpClient{
		client:    &http.Client{Transport: h.transport, Timeout: h.client.Timeout, Jar: jar},
		transport: h.transport, jar: jar, session: session, domain: domain, headers: headers,
		logger: logger, semaphore: h.semaphore,
	}
}

// Session 返回当前操作的显式 Cookie 会话；默认客户端返回 Jar 快照。
// 不复制默认 Header，当前请求应显式提供 Header 快照。
func (h *HttpClient) Session() *Session {
	if h.session != nil {
		return h.session
	}
	return &Session{jar: h.defaultJar(), headers: make(map[string]string)}
}
