package model

// MappingVO 关键词映射 VO（和 Java QueryTermMappingVO / 前端 QueryTermMapping 一致）。
type MappingVO struct {
	ID         string `json:"id"`
	SourceTerm string `json:"sourceTerm"`
	TargetTerm string `json:"targetTerm"`
	MatchType  int    `json:"matchType"`
	Priority   int    `json:"priority"`
	Enabled    bool   `json:"enabled"`
	Remark     string `json:"remark,omitempty"`
	CreateTime string `json:"createTime,omitempty"`
	UpdateTime string `json:"updateTime,omitempty"`
}

// MappingCreateReq 创建映射请求体。
type MappingCreateReq struct {
	SourceTerm string  `json:"sourceTerm" binding:"required"`
	TargetTerm string  `json:"targetTerm" binding:"required"`
	MatchType  *int    `json:"matchType"`
	Priority   *int    `json:"priority"`
	Enabled    *bool   `json:"enabled"`
	Remark     *string `json:"remark"`
}

// MappingUpdateReq 更新映射请求体。
type MappingUpdateReq struct {
	SourceTerm *string `json:"sourceTerm"`
	TargetTerm *string `json:"targetTerm"`
	MatchType  *int    `json:"matchType"`
	Priority   *int    `json:"priority"`
	Enabled    *bool   `json:"enabled"`
	Remark     *string `json:"remark"`
}

const mappingTimeLayout = "2006-01-02 15:04:05"

// MappingToVO TermMappingDO → MappingVO 转换。
func MappingToVO(d TermMappingDO) MappingVO {
	vo := MappingVO{
		ID: d.ID, SourceTerm: d.SourceTerm, TargetTerm: d.TargetTerm,
		MatchType: d.MatchType, Priority: d.Priority,
		Enabled: d.Enabled == 1, Remark: d.Remark,
	}
	if !d.CreateTime.IsZero() {
		vo.CreateTime = d.CreateTime.Format(mappingTimeLayout)
	}
	if !d.UpdateTime.IsZero() {
		vo.UpdateTime = d.UpdateTime.Format(mappingTimeLayout)
	}
	return vo
}

// MappingCreateReqToDO 创建请求 → DO（默认值：MatchType=Exact, Priority=0, Enabled=1）。
func MappingCreateReqToDO(req MappingCreateReq, id, operator string) TermMappingDO {
	do := TermMappingDO{
		ID: id, SourceTerm: req.SourceTerm, TargetTerm: req.TargetTerm,
		MatchType: MatchTypeExact, Priority: 0, Enabled: 1,
		CreateBy: operator, UpdateBy: operator,
	}
	if req.MatchType != nil {
		do.MatchType = *req.MatchType
	}
	if req.Priority != nil {
		do.Priority = *req.Priority
	}
	if req.Enabled != nil && !*req.Enabled {
		do.Enabled = 0
	}
	if req.Remark != nil {
		do.Remark = *req.Remark
	}
	return do
}

// MappingUpdateReqToUpdates 更新请求 → updates map。
func MappingUpdateReqToUpdates(req MappingUpdateReq, operator string) map[string]any {
	updates := map[string]any{"update_by": operator}
	if req.SourceTerm != nil {
		updates["source_term"] = *req.SourceTerm
	}
	if req.TargetTerm != nil {
		updates["target_term"] = *req.TargetTerm
	}
	if req.MatchType != nil {
		updates["match_type"] = *req.MatchType
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if req.Enabled != nil {
		v := 0
		if *req.Enabled {
			v = 1
		}
		updates["enabled"] = v
	}
	if req.Remark != nil {
		updates["remark"] = *req.Remark
	}
	return updates
}
