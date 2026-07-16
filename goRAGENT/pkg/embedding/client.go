package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// Service 向量化服务
// 调用本地 BGE-M3 HTTP 服务（和 CarAgent 共用同一套 embedding server）
type Service struct {
	baseURL    string
	httpClient *http.Client
}

func NewService(baseURL string) *Service {
	return &Service{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

type encodeResp struct {
	Dense []float32        `json:"dense"`
	Sparse map[string]float64 `json:"sparse"`
	Dim   int              `json:"dim"`
}

// Embed 单条文本向量化 → POST /encode
func (s *Service) Embed(ctx context.Context, text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]string{"text": text})
	resp, err := s.httpClient.Post(s.baseURL+"/encode", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedding 请求失败: %w", err)
	}
	defer resp.Body.Close()

	var r encodeResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("embedding 解析失败: %w", err)
	}
	zap.L().Debug("embedding done", zap.Int("dim", len(r.Dense)))
	return r.Dense, nil
}

// EmbedBatch 批量向量化 → POST /encode_batch
func (s *Service) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 { return nil, nil }
	body, _ := json.Marshal(map[string][]string{"texts": texts})
	resp, err := s.httpClient.Post(s.baseURL+"/encode_batch", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedding batch 请求失败: %w", err)
	}
	defer resp.Body.Close()

	var r struct {
		Dense [][]float32 `json:"dense"`
		Dim   int         `json:"dim"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("embedding batch 解析失败: %w", err)
	}
	return r.Dense, nil
}
