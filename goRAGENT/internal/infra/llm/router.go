package llm

import (
	"sync"
	"time"

	"github.com/nageoffer/ragent/goRAGENT/internal/config"
)

// ModelTarget 模型目标
type ModelTarget struct {
	ID       string `json:"id"`
	Model    string `json:"model"`
	Provider string `json:"provider"`
	BaseURL  string `json:"-"`
	APIKey   string `json:"-"`
}

// ModelRouter 多模型路由
type ModelRouter struct {
	mu       sync.RWMutex
	cfg      *config.LLMConfig
	breakres map[string]*CircuitBreaker
}

func NewModelRouter(cfg *config.LLMConfig) *ModelRouter {
	return &ModelRouter{cfg: cfg, breakres: make(map[string]*CircuitBreaker)}
}

func (r *ModelRouter) getBreaker(id string) *CircuitBreaker {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cb, ok := r.breakres[id]; ok { return cb }
	cb := NewCircuitBreaker(2, 30*time.Second)
	r.breakres[id] = cb
	return cb
}

// SelectCandidates 按优先级返回可用模型
// 降级链: 主模型(LLM_PROVIDER) → 其他已配置的模型
func (r *ModelRouter) SelectCandidates() []ModelTarget {
	// 所有已配置的 provider, 按优先级排序
	allProviders := []string{"glm", "openai", "deepseek", "qwen"}

	// 主模型提前到第一位
	primary := r.cfg.PrimaryProvider()
	var ordered []string
	ordered = append(ordered, primary)
	for _, p := range allProviders {
		if p != primary { ordered = append(ordered, p) }
	}

	var targets []ModelTarget
	for _, p := range ordered {
		pm := r.cfg.Resolve(p)
		if pm.Key == "" || pm.Model == "" { continue }
		cb := r.getBreaker(pm.Name)
		if !cb.Allow() { continue }
		targets = append(targets, ModelTarget{
			ID: pm.Name, Model: pm.Model,
			Provider: pm.Name, BaseURL: pm.BaseURL, APIKey: pm.Key,
		})
	}
	return targets
}

func (r *ModelRouter) MarkSuccess(id string) { r.getBreaker(id).MarkSuccess() }
func (r *ModelRouter) MarkFailure(id string) { r.getBreaker(id).MarkFailure() }
