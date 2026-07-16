package model

// KnowledgeBaseVO 知识库列表/详情 VO（字段与前端知识库管理页面一致）。
type KnowledgeBaseVO struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	EmbeddingModel string `json:"embeddingModel,omitempty"`
	CollectionName string `json:"collectionName,omitempty"`
	Dimension      int    `json:"dimension"`
	CreateTime     string `json:"createTime"`
}

// KnowledgeBaseCreateReq 创建知识库请求体。
type KnowledgeBaseCreateReq struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// KnowledgeBaseUpdateReq 更新知识库请求体（指针字段区分「未传」和「传空」）。
type KnowledgeBaseUpdateReq struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// KnowledgeBaseDOToVO DO → VO 转换（CreateTime 格式与现有 handler 一致）。
func KnowledgeBaseDOToVO(d KnowledgeBaseDO) KnowledgeBaseVO {
	return KnowledgeBaseVO{
		ID: d.ID, Name: d.Name, Description: d.Description,
		EmbeddingModel: d.EmbeddingModel, CollectionName: d.CollectionName,
		Dimension: d.Dimension, CreateTime: d.CreateTime.Format("2006-01-02 15:04:05"),
	}
}
