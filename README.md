# req

项目依赖http请求库

```go
import (
    client "github.com/szwtdl/req"
    logger2 "github.com/szwtdl/req/logger"
)
headers := map[string]string{
    "Content-Type": "application/x-www-form-urlencoded",
    "User-Agent":   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3",
}
HttpClient := client.NewHttpClient("https://api.xxxx.cn", logger2.GetLogger())


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