package rag

import (
	"context"
	"fmt"
	"goRAGENT/internal/model"
	"sort"
	"sync"

	"go.uber.org/zap"
)

// MultiChannelEngine 多通道检索引擎（和 Java MultiChannelRetrievalEngine 一致）
type MultiChannelEngine struct {
	channels       []model.SearchChannel
	postProcessors []PostProcessor
}

// NewMultiChannelEngine 创建多通道检索引擎
func NewMultiChannelEngine(channels []model.SearchChannel, postProcessors []PostProcessor) *MultiChannelEngine {
	// 按优先级排序
	sort.Slice(channels, func(i, j int) bool {
		return channels[i].Priority() < channels[j].Priority()
	})
	sort.Slice(postProcessors, func(i, j int) bool {
		return postProcessors[i].Order() < postProcessors[j].Order()
	})
	return &MultiChannelEngine{
		channels:       channels,
		postProcessors: postProcessors,
	}
}

// RetrieveKnowledgeChannels 执行多通道 KB 检索
func (e *MultiChannelEngine) RetrieveKnowledgeChannels(ctx context.Context, sc *model.SearchContext) ([]model.RetrievedChunk, error) {
	// 阶段 1：筛选启用通道 → 并行执行
	results := e.executeChannels(ctx, sc)
	if len(results) == 0 {
		return nil, nil
	}

	// 阶段 2：合并 → 后处理链
	chunks := mergeChunks(results)
	chunks = e.executePostProcessors(ctx, chunks, sc)

	zap.L().Info("多通道检索完成",
		zap.Int("channels", len(results)),
		zap.Int("final_chunks", len(chunks)),
	)
	return chunks, nil
}

func (e *MultiChannelEngine) executeChannels(ctx context.Context, sc *model.SearchContext) []*model.ChannelResult {
	var enabled []model.SearchChannel
	for _, ch := range e.channels {
		if ch.IsEnabled(ctx, sc) {
			enabled = append(enabled, ch)
		}
	}

	if len(enabled) == 0 {
		return nil
	}

	zap.L().Info("启用的检索通道", zap.Int("count", len(enabled)))

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results []*model.ChannelResult
	)

	for _, ch := range enabled {
		wg.Add(1)
		go func(c model.SearchChannel) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					zap.L().Error("检索通道 panic",
						zap.String("channel", c.Name()),
						zap.Any("recover", r),
					)
				}
			}()

			zap.L().Info("执行检索通道", zap.String("channel", c.Name()))
			result, err := c.Search(ctx, sc)
			if err != nil {
				zap.L().Error("检索通道执行失败",
					zap.String("channel", c.Name()),
					zap.Error(err),
				)
				return
			}

			mu.Lock()
			results = append(results, result)
			mu.Unlock()

			zap.L().Info("通道检索完成",
				zap.String("channel", c.Name()),
				zap.Int("chunks", len(result.Chunks)),
				zap.Int64("latency_ms", result.LatencyMs),
			)
		}(ch)
	}
	wg.Wait()

	return results
}

func (e *MultiChannelEngine) executePostProcessors(ctx context.Context, chunks []model.RetrievedChunk, sc *model.SearchContext) []model.RetrievedChunk {
	for _, pp := range e.postProcessors {
		if !pp.IsEnabled(ctx, sc) {
			continue
		}

		before := len(chunks)
		var err error
		chunks, err = pp.Process(ctx, chunks, sc)
		if err != nil {
			zap.L().Error("后处理器执行失败，跳过",
				zap.String("processor", pp.Name()),
				zap.Error(err),
			)
			continue
		}

		zap.L().Info("后处理器完成",
			zap.String("processor", pp.Name()),
			zap.Int("before", before),
			zap.Int("after", len(chunks)),
		)
	}
	return chunks
}

func mergeChunks(results []*model.ChannelResult) []model.RetrievedChunk {
	var all []model.RetrievedChunk
	for _, r := range results {
		all = append(all, r.Chunks...)
	}
	return all
}

// Ensure fmt import
var _ = fmt.Sprintf
