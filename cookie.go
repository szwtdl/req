package client

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
)

// GetCookies 返回默认域名下的所有 Cookie。
func (h *HttpClient) GetCookies() []*http.Cookie {
	u, err := url.Parse(h.domain)
	if err != nil {
		return nil
	}
	return h.jar.Cookies(u)
}

// GetCookieValue 返回默认域名下指定名称 Cookie 的值。
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

// SetCookies 设置默认域名下的 Cookie；reset=true 时重新创建 CookieJar，彻底清除已有 Cookie。
func (h *HttpClient) SetCookies(cookies map[string]string, opts ...bool) {
	u, err := url.Parse(h.domain)
	if err != nil {
		h.LogError("SetCookies failed", err)
		return
	}
	if len(opts) > 0 && opts[0] {
		newJar, _ := cookiejar.New(nil)
		h.jar = newJar
		h.client.Jar = newJar
	}
	secure := u.Scheme == "https"
	var list []*http.Cookie
	for k, v := range cookies {
		list = append(list, &http.Cookie{
			Name:   k,
			Value:  v,
			Path:   "/",
			Domain: u.Hostname(),
			Secure: secure,
		})
	}
	h.jar.SetCookies(u, list)
}

// GetCookiesFor 获取指定 URL 域名下的所有 Cookie（多域名场景）。
func (h *HttpClient) GetCookiesFor(rawURL string) []*http.Cookie {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	return h.jar.Cookies(u)
}

// GetCookieValueFor 获取指定 URL 域名下某个 Cookie 的值（多域名场景）。
func (h *HttpClient) GetCookieValueFor(rawURL, name string) string {
	for _, c := range h.GetCookiesFor(rawURL) {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

// SetCookiesFor 设置指定 URL 域名下的 Cookie（多域名场景）；reset=true 时重新创建 CookieJar。
func (h *HttpClient) SetCookiesFor(rawURL string, cookies map[string]string, opts ...bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		h.LogError("SetCookiesFor failed", err)
		return
	}
	if len(opts) > 0 && opts[0] {
		newJar, _ := cookiejar.New(nil)
		h.jar = newJar
		h.client.Jar = newJar
	}
	secure := u.Scheme == "https"
	var list []*http.Cookie
	for k, v := range cookies {
		list = append(list, &http.Cookie{
			Name: k, Value: v, Path: "/", Domain: u.Hostname(), Secure: secure,
		})
	}
	h.jar.SetCookies(u, list)
}

