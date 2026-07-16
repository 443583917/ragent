package model

// IngestionTaskVO 入库任务列表/详情 VO。
type IngestionTaskVO struct {
	ID              int64  `json:"id"`
	KbID            string `json:"kbId"`
	DocID           string `json:"docId"`
	Status          string `json:"status"`
	TotalChunks     int    `json:"totalChunks"`
	CompletedChunks int    `json:"completedChunks"`
	ErrorMessage    string `json:"errorMessage,omitempty"`
	CreateTime      string `json:"createTime"`
}

// IngestionNodeVO 入库节点 VO（Fetcher / Parser / Chunker / Indexer 状态）。
type IngestionNodeVO struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// TaskDOToVO DO → VO 转换。
func TaskDOToVO(d IngestionTaskDO) IngestionTaskVO {
	return IngestionTaskVO{
		ID: d.ID, KbID: d.KbID, DocID: d.DocID, Status: d.Status,
		TotalChunks: d.TotalChunks, CompletedChunks: d.CompletedChunks,
		ErrorMessage: d.ErrorMessage,
		CreateTime:   d.CreateTime.Format("2006-01-02 15:04:05"),
	}
}
