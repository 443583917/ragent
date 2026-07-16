package ingestion

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nageoffer/ragent/goRAGENT/internal/infra/mineru"
	"go.uber.org/zap"
)

// Parser 文档解析节点 — 文本文件直读，其他格式走 MinerU
type Parser struct {
	mineru  *mineru.Client
	DataDir string
}

func NewParser(mineruClient *mineru.Client, dataDir string) *Parser {
	return &Parser{mineru: mineruClient, DataDir: dataDir}
}

func (p *Parser) Name() string { return "Parser" }

func (p *Parser) Execute(ctx context.Context, pc *PipelineContext) error {
	ext := strings.ToLower(filepath.Ext(pc.Doc.FileName))

	var markdown string
	var err error

	switch ext {
	case ".md", ".markdown", ".txt":
		data, readErr := os.ReadFile(pc.FilePath)
		if readErr != nil {
			return fmt.Errorf("读取文本文件失败: %w", readErr)
		}
		markdown = string(data)
	default:
		markdown, err = p.mineru.Parse(ctx, pc.FilePath)
		if err != nil {
			return fmt.Errorf("MinerU 解析失败: %w", err)
		}
	}

	pc.Markdown = markdown

	// 解析产物落盘
	parsedDir := filepath.Join(p.DataDir, "parsed")
	os.MkdirAll(parsedDir, 0755)
	parsedPath := filepath.Join(parsedDir, pc.Doc.ID+".md")
	if err := os.WriteFile(parsedPath, []byte(markdown), 0644); err != nil {
		zap.L().Warn("保存解析产物失败", zap.String("path", parsedPath), zap.Error(err))
	}

	zap.L().Info("文档解析完成",
		zap.String("doc_id", pc.Doc.ID),
		zap.Int("chars", len(markdown)),
	)
	return nil
}
