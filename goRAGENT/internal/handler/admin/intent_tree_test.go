package admin

import (
	"strings"
	"testing"

	"goRAGENT/internal/model"
)

func intentDO(id, code, parent, name string, sortOrder, enabled int) model.IntentNodeDO {
	return model.IntentNodeDO{
		ID: id, IntentCode: code, ParentCode: parent, Name: name,
		SortOrder: sortOrder, Enabled: enabled, Kind: 0, Level: 0,
	}
}

// ========== 树 VO 构建 ==========

func TestBuildIntentTreeVOs_NestedAndSorted(t *testing.T) {
	dos := []model.IntentNodeDO{
		intentDO("2", "child-b", "root", "子B", 2, 1),
		intentDO("1", "root", "", "根", 0, 1),
		intentDO("3", "child-a", "root", "子A", 1, 0), // 禁用节点也要展示
	}
	vos := model.BuildIntentTreeVOs(dos)
	if len(vos) != 1 {
		t.Fatalf("应有 1 个根 VO，实际 %d", len(vos))
	}
	root := vos[0]
	if root.ID != "1" || root.IntentCode != "root" {
		t.Errorf("根 VO 字段错误: %+v", root)
	}
	if len(root.Children) != 2 {
		t.Fatalf("根应有 2 个子节点（含禁用），实际 %d", len(root.Children))
	}
	if root.Children[0].IntentCode != "child-a" || root.Children[1].IntentCode != "child-b" {
		t.Errorf("子节点应按 sortOrder 排序: %s, %s",
			root.Children[0].IntentCode, root.Children[1].IntentCode)
	}
	if root.Children[0].Enabled != 0 {
		t.Errorf("禁用状态应保留: %d", root.Children[0].Enabled)
	}
}

func TestBuildIntentTreeVOs_MapsAllFields(t *testing.T) {
	topK := 8
	do := model.IntentNodeDO{
		ID: "row-1", IntentCode: "sales", Name: "销售", Level: 2, ParentCode: "",
		Description: "销售统计", Examples: `["q1","q2"]`, CollectionName: "col_s",
		TopK: &topK, McpToolID: "sales_query", Kind: 2, SortOrder: 5, Enabled: 1,
		KbID: "kb-1", PromptSnippet: "snip", PromptTemplate: "tpl", ParamPromptTemplate: "ptpl",
	}
	vo := model.BuildIntentTreeVOs([]model.IntentNodeDO{do})[0]
	if vo.ID != "row-1" || vo.IntentCode != "sales" || vo.Level != 2 || vo.Kind != 2 {
		t.Errorf("基础字段映射错误: %+v", vo)
	}
	if vo.Examples != `["q1","q2"]` { // 树 VO 中 examples 保持原始 JSON 字符串（对齐 Java VO）
		t.Errorf("examples 应为原始字符串: %q", vo.Examples)
	}
	if vo.CollectionName != "col_s" || vo.TopK == nil || *vo.TopK != 8 || vo.McpToolID != "sales_query" {
		t.Errorf("检索/MCP 字段映射错误: %+v", vo)
	}
	if vo.SortOrder != 5 || vo.PromptSnippet != "snip" {
		t.Errorf("其余字段映射错误: %+v", vo)
	}
}

// ========== 创建请求 → DO ==========

func TestCreateReqToDO_SerializesExamplesAndDefaults(t *testing.T) {
	req := model.IntentNodeCreateReq{
		KbID: "kb-1", IntentCode: "group-hr", Name: "人事", Level: 1,
		Examples: []string{"请假流程？", "转正时间？"},
	}
	do := model.IntentCreateReqToDO(req, "snowflake-id", "user-1")
	if do.ID != "snowflake-id" || do.IntentCode != "group-hr" || do.CreateBy != "user-1" {
		t.Errorf("基础字段错误: %+v", do)
	}
	if !strings.Contains(do.Examples, "请假流程？") || !strings.HasPrefix(do.Examples, "[") {
		t.Errorf("examples 应序列化为 JSON 数组: %q", do.Examples)
	}
	if do.Enabled != 1 {
		t.Errorf("enabled 缺省应为 1: %d", do.Enabled)
	}
	if do.Kind != 0 {
		t.Errorf("kind 缺省应为 0(KB): %d", do.Kind)
	}
}

func TestCreateReqToDO_ExplicitValues(t *testing.T) {
	kind, enabled, sortOrder, topK := 2, 0, 7, 6
	mcp := "tool-x"
	req := model.IntentNodeCreateReq{
		IntentCode: "s", Name: "n", Kind: &kind, Enabled: &enabled,
		SortOrder: &sortOrder, TopK: &topK, McpToolID: &mcp,
	}
	do := model.IntentCreateReqToDO(req, "id", "u")
	if do.Kind != 2 || do.Enabled != 0 || do.SortOrder != 7 {
		t.Errorf("显式值应生效: %+v", do)
	}
	if do.TopK == nil || *do.TopK != 6 || do.McpToolID != "tool-x" {
		t.Errorf("指针字段错误: %+v", do)
	}
}

// ========== 更新请求 → updates map ==========

func TestUpdateReqToUpdates_OnlyProvidedFields(t *testing.T) {
	name := "新名字"
	enabled := 0
	req := model.IntentNodeUpdateReq{Name: &name, Enabled: &enabled}
	updates := model.IntentUpdateReqToUpdates(req, "user-2")

	if updates["name"] != "新名字" || updates["enabled"] != 0 {
		t.Errorf("提供的字段应进入 updates: %+v", updates)
	}
	if updates["update_by"] != "user-2" {
		t.Errorf("update_by 应设置: %+v", updates)
	}
	for _, forbidden := range []string{"description", "kind", "sort_order", "collection_name"} {
		if _, exists := updates[forbidden]; exists {
			t.Errorf("未提供的字段 %s 不应出现在 updates 中", forbidden)
		}
	}
}

func TestUpdateReqToUpdates_ExamplesSerialized(t *testing.T) {
	req := model.IntentNodeUpdateReq{Examples: []string{"a", "b"}}
	updates := model.IntentUpdateReqToUpdates(req, "u")
	if updates["examples"] != `["a","b"]` {
		t.Errorf("examples 应序列化: %v", updates["examples"])
	}
}
