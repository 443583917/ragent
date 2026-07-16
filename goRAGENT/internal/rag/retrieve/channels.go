package retrieve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"goRAGENT/internal/config"

	"go.uber.org/zap"
)

// ========== IntentDirectedSearchChannel 意图定向检索 ==========

// IntentDirectedSearchChannel 意图定向检索通道（priority=1, 最高）
type IntentDirectedSearchChannel struct {
	cfg       config.IntentDirectedConfig
	retriever VectorRetriever
}

// VectorRetriever 向量检索器接口
type VectorRetriever interface {
	Search(ctx context.Context, collection string, query string, topK int) ([]RetrievedChunk, error)
	ListCollections(ctx context.Context) ([]string, error)
}

func NewIntentDirectedChannel(cfg config.IntentDirectedConfig, retriever VectorRetriever) *IntentDirectedSearchChannel {
	return &IntentDirectedSearchChannel{cfg: cfg, retriever: retriever}
}

func (c *IntentDirectedSearchChannel) Name() string     { return "IntentDirectedSearch" }
func (c *IntentDirectedSearchChannel) Priority() int     { return 1 }
func (c *IntentDirectedSearchChannel) Type() SearchChannelType { return ChannelIntentDirected }

func (c *IntentDirectedSearchChannel) IsEnabled(ctx context.Context, sc *SearchContext) bool {
	if !c.cfg.Enabled {
		return false
	}
	return len(sc.KBIntents()) > 0
}

func (c *IntentDirectedSearchChannel) Search(ctx context.Context, sc *SearchContext) (*ChannelResult, error) {
	start := time.Now()
	kbIntents := sc.KBIntents()

	if len(kbIntents) == 0 {
		return emptyResult(ChannelIntentDirected, c.Name(), start), nil
	}

	zap.L().Info("执行意图定向检索", zap.Int("intents", len(kbIntents)))

	// 并行检索每个意图对应的 collection
	type intentResult struct {
		intent *NodeScore
		chunks  []RetrievedChunk
	}

	var allChunks []RetrievedChunk
	multiplier := c.cfg.TopKMultiplier
	if multiplier <= 0 {
		multiplier = 2
	}
	perIntentTopK := sc.TopK * multiplier

	for _, ns := range kbIntents {
		if ns.Node == nil || ns.Node.CollectionName == "" {
			continue
		}
		chunks, err := c.retriever.Search(ctx, ns.Node.CollectionName, sc.RewrittenQuestion, perIntentTopK)
		if err != nil {
			zap.L().Warn("意图定向检索失败",
				zap.String("collection", ns.Node.CollectionName),
				zap.Error(err),
			)
			continue
		}
		_ = intentResult{intent: &ns, chunks: chunks}
		allChunks = append(allChunks, chunks...)
	}

	latency := time.Since(start).Milliseconds()
	zap.L().Info("意图定向检索完成",
		zap.Int("chunks", len(allChunks)),
		zap.Int64("latency_ms", latency),
	)

	return &ChannelResult{
		ChannelType: ChannelIntentDirected,
		ChannelName: c.Name(),
		Chunks:      allChunks,
		LatencyMs:   latency,
	}, nil
}

// ========== VectorGlobalSearchChannel 向量全局检索（兜底）==========

// VectorGlobalSearchChannel 向量全局检索通道（priority=10）
type VectorGlobalSearchChannel struct {
	cfg                       config.VectorGlobalConfig
	intentDirectedEnabled     bool
	retriever                 VectorRetriever
}

func NewVectorGlobalChannel(cfg config.VectorGlobalConfig, intentDirectedEnabled bool, retriever VectorRetriever) *VectorGlobalSearchChannel {
	return &VectorGlobalSearchChannel{
		cfg:                   cfg,
		intentDirectedEnabled: intentDirectedEnabled,
		retriever:             retriever,
	}
}

func (c *VectorGlobalSearchChannel) Name() string             { return "VectorGlobalSearch" }
func (c *VectorGlobalSearchChannel) Priority() int             { return 10 }
func (c *VectorGlobalSearchChannel) Type() SearchChannelType   { return ChannelVectorGlobal }

func (c *VectorGlobalSearchChannel) IsEnabled(ctx context.Context, sc *SearchContext) bool {
	// 1. 配置关闭
	if !c.cfg.Enabled {
		return false
	}
	// 2. 意图定向关闭时，全局检索必须兜底
	if !c.intentDirectedEnabled {
		return true
	}

	kbIntents := sc.KBIntents()
	// 3. 无 KB 意图 → 兜底
	if len(kbIntents) == 0 {
		zap.L().Info("未识别出任何意图，启用全局检索")
		return true
	}

	maxScore := sc.MaxScore()
	// 4. 最高分 < 置信度阈值 → 兜底
	if maxScore < c.cfg.ConfidenceThreshold {
		zap.L().Info("意图置信度过低，启用全局检索", zap.Float64("max_score", maxScore))
		return true
	}
	// 5. 单意图 + 分 < 补充阈值 → 兜底
	if len(kbIntents) == 1 && maxScore < c.cfg.SingleIntentSupplementThreshold {
		zap.L().Info("单一中等置信度意图，启用补充全局检索", zap.Float64("max_score", maxScore))
		return true
	}

	return false
}

