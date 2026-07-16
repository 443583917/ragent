package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nageoffer/ragent/goRAGENT/internal/config"
	"go.uber.org/zap"
)

type ChatRequest struct {
	Messages    []Message `json:"messages"`
	Temperature *float64  `json:"temperature,omitempty"`
	TopP        *float64  `json:"top_p,omitempty"`
	TopK        *int      `json:"top_k,omitempty"`
	MaxTokens   *int      `json:"max_tokens,omitempty"`
	Thinking    bool      `json:"-"`
	Stream      bool      `json:"stream"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type StreamCallback interface {
	OnContent(chunk string)
	OnThinking(chunk string)
	OnComplete()
	OnError(err error)
}

type StreamCanceler func()

// ChatService LLM 服务
type ChatService struct {
	cfg        *config.LLMConfig
	router     *ModelRouter
	httpClient *http.Client
}

func NewChatService(cfg *config.Config) *ChatService {
	return &ChatService{
		cfg:        &cfg.LLM,
		router:     NewModelRouter(&cfg.LLM),
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

func (s *ChatService) Chat(ctx context.Context, req ChatRequest) (string, error) {
	req.Stream = false
	candidates := s.router.SelectCandidates()
	for _, t := range candidates {
		result, err := s.doChat(ctx, t, req)
		if err == nil { s.router.MarkSuccess(t.ID); return result, nil }
		s.router.MarkFailure(t.ID)
		zap.L().Warn("模型调用失败，切换", zap.String("model", t.ID), zap.Error(err))
	}
	return "", fmt.Errorf("all models unavailable")
}

func (s *ChatService) StreamChat(ctx context.Context, req ChatRequest, cb StreamCallback) StreamCanceler {
	req.Stream = true
	candidates := s.router.SelectCandidates()
	for _, t := range candidates {
		cancel, err := s.doStream(ctx, t, req, cb)
		if err == nil { s.router.MarkSuccess(t.ID); return cancel }
		s.router.MarkFailure(t.ID)
		zap.L().Warn("流式调用失败，切换", zap.String("model", t.ID), zap.Error(err))
	}
	cb.OnError(fmt.Errorf("all models unavailable"))
	return func() {}
}

func (s *ChatService) doChat(ctx context.Context, t ModelTarget, req ChatRequest) (string, error) {
	body := buildJSON(req, t)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", t.BaseURL+"/chat/completions", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	if t.APIKey != "" { httpReq.Header.Set("Authorization", "Bearer "+t.APIKey) }
	resp, err := s.httpClient.Do(httpReq)
	if err != nil { return "", err }
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var r struct{ Choices []struct{ Message struct{ Content string } } }
	if err := json.Unmarshal(b, &r); err != nil { return "", err }
	if len(r.Choices) == 0 { return "", fmt.Errorf("no choices") }
	return r.Choices[0].Message.Content, nil
}

func (s *ChatService) doStream(ctx context.Context, t ModelTarget, req ChatRequest, cb StreamCallback) (StreamCanceler, error) {
	body := buildJSON(req, t)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", t.BaseURL+"/chat/completions", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if t.APIKey != "" { httpReq.Header.Set("Authorization", "Bearer "+t.APIKey) }
	resp, err := s.httpClient.Do(httpReq)
	if err != nil { return nil, err }
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body); resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	canceled := false
	cancel := func() { canceled = true; resp.Body.Close() }
	go func() {
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() && !canceled {
			l := sc.Text()
			if !strings.HasPrefix(l, "data: ") || l == "data: [DONE]" { continue }
			var ch struct{ Choices []struct{ Delta struct{ Content string `json:"content"` } } }
			if err := json.Unmarshal([]byte(l[6:]), &ch); err != nil { continue }
			for _, c := range ch.Choices {
				if c.Delta.Content != "" { cb.OnContent(c.Delta.Content) }
			}
		}
		if !canceled { cb.OnComplete() }
	}()
	return cancel, nil
}

func buildJSON(req ChatRequest, t ModelTarget) []byte {
	type r struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
		Stream   bool      `json:"stream"`
		Temperature *float64 `json:"temperature,omitempty"`
		TopP     *float64  `json:"top_p,omitempty"`
	}
	b, _ := json.Marshal(r{Model: t.Model, Messages: req.Messages, Stream: req.Stream, Temperature: req.Temperature, TopP: req.TopP})
	return b
}
