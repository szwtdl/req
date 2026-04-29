package client

import (
	"net/http"
	"net/http/cookiejar"
	"net/textproto"
	"net/url"
	"sync"
)

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
// reset=true 时重新创建 CookieJar，彻底清除所有已有 Cookie。
func (s *Session) SetCookies(rawURL string, cookies map[string]string, reset ...bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	if len(reset) > 0 && reset[0] {
		newJar, _ := cookiejar.New(nil)
		s.jar = newJar
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

