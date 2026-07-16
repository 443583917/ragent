package e2e

import (
	"net/http"
	"testing"
)

// TestHealthEndpoint 健康检查端点
func TestHealthEndpoint(t *testing.T) {
	t.Skip("需要启动服务后运行")

	resp, err := http.Get("http://localhost:9090/api/ragent/health")
	if err != nil { t.Fatalf("health check failed: %v", err) }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { t.Errorf("expected 200, got %d", resp.StatusCode) }
}

// TestChatSSEFormat SSE 事件格式验证
func TestChatSSEFormat(t *testing.T) {
	t.Skip("需要启动服务后运行")

	// curl http://localhost:9090/api/ragent/rag/v3/chat?question=test
	// 验证:
	// 1. event:meta 在第一个
	// 2. event:message 逐字推送
	// 3. event:finish 有 messageId
	// 4. event:done 以 [DONE] 结尾
}

// TestMultiTurnConversation 多轮对话上下文保持
func TestMultiTurnConversation(t *testing.T) {
	t.Skip("需要启动服务后运行")

	// 第1轮: question=OA系统数据安全
	// 第2轮: question=那保险系统的呢?&conversationId=<上轮ID>
	// 验证: 第2轮回答指代消解为"保险系统的数据安全规范"
}

// TestRateLimitRejection 限流拒绝
func TestRateLimitRejection(t *testing.T) {
	t.Skip("需要启动服务后运行")

	// 并发 15 请求 → 前 10 执行, 后 5 排队或 reject
	// 验证: SSE event:reject 返回"系统繁忙"
}
