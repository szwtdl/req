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
