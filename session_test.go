package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewSession(t *testing.T) {
	s := NewSession()
	if s == nil {
		t.Fatal("NewSession returned nil")
	}
	if s.jar == nil {
		t.Fatal("session jar should not be nil")
	}
}

func TestSession_SetAndGetCookies(t *testing.T) {
	s := NewSession()
	s.SetCookies("https://example.com", map[string]string{"uid": "42"})
	cookies := s.GetCookies("https://example.com")
	if len(cookies) == 0 {
		t.Fatal("expected at least one cookie")
	}
	found := false
	for _, c := range cookies {
		if c.Name == "uid" && c.Value == "42" {
			found = true
		}
	}
	if !found {
		t.Fatal("cookie uid=42 not found")
	}
}

func TestSession_GetCookieValue(t *testing.T) {
	s := NewSession()
	s.SetCookies("https://example.com", map[string]string{"token": "secret"})
	v := s.GetCookieValue("https://example.com", "token")
	if v != "secret" {
		t.Fatalf("expected secret, got %s", v)
	}
}

func TestSession_GetCookieValue_NotFound(t *testing.T) {
	s := NewSession()
	v := s.GetCookieValue("https://example.com", "ghost")
	if v != "" {
		t.Fatal("should return empty string")
	}
}

func TestSession_SetCookies_Reset(t *testing.T) {
	s := NewSession()
	s.SetCookies("https://example.com", map[string]string{"a": "1"})
	s.SetCookies("https://example.com", map[string]string{"b": "2"}, true)
	if s.GetCookieValue("https://example.com", "a") != "" {
		t.Fatal("cookie a should be cleared after reset")
	}
	if s.GetCookieValue("https://example.com", "b") != "2" {
		t.Fatal("cookie b should be set")
	}
}

func TestSession_SetCookies_InvalidURL(t *testing.T) {
	s := NewSession()
	// 不应 panic
	s.SetCookies("://bad", map[string]string{"k": "v"})
}

func TestSession_GetCookies_InvalidURL(t *testing.T) {
	s := NewSession()
	if s.GetCookies("://bad") != nil {
		t.Fatal("invalid URL should return nil")
	}
}

func TestSession_SetHeader(t *testing.T) {
	s := NewSession()
	s.SetHeader("X-Session-ID", "sess-001")
	h := s.getHeaders()
	if h["X-Session-Id"] != "sess-001" && h["X-Session-ID"] != "sess-001" {
		// canonical key 是 X-Session-Id
		t.Fatalf("session header not found: %v", h)
	}
}

func TestSession_getHeaders_ReturnsCopy(t *testing.T) {
	s := NewSession()
	s.SetHeader("X-A", "original")
	h := s.getHeaders()
	h["X-A"] = "mutated"
	h2 := s.getHeaders()
	if h2["X-A"] == "mutated" {
		t.Fatal("getHeaders should return a copy")
	}
}

func TestDoGetWithSession(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证 session 级别的自定义 header 被发送
		if r.Header.Get("X-Session") != "test-session" {
			t.Errorf("expected X-Session header, got %s", r.Header.Get("X-Session"))
		}
		w.Write([]byte("session-get-ok"))
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	s := NewSession()
	s.SetHeader("X-Session", "test-session")

	body, err := c.DoGetWithSession(s, "/path")
	if err != nil {
		t.Fatalf("DoGetWithSession failed: %v", err)
	}
	if string(body) != "session-get-ok" {
		t.Fatalf("unexpected response: %s", body)
	}
}

func TestDoPostWithSession(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("X-User") != "user1" {
			t.Errorf("expected X-User header, got %s", r.Header.Get("X-User"))
		}
		w.Write([]byte("session-post-ok"))
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	c.AddHeader("Content-Type", "application/x-www-form-urlencoded")
	s := NewSession()
	s.SetHeader("X-User", "user1")

	body, err := c.DoPostWithSession(s, "/path", map[string]string{"field": "value"})
	if err != nil {
		t.Fatalf("DoPostWithSession failed: %v", err)
	}
	if string(body) != "session-post-ok" {
		t.Fatalf("unexpected response: %s", body)
	}
}

// 验证两个 Session 的 CookieJar 互相隔离
func TestSession_CookieIsolation(t *testing.T) {
	s1 := NewSession()
	s2 := NewSession()
	s1.SetCookies("https://example.com", map[string]string{"user": "alice"})
	s2.SetCookies("https://example.com", map[string]string{"user": "bob"})

	if s1.GetCookieValue("https://example.com", "user") != "alice" {
		t.Fatal("s1 should have user=alice")
	}
	if s2.GetCookieValue("https://example.com", "user") != "bob" {
		t.Fatal("s2 should have user=bob")
	}
}

