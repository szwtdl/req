package client

import (
	"testing"
)

func TestAddHeader(t *testing.T) {
	c := NewHttpClient("http://example.com")
	c.AddHeader("X-Custom", "hello")
	headers := c.GetHeader()
	if headers["X-Custom"] != "hello" {
		t.Fatalf("expected X-Custom=hello, got %s", headers["X-Custom"])
	}
}

func TestSetHeader_Overwrite(t *testing.T) {
	c := NewHttpClient("http://example.com")
	c.AddHeader("X-Token", "old")
	c.AddHeader("X-Token", "new")
	if c.GetHeader()["X-Token"] != "new" {
		t.Fatal("AddHeader should overwrite existing key")
	}
}

func TestSetHeader_CanonicalMIME(t *testing.T) {
	c := NewHttpClient("http://example.com")
	// 非标准大小写写入
	c.AddHeader("content-type", "application/json")
	headers := c.GetHeader()
	// 应以 Canonical 格式存储
	if headers["Content-Type"] != "application/json" {
		t.Fatalf("expected Content-Type=application/json, got %v", headers)
	}
}

func TestGetHeader_ReturnsCopy(t *testing.T) {
	c := NewHttpClient("http://example.com")
	c.AddHeader("X-A", "1")
	h1 := c.GetHeader()
	h1["X-A"] = "mutated"
	// 原始 headers 不应被修改
	h2 := c.GetHeader()
	if h2["X-A"] != "1" {
		t.Fatal("GetHeader should return a copy, not a reference")
	}
}

func TestSetHeader_Multiple(t *testing.T) {
	c := NewHttpClient("http://example.com")
	c.SetHeader(map[string]string{
		"X-One": "1",
		"X-Two": "2",
	})
	headers := c.GetHeader()
	if headers["X-One"] != "1" || headers["X-Two"] != "2" {
		t.Fatal("SetHeader batch failed")
	}
}

