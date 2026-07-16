package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"goRAGENT/internal/framework/response"
	"goRAGENT/internal/framework/snowflake"
	"goRAGENT/internal/framework/userctx"
	"goRAGENT/internal/rag/intent"
	"go.uber.org/zap"
)

// CacheClearer 意图树缓存清除抽象（*intent.TreeLoader 满足）
type CacheClearer interface {
	ClearCache(ctx context.Context)
}

// IntentNodeTreeVO 意图树节点 VO（字段和 Java IntentNodeTreeVO / 前端 IntentNodeTree 一致）
type IntentNodeTreeVO struct {
	ID                  string             `json:"id"`
	IntentCode          string             `json:"intentCode"`
	Name                string             `json:"name"`
	Level               int                `json:"level"`
	ParentCode          string             `json:"parentCode,omitempty"`
	Description         string             `json:"description,omitempty"`
	Examples            string             `json:"examples,omitempty"` // 原始 JSON 字符串
	CollectionName      string             `json:"collectionName,omitempty"`
	TopK                *int               `json:"topK,omitempty"`
	Kind                int                `json:"kind"`
	SortOrder           int                `json:"sortOrder"`
	Enabled             int                `json:"enabled"`
	McpToolID           string             `json:"mcpToolId,omitempty"`
	PromptSnippet       string             `json:"promptSnippet,omitempty"`
	PromptTemplate      string             `json:"promptTemplate,omitempty"`
	ParamPromptTemplate string             `json:"paramPromptTemplate,omitempty"`
	Children            []*IntentNodeTreeVO `json:"children,omitempty"`
}

type intentNodeCreateReq struct {
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

type intentNodeUpdateReq struct {
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

type intentNodeBatchReq struct {
	Ids []string `json:"ids" binding:"required"`
}

// ========== 纯转换函数 ==========

// buildIntentTreeVOs 扁平 DO → 树形 VO（含禁用节点，兄弟按 sort_order 排序）
func buildIntentTreeVOs(dos []intent.IntentNodeDO) []*IntentNodeTreeVO {
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
		if !ok { // 孤儿提升为根
			roots = append(roots, vo)
			continue
		}
		parent.Children = append(parent.Children, vo)
	}

	var sortRec func(ns []*IntentNodeTreeVO)
	sortRec = func(ns []*IntentNodeTreeVO) {
		for i := 1; i < len(ns); i++ { // 插入排序保持稳定
			for j := i; j > 0 && less(ns[j], ns[j-1]); j-- {
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

func less(a, b *IntentNodeTreeVO) bool {
	if a.SortOrder != b.SortOrder {
		return a.SortOrder < b.SortOrder
	}
	return a.IntentCode < b.IntentCode
}

func marshalExamples(examples []string) string {
	if len(examples) == 0 {
		return ""
	}
	b, err := json.Marshal(examples)
	if err != nil {
		return ""
	}
	return string(b)
}

func createReqToDO(req intentNodeCreateReq, id, operator string) intent.IntentNodeDO {
	do := intent.IntentNodeDO{
		ID: id, KbID: req.KbID, IntentCode: req.IntentCode, Name: req.Name,
		Level: req.Level, Examples: marshalExamples(req.Examples),
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

// updateReqToUpdates 只把请求中提供的字段放进 updates map
func updateReqToUpdates(req intentNodeUpdateReq, operator string) map[string]any {
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
		updates["examples"] = marshalExamples(req.Examples)
	}
	return updates
}

// ========== Handler 方法（覆盖 admin.go 中的空壳） ==========

func (h *Handler) clearIntentCache(c *gin.Context) {
	if h.intentCache != nil {
		h.intentCache.ClearCache(c.Request.Context())
	}
}

func (h *Handler) intentTrees(c *gin.Context) {
	var dos []intent.IntentNodeDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("deleted = 0").Find(&dos).Error; err != nil {
		zap.L().Error("查询意图树失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询意图树失败"))
		return
	}
	vos := buildIntentTreeVOs(dos)
	if vos == nil {
		vos = []*IntentNodeTreeVO{}
	}
	c.JSON(http.StatusOK, response.Success(vos))
}

func (h *Handler) createIntentNode(c *gin.Context) {
	var req intentNodeCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "intentCode/name 不能为空"))
		return
	}
	do := createReqToDO(req, snowflake.NextID(), userctx.GetUserID(c.Request.Context()))
	if err := h.db.WithContext(c.Request.Context()).Create(&do).Error; err != nil {
		zap.L().Error("创建意图节点失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "创建失败（intentCode 可能重复）"))
		return
	}
	h.clearIntentCache(c)
	c.JSON(http.StatusOK, response.Success(do.ID))
}

func (h *Handler) updateIntentNode(c *gin.Context) {
	var req intentNodeUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "参数错误"))
		return
	}
	updates := updateReqToUpdates(req, userctx.GetUserID(c.Request.Context()))
	if err := h.db.WithContext(c.Request.Context()).
		Model(&intent.IntentNodeDO{}).
		Where("id = ? AND deleted = 0", c.Param("id")).
		Updates(updates).Error; err != nil {
		zap.L().Error("更新意图节点失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "更新失败"))
		return
	}
	h.clearIntentCache(c)
	c.JSON(http.StatusOK, response.SuccessOK())
}

func (h *Handler) deleteIntentNode(c *gin.Context) {
	if err := h.db.WithContext(c.Request.Context()).
		Model(&intent.IntentNodeDO{}).
		Where("id = ?", c.Param("id")).
		Updates(map[string]any{"deleted": 1, "update_by": userctx.GetUserID(c.Request.Context())}).Error; err != nil {
		zap.L().Error("删除意图节点失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "删除失败"))
		return
	}
	h.clearIntentCache(c)
	c.JSON(http.StatusOK, response.SuccessOK())
}

func (h *Handler) batchUpdateIntent(c *gin.Context, updates map[string]any) {
	var req intentNodeBatchReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "ids 不能为空"))
		return
	}
	updates["update_by"] = userctx.GetUserID(c.Request.Context())
	if err := h.db.WithContext(c.Request.Context()).
		Model(&intent.IntentNodeDO{}).
		Where("id IN ?", req.Ids).
		Updates(updates).Error; err != nil {
		zap.L().Error("批量更新意图节点失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "批量操作失败"))
		return
	}
	h.clearIntentCache(c)
	c.JSON(http.StatusOK, response.SuccessOK())
}
