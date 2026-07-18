package model

import "encoding/json"

// IntentNodeTreeVO 意图树节点 VO（字段和前端 IntentNodeTree 一致）。
type IntentNodeTreeVO struct {
	ID                  string              `json:"id"`
	IntentCode          string              `json:"intentCode"`
	Name                string              `json:"name"`
	Level               int                 `json:"level"`
	ParentCode          string              `json:"parentCode,omitempty"`
	Description         string              `json:"description,omitempty"`
	Examples            string              `json:"examples,omitempty"`
	CollectionName      string              `json:"collectionName,omitempty"`
	TopK                *int                `json:"topK,omitempty"`
	Kind                int                 `json:"kind"`
	SortOrder           int                 `json:"sortOrder"`
	Enabled             int                 `json:"enabled"`
	McpToolID           string              `json:"mcpToolId,omitempty"`
	PromptSnippet       string              `json:"promptSnippet,omitempty"`
	PromptTemplate      string              `json:"promptTemplate,omitempty"`
	ParamPromptTemplate string              `json:"paramPromptTemplate,omitempty"`
	Children            []*IntentNodeTreeVO `json:"children,omitempty"`
}

// IntentNodeCreateReq 创建意图节点请求体。
type IntentNodeCreateReq struct {
	KbID                string   `json:"kbId"`
	IntentCode          string   `json:"intentCode" binding:"required"`
	Name                string   `json:"name" binding:"required"`
	Level               int      `json:"level"`
	ParentCode          *string  `json:"parentCode"`
	Description         *string  `json:"description"`
	Examples            []string `json:"examples"`
	McpToolID           *string  `json:"mcpToolId"`
	TopK                *int     `json:"topK"`
	Kind                *int     `json:"kind"`
	SortOrder           *int     `json:"sortOrder"`
	Enabled             *int     `json:"enabled"`
	PromptSnippet       *string  `json:"promptSnippet"`
	PromptTemplate      *string  `json:"promptTemplate"`
	ParamPromptTemplate *string  `json:"paramPromptTemplate"`
}

// IntentNodeUpdateReq 更新意图节点请求体。
type IntentNodeUpdateReq struct {
	Name                *string  `json:"name"`
	Level               *int     `json:"level"`
	ParentCode          *string  `json:"parentCode"`
	Description         *string  `json:"description"`
	Examples            []string `json:"examples"`
	CollectionName      *string  `json:"collectionName"`
	McpToolID           *string  `json:"mcpToolId"`
	TopK                *int     `json:"topK"`
	Kind                *int     `json:"kind"`
	SortOrder           *int     `json:"sortOrder"`
	Enabled             *int     `json:"enabled"`
	PromptSnippet       *string  `json:"promptSnippet"`
	PromptTemplate      *string  `json:"promptTemplate"`
	ParamPromptTemplate *string  `json:"paramPromptTemplate"`
}

// IntentNodeBatchReq 批量操作请求体。
type IntentNodeBatchReq struct {
	Ids []string `json:"ids" binding:"required"`
}

// MarshalExamples 将 []string 序列化为 JSON 字符串（空切片返回 ""）。
func MarshalExamples(examples []string) string {
	if len(examples) == 0 {
		return ""
	}
	b, err := json.Marshal(examples)
	if err != nil {
		return ""
	}
	return string(b)
}

