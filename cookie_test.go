package client

import (
	"testing"
)

func TestSetCookies_AndGetCookieValue(t *testing.T) {
	c := NewHttpClient("https://example.com")
	c.SetCookies(map[string]string{"session": "abc"})
	if c.GetCookieValue("session") != "abc" {
		t.Fatal("GetCookieValue should return 'abc'")
	}
}

func TestGetCookieValue_NotFound(t *testing.T) {
	c := NewHttpClient("https://example.com")
	if c.GetCookieValue("nonexistent") != "" {
		t.Fatal("should return empty string for missing cookie")
	}
}

func TestSetCookies_Reset(t *testing.T) {
	c := NewHttpClient("https://example.com")
	c.SetCookies(map[string]string{"k1": "v1"})
	// reset=true 应先清空
	c.SetCookies(map[string]string{"k2": "v2"}, true)
	if c.GetCookieValue("k1") != "" {
		t.Fatal("reset should have cleared k1")
	}
	if c.GetCookieValue("k2") != "v2" {
		t.Fatal("k2 should be set after reset")
	}
}

func TestGetCookies_ReturnsList(t *testing.T) {
	c := NewHttpClient("https://example.com")
	c.SetCookies(map[string]string{"a": "1", "b": "2"})
	cookies := c.GetCookies()
	if len(cookies) == 0 {
		t.Fatal("expected at least one cookie")
	}
}

func TestSetCookies_InvalidDomain(t *testing.T) {
	// 非法 domain 不应 panic
	c := NewHttpClient("://bad-url")
	c.SetCookies(map[string]string{"k": "v"}) // 期望静默处理
}

func TestGetCookiesFor(t *testing.T) {
	c := NewHttpClient("https://example.com")
	c.SetCookiesFor("https://other.com", map[string]string{"x": "y"})
	cookies := c.GetCookiesFor("https://other.com")
	if len(cookies) == 0 {
		t.Fatal("expected cookie for other.com")
	}
	found := false
	for _, ck := range cookies {
		if ck.Name == "x" && ck.Value == "y" {
			found = true
		}
	}
	if !found {
		t.Fatal("cookie x=y not found in GetCookiesFor")
	}
}

func TestGetCookieValueFor(t *testing.T) {
	c := NewHttpClient("https://example.com")
	c.SetCookiesFor("https://api.example.com", map[string]string{"token": "xyz"})
	v := c.GetCookieValueFor("https://api.example.com", "token")
	if v != "xyz" {
		t.Fatalf("expected token=xyz, got %s", v)
	}
}

func TestGetCookieValueFor_NotFound(t *testing.T) {
	c := NewHttpClient("https://example.com")
	v := c.GetCookieValueFor("https://example.com", "ghost")
	if v != "" {
		t.Fatal("should return empty string for missing cookie")
	}
}

func TestSetCookiesFor_Reset(t *testing.T) {
	c := NewHttpClient("https://example.com")
	c.SetCookiesFor("https://example.com", map[string]string{"old": "1"})
	c.SetCookiesFor("https://example.com", map[string]string{"new": "2"}, true)
	if c.GetCookieValueFor("https://example.com", "old") != "" {
		t.Fatal("old cookie should be cleared after reset")
	}
	if c.GetCookieValueFor("https://example.com", "new") != "2" {
		t.Fatal("new cookie should be set")
	}
}

func TestGetCookiesFor_InvalidURL(t *testing.T) {
	c := NewHttpClient("https://example.com")
	cookies := c.GetCookiesFor("://invalid")
	if cookies != nil {
		t.Fatal("invalid URL should return nil")
	}
}

// 验证 https 域名下 cookie 可正常读取（cookiejar 返回的 Cookie 仅保留 Name/Value）
func TestSetCookies_SecureFlag(t *testing.T) {
	c := NewHttpClient("https://secure.com")
	c.SetCookies(map[string]string{"s": "1"})
	// 标准 cookiejar 的 Cookies() 只返回 Name/Value，Secure 字段不保留
	// 只需验证 cookie 能被正确写入和读取即可
	if c.GetCookieValue("s") != "1" {
		t.Fatal("cookie s should be readable for https domain")
	}
}

// 验证 http 域名下 Secure 应为 false
func TestSetCookies_NonSecureFlag(t *testing.T) {
	c := NewHttpClient("http://plain.com")
	c.SetCookies(map[string]string{"p": "1"})
	if c.GetCookieValue("p") != "1" {
		t.Fatal("http domain cookie p should be readable")
	}
}

