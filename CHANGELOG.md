# 更新日志

## [Unreleased] 2026-08-29

### 新增 API

- **`DoGetRawWithHeader(path string) ([]byte, http.Header, error)`** — 发送 GET 请求并额外返回响应头（带详细日志）。
  用于下载类接口读取响应头信息，如 `Content-Disposition` 中的文件名。
  ```go
  body, headers, err := c.DoGetRawWithHeader("/api/download")
  // headers.Get("Content-Disposition") → attachment; filename="report.pdf"
  ```

- **`GetHeaderValue(name string) string`** — 大小写不敏感地读取当前请求头中指定键的值，不存在时返回空字符串。
  ```go
  auth := c.GetHeaderValue("Authorization")
  ```

### 变更

- **内部重构**：`doRequestWith` 抽取为 `doRequestWithHeader`（额外返回响应头），为返回响应头提供统一基础实现，对外行为不变（含重试、限速、gzip 解压、错误映射、非 2xx 返回 body 的既有语义）。
- **JA3 指纹升级**：Chrome 指纹从 `HelloChrome_120` 升级为 `HelloChrome_133`（默认 profile 同步升级），修复部分网关将旧版 Chrome 指纹识别为风控（伪装 404）的问题。

### 已知注意事项

- `DoGetRawWithHeader` 在非 2xx 状态码时仍返回 `(body, headers, nil)`（与库内既有约定一致），调用方需自行判断内容或响应头确认下载是否成功。
- 启用日志（`SetLogger`）时，请求/响应头与 body 会明文记录到日志，注意避免将敏感凭据写入日志环境。