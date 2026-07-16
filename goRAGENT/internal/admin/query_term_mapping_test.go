package admin

import (
	"testing"
	"time"

	"github.com/nageoffer/ragent/goRAGENT/internal/rag/rewrite"
)

func TestMappingToVO_EnabledIntToBool(t *testing.T) {
	ts := time.Date(2026, 7, 17, 12, 0, 0, 0, time.Local)
	vo := mappingToVO(rewrite.TermMappingDO{
		ID: "m1", SourceTerm: "保司", TargetTerm: "保险公司",
		MatchType: 1, Priority: 100, Enabled: 1, Remark: "备注",
		CreateTime: ts, UpdateTime: ts,
	})
	if vo.ID != "m1" || vo.SourceTerm != "保司" || vo.TargetTerm != "保险公司" {
		t.Errorf("基础字段错误: %+v", vo)
	}
	if vo.Enabled != true {
		t.Errorf("enabled=1 应转为 true")
	}
	if vo.CreateTime != "2026-07-17 12:00:00" {
		t.Errorf("时间格式错误: %q", vo.CreateTime)
	}

	vo2 := mappingToVO(rewrite.TermMappingDO{ID: "m2", Enabled: 0})
	if vo2.Enabled != false {
		t.Errorf("enabled=0 应转为 false")
	}
}

func TestMappingCreateReqToDO_Defaults(t *testing.T) {
	req := mappingCreateReq{SourceTerm: "保司", TargetTerm: "保险公司"}
	do := mappingCreateReqToDO(req, "id-1", "user-1")
	if do.ID != "id-1" || do.CreateBy != "user-1" {
		t.Errorf("基础字段错误: %+v", do)
	}
	if do.MatchType != 1 {
		t.Errorf("matchType 缺省应为 1: %d", do.MatchType)
	}
	if do.Enabled != 1 {
		t.Errorf("enabled 缺省应为 1: %d", do.Enabled)
	}
	if do.Priority != 0 {
		t.Errorf("priority 缺省应为 0: %d", do.Priority)
	}
}

func TestMappingCreateReqToDO_ExplicitValues(t *testing.T) {
	mt, pri, enabled := 2, 50, false
	remark := "r"
	req := mappingCreateReq{
		SourceTerm: "a", TargetTerm: "b",
		MatchType: &mt, Priority: &pri, Enabled: &enabled, Remark: &remark,
	}
	do := mappingCreateReqToDO(req, "id", "u")
	if do.MatchType != 2 || do.Priority != 50 || do.Enabled != 0 || do.Remark != "r" {
		t.Errorf("显式值应生效: %+v", do)
	}
}

func TestMappingUpdateReqToUpdates_OnlyProvided(t *testing.T) {
	enabled := false
	target := "新目标"
	req := mappingUpdateReq{TargetTerm: &target, Enabled: &enabled}
	updates := mappingUpdateReqToUpdates(req, "user-2")
	if updates["target_term"] != "新目标" || updates["enabled"] != 0 {
		t.Errorf("提供字段应进 updates（enabled bool→int）: %+v", updates)
	}
	if updates["update_by"] != "user-2" {
		t.Errorf("update_by 应设置: %+v", updates)
	}
	for _, forbidden := range []string{"source_term", "match_type", "priority", "remark"} {
		if _, ok := updates[forbidden]; ok {
			t.Errorf("未提供字段 %s 不应出现", forbidden)
		}
	}
}

func TestBuildPageResult_Math(t *testing.T) {
	pr := buildPageResult([]mappingVO{{ID: "a"}, {ID: "b"}}, 25, 3, 10)
	if pr.Total != 25 || pr.Current != 3 || pr.Size != 10 {
		t.Errorf("分页字段错误: %+v", pr)
	}
	if pr.Pages != 3 { // ceil(25/10)
		t.Errorf("pages 应为 3: %d", pr.Pages)
	}
	if len(pr.Records) != 2 {
		t.Errorf("records 错误: %d", len(pr.Records))
	}
}
