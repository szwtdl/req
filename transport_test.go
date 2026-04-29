package client

import (
	"testing"
	"time"
)

// ----- SetTimeout / GetTimeout -----

func TestSetTimeout_AndGetTimeout(t *testing.T) {
	c := NewHttpClient("http://example.com")
	c.SetTimeout(10 * time.Second)
	if c.GetTimeout() != 10*time.Second {
		t.Fatalf("expected 10s, got %v", c.GetTimeout())
	}
}

func TestSetTimeout_UpdatesIdleConnTimeout(t *testing.T) {
	c := NewHttpClient("http://example.com")
	c.SetTimeout(7 * time.Second)
	if c.transport.IdleConnTimeout != 7*time.Second {
		t.Fatalf("transport.IdleConnTimeout should be 7s, got %v", c.transport.IdleConnTimeout)
	}
}

// ----- SetProxy -----

func TestSetProxy_Nil_ClearsProxy(t *testing.T) {
	c := NewHttpClient("http://example.com")
	if err := c.SetProxy(nil); err != nil {
		t.Fatalf("SetProxy(nil) failed: %v", err)
	}
	if c.transport.Proxy != nil {
		t.Fatal("transport.Proxy should be nil after SetProxy(nil)")
	}
}

func TestSetProxy_HTTP(t *testing.T) {
	c := NewHttpClient("http://example.com")
	err := c.SetProxy(&ProxyConfig{
		Type:    "http",
		Address: "127.0.0.1:8080",
	})
	if err != nil {
		t.Fatalf("SetProxy http failed: %v", err)
	}
	if c.transport.Proxy == nil {
		t.Fatal("transport.Proxy should not be nil after setting http proxy")
	}
}

func TestSetProxy_HTTP_WithAuth(t *testing.T) {
	c := NewHttpClient("http://example.com")
	err := c.SetProxy(&ProxyConfig{
		Type:     "http",
		Address:  "127.0.0.1:8080",
		Username: "user",
		Password: "pass",
	})
	if err != nil {
		t.Fatalf("SetProxy with auth failed: %v", err)
	}
}

func TestSetProxy_UnsupportedType(t *testing.T) {
	c := NewHttpClient("http://example.com")
	err := c.SetProxy(&ProxyConfig{Type: "ftp", Address: "1.2.3.4:21"})
	if err == nil {
		t.Fatal("expected error for unsupported proxy type")
	}
}

func TestSetProxy_SOCKS5_InvalidAddress(t *testing.T) {
	c := NewHttpClient("http://example.com")
	// 使用不存在的地址，proxy.SOCKS5 只做参数校验，不拨号，应成功
	err := c.SetProxy(&ProxyConfig{
		Type:    "socks5",
		Address: "127.0.0.1:1080",
	})
	if err != nil {
		t.Fatalf("SetProxy socks5 should not fail at setup: %v", err)
	}
}

// ----- EnableJA3 / DisableJA3 -----

func TestEnableJA3_EmptyProfile_EqualToDisable(t *testing.T) {
	c := NewHttpClient("https://example.com")
	if err := c.EnableJA3(""); err != nil {
		t.Fatalf("EnableJA3('') failed: %v", err)
	}
	if c.transport.DialTLSContext != nil {
		t.Fatal("DialTLSContext should be nil for empty profile")
	}
}

func TestEnableJA3_Chrome_SetsDialTLS(t *testing.T) {
	c := NewHttpClient("https://example.com")
	if err := c.EnableJA3("chrome"); err != nil {
		t.Fatalf("EnableJA3(chrome) failed: %v", err)
	}
	if c.transport.DialTLSContext == nil {
		t.Fatal("DialTLSContext should be set after EnableJA3")
	}
}

func TestDisableJA3(t *testing.T) {
	c := NewHttpClient("https://example.com")
	_ = c.EnableJA3("firefox")
	c.DisableJA3()
	if c.transport.DialTLSContext != nil {
		t.Fatal("DialTLSContext should be nil after DisableJA3")
	}
}

// ----- getClientHelloID -----

func TestGetClientHelloID_AllProfiles(t *testing.T) {
	profiles := []string{"chrome", "firefox", "safari", "edge", "ios", "unknown"}
	for _, p := range profiles {
		id := getClientHelloID(p)
		if id.Client == "" {
			t.Fatalf("getClientHelloID(%s) returned empty", p)
		}
	}
}

