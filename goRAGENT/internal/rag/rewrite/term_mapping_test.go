package rewrite

import (
	"testing"
)

func mapping(source, target string, priority, matchType, enabled int) TermMappingDO {
	return TermMappingDO{
		SourceTerm: source, TargetTerm: target,
		Priority: priority, MatchType: matchType, Enabled: enabled,
	}
}

// ========== applyMapping 替换算法 ==========

func TestApplyMapping_GlobalSubstring(t *testing.T) {
	got := applyMapping("保司的保司规定", "保司", "保险公司")
	if got != "保险公司的保险公司规定" {
		t.Errorf("应全局替换所有出现: %q", got)
	}
}

func TestApplyMapping_SkipsAlreadyNormalized(t *testing.T) {
	// "保司" 是 "保险公司"…不构成前缀关系，用构造用例：源词是目标词前缀
	got := applyMapping("平安保险公司和平安", "平安", "平安保险公司")
	// 第一处 "平安" 已是 "平安保险公司" 的开头 → 跳过；第二处独立 "平安" → 替换
	if got != "平安保险公司和平安保险公司" {
		t.Errorf("已归一化位置应跳过，独立位置应替换: %q", got)
	}
}

func TestApplyMapping_NoMatch(t *testing.T) {
	if got := applyMapping("无关文本", "保司", "保险公司"); got != "无关文本" {
		t.Errorf("无匹配应原样返回: %q", got)
	}
}

// ========== 排序规则 ==========

func TestSortMappings_PriorityDescThenLengthDesc(t *testing.T) {
	ms := []TermMappingDO{
		mapping("短", "a", 50, 1, 1),
		mapping("很长的源词", "b", 100, 1, 1),
		mapping("中等词", "c", 100, 1, 1),
	}
	sortMappings(ms)
	if ms[0].SourceTerm != "很长的源词" || ms[1].SourceTerm != "中等词" || ms[2].SourceTerm != "短" {
		t.Errorf("应按 priority 降序、同优先级按源词长度降序: %v", []string{ms[0].SourceTerm, ms[1].SourceTerm, ms[2].SourceTerm})
	}
}

// ========== Normalize 过滤规则 ==========

func TestNormalize_FiltersDisabledAndNonExact(t *testing.T) {
	l := NewMappingLoader(nil, nil)
	l.mappingsOverride = []TermMappingDO{
		mapping("A词", "X", 100, 1, 0), // 禁用 → 跳过
		mapping("B词", "Y", 100, 2, 1), // matchType=2 前缀 → 未实现，跳过
		mapping("C词", "Z", 100, 1, 1), // 生效
	}
	got := l.Normalize("A词 B词 C词")
	if got != "A词 B词 Z" {
		t.Errorf("只有 enabled 且 matchType=1 的规则生效: %q", got)
	}
}

func TestNormalize_AppliesInPriorityOrder(t *testing.T) {
	l := NewMappingLoader(nil, nil)
	l.mappingsOverride = []TermMappingDO{
		mapping("OA系统", "办公自动化系统", 100, 1, 1),
		mapping("OA", "办公自动化", 50, 1, 1),
	}
	// 高优先级 "OA系统" 先替换，低优先级 "OA" 不再命中已替换部分
	got := l.Normalize("OA系统怎么用")
	if got != "办公自动化系统怎么用" {
		t.Errorf("长词/高优先级应先替换: %q", got)
	}
}

func TestNormalize_EmptyRulesReturnsOriginal(t *testing.T) {
	l := NewMappingLoader(nil, nil)
	if got := l.Normalize("原文"); got != "原文" {
		t.Errorf("无规则应原样返回: %q", got)
	}
}
