package intent

import (
	"testing"
)

func nodeDO(code, parent, name string, sortOrder int) IntentNodeDO {
	return IntentNodeDO{
		ID: "id-" + code, IntentCode: code, ParentCode: parent,
		Name: name, Level: 0, Kind: 0, Enabled: 1, SortOrder: sortOrder,
	}
}

func TestBuildTree_AssemblesParentChild(t *testing.T) {
	dos := []IntentNodeDO{
		nodeDO("root-a", "", "集团信息化", 0),
		nodeDO("child-hr", "root-a", "人事", 0),
		nodeDO("child-it", "root-a", "IT支持", 1),
	}
	roots := buildTree(dos)
	if len(roots) != 1 {
		t.Fatalf("应有 1 个根节点，实际 %d", len(roots))
	}
	if roots[0].ID != "root-a" {
		t.Errorf("根节点 ID 应为 intent_code root-a，实际 %q", roots[0].ID)
	}
	if len(roots[0].Children) != 2 {
		t.Fatalf("根节点应有 2 个子节点，实际 %d", len(roots[0].Children))
	}
}

func TestBuildTree_FullPathRecursive(t *testing.T) {
	dos := []IntentNodeDO{
		nodeDO("d", "", "集团信息化", 0),
		nodeDO("c", "d", "人事", 0),
		nodeDO("t", "c", "考勤", 0),
	}
	roots := buildTree(dos)
	leaf := roots[0].Children[0].Children[0]
	if leaf.FullPath != "集团信息化 > 人事 > 考勤" {
		t.Errorf("fullPath 错误: %q", leaf.FullPath)
	}
	if roots[0].FullPath != "集团信息化" {
		t.Errorf("根 fullPath 应为自身名称: %q", roots[0].FullPath)
	}
}

func TestBuildTree_OrphanBecomesRoot(t *testing.T) {
	dos := []IntentNodeDO{
		nodeDO("a", "", "正常根", 0),
		nodeDO("orphan", "not-exist", "孤儿节点", 0),
	}
	roots := buildTree(dos)
	if len(roots) != 2 {
		t.Fatalf("孤儿节点应提升为根，共 2 个根，实际 %d", len(roots))
	}
}

func TestBuildTree_SortedBySortOrder(t *testing.T) {
	dos := []IntentNodeDO{
		nodeDO("b", "", "第二", 2),
		nodeDO("a", "", "第一", 1),
		nodeDO("r", "", "根", 0),
	}
	roots := buildTree(dos)
	got := []string{roots[0].ID, roots[1].ID, roots[2].ID}
	want := []string{"r", "a", "b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("排序错误: got %v want %v", got, want)
		}
	}
}

func TestBuildTree_ParsesExamplesJSON(t *testing.T) {
	do := nodeDO("a", "", "人事", 0)
	do.Examples = `["请假流程是怎样的？","试用期多久转正？"]`
	roots := buildTree([]IntentNodeDO{do})
	if len(roots[0].Examples) != 2 {
		t.Fatalf("examples 应解析出 2 条，实际 %d", len(roots[0].Examples))
	}
	// 非 JSON 内容容错：不 panic，忽略
	do2 := nodeDO("b", "", "IT", 0)
	do2.Examples = "不是JSON"
	roots2 := buildTree([]IntentNodeDO{do2})
	if len(roots2[0].Examples) != 0 {
		t.Errorf("非法 examples 应容错为空，实际 %v", roots2[0].Examples)
	}
}

func TestBuildTree_MapsAllFields(t *testing.T) {
	topK := 8
	do := IntentNodeDO{
		ID: "row-1", IntentCode: "sales-data", ParentCode: "",
		Name: "销售数据", Description: "销售统计", Level: 2,
		Kind: 2, McpToolID: "sales_query", CollectionName: "col_sales",
		TopK: &topK, PromptSnippet: "snip", PromptTemplate: "tpl",
		ParamPromptTemplate: "ptpl", KbID: "kb-1", Enabled: 1,
	}
	n := buildTree([]IntentNodeDO{do})[0]
	if n.ID != "sales-data" { // IntentNode.ID = intent_code（对齐 Java）
		t.Errorf("ID 应映射 intent_code: %q", n.ID)
	}
	if n.Kind != KindMCP || n.McpToolID != "sales_query" {
		t.Errorf("MCP 字段映射错误: kind=%v toolId=%q", n.Kind, n.McpToolID)
	}
	if n.CollectionName != "col_sales" || n.TopK == nil || *n.TopK != 8 {
		t.Errorf("检索字段映射错误")
	}
	if n.KbID != "kb-1" || n.Description != "销售统计" || n.Level != 2 {
		t.Errorf("基础字段映射错误")
	}
	if n.PromptSnippet != "snip" || n.PromptTemplate != "tpl" || n.ParamPromptTemplate != "ptpl" {
		t.Errorf("Prompt 字段映射错误")
	}
}

func TestLeaves_ReturnsOnlyLeafNodes(t *testing.T) {
	dos := []IntentNodeDO{
		nodeDO("d", "", "域", 0),
		nodeDO("c1", "d", "类目1", 0),
		nodeDO("t1", "c1", "主题1", 0),
		nodeDO("t2", "c1", "主题2", 1),
		nodeDO("c2", "d", "类目2(无子)", 1),
	}
	roots := buildTree(dos)
	leaves := Leaves(roots)
	if len(leaves) != 3 { // t1, t2, c2
		t.Fatalf("应有 3 个叶子，实际 %d", len(leaves))
	}
	ids := map[string]bool{}
	for _, l := range leaves {
		ids[l.ID] = true
	}
	for _, want := range []string{"t1", "t2", "c2"} {
		if !ids[want] {
			t.Errorf("缺少叶子 %s", want)
		}
	}
}
