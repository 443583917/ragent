package sse

import (
	"encoding/json"
	"testing"
)

func TestMetaPayload_JSONFields(t *testing.T) {
	mp := MetaPayload{ConversationID: "conv_123", TaskID: "task_456"}
	data, _ := json.Marshal(mp)
	if string(data) != `{"conversationId":"conv_123","taskId":"task_456"}` {
		t.Errorf("JSON mismatch: %s", string(data))
	}
}

func TestMessageDelta_JSONFields(t *testing.T) {
	md := MessageDelta{Type: "response", Delta: "你好"}
	data, _ := json.Marshal(md)
	if string(data) != `{"type":"response","delta":"你好"}` {
		t.Errorf("JSON mismatch: %s", string(data))
	}
}

func TestCompletionPayload_OmitEmptyTitle(t *testing.T) {
	cp := CompletionPayload{MessageID: "msg_789"}
	data, _ := json.Marshal(cp)
	// title should be omitted when empty
	var result map[string]any
	json.Unmarshal(data, &result)
	if _, exists := result["title"]; exists {
		t.Error("empty title should be omitted")
	}
}

func TestCompletionPayload_WithTitle(t *testing.T) {
	cp := CompletionPayload{MessageID: "msg_789", Title: "测试标题"}
	data, _ := json.Marshal(cp)
	var result map[string]any
	json.Unmarshal(data, &result)
	if result["title"] != "测试标题" {
		t.Error("title should be present")
	}
}
