package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ----- encodeBody -----

func TestEncodeBody_JSON(t *testing.T) {
	headers := map[string]string{"Content-Type": "application/json"}
	b, err := encodeBody(headers, map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("encodeBody JSON failed: %v", err)
	}
	var out map[string]string
	if err := json.Unmarshal(b, &out); err != nil || out["k"] != "v" {
		t.Fatalf("bad JSON output: %s", string(b))
	}
}

func TestEncodeBody_Form(t *testing.T) {
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	b, err := encodeBody(headers, map[string]string{"foo": "bar"})
	if err != nil {
		t.Fatalf("encodeBody form failed: %v", err)
	}
	if !strings.Contains(string(b), "foo=bar") {
		t.Fatalf("expected foo=bar in form body, got: %s", string(b))
	}
}

func TestEncodeBody_UnsupportedType(t *testing.T) {
	headers := map[string]string{"Content-Type": "text/xml"}
	_, err := encodeBody(headers, map[string]string{"k": "v"})
	if err == nil {
		t.Fatal("expected error for unsupported content type")
	}
}

func TestEncodeBody_DefaultsToJSON(t *testing.T) {
	// 无 Content-Type header 时默认 JSON
	b, err := encodeBody(map[string]string{}, map[string]string{"a": "b"})
	if err != nil {
		t.Fatalf("encodeBody default failed: %v", err)
	}
	var out map[string]string
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("expected JSON output, got: %s", b)
	}
}

// ----- DoGetRaw -----

func TestDoGetRaw(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("raw-get"))
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	body, err := c.DoGetRaw("/raw")
	if err != nil {
		t.Fatalf("DoGetRaw failed: %v", err)
	}
	if string(body) != "raw-get" {
		t.Fatalf("unexpected: %s", body)
	}
}

// ----- DoPostAny -----

func TestDoPostAny_JSON(t *testing.T) {
	type Payload struct {
		Name string `json:"name"`
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p Payload
		json.NewDecoder(r.Body).Decode(&p)
		if p.Name != "alice" {
			t.Errorf("expected name=alice, got %s", p.Name)
		}
		w.Write([]byte("any-ok"))
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	c.AddHeader("Content-Type", "application/json")
	body, err := c.DoPostAny("/any", Payload{Name: "alice"})
	if err != nil {
		t.Fatalf("DoPostAny failed: %v", err)
	}
	if string(body) != "any-ok" {
		t.Fatalf("unexpected: %s", body)
	}
}

func TestDoPostAny_NonJSONFails(t *testing.T) {
	c := NewHttpClient("http://example.com")
	c.AddHeader("Content-Type", "application/x-www-form-urlencoded")
	_, err := c.DoPostAny("/any", map[string]string{"k": "v"})
	if err == nil {
		t.Fatal("expected error for non-JSON content type")
	}
}

// ----- DoPostRaw -----

func TestDoPostRaw(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != "raw-body" {
			t.Errorf("expected raw-body, got %s", body)
		}
		w.Write([]byte("post-raw-ok"))
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	body, err := c.DoPostRaw("/raw", "raw-body")
	if err != nil {
		t.Fatalf("DoPostRaw failed: %v", err)
	}
	if string(body) != "post-raw-ok" {
		t.Fatalf("unexpected: %s", body)
	}
}

// ----- DoPostMultipart -----

func TestDoPostMultipart(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(1 << 20)
		if r.FormValue("field1") != "hello" {
			t.Errorf("expected field1=hello, got %s", r.FormValue("field1"))
		}
		w.Write([]byte("multipart-ok"))
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	body, err := c.DoPostMultipart("/mp", map[string]string{"field1": "hello"})
	if err != nil {
		t.Fatalf("DoPostMultipart failed: %v", err)
	}
	if string(body) != "multipart-ok" {
		t.Fatalf("unexpected: %s", body)
	}
}

// ----- DoPutRaw -----

func TestDoPutRaw(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "raw-put" {
			t.Errorf("expected raw-put, got %s", body)
		}
		w.Write([]byte("put-raw-ok"))
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	body, err := c.DoPutRaw("/", []byte("raw-put"))
	if err != nil {
		t.Fatalf("DoPutRaw failed: %v", err)
	}
	if string(body) != "put-raw-ok" {
		t.Fatalf("unexpected: %s", body)
	}
}

// ----- DoDelete -----

func TestDoDelete_NoBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.Write([]byte("del-ok"))
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	body, err := c.DoDelete("/res")
	if err != nil {
		t.Fatalf("DoDelete failed: %v", err)
	}
	if string(body) != "del-ok" {
		t.Fatalf("unexpected: %s", body)
	}
}

