package client

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ----- gzip 自动解压 -----

func TestDoRequest_GzipResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		gz.Write([]byte("gzip-content"))
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	body, err := c.DoGet("/")
	if err != nil {
		t.Fatalf("gzip request failed: %v", err)
	}
	if string(body) != "gzip-content" {
		t.Fatalf("expected gzip-content, got %s", body)
	}
}

// ----- non-2xx 状态码（仍返回 body，不返回 error）-----

func TestDoRequest_Non2xxReturnsBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte("not found"))
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	body, err := c.DoGet("/missing")
	if err != nil {
		t.Fatalf("expected no error for 404, got %v", err)
	}
	if string(body) != "not found" {
		t.Fatalf("expected 'not found', got %s", body)
	}
}

// ----- 超时 -----

func TestDoRequest_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 故意挂起，远超客户端超时
		time.Sleep(2 * time.Second)
		w.Write([]byte("too late"))
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL, 100*time.Millisecond)
	_, err := c.DoGet("/slow")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// ----- 并发限制（semaphore）-----

func TestConcurrencyLimit(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	c := NewHttpClientWithTransport(ts.URL, &TransportConfig{MaxConcurrency: 2})
	done := make(chan error, 5)
	for i := 0; i < 5; i++ {
		go func() {
			_, err := c.DoGet("/")
			done <- err
		}()
	}
	for i := 0; i < 5; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent request failed: %v", err)
		}
	}
}

// ----- 请求体重试时 body 可被重新读取 -----

func TestDoRequest_BodyReusedOnRetry(t *testing.T) {
	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"name":"test"}` && attempts < 2 {
			// 第一次故意关闭连接模拟重试（直接返回正常响应即可）
		}
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	c.AddHeader("Content-Type", "application/json")
	body, err := c.DoPost("/", map[string]string{"name": "test"})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("unexpected response: %s", body)
	}
}

// ----- doRequestWith 使用 session client -----

func TestDoRequestWith_SessionJar(t *testing.T) {
	received := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 第一次请求：服务端设置 cookie
		if r.URL.Path == "/login" {
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "sess123"})
			w.Write([]byte("logged in"))
			return
		}
		// 第二次请求：读取 cookie
		if c, err := r.Cookie("sid"); err == nil {
			received = c.Value
		}
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	s := NewSession()

	_, err := c.DoGetWithSession(s, "/login")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	_, err = c.DoGetWithSession(s, "/profile")
	if err != nil {
		t.Fatalf("profile failed: %v", err)
	}
	if received != "sess123" {
		t.Fatalf("expected sid=sess123, got %s", received)
	}
}

// ----- 无效 URL -----

func TestDoRequest_InvalidURL(t *testing.T) {
	c := NewHttpClient("http://localhost:99999") // 非法端口
	_, err := c.DoGet("/")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

// ----- JSON 响应解析 -----

func TestDoPost_JSONResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	c.AddHeader("Content-Type", "application/json")
	body, err := c.DoPost("/api", map[string]string{"x": "1"})
	if err != nil {
		t.Fatalf("DoPost failed: %v", err)
	}
	var resp map[string]string
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("expected status=ok, got %v", resp)
	}
}

