package ingestion

import (
	"context"
	"strings"
	"unicode/utf8"

	"goRAGENT/internal/config"
	"go.uber.org/zap"
)

// Chunker Markdown 标题混合切分节点（）
type Chunker struct {
	ChunkSize    int
	ChunkOverlap int
}

func NewChunker(cfg config.IngestionConfig) *Chunker {
	return &Chunker{ChunkSize: cfg.ChunkSize, ChunkOverlap: cfg.ChunkOverlap}
}

func (c *Chunker) Name() string { return "Chunker" }

func (c *Chunker) Execute(ctx context.Context, pc *PipelineContext) error {
	markdown := pc.Markdown

	// Step 1: 按 # 标题分割 section
	sections := splitByHeading(markdown)

	// Step 2: 超过 ChunkSize 的 section 递归按固定大小切
	var chunks []ChunkSegment
	for _, sec := range sections {
		secLen := utf8.RuneCountInString(sec)
		if secLen <= c.ChunkSize {
			if strings.TrimSpace(sec) != "" {
				chunks = append(chunks, ChunkSegment{Index: len(chunks), Text: sec, CharCount: secLen})
			}
		} else {
			subs := splitFixedSize(sec, c.ChunkSize, c.ChunkOverlap)
			for _, sub := range subs {
				subLen := utf8.RuneCountInString(sub)
				if strings.TrimSpace(sub) != "" {
					chunks = append(chunks, ChunkSegment{Index: len(chunks), Text: sub, CharCount: subLen})
				}
			}
		}
	}

	pc.Chunks = chunks
	zap.L().Info("文档切分完成",
		zap.String("doc_id", pc.Doc.ID),
		zap.Int("chunks", len(chunks)),
	)
	return nil
}

// splitByHeading 按 Markdown 标题 (# ## ### ...) 分割
func splitByHeading(text string) []string {
	var sections []string
	lines := strings.Split(text, "\n")
	var buf strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trimmed, "#") && buf.Len() > 0 {
			sections = append(sections, buf.String())
			buf.Reset()
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	if buf.Len() > 0 {
		sections = append(sections, buf.String())
	}
	if len(sections) == 0 {
		sections = []string{text}
	}
	return sections
}

// splitFixedSize 固定大小切分（带重叠）
func splitFixedSize(text string, chunkSize, overlap int) []string {
	if chunkSize <= 0 {
		chunkSize = 1024
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 10
	}

	runes := []rune(text)
	if len(runes) <= chunkSize {
		return []string{text}
	}

	var chunks []string
	step := chunkSize - overlap
	if step <= 0 {
		step = 1
	}

	for i := 0; i < len(runes); i += step {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
		if end == len(runes) {
			break
		}
	}
	return chunks
}
