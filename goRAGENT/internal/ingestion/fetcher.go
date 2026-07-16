package ingestion

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Fetcher 文件读取节点 — 按 doc 记录找到本地文件
type Fetcher struct {
	DataDir string
}

func (f *Fetcher) Name() string { return "Fetcher" }

func (f *Fetcher) Execute(ctx context.Context, pc *PipelineContext) error {
	ext := filepath.Ext(pc.Doc.FileName)
	if ext == "" {
		ext = ".txt"
	}
	filePath := filepath.Join(f.DataDir, "files", pc.Doc.KbID, pc.Doc.ID+ext)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("文件不存在: %s", filePath)
	}

	pc.FilePath = filePath
	return nil
}