// IntentCreateReqToDO 创建请求 → IntentNodeDO（填充默认值：Enabled=1, Kind=0）。
func IntentCreateReqToDO(req IntentNodeCreateReq, id, operator string) IntentNodeDO {
	do := IntentNodeDO{
		ID: id, KbID: req.KbID, IntentCode: req.IntentCode, Name: req.Name,
		Level: req.Level, Examples: MarshalExamples(req.Examples),
		Enabled: 1, Kind: 0, CreateBy: operator, UpdateBy: operator,
	}
	if req.ParentCode != nil {
		do.ParentCode = *req.ParentCode
	}
	if req.Description != nil {
		do.Description = *req.Description
	}
	if req.McpToolID != nil {
		do.McpToolID = *req.McpToolID
	}
	do.TopK = req.TopK
	if req.Kind != nil {
		do.Kind = *req.Kind
	}
	if req.SortOrder != nil {
		do.SortOrder = *req.SortOrder
	}
	if req.Enabled != nil {
		do.Enabled = *req.Enabled
	}
	if req.PromptSnippet != nil {
		do.PromptSnippet = *req.PromptSnippet
	}
	if req.PromptTemplate != nil {
		do.PromptTemplate = *req.PromptTemplate
	}
	if req.ParamPromptTemplate != nil {
		do.ParamPromptTemplate = *req.ParamPromptTemplate
	}
	return do
}

// IntentUpdateReqToUpdates 更新请求 → updates map（只写有提供的字段）。
func IntentUpdateReqToUpdates(req IntentNodeUpdateReq, operator string) map[string]any {
	updates := map[string]any{"update_by": operator}
	set := func(key string, p *string) {
		if p != nil {
			updates[key] = *p
		}
	}
	setInt := func(key string, p *int) {
		if p != nil {
			updates[key] = *p
		}
	}
	set("name", req.Name)
	setInt("level", req.Level)
	set("parent_code", req.ParentCode)
	set("description", req.Description)
	set("collection_name", req.CollectionName)
	set("mcp_tool_id", req.McpToolID)
	setInt("top_k", req.TopK)
	setInt("kind", req.Kind)
	setInt("sort_order", req.SortOrder)
	setInt("enabled", req.Enabled)
	set("prompt_snippet", req.PromptSnippet)
	set("prompt_template", req.PromptTemplate)
	set("param_prompt_template", req.ParamPromptTemplate)
	if req.Examples != nil {
		updates["examples"] = MarshalExamples(req.Examples)
	}
	return updates
}

// BuildIntentTreeVOs 扁平 DO → 树形 VO（含禁用节点，兄弟按 sort_order 排序）。
func BuildIntentTreeVOs(dos []IntentNodeDO) []*IntentNodeTreeVO {
	code2vo := make(map[string]*IntentNodeTreeVO, len(dos))
	vos := make([]*IntentNodeTreeVO, 0, len(dos))
	for _, d := range dos {
		vo := &IntentNodeTreeVO{
			ID: d.ID, IntentCode: d.IntentCode, Name: d.Name, Level: d.Level,
			ParentCode: d.ParentCode, Description: d.Description, Examples: d.Examples,
			CollectionName: d.CollectionName, TopK: d.TopK, Kind: d.Kind,
			SortOrder: d.SortOrder, Enabled: d.Enabled, McpToolID: d.McpToolID,
			PromptSnippet: d.PromptSnippet, PromptTemplate: d.PromptTemplate,
			ParamPromptTemplate: d.ParamPromptTemplate,
		}
		code2vo[d.IntentCode] = vo
		vos = append(vos, vo)
	}

	var roots []*IntentNodeTreeVO
	for _, vo := range vos {
		if vo.ParentCode == "" {
			roots = append(roots, vo)
			continue
		}
		parent, ok := code2vo[vo.ParentCode]
		if !ok {
			roots = append(roots, vo)
			continue
		}
		parent.Children = append(parent.Children, vo)
	}

	var sortRec func(ns []*IntentNodeTreeVO)
	sortRec = func(ns []*IntentNodeTreeVO) {
		for i := 1; i < len(ns); i++ {
			for j := i; j > 0 && intentLess(ns[j], ns[j-1]); j-- {
				ns[j], ns[j-1] = ns[j-1], ns[j]
			}
		}
		for _, n := range ns {
			sortRec(n.Children)
		}
	}
	sortRec(roots)
	return roots
}

func intentLess(a, b *IntentNodeTreeVO) bool {
	if a.SortOrder != b.SortOrder {
		return a.SortOrder < b.SortOrder
	}
	return a.IntentCode < b.IntentCode
}
