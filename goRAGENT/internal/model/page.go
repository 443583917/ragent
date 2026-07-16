package model

// 分页默认值与上限
const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 200
)

// PageQuery 统一分页入参（handler 层负责从两种参数风格解析）。
type PageQuery struct {
	Page int // 从 1 开始
	Size int
}

// Normalize 纠正非法分页参数，返回可安全用于 SQL 的值。
func (q PageQuery) Normalize() PageQuery {
	if q.Page < 1 {
		q.Page = DefaultPage
	}
	if q.Size < 1 {
		q.Size = DefaultPageSize
	}
	if q.Size > MaxPageSize {
		q.Size = MaxPageSize
	}
	return q
}

// Offset 计算 SQL OFFSET。
func (q PageQuery) Offset() int { return (q.Page - 1) * q.Size }

// PageResult 对齐前端 PageResult<T> 的分页响应体（current/size 风格端点使用）。
type PageResult struct {
	Records any   `json:"records"`
	Total   int64 `json:"total"`
	Size    int   `json:"size"`
	Current int   `json:"current"`
	Pages   int64 `json:"pages"`
}

// NewPageResult 构造分页响应；pages 向上取整。
func NewPageResult(records any, total int64, q PageQuery) PageResult {
	pages := total / int64(q.Size)
	if total%int64(q.Size) != 0 {
		pages++
	}
	return PageResult{Records: records, Total: total, Size: q.Size, Current: q.Page, Pages: pages}
}
