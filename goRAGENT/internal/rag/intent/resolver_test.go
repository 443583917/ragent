package intent

import (
	"context"
	"testing"
)

func scoresFixture() []NodeScore {
	return []NodeScore{
		{Node: &IntentNode{ID: "a"}, Score: 0.92},
		{Node: &IntentNode{ID: "b"}, Score: 0.80},
		{Node: &IntentNode{ID: "c"}, Score: 0.50},
		{Node: &IntentNode{ID: "d"}, Score: 0.40},
		{Node: &IntentNode{ID: "e"}, Score: 0.34}, // 低于阈值
	}
}

func TestFilterAndCap_ThresholdAndLimit(t *testing.T) {
	got := filterAndCap(scoresFixture())
	if len(got) != MaxIntentCount {
		t.Fatalf("应过滤 <0.35 并 cap 到 %d，实际 %d", MaxIntentCount, len(got))
	}
	want := []string{"a", "b", "c"}
	for i, w := range want {
		if got[i].Node.ID != w {
			t.Errorf("第 %d 位应为 %s，实际 %s", i, w, got[i].Node.ID)
		}
	}
}

func TestFilterAndCap_AllBelowThreshold(t *testing.T) {
	got := filterAndCap([]NodeScore{{Node: &IntentNode{ID: "x"}, Score: 0.1}})
	if len(got) != 0 {
		t.Fatalf("全部低于阈值应为空: %+v", got)
	}
}

func TestToNodeRef_MapsAllFields(t *testing.T) {
	topK := 8
	n := &IntentNode{
		ID: "sales", Name: "销售数据", FullPath: "销售 > 数据",
		CollectionName: "col_sales", McpToolID: "sales_query",
		PromptSnippet: "snip", PromptTemplate: "tpl",
		TopK: &topK, Kind: KindMCP,
	}
	ref := toNodeRef(n)
	if ref.ID != "sales" || ref.Name != "销售数据" || ref.FullPath != "销售 > 数据" {
		t.Errorf("基础字段映射错误: %+v", ref)
	}
	if ref.CollectionName != "col_sales" || ref.McpToolID != "sales_query" {
		t.Errorf("检索/MCP 字段映射错误: %+v", ref)
	}
	if ref.PromptSnippet != "snip" || ref.PromptTemplate != "tpl" {
		t.Errorf("Prompt 字段映射错误: %+v", ref)
	}
	if ref.TopK == nil || *ref.TopK != 8 {
		t.Errorf("TopK 映射错误")
	}
	if ref.IsMCP != true || ref.IsKB != false {
		t.Errorf("Kind 标志错误: IsKB=%v IsMCP=%v", ref.IsKB, ref.IsMCP)
	}

	kb := toNodeRef(&IntentNode{ID: "hr", Kind: KindKB})
	if !kb.IsKB || kb.IsMCP {
		t.Errorf("KB 节点标志错误: IsKB=%v IsMCP=%v", kb.IsKB, kb.IsMCP)
	}
	sys := toNodeRef(&IntentNode{ID: "s", Kind: KindSystem})
	if sys.IsKB || sys.IsMCP {
		t.Errorf("SYSTEM 节点标志错误: IsKB=%v IsMCP=%v", sys.IsKB, sys.IsMCP)
	}
}

func TestResolve_WholeQuestionAsSingleSubQuestion(t *testing.T) {
	fake := &fakeChat{resp: `[{"id":"group-hr","score":0.92},{"id":"low","score":0.1}]`}
	c := NewClassifier(NewTreeLoader(nil, nil), fake, nil)
	c.treeOverride = []*IntentNode{
		{ID: "group-hr", Name: "人事", FullPath: "域 > 人事", Kind: KindKB, CollectionName: "col_hr"},
		{ID: "low", Name: "低分", FullPath: "域 > 低分", Kind: KindKB},
	}
	r := NewResolver(c)

	got := r.Resolve(context.Background(), "请假流程是怎样的？")

	if len(got) != 1 {
		t.Fatalf("应返回单个子问题意图，实际 %d", len(got))
	}
	si := got[0]
	if si.SubQuestion != "请假流程是怎样的？" {
		t.Errorf("子问题应为原始问题: %q", si.SubQuestion)
	}
	if len(si.NodeScores) != 1 {
		t.Fatalf("低分意图应被过滤，剩 1 个，实际 %d", len(si.NodeScores))
	}
	ns := si.NodeScores[0]
	if ns.Node.ID != "group-hr" || ns.Score != 0.92 || !ns.Node.IsKB || ns.Node.CollectionName != "col_hr" {
		t.Errorf("NodeScore 转换错误: node=%+v score=%v", ns.Node, ns.Score)
	}
}

func TestResolve_EmptyClassificationReturnsNil(t *testing.T) {
	fake := &fakeChat{resp: `[]`}
	c := NewClassifier(NewTreeLoader(nil, nil), fake, nil)
	c.treeOverride = []*IntentNode{{ID: "a", Name: "A", Kind: KindKB}}
	r := NewResolver(c)
	if got := r.Resolve(context.Background(), "问题"); len(got) != 0 {
		t.Fatalf("无命中意图应返回空: %+v", got)
	}
}
