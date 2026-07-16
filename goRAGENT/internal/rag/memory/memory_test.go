package memory

import (
	"testing"
)

func msgDO(id int64, role, content string) ConversationMessageDO {
	return ConversationMessageDO{ID: id, Role: role, Content: content}
}

// normalizeHistory 输入为按 id DESC 的最近 N 条，输出应为时间正序、
// 且去掉前导 assistant（保证以 user 开头的完整问答对）
func TestNormalizeHistory_ReversesAndTrimsLeadingAssistant(t *testing.T) {
	descRows := []ConversationMessageDO{
		msgDO(4, "assistant", "答2"),
		msgDO(3, "user", "问2"),
		msgDO(2, "assistant", "答1"), // 窗口截断导致的孤立 assistant（对应的 user 已滚出窗口）
	}
	got := normalizeHistory(descRows)
	if len(got) != 2 {
		t.Fatalf("前导 assistant 应被去掉，剩 2 条，实际 %d: %+v", len(got), got)
	}
	if got[0].Role != "user" || got[0].Content != "问2" {
		t.Errorf("第一条应为 user 问2: %+v", got[0])
	}
	if got[1].Role != "assistant" || got[1].Content != "答2" {
		t.Errorf("第二条应为 assistant 答2: %+v", got[1])
	}
}

func TestNormalizeHistory_EmptyAndAllAssistant(t *testing.T) {
	if got := normalizeHistory(nil); len(got) != 0 {
		t.Errorf("空输入应返回空: %+v", got)
	}
	onlyAssistant := []ConversationMessageDO{msgDO(1, "assistant", "a")}
	if got := normalizeHistory(onlyAssistant); len(got) != 0 {
		t.Errorf("全 assistant 应返回空: %+v", got)
	}
}

func TestNormalizeHistory_KeepsProperOrder(t *testing.T) {
	descRows := []ConversationMessageDO{
		msgDO(4, "assistant", "答2"),
		msgDO(3, "user", "问2"),
		msgDO(2, "assistant", "答1"),
		msgDO(1, "user", "问1"),
	}
	got := normalizeHistory(descRows)
	want := []string{"问1", "答1", "问2", "答2"}
	if len(got) != 4 {
		t.Fatalf("应保留 4 条: %+v", got)
	}
	for i, w := range want {
		if got[i].Content != w {
			t.Errorf("顺序错误，第 %d 条应为 %q 实际 %q", i, w, got[i].Content)
		}
	}
}

// truncateTitle 标题按 rune 截断（中文安全）
func TestTruncateTitle_RuneSafe(t *testing.T) {
	if got := truncateTitle("请假流程是怎样的？需要提前几天申请？", 8); got != "请假流程是怎样的" {
		t.Errorf("应按 rune 截断 8 字: %q", got)
	}
	if got := truncateTitle("短问题", 20); got != "短问题" {
		t.Errorf("不足上限应原样返回: %q", got)
	}
	if got := truncateTitle("  带空白  ", 20); got != "带空白" {
		t.Errorf("应去首尾空白: %q", got)
	}
}
