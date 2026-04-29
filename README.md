# req

Go HTTP 请求库，支持高并发、多域名、Cookie/Session 管理、代理、TLS 指纹伪装（JA3）等特性。

---

## 安装

```bash
go get -u github.com/szwtdl/req
```

---

## 目录

- [快速开始](#快速开始)
- [创建客户端](#创建客户端)
- [请求方法](#请求方法)
  - GET / POST / PUT / DELETE / PATCH / HEAD / OPTIONS
- [Header 管理](#header-管理)
- [Cookie 管理](#cookie-管理)
- [Session 管理（多账号并发）](#session-管理多账号并发)
- [代理配置](#代理配置)
- [TLS 指纹伪装（JA3）](#tls-指纹伪装ja3)
- [高并发 & 连接池配置](#高并发--连接池配置)
- [并发限速（Semaphore）](#并发限速semaphore)
- [超时配置](#超时配置)
- [文件上传 & 下载](#文件上传--下载)
- [日志配置](#日志配置)

---

## 快速开始

```go
package main

import (
    "fmt"
    client "github.com/szwtdl/req"
)

func main() {
    c := client.NewHttpClient("https://httpbin.org")

    // GET 请求
    resp, err := c.DoGet("/get")
    if err != nil {
        panic(err)
    }
    fmt.Println(string(resp))

    // POST 请求
    resp, err = c.DoPost("/post", map[string]string{
        "name": "req",
        "age":  "18",
    })
    if err != nil {
        panic(err)
    }
    fmt.Println(string(resp))
}
```

---

## 创建客户端

### 默认客户端（连接池默认 10000）

```go
c := client.NewHttpClient("https://httpbin.org")
```

### 指定超时

```go
c := client.NewHttpClient("https://httpbin.org", 10*time.Second)
```

### 自定义连接池 & 并发参数

```go
c := client.NewHttpClientWithTransport("https://httpbin.org", &client.TransportConfig{
    MaxIdleConns:        5000,  // 全局最大空闲连接数，默认 10000
    MaxIdleConnsPerHost: 1000,  // 每个 Host 最大空闲连接数，默认 10000
    MaxConnsPerHost:     1000,  // 每个 Host 最大连接数，默认 10000
    IdleConnTimeout:     60 * time.Second,
    MaxConcurrency:      500,   // 最大并发请求数（Semaphore），0 表示不限
}, 30*time.Second)
```

---

## 请求方法

### GET

```go
resp, err := c.DoGet("/api/users")
```

### GET（完整 URL，忽略 domain）

```go
resp, err := c.DoGet("https://other.example.com/api/data")
```

### GET Raw（同 DoGet，附带详细请求日志）

```go
resp, err := c.DoGetRaw("/api/users?page=1")
```

### POST（JSON / Form，根据 Content-Type 自动选择）

```go
// Content-Type: application/json（默认）
c.SetHeader(map[string]string{"Content-Type": "application/json"})
resp, err := c.DoPost("/api/login", map[string]string{
    "username": "admin",
    "password": "123456",
})

// Content-Type: application/x-www-form-urlencoded
c.SetHeader(map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
resp, err = c.DoPost("/api/login", map[string]string{
    "username": "admin",
    "password": "123456",
})
```

### POST（任意结构体，仅支持 JSON）

```go
type LoginReq struct {
    Username string `json:"username"`
    Password string `json:"password"`
}
resp, err := c.DoPostAny("/api/login", LoginReq{Username: "admin", Password: "123456"})
```

### POST Raw（原始字符串 body）

```go
resp, err := c.DoPostRaw("/api/raw", `{"key":"value","num":42}`)
```

### PUT

```go
resp, err := c.DoPut("/api/user/1", map[string]string{
    "name": "new name",
})
```

### PUT Raw（二进制，适合 OSS 上传）

```go
data, _ := os.ReadFile("./photo.jpg")
resp, err := c.DoPutRaw("https://oss.example.com/bucket/photo.jpg", data)
```

### POST Multipart（表单字段，无文件）

```go
resp, err := c.DoPostMultipart("/api/form", map[string]string{
    "field1": "value1",
    "field2": "value2",
})
```

---

### DELETE

```go
// 无 body
resp, err := c.DoDelete("/api/user/1")

// 带 body（根据 Content-Type 自动序列化）
resp, err = c.DoDelete("/api/user/1", map[string]string{
    "reason": "inactive",
})

// 带原始字符串 body
resp, err = c.DoDeleteRaw("/api/user/1", `{"reason":"inactive"}`)
```

### PATCH（局部更新，根据 Content-Type 自动序列化）

```go
// JSON / Form
c.SetHeader(map[string]string{"Content-Type": "application/json"})
resp, err := c.DoPatch("/api/user/1", map[string]string{
    "nickname": "new name",
    "avatar":   "https://cdn.example.com/avatar.jpg",
})

// 任意结构体（仅 JSON）
type PatchReq struct {
    Nickname string `json:"nickname"`
}
resp, err = c.DoPatchAny("/api/user/1", PatchReq{Nickname: "new name"})

// 原始字符串 body
resp, err = c.DoPatchRaw("/api/user/1", `{"nickname":"new name"}`)
```

### HEAD（只获取响应头，不返回 body）

```go
headers, err := c.DoHead("/api/resource")
if err != nil {
    panic(err)
}
fmt.Println("Content-Length:", headers.Get("Content-Length"))
fmt.Println("Content-Type:  ", headers.Get("Content-Type"))
fmt.Println("Last-Modified: ", headers.Get("Last-Modified"))
```

### OPTIONS（获取服务端支持的方法 / CORS 预检）

```go
headers, err := c.DoOptions("/api/user")
if err != nil {
    panic(err)
}
fmt.Println("Allow:                          ", headers.Get("Allow"))
fmt.Println("Access-Control-Allow-Methods:   ", headers.Get("Access-Control-Allow-Methods"))
fmt.Println("Access-Control-Allow-Headers:   ", headers.Get("Access-Control-Allow-Headers"))
```

---

## Header 管理

```go
// 批量设置（覆盖同名）
c.SetHeader(map[string]string{
    "Content-Type":  "application/json",
    "Authorization": "Bearer eyJhbGci...",
    "X-Request-ID":  "abc123",
})

// 单个添加/更新
c.AddHeader("X-Custom-Header", "custom-value")

// 读取当前所有 header（返回副本，线程安全）
headers := c.GetHeader()
fmt.Println(headers)
```

---

## Cookie 管理

### 单域名（使用 client 绑定的 domain）

```go
// 设置 Cookie
c.SetCookies(map[string]string{
    "session_id": "abc123xyz",
    "user_token": "eyJhbGci...",
})

// 重置并重新设置（第二个参数 true 表示先清空）
c.SetCookies(map[string]string{
    "session_id": "new_session",
}, true)

// 读取所有 Cookie
cookies := c.GetCookies()
for _, ck := range cookies {
    fmt.Printf("%s = %s\n", ck.Name, ck.Value)
}

// 读取单个 Cookie 值
token := c.GetCookieValue("user_token")
fmt.Println(token)
```

### 多域名（显式指定 URL）

```go
// 为不同域名分别设置 Cookie
c.SetCookiesFor("https://api.domain-a.com", map[string]string{
    "token": "token-for-a",
})
c.SetCookiesFor("https://api.domain-b.com", map[string]string{
    "token": "token-for-b",
}, true) // true 表示先清空该域名下的所有 Cookie

// 读取指定域名的所有 Cookie
cookiesA := c.GetCookiesFor("https://api.domain-a.com")
for _, ck := range cookiesA {
    fmt.Printf("%s = %s\n", ck.Name, ck.Value)
}

// 读取指定域名的单个 Cookie 值
tokenB := c.GetCookieValueFor("https://api.domain-b.com", "token")
fmt.Println(tokenB)
```

---

## Session 管理（多账号并发）

`Session` 拥有独立的 CookieJar，适合多账号/多用户同时并发请求，各 goroutine 之间 Cookie 完全隔离，底层共享同一个连接池。

```go
package main

import (
    "fmt"
    "sync"
    client "github.com/szwtdl/req"
)

func main() {
    // 共享一个 HttpClient（共享连接池）
    c := client.NewHttpClientWithTransport("https://httpbin.org", &client.TransportConfig{
        MaxConcurrency: 200,
    })
    defer c.Close()

    accounts := []struct{ Username, Password string }{
        {"user1", "pass1"},
        {"user2", "pass2"},
        {"user3", "pass3"},
    }

    var wg sync.WaitGroup
    for _, acc := range accounts {
        wg.Add(1)
        go func(username, password string) {
            defer wg.Done()

            // 每个 goroutine 创建独立 Session，Cookie 互不干扰
            sess := client.NewSession()

            // Session 级别设置 Cookie
            sess.SetCookies("https://httpbin.org", map[string]string{
                "uid": username,
            })

            // Session 级别设置 Header（优先级高于 client 级别）
            sess.SetHeader("Authorization", "Bearer token-"+username)
            sess.SetHeader("X-User-ID", username)

            // 用该 Session 发 GET 请求
            resp, err := c.DoGetWithSession(sess, "/get")
            if err != nil {
                fmt.Printf("[%s] GET error: %v\n", username, err)
                return
            }
            fmt.Printf("[%s] GET: %s\n", username, string(resp))

            // 用该 Session 发 POST 请求
            resp, err = c.DoPostWithSession(sess, "/post", map[string]string{
                "action": "buy",
                "item":   "book",
            })
            if err != nil {
                fmt.Printf("[%s] POST error: %v\n", username, err)
                return
            }
            fmt.Printf("[%s] POST: %s\n", username, string(resp))

            // 读取 Session 中的 Cookie
            uid := sess.GetCookieValue("https://httpbin.org", "uid")
            fmt.Printf("[%s] cookie uid = %s\n", username, uid)

        }(acc.Username, acc.Password)
    }
    wg.Wait()
}
```

---

## 代理配置

### HTTP 代理

```go
err := c.SetProxy(&client.ProxyConfig{
    Type:     "http",
    Address:  "proxy.example.com:8080",
    Username: "proxyuser", // 可选
    Password: "proxypass", // 可选
})
if err != nil {
    panic(err)
}
```

### SOCKS5 代理

```go
err := c.SetProxy(&client.ProxyConfig{
    Type:     "socks5",
    Address:  "socks.example.com:1080",
    Username: "proxyuser", // 可选
    Password: "proxypass", // 可选
})
if err != nil {
    panic(err)
}
```

### 禁用代理

```go
c.SetProxy(nil)
```

---

## TLS 指纹伪装（JA3）

模拟浏览器 TLS 握手指纹，绕过基于 JA3 的检测。

| profile   | 对应浏览器       |
|-----------|-----------------|
| `chrome`  | Chrome 120      |
| `firefox` | Firefox 102     |
| `safari`  | Safari 16.0     |
| `edge`    | Edge 106        |
| `ios`     | iOS 14          |

```go
// 启用 Chrome 指纹
err := c.EnableJA3("chrome")
if err != nil {
    panic(err)
}

resp, err := c.DoGet("/api/data")
fmt.Println(string(resp))

// 切换为 Firefox 指纹
_ = c.EnableJA3("firefox")

// 禁用 JA3，恢复默认 TLS
c.DisableJA3()
// 或传空字符串
_ = c.EnableJA3("")
```

---

## 高并发 & 连接池配置

默认参数已针对万级并发优化，通常直接使用 `NewHttpClient` 即可：

```go
// 默认：MaxIdleConns=10000，MaxIdleConnsPerHost=10000，MaxConnsPerHost=10000
c := client.NewHttpClient("https://api.example.com")
defer c.Close() // 程序退出时释放空闲连接
```

需要精细调整时：

```go
c := client.NewHttpClientWithTransport("https://api.example.com", &client.TransportConfig{
    MaxIdleConns:        10000,
    MaxIdleConnsPerHost: 2000,  // 单域名最大空闲连接
    MaxConnsPerHost:     2000,  // 单域名最大总连接（含活跃）
    IdleConnTimeout:     90 * time.Second,
}, 30*time.Second)
defer c.Close()
```

---

## 并发限速（Semaphore）

控制同一时刻最大并发请求数，防止瞬间打爆目标服务器或耗尽本机文件描述符：

```go
c := client.NewHttpClientWithTransport("https://api.example.com", &client.TransportConfig{
    MaxConcurrency: 500, // 同时最多 500 个并发，超出的 goroutine 自动排队
})

var wg sync.WaitGroup
for i := 0; i < 10000; i++ {
    wg.Add(1)
    go func(i int) {
        defer wg.Done()
        resp, err := c.DoGet(fmt.Sprintf("/api/item/%d", i))
        if err != nil {
            fmt.Println("error:", err)
            return
        }
        _ = resp
    }(i)
}
wg.Wait()
fmt.Println("10000 个请求全部完成")
```

> `MaxConcurrency: 0` 或不设置表示不限速。

---

## 超时配置

```go
// 创建时指定（推荐）
c := client.NewHttpClient("https://httpbin.org", 15*time.Second)

// 动态修改
c.SetTimeout(20 * time.Second)

// 读取当前超时
d := c.GetTimeout()
fmt.Println("current timeout:", d)
```

---

## 文件上传 & 下载

### 上传文件（multipart/form-data）

```go
resp, err := c.UploadFile(
    "/api/upload",           // 接口路径
    "file",                  // 表单 field 名
    "/local/path/photo.jpg", // 本地文件路径
    map[string]string{       // 附加表单字段（可为 nil）
        "desc": "my photo",
        "type": "avatar",
    },
)
if err != nil {
    panic(err)
}
fmt.Println(string(resp))
```

### 下载文件

```go
err := c.DownloadFile("/files/report.pdf", "/local/save/report.pdf")
if err != nil {
    panic(err)
}
fmt.Println("下载完成")
```

---

## 日志配置

使用 `go.uber.org/zap` 结构化日志，自动记录每次请求/响应的方法、URL、Headers、Body、状态码及错误。

```go
package main

import (
    "fmt"
    "go.uber.org/zap"
    client "github.com/szwtdl/req"
)

func main() {
    zapLogger, _ := zap.NewProduction()
    defer zapLogger.Sync()

    c := client.NewHttpClient("https://httpbin.org")
    c.SetLogger(zapLogger.Sugar())

    // 之后所有请求自动打印结构化日志
    resp, err := c.DoGet("/get")
    if err != nil {
        panic(err)
    }
    fmt.Println(string(resp))
}
```

---

## 综合示例

```go
package main

import (
    "fmt"
    "sync"
    "time"
    "go.uber.org/zap"
    client "github.com/szwtdl/req"
)

func main() {
    zapLogger, _ := zap.NewProduction()
    defer zapLogger.Sync()

    // 创建高并发客户端
    c := client.NewHttpClientWithTransport("https://httpbin.org", &client.TransportConfig{
        MaxIdleConns:        10000,
        MaxIdleConnsPerHost: 2000,
        MaxConnsPerHost:     2000,
        IdleConnTimeout:     90 * time.Second,
        MaxConcurrency:      300,
    }, 30*time.Second)
    defer c.Close()

    c.SetLogger(zapLogger.Sugar())
    c.SetHeader(map[string]string{"Content-Type": "application/json"})

    // 启用 Chrome JA3 指纹
    _ = c.EnableJA3("chrome")

    // 配置 SOCKS5 代理
    _ = c.SetProxy(&client.ProxyConfig{
        Type:    "socks5",
        Address: "127.0.0.1:1080",
    })

    // 模拟 1000 个不同账号并发请求，每个账号独立 Session
    var wg sync.WaitGroup
    for i := 0; i < 1000; i++ {
        wg.Add(1)
        go func(uid int) {
            defer wg.Done()

            sess := client.NewSession()
            sess.SetCookies("https://httpbin.org", map[string]string{
                "uid": fmt.Sprintf("user-%d", uid),
            })
            sess.SetHeader("X-User-ID", fmt.Sprintf("%d", uid))

            resp, err := c.DoGetWithSession(sess, "/get")
            if err != nil {
                fmt.Printf("[uid=%d] error: %v\n", uid, err)
                return
            }
            _ = resp
        }(i)
    }
    wg.Wait()
    fmt.Println("全部完成")
}
```
