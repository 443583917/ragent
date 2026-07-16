package model

// DocumentVO 文档列表/详情 VO（字段与前端文档管理页面一致）。
type DocumentVO struct {
	ID         string `json:"id"`
	KbID       string `json:"kbId"`
	FileName   string `json:"fileName"`
	FileType   string `json:"fileType"`
	FileSize   int64  `json:"fileSize"`
	Status     string `json:"status"`
	ChunkCount int    `json:"chunkCount"`
	CreateTime string `json:"createTime"`
}

// DocumentDOToVO DO → VO 转换（CreateTime 格式 yyyy-MM-dd HH:mm:ss）。
func DocumentDOToVO(d DocumentDO) DocumentVO {
	return DocumentVO{
		ID: d.ID, KbID: d.KbID, FileName: d.FileName, FileType: d.FileType,
		FileSize: d.FileSize, Status: d.Status, ChunkCount: d.ChunkCount,
		CreateTime: d.CreateTime.Format("2006-01-02 15:04:05"),
	}
}
