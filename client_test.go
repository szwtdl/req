package client

import (
	"go.uber.org/zap"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func setupLogger() *zap.SugaredLogger {
	logger, _ := zap.NewDevelopment()
	return logger.Sugar()
}

// 测试 GET 请求
func TestDoGet(t *testing.T) {
	// 创建模拟 HTTP 服务器
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/test-get" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("GET OK"))
	}))
	defer ts.Close()

	client := NewHttpClient(ts.URL)
	client.SetLogger(setupLogger())

	body, err := client.DoGet("test-get")
	if err != nil {
		t.Fatalf("DoGet failed: %v", err)
	}
	if string(body) != "GET OK" {
		t.Fatalf("Unexpected response: %s", string(body))
	}
}

// 测试 POST 请求
func TestDoPost(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		r.ParseForm()
		if r.FormValue("key") != "value" {
			t.Errorf("Expected key=value, got %s", r.FormValue("key"))
		}
		w.WriteHeader(200)
		w.Write([]byte("POST OK"))
	}))
	defer ts.Close()

	client := NewHttpClient(ts.URL)
	client.SetLogger(setupLogger())

	body, err := client.DoPost("/test-post", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("DoPost failed: %v", err)
	}
	if string(body) != "POST OK" {
		t.Fatalf("Unexpected response: %s", string(body))
	}
}

// 测试 PUT 请求
func TestDoPut(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("Expected PUT, got %s", r.Method)
		}
		r.ParseForm()
		if r.FormValue("key") != "value" {
			t.Errorf("Expected key=value, got %s", r.FormValue("key"))
		}
		w.Write([]byte("PUT OK"))
	}))
	defer ts.Close()

	client := NewHttpClient(ts.URL)
	client.SetLogger(setupLogger())

	body, err := client.DoPut("/test-put", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("DoPut failed: %v", err)
	}
	if string(body) != "PUT OK" {
		t.Fatalf("Unexpected response: %s", string(body))
	}
}

// 测试文件上传
func TestUploadFile(t *testing.T) {
	fileName := "test.txt"
	os.WriteFile(fileName, []byte("hello world"), 0644)
	defer os.Remove(fileName)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(10 << 20)
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile failed: %v", err)
		}
		buf := make([]byte, 11)
		file.Read(buf)
		if string(buf) != "hello world" {
			t.Fatalf("Unexpected file content: %s", string(buf))
		}
		w.Write([]byte("UPLOAD OK"))
	}))
	defer ts.Close()

	client := NewHttpClient(ts.URL)
	client.SetLogger(setupLogger())
	body, err := client.UploadFile("/upload", "file", fileName, map[string]string{"param": "value"})
	if err != nil {
		t.Fatalf("UploadFile failed: %v", err)
	}
	if string(body) != "UPLOAD OK" {
		t.Fatalf("Unexpected response: %s", string(body))
	}
}

// 测试下载文件
func TestDownloadFile(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("download content"))
	}))
	defer ts.Close()

	savePath := "download_test.txt"
	defer os.Remove(savePath)

	client := NewHttpClient(ts.URL)
	client.SetLogger(setupLogger())
	err := client.DownloadFile("/file", savePath)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	content, _ := os.ReadFile(savePath)
	if string(content) != "download content" {
		t.Fatalf("Unexpected file content: %s", string(content))
	}
}

// 测试 Cookie 设置与获取
func TestCookies(t *testing.T) {
	client := NewHttpClient("https://example.com")
	client.SetLogger(setupLogger())

	client.SetCookies(map[string]string{"token": "123"})
	value := client.GetCookieValue("token")
	if value != "123" {
		t.Fatalf("Expected cookie value 123, got %s", value)
	}
}

// 测试 Header 设置
func TestHeaders(t *testing.T) {
	client := NewHttpClient("http://example.com")
	client.SetLogger(setupLogger())

	client.SetHeader(map[string]string{"X-Test": "abc"})
	headers := client.GetHeader()
	if headers["X-Test"] != "abc" {
		t.Fatalf("Expected header X-Test=abc, got %s", headers["X-Test"])
	}
}

// 测试 Timeout
func TestTimeout(t *testing.T) {
	client := NewHttpClient("http://example.com")
	client.SetLogger(setupLogger())

	client.SetTimeout(5 * time.Second)
	if client.GetTimeout() != 5*time.Second {
		t.Fatalf("Expected timeout 5s, got %v", client.GetTimeout())
	}
}

// ----- NewHttpClient 默认值 -----

