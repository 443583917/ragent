package model

// ChunkVO 文档块列表/详情 VO。
type ChunkVO struct {
	ID              string `json:"id"`
	DocID           string `json:"docId"`
	KbID            string `json:"kbId"`
	ChunkIndex      int    `json:"chunkIndex"`
	Text            string `json:"text"`
	CharCount       int    `json:"charCount"`
	TokenCount      int    `json:"tokenCount"`
	EmbeddingStatus string `json:"embeddingStatus"`
	Enabled         int    `json:"enabled"`
}

// ChunkUpdateReq 更新 Chunk 请求体。
type ChunkUpdateReq struct {
	Text *string `json:"text"`
}

// ChunkDOToVO DO → VO 转换。
func ChunkDOToVO(d ChunkDO) ChunkVO {
	return ChunkVO{
		ID: d.ID, DocID: d.DocID, KbID: d.KbID, ChunkIndex: d.ChunkIndex,
		Text: d.Text, CharCount: d.CharCount, TokenCount: d.TokenCount,
		EmbeddingStatus: d.EmbeddingStatus, Enabled: d.Enabled,
	}
}
