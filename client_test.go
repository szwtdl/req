package client

import "testing"

func TestHttpClient_DoGet(t *testing.T) {
	h := NewHttpClient("http://localhost:5678")
	h.SetHeader(map[string]string{
		"Content-Type": "application/json",
		"User-Agent":   "Mozilla/5.0 (Linux; U; Android 12.1.1; zh-cn; OPPO R9sk Build/NMF26F) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/70.0.3538.80 Mobile Safari/537.36 OppoBrowser/10.5.1.2",
	})
	h.SetDomain("http://localhost:5678")
	err := h.SetProxy(&ProxyConfig{
		Type:     "http",
		Address:  "localhost:5678",
		Username: "admin",
		Password: "admin",
	})
	if err != nil {
		return
	}
	h.SetHeader(map[string]string{
		"Content-Type": "application/json",
	})
	_, err = h.DoGet("api/admin/info")
	if err != nil {
		t.Errorf("failed to get course: %v", err)
	}
}

func TestHttpClient_DoPost(t *testing.T) {
	h := NewHttpClient("http://localhost:5678")
	h.SetDomain("http://localhost:5678")
	h.SetHeader(map[string]string{
		"Content-Type": "application/json",
		"User-Agent":   "Mozilla/5.0 (Linux; U; Android 12.1.1; zh-cn; OPPO R9sk Build/NMF26F) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/70.0.3538.80 Mobile Safari/537.36 OppoBrowser/10.5.1.2",
	})
	_, err := h.DoPost("api/admin/login", map[string]string{
		"id":   "1",
		"name": "test",
	})
	if err != nil {
		t.Errorf("failed to post course: %v", err)
	}
}