func TestNewHttpClient_DefaultHeaders(t *testing.T) {
	c := NewHttpClient("http://example.com")
	headers := c.GetHeader()
	if headers["Content-Type"] == "" {
		t.Fatal("default Content-Type should be set")
	}
	if headers["User-Agent"] == "" {
		t.Fatal("default User-Agent should be set")
	}
}

func TestNewHttpClient_DefaultTimeout(t *testing.T) {
	c := NewHttpClient("http://example.com")
	if c.GetTimeout() != 30*time.Second {
		t.Fatalf("default timeout should be 30s, got %v", c.GetTimeout())
	}
}

func TestNewHttpClient_CustomTimeout(t *testing.T) {
	c := NewHttpClient("http://example.com", 15*time.Second)
	if c.GetTimeout() != 15*time.Second {
		t.Fatalf("expected 15s, got %v", c.GetTimeout())
	}
}

// ----- NewHttpClientWithTransport -----

func TestNewHttpClientWithTransport_CustomConfig(t *testing.T) {
	tc := &TransportConfig{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 50,
		MaxConnsPerHost:     50,
		IdleConnTimeout:     10 * time.Second,
	}
	c := NewHttpClientWithTransport("http://example.com", tc)
	if c.transport.MaxIdleConns != 100 {
		t.Fatalf("expected MaxIdleConns=100, got %d", c.transport.MaxIdleConns)
	}
	if c.transport.MaxIdleConnsPerHost != 50 {
		t.Fatalf("expected MaxIdleConnsPerHost=50")
	}
}

func TestNewHttpClientWithTransport_NilConfig(t *testing.T) {
	c := NewHttpClientWithTransport("http://example.com", nil)
	if c == nil {
		t.Fatal("should return non-nil client with nil config")
	}
}

func TestNewHttpClientWithTransport_MaxConcurrency(t *testing.T) {
	tc := &TransportConfig{MaxConcurrency: 3}
	c := NewHttpClientWithTransport("http://example.com", tc)
	if cap(c.semaphore) != 3 {
		t.Fatalf("expected semaphore capacity 3, got %d", cap(c.semaphore))
	}
}

func TestNewHttpClientWithTransport_ZeroConcurrency(t *testing.T) {
	c := NewHttpClientWithTransport("http://example.com", &TransportConfig{MaxConcurrency: 0})
	if c.semaphore != nil {
		t.Fatal("semaphore should be nil when MaxConcurrency=0")
	}
}

// ----- SetDomain / GetDomain -----

func TestSetDomain_AndGetDomain(t *testing.T) {
	c := NewHttpClient("http://old.com")
	c.SetDomain("http://new.com")
	if c.GetDomain() != "http://new.com" {
		t.Fatalf("expected http://new.com, got %s", c.GetDomain())
	}
}

// ----- buildFullURL -----

func TestBuildFullURL_AbsoluteURL(t *testing.T) {
	c := NewHttpClient("http://default.com")
	u := c.buildFullURL("https://other.com/api/v1")
	if u != "https://other.com/api/v1" {
		t.Fatalf("absolute URL should be returned as-is, got %s", u)
	}
}

func TestBuildFullURL_RelativePath(t *testing.T) {
	c := NewHttpClient("http://default.com")
	u := c.buildFullURL("/api/v1")
	if u != "http://default.com/api/v1" {
		t.Fatalf("expected http://default.com/api/v1, got %s", u)
	}
}

func TestBuildFullURL_TrailingSlashDomain(t *testing.T) {
	c := NewHttpClient("http://default.com/")
	u := c.buildFullURL("path")
	if u != "http://default.com/path" {
		t.Fatalf("expected http://default.com/path, got %s", u)
	}
}

func TestBuildFullURL_EmptyPath(t *testing.T) {
	c := NewHttpClient("http://default.com")
	u := c.buildFullURL("")
	if u != "http://default.com/" {
		t.Fatalf("expected http://default.com/, got %s", u)
	}
}

// ----- Close -----

func TestClose_DoesNotPanic(t *testing.T) {
	c := NewHttpClient("http://example.com")
	c.Close() // 应不 panic
}

// ----- 实际请求里的 header 透传 -----

func TestDoGet_HeaderPassthrough(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Trace-Id") != "trace-123" {
			t.Errorf("expected X-Trace-Id=trace-123, got %s", r.Header.Get("X-Trace-Id"))
		}
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	c.AddHeader("X-Trace-Id", "trace-123")
	if _, err := c.DoGet("/"); err != nil {
		t.Fatalf("DoGet failed: %v", err)
	}
}

