package rag

import (
	"testing"
	"time"

	"goRAGENT/internal/rag/memory"
)

func TestConvToVO_Format(t *testing.T) {
	ts := time.Date(2026, 7, 17, 10, 30, 0, 0, time.Local)
	vo := convToVO(memory.ConversationDO{
		ConversationID: "c1", Title: "请假流程", LastTime: ts,
	})
	if vo.ConversationID != "c1" || vo.Title != "请假流程" {
		t.Errorf("基础字段错误: %+v", vo)
	}
	if vo.LastTime != "2026-07-17 10:30:00" {
		t.Errorf("时间格式错误: %q", vo.LastTime)
	}
}

func TestConvToVO_ZeroTimeEmpty(t *testing.T) {
	vo := convToVO(memory.ConversationDO{ConversationID: "c1"})
	if vo.LastTime != "" {
		t.Errorf("零值时间应为空串: %q", vo.LastTime)
	}
}

func TestMsgToVO_AllFields(t *testing.T) {
	vote := 1
	dur := 3
	ts := time.Date(2026, 7, 17, 11, 0, 0, 0, time.Local)
	vo := msgToVO(memory.ConversationMessageDO{
		ID: 42, ConversationID: "c1", Role: "assistant", Content: "回答",
		ThinkingContent: "思考", ThinkingDuration: &dur, Vote: &vote, CreateTime: ts,
	})
	if vo.ID != 42 || vo.ConversationID != "c1" || vo.Role != "assistant" || vo.Content != "回答" {
		t.Errorf("基础字段错误: %+v", vo)
	}
	if vo.ThinkingContent != "思考" || vo.ThinkingDuration == nil || *vo.ThinkingDuration != 3 {
		t.Errorf("thinking 字段错误: %+v", vo)
	}
	if vo.Vote == nil || *vo.Vote != 1 {
		t.Errorf("vote 应为 1: %+v", vo.Vote)
	}
	if vo.CreateTime != "2026-07-17 11:00:00" {
		t.Errorf("时间格式错误: %q", vo.CreateTime)
	}
}

func TestMsgToVO_NullableFields(t *testing.T) {
	vo := msgToVO(memory.ConversationMessageDO{ID: 1, Role: "user", Content: "问"})
	if vo.Vote != nil || vo.ThinkingDuration != nil {
		t.Errorf("未投票/无思考时长应为 null: %+v", vo)
	}
}
