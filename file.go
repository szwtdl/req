package client

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

// UploadFile 通过 multipart/form-data 上传本地文件。
// fieldName 为文件字段名，filePath 为本地路径，extraParams 为附加表单字段。
func (h *HttpClient) UploadFile(path, fieldName, filePath string, extraParams map[string]string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		h.LogInfo(fmt.Sprintf("failed to open file: %s", filePath))
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	for k, v := range extraParams {
		_ = writer.WriteField(k, v)
	}
	part, err := writer.CreateFormFile(fieldName, filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("创建文件字段失败: %w", err)
	}
	if _, err = io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("写入文件失败: %w", err)
	}
	_ = writer.Close()

	req, err := http.NewRequest("POST", h.buildFullURL(path), &requestBody)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return h.doRequest(req)
}

// DownloadFile 下载远程文件并保存到本地 savePath。
func (h *HttpClient) DownloadFile(path, savePath string) error {
	h.LogInfo("DownloadFile called", "url", path, "savePath", savePath)
	req, err := http.NewRequest("GET", h.buildFullURL(path), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range h.GetHeader() {
		req.Header.Set(k, v)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed, status code: %d", resp.StatusCode)
	}

	out, err := os.Create(savePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	if _, err = io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	h.LogInfo("File downloaded successfully", "path", savePath)
	return nil
}

