package rerank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"go.uber.org/zap"
)

// Service Rerank 重排序服务
// 调用本地 BGE-M3 compute_score 做交叉编码重排
// 和 CarAgent app/rag/reranker.py 逻辑完全一致
type Service struct {
	baseURL    string
	topK       int
	httpClient *http.Client
}

func NewService(baseURL string, topK int) *Service {
	return &Service{
		baseURL: baseURL,
		topK:    topK,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// Rerank 重排序：query + documents → 按 colbert 交叉编码得分降序返回
func (s *Service) Rerank(ctx context.Context, query string, documents []string) ([]string, []float64, error) {
	if len(documents) == 0 {
		return nil, nil, nil
	}

	// POST /compute_score  (BGE-M3 cross-encoder)
	body, _ := json.Marshal(map[string]any{
		"query":      query,
		"documents":  documents,
	})

	resp, err := s.httpClient.Post(s.baseURL+"/compute_score", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("rerank 请求失败: %w", err)
	}
	defer resp.Body.Close()

	var r struct {
		Scores []float64 `json:"scores"`
		Error  string    `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, nil, fmt.Errorf("rerank 解析失败: %w", err)
	}
	if r.Error != "" {
		return nil, nil, fmt.Errorf("rerank error: %s", r.Error)
	}

	// 按分数降序排列 (和 CarAgent 一致)
	type pair struct {
		idx   int
		score float64
	}
	var pairs []pair
	for i, s := range r.Scores {
		pairs = append(pairs, pair{idx: i, score: s})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].score > pairs[j].score })

	limit := s.topK
	if limit <= 0 || limit > len(pairs) { limit = len(pairs) }

	var rankedDocs []string
	var rankedScores []float64
	for i := 0; i < limit; i++ {
		rankedDocs = append(rankedDocs, documents[pairs[i].idx])
		rankedScores = append(rankedScores, pairs[i].score)
	}

	zap.L().Debug("rerank done", zap.Int("input", len(documents)), zap.Int("output", len(rankedDocs)))
	return rankedDocs, rankedScores, nil
}
