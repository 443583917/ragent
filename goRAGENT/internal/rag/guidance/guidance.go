package guidance

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/nageoffer/ragent/goRAGENT/internal/config"
	"github.com/nageoffer/ragent/goRAGENT/internal/infra/llm"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag/prompt"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag/retrieve"
	"go.uber.org/zap"
)

const (
	ambiguityCheckPromptPath = "guidance-ambiguity-check.st"
	guidancePromptPath       = "guidance-prompt.st"
)

// chatClient LLM 同步调用抽象
type chatClient interface {
	Chat(ctx context.Context, req llm.ChatRequest) (string, error)
}

// Detector 歧义引导检测器（和 Java IntentGuidanceService + AmbiguityLLMChecker 对应）
type Detector struct {
	cfg     config.GuidanceConfig
	llm     chatClient
	prompts *prompt.TemplateLoader
}

func NewDetector(cfg config.GuidanceConfig, llmSvc chatClient, prompts *prompt.TemplateLoader) *Detector {
	if prompts == nil {
		prompts = prompt.NewTemplateLoader()
	}
	return &Detector{cfg: cfg, llm: llmSvc, prompts: prompts}
}

// Detect 检测歧义并返回引导话术；空串表示无需引导。
// 算法（和 Java detectAmbiguity 一致）：
//   - 仅单子问题时检测；候选 = KB 意图按 ID 聚合取最高分、降序
//   - ratio = 次高/最高：< threshold-margin 明确；≥ threshold 直接歧义；
//     [threshold-margin, threshold) LLM 二次确认（失败保守判歧义）
//   - 问题含候选域名线索（FullPath 首段）→ 跳过
func (d *Detector) Detect(ctx context.Context, question string, subs []retrieve.SubQuestionIntent) string {
	if !d.cfg.Enabled || len(subs) != 1 {
		return ""
	}
	ranked := rankCandidates(subs)
	if len(ranked) < 2 {
		return ""
	}

	threshold := d.cfg.AmbiguityScoreRatio
	margin := d.cfg.AmbiguityMargin
	top := ranked[0].Score
	if top <= 0 {
		return ""
	}
	ratio := ranked[1].Score / top

	if ratio < threshold-margin {
		return "" // 意图明确
	}
	if domainMentioned(question, ranked) {
		return "" // 问题已含域名线索
	}

	ambiguous := ratio >= threshold
	if !ambiguous {
		// 边界区间 → LLM 二次确认（失败保守判歧义）
		ambiguous = d.confirmByLLM(ctx, question, ranked)
	}
	if !ambiguous {
		return ""
	}

	text := d.buildGuidancePrompt(ranked)
	if text != "" {
		zap.L().Info("触发歧义引导", zap.Float64("ratio", ratio), zap.Int("options", min(len(ranked), d.cfg.MaxOptions)))
	}
	return text
}

func (d *Detector) confirmByLLM(ctx context.Context, question string, ranked []retrieve.NodeScore) bool {
	tpl, err := d.prompts.Load(ambiguityCheckPromptPath)
	if err != nil {
		return true
	}
	var b strings.Builder
	for _, ns := range ranked {
		b.WriteString(fmt.Sprintf("- 品类ID: %s, 名称: %s, 路径: %s, 分数: %.2f\n",
			ns.Node.ID, ns.Node.Name, ns.Node.FullPath, ns.Score))
	}
	promptText := strings.ReplaceAll(tpl, "{question}", question)
	promptText = strings.ReplaceAll(promptText, "{candidates}", b.String())

	temp, topP := 0.1, 0.3
	raw, err := d.llm.Chat(ctx, llm.ChatRequest{
		Messages:    []llm.Message{{Role: "user", Content: promptText}},
		Temperature: &temp, TopP: &topP,
	})
	if err != nil {
		zap.L().Warn("歧义二次确认 LLM 失败，保守判歧义", zap.Error(err))
		return true
	}
	var out struct {
		Ambiguous *bool `json:"ambiguous"`
	}
	if err := json.Unmarshal([]byte(stripMarkdownCodeFence(raw)), &out); err != nil || out.Ambiguous == nil {
		return true
	}
	return *out.Ambiguous
}

func (d *Detector) buildGuidancePrompt(ranked []retrieve.NodeScore) string {
	tpl, err := d.prompts.Load(guidancePromptPath)
	if err != nil {
		return ""
	}
	text := strings.ReplaceAll(tpl, "{topic_name}", ranked[0].Node.Name)
	text = strings.ReplaceAll(text, "{options}", renderOptions(ranked, d.cfg.MaxOptions))
	return strings.TrimSpace(text)
}

// rankCandidates KB 意图按 ID 聚合取最高分，按分数降序
// （Java 按 CATEGORY 系统级聚合；Go 分类输出为叶子节点，简化为按 ID 聚合）
func rankCandidates(subs []retrieve.SubQuestionIntent) []retrieve.NodeScore {
	best := map[string]retrieve.NodeScore{}
	for _, s := range subs {
		for _, ns := range s.NodeScores {
			if ns.Node == nil || !ns.Node.IsKB {
				continue
			}
			if cur, ok := best[ns.Node.ID]; !ok || ns.Score > cur.Score {
				best[ns.Node.ID] = ns
			}
		}
	}
	ranked := make([]retrieve.NodeScore, 0, len(best))
	for _, ns := range best {
		ranked = append(ranked, ns)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score > ranked[j].Score
		}
		return ranked[i].Node.ID < ranked[j].Node.ID
	})
	return ranked
}

// renderOptions 选项列表 "1) 集团信息化 > 人事\n..."，按 max 截断
func renderOptions(ranked []retrieve.NodeScore, max int) string {
	var b strings.Builder
	for i, ns := range ranked {
		if max > 0 && i >= max {
			break
		}
		label := ns.Node.FullPath
		if label == "" {
			label = ns.Node.Name
		}
		if label == "" {
			label = ns.Node.ID
		}
		b.WriteString(fmt.Sprintf("%d) %s\n", i+1, label))
	}
	return strings.TrimRight(b.String(), "\n")
}

// domainMentioned 问题中是否已包含候选的域名线索（FullPath 首段）
func domainMentioned(question string, ranked []retrieve.NodeScore) bool {
	for _, ns := range ranked {
		domain := ns.Node.FullPath
		if idx := strings.Index(domain, " > "); idx > 0 {
			domain = domain[:idx]
		}
		domain = strings.TrimSpace(domain)
		if domain != "" && strings.Contains(question, domain) {
			return true
		}
	}
	return false
}

func stripMarkdownCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if idx := strings.Index(s, "\n"); idx >= 0 {
		s = s[idx+1:]
	} else {
		return ""
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}
