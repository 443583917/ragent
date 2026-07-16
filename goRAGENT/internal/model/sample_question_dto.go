package model

// SampleQuestionItemVO 匹配前端 SampleQuestion 类型。
type SampleQuestionItemVO struct {
	ID          string `json:"id"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Question    string `json:"question"`
	CreateTime  string `json:"createTime,omitempty"`
	UpdateTime  string `json:"updateTime,omitempty"`
}

// SampleQuestionPayload 创建/更新示例问题请求体。
type SampleQuestionPayload struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Question    *string `json:"question"`
}

// SQDOToItem DO → SampleQuestionItemVO 转换。
func SQDOToItem(d SampleQuestionDO) SampleQuestionItemVO {
	return SampleQuestionItemVO{
		ID: d.ID, Title: d.Title, Description: d.Description,
		Question:   d.Question,
		CreateTime: d.CreateTime.Format("2006-01-02 15:04:05"),
		UpdateTime: d.UpdateTime.Format("2006-01-02 15:04:05"),
	}
}
