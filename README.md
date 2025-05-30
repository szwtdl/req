# req

项目依赖http请求库

```go
import (
client "github.com/szwtdl/req"
"github.com/szwtdl/req/logger"
)
headers := map[string]string{
    "Content-Type": "application/x-www-form-urlencoded",
    "User-Agent":   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3",
}
HttpClient := client.NewHttpClient("https://api.xxxx.cn")
httpClient.SetHeader(headers)
httpClient.SetLogger(logger.GetLogger())
// http代理
err := httpClient.SetProxy(&client.ProxyConfig{
    Type:     "http",
    Address:  "27.185.204.190:1077",
    Username: "syt8179945897",
    Password: "477276",
})
// socks5
err := httpClient.SetProxy(&client.ProxyConfig{
    Type:     "socks5",
    Address:  "nu5d46s1.user.wuyouip.com:8011",
    Username: "NU5d46S1",
    Password: "5613E4",
})

```

### GET请求

```go
response, err := client.DoGet("/login.html")
```

### POST请求

```go
response, err := client.DoPost("/xxx.html", map[string]string{
"data": encryptData,
})
```