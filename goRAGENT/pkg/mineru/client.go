package mineru

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// Client MinerU 文档解析客户端
// 和 CarAgent app/services/mineru_service.py 逻辑一致
// v1 API: pdf/docx/pptx/png/jpg → 单文件快速解析
// v4 API: .doc + 批量 → 需要 Token
type Client struct {
	apiToken   string
	httpClient *http.Client
}

func NewClient(apiToken string) *Client {
	return &Client{
		apiToken:   apiToken,
		httpClient: &http.Client{Timeout: 600 * time.Second},
	}
}

const apiBase = "https://mineru.net"

// Parse 单文件解析 (v1)
func (c *Client) Parse(ctx context.Context, filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil { return "", err }
	defer file.Close()

	fname := filepath.Base(filePath)
	ext := filepath.Ext(filePath)

	// 文本文件直接读
	switch ext {
	case ".md", ".txt", ".markdown":
		data, _ := os.ReadFile(filePath)
		return string(data), nil
	}

	// v1 Agent API
	t0 := time.Now()

	// Step 1: 提交任务
	body, _ := json.Marshal(map[string]any{
		"file_name": fname, "is_ocr": false,
		"enable_formula": true, "enable_table": true, "language": "ch",
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", apiBase+"/api/v1/agent/parse/file",
		io.NopCloser(io.LimitReader(nil, 0)))
	req.Header.Set("Content-Type", "application/json")
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}
	// rebuild request with body
	req.Body = io.NopCloser(newBytesReader(body))
	req.ContentLength = int64(len(body))

	resp, err := c.httpClient.Do(req)
	if err != nil { return "", fmt.Errorf("v1 提交失败: %w", err) }
	defer resp.Body.Close()

	var d struct {
		Data struct {
			TaskID  string `json:"task_id"`
			FileURL string `json:"file_url"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&d)
	taskID := d.Data.TaskID
	fileURL := d.Data.FileURL

	// Step 2: 上传文件
	data, _ := os.ReadFile(filePath)
	putReq, _ := http.NewRequestWithContext(ctx, "PUT", fileURL, io.NopCloser(newBytesReader(data)))
	putReq.ContentLength = int64(len(data))
	http.DefaultClient.Do(putReq)

	// Step 3: 轮询
	for range 600 / 5 {
		select {
		case <-ctx.Done(): return "", ctx.Err()
		case <-time.After(5 * time.Second):
		}

		req, _ := http.NewRequestWithContext(ctx, "GET", apiBase+"/api/v1/agent/parse/"+taskID, nil)
		if c.apiToken != "" { req.Header.Set("Authorization", "Bearer "+c.apiToken) }
		resp, err := c.httpClient.Do(req)
		if err != nil { continue }
		var td struct {
			Data struct {
				State      string `json:"state"`
				MarkdownURL string `json:"markdown_url"`
			} `json:"data"`
		}
		json.NewDecoder(resp.Body).Decode(&td)
		resp.Body.Close()

		if td.Data.State == "done" {
			mdResp, _ := http.Get(td.Data.MarkdownURL)
			content, _ := io.ReadAll(mdResp.Body)
			mdResp.Body.Close()
			zap.L().Info("MinerU v1 完成",
				zap.String("file", fname),
				zap.Int("chars", len(content)),
				zap.Duration("latency", time.Since(t0)),
			)
			return string(content), nil
		}
		if td.Data.State == "failed" {
			return "", fmt.Errorf("MinerU 解析失败: %s", taskID)
		}
	}
	return "", fmt.Errorf("MinerU 超时: %s", taskID)
}

func newBytesReader(b []byte) *bytesReader { return &bytesReader{buf: b, idx: 0} }
type bytesReader struct{ buf []byte; idx int }
func (r *bytesReader) Read(p []byte) (int, error) {
	if r.idx >= len(r.buf) { return 0, io.EOF }
	n := copy(p, r.buf[r.idx:])
	r.idx += n
	return n, nil
}
func (r *bytesReader) Close() error { return nil }