func TestDoDelete_WithBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if len(b) == 0 {
			t.Error("expected non-empty body")
		}
		w.Write([]byte("del-body-ok"))
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	c.AddHeader("Content-Type", "application/x-www-form-urlencoded")
	body, err := c.DoDelete("/res", map[string]string{"id": "1"})
	if err != nil {
		t.Fatalf("DoDelete with body failed: %v", err)
	}
	if string(body) != "del-body-ok" {
		t.Fatalf("unexpected: %s", body)
	}
}

// ----- DoDeleteRaw -----

func TestDoDeleteRaw(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		b, _ := io.ReadAll(r.Body)
		if string(b) != "raw-del" {
			t.Errorf("expected raw-del body, got %s", b)
		}
		w.Write([]byte("del-raw-ok"))
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	body, err := c.DoDeleteRaw("/res", "raw-del")
	if err != nil {
		t.Fatalf("DoDeleteRaw failed: %v", err)
	}
	if string(body) != "del-raw-ok" {
		t.Fatalf("unexpected: %s", body)
	}
}

// ----- DoPatch -----

func TestDoPatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		r.ParseForm()
		if r.FormValue("status") != "active" {
			t.Errorf("expected status=active, got %s", r.FormValue("status"))
		}
		w.Write([]byte("patch-ok"))
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	c.AddHeader("Content-Type", "application/x-www-form-urlencoded")
	body, err := c.DoPatch("/res", map[string]string{"status": "active"})
	if err != nil {
		t.Fatalf("DoPatch failed: %v", err)
	}
	if string(body) != "patch-ok" {
		t.Fatalf("unexpected: %s", body)
	}
}

// ----- DoPatchAny -----

func TestDoPatchAny(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]interface{}
		json.NewDecoder(r.Body).Decode(&m)
		if m["score"] != float64(99) {
			t.Errorf("expected score=99, got %v", m["score"])
		}
		w.Write([]byte("patch-any-ok"))
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	c.AddHeader("Content-Type", "application/json")
	body, err := c.DoPatchAny("/res", map[string]int{"score": 99})
	if err != nil {
		t.Fatalf("DoPatchAny failed: %v", err)
	}
	if string(body) != "patch-any-ok" {
		t.Fatalf("unexpected: %s", body)
	}
}

// ----- DoPatchRaw -----

func TestDoPatchRaw(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if string(b) != "raw-patch" {
			t.Errorf("expected raw-patch, got %s", b)
		}
		w.Write([]byte("patch-raw-ok"))
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	body, err := c.DoPatchRaw("/res", "raw-patch")
	if err != nil {
		t.Fatalf("DoPatchRaw failed: %v", err)
	}
	if string(body) != "patch-raw-ok" {
		t.Fatalf("unexpected: %s", body)
	}
}

// ----- DoHead -----

func TestDoHead(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("expected HEAD, got %s", r.Method)
		}
		w.Header().Set("X-Custom-Meta", "meta-value")
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	headers, err := c.DoHead("/")
	if err != nil {
		t.Fatalf("DoHead failed: %v", err)
	}
	if headers.Get("X-Custom-Meta") != "meta-value" {
		t.Fatalf("expected X-Custom-Meta=meta-value, got %v", headers)
	}
}

// ----- DoOptions -----

func TestDoOptions(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodOptions {
			t.Errorf("expected OPTIONS, got %s", r.Method)
		}
		w.Header().Set("Allow", "GET, POST, OPTIONS")
	}))
	defer ts.Close()

	c := NewHttpClient(ts.URL)
	headers, err := c.DoOptions("/")
	if err != nil {
		t.Fatalf("DoOptions failed: %v", err)
	}
	if !strings.Contains(headers.Get("Allow"), "GET") {
		t.Fatalf("expected Allow header with GET, got %v", headers)
	}
}

