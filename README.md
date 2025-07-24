# req

项目依赖http请求库


# 安装
```bash
go get -u github.com/szwtdl/req
```

# 使用
```go
package main
import (
    "fmt"
	client "github.com/szwtdl/req"
)
func main() {
    // create a new HttpClient
	HttpClient := client.NewHttpClient("https://examples.com")
    // POST请求
    resp, err = HttpClient.DoPost("post", map[string]string{
        "name": "req",
        "age":  "18",
    })
    if err != nil {
        panic(err)
    }
	fmt.Println(string(resp))
	
	// GET请求
	resp, err = HttpClient.DoGet("post")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(resp))
	
}
```

### 代理配置
```go
package main
import (
    "fmt"
    client "github.com/szwtdl/req"
)
func main() {
	// create a new HttpClient with proxy
	HttpClient := client.NewHttpClient("https://examples.com")
	// 设置日记
	HttpClient.SetLogger(logger.GetLogger())
	// 设置header
	HttpClient.SetHeader(map[string]string{
		"Content-Type": "application/json",
	})
	// 设置cookies
	HttpClient.SetCookies(map[string]string{
		"JSESSIONID": strings.ToUpper(fmt.Sprintf("%v", md5.Sum([]byte("12345678")))),
	})
	// 配置 HTTP 代理
	err := HttpClient.SetProxy(&ProxyConfig{
		Type:    "http",
		Address: "proxy.example.com:8080",
		Username: "proxyuser",  // 可选
		Password: "proxypass",  // 可选
	})
	// 配置 SOCKS5 代理
	err := HttpClient.SetProxy(&ProxyConfig{
		Type:    "socks5",
		Address: "socks.example.com:1080",
		Username: "proxyuser",  // 可选
		Password: "proxypass",  // 可选
	})
	// 禁用代理
	HttpClient.SetProxy(nil)
}
```