func (c *VectorGlobalSearchChannel) Search(ctx context.Context, sc *SearchContext) (*ChannelResult, error) {
	start := time.Now()

	collections, err := c.retriever.ListCollections(ctx)
	if err != nil || len(collections) == 0 {
		zap.L().Warn("未找到任何 KB collection，跳过全局检索")
		return emptyResult(ChannelVectorGlobal, c.Name(), start), nil
	}

	// 每库并行 Fan-out
	multiplier := c.cfg.TopKMultiplier
	if multiplier <= 0 {
		multiplier = 3
	}
	perCollectionTopK := sc.TopK * multiplier

	var allChunks []RetrievedChunk
	for _, col := range collections {
		chunks, err := c.retriever.Search(ctx, col, sc.RewrittenQuestion, perCollectionTopK)
		if err != nil {
			zap.L().Warn("全局检索失败", zap.String("collection", col), zap.Error(err))
			continue
		}
		allChunks = append(allChunks, chunks...)
	}

	latency := time.Since(start).Milliseconds()
	zap.L().Info("向量全局检索完成",
		zap.Int("chunks", len(allChunks)),
		zap.Int64("latency_ms", latency),
	)

	return &ChannelResult{
		ChannelType: ChannelVectorGlobal,
		ChannelName: c.Name(),
		Chunks:      allChunks,
		LatencyMs:   latency,
	}, nil
}

func emptyResult(ct SearchChannelType, name string, start time.Time) *ChannelResult {
	return &ChannelResult{
		ChannelType: ct,
		ChannelName: name,
		Chunks:      nil,
		LatencyMs:   time.Since(start).Milliseconds(),
	}
}

// ========== YouComWebSearchChannel 联网检索兜底（priority=20）==========

// YouComWebSearchChannel You.com 联网检索通道（最低优先级）
type YouComWebSearchChannel struct {
	cfg    config.WebSearchConfig
	client *http.Client
}

func NewYouComWebSearchChannel(cfg config.WebSearchConfig) *YouComWebSearchChannel {
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = 10
	}
	return &YouComWebSearchChannel{
		cfg:    cfg,
		client: &http.Client{Timeout: time.Duration(timeout) * time.Second},
	}
}

func (c *YouComWebSearchChannel) Name() string          { return "YouComWebSearch" }
func (c *YouComWebSearchChannel) Priority() int          { return 20 }
func (c *YouComWebSearchChannel) Type() SearchChannelType { return ChannelWebSearch }

func (c *YouComWebSearchChannel) IsEnabled(ctx context.Context, sc *SearchContext) bool {
	return c.cfg.Enabled && c.cfg.APIKey != ""
}

func (c *YouComWebSearchChannel) Search(ctx context.Context, sc *SearchContext) (*ChannelResult, error) {
	start := time.Now()

	query := sc.RewrittenQuestion
	if query == "" {
		query = sc.OriginalQuestion
	}

	params := url.Values{}
	params.Set("query", query)
	params.Set("count", fmt.Sprintf("%d", c.cfg.Count))

	apiURL := c.cfg.APIURL
	if apiURL == "" {
		apiURL = "https://api.ydc-index.io/search"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL+"?"+params.Encode(), nil)
	if err != nil {
		return emptyResult(ChannelWebSearch, c.Name(), start), nil
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		zap.L().Warn("You.com 搜索失败", zap.Error(err))
		return emptyResult(ChannelWebSearch, c.Name(), start), nil
	}
	defer resp.Body.Close()

	var result struct {
		Hits []struct {
			Title    string   `json:"title"`
			URL      string   `json:"url"`
			Snippets []string `json:"snippets"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		zap.L().Warn("You.com 解析失败", zap.Error(err))
		return emptyResult(ChannelWebSearch, c.Name(), start), nil
	}

	var chunks []RetrievedChunk
	for _, hit := range result.Hits {
		for _, snippet := range hit.Snippets {
			chunks = append(chunks, RetrievedChunk{
				ID:    hit.URL,
				Text:  snippet,
				Score: 0.5,
				Metadata: map[string]any{
					"title":  hit.Title,
					"url":    hit.URL,
					"source": "web_search",
				},
			})
		}
	}

	latency := time.Since(start).Milliseconds()
	zap.L().Info("You.com 搜索完成",
		zap.Int("chunks", len(chunks)),
		zap.Int64("latency_ms", latency),
	)

	return &ChannelResult{
		ChannelType: ChannelWebSearch,
		ChannelName: c.Name(),
		Chunks:      chunks,
		LatencyMs:   latency,
	}, nil
}
