package rag

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"goRAGENT/internal/model"
)

// MappingLoader 同义词映射加载器：Redis 缓存 → MySQL fallback
type MappingLoader struct {
	db  *gorm.DB
	rdb *redis.Client

	mappingsOverride []model.TermMappingDO // 测试注入用
}

func NewMappingLoader(db *gorm.DB, rdb *redis.Client) *MappingLoader {
	return &MappingLoader{db: db, rdb: rdb}
}

// Normalize 同义词归一化：按优先级依次做精确子串替换（和 Java QueryTermMappingService 一致）
func (l *MappingLoader) Normalize(text string) string {
	mappings := l.load()
	result := text
	for _, m := range mappings {
		if m.Enabled == 0 || m.MatchType != model.MatchTypeExact {
			continue
		}
		if m.SourceTerm == "" || m.TargetTerm == "" {
			continue
		}
		result = applyMapping(result, m.SourceTerm, m.TargetTerm)
	}
	return result
}

// ClearCache 映射规则变更后清缓存
func (l *MappingLoader) ClearCache(ctx context.Context) {
	if l.rdb == nil {
		return
	}
	if err := l.rdb.Del(ctx, model.MappingCacheKey).Err(); err != nil {
		zap.L().Warn("清除同义词映射缓存失败", zap.Error(err))
	}
}

func (l *MappingLoader) load() []model.TermMappingDO {
	if l.mappingsOverride != nil {
		ms := append([]model.TermMappingDO(nil), l.mappingsOverride...)
		sortMappings(ms)
		return ms
	}
	ctx := context.Background()
	if ms := l.fromCache(ctx); ms != nil {
		return ms
	}
	ms := l.fromDB(ctx)
	if len(ms) > 0 {
		l.saveCache(ctx, ms)
	}
	return ms
}

func (l *MappingLoader) fromCache(ctx context.Context) []model.TermMappingDO {
	if l.rdb == nil {
		return nil
	}
	raw, err := l.rdb.Get(ctx, model.MappingCacheKey).Result()
	if err != nil {
		if err != redis.Nil {
			zap.L().Warn("读取同义词映射缓存失败", zap.Error(err))
		}
		return nil
	}
	var ms []model.TermMappingDO
	if err := json.Unmarshal([]byte(raw), &ms); err != nil {
		return nil
	}
	return ms
}

func (l *MappingLoader) saveCache(ctx context.Context, ms []model.TermMappingDO) {
	if l.rdb == nil {
		return
	}
	raw, err := json.Marshal(ms)
	if err != nil {
		return
	}
	if err := l.rdb.Set(ctx, model.MappingCacheKey, raw, model.MappingCacheTTL).Err(); err != nil {
		zap.L().Warn("写入同义词映射缓存失败", zap.Error(err))
	}
}

func (l *MappingLoader) fromDB(ctx context.Context) []model.TermMappingDO {
	if l.db == nil {
		return nil
	}
	var ms []model.TermMappingDO
	if err := l.db.WithContext(ctx).
		Where("enabled = 1 AND deleted = 0").
		Find(&ms).Error; err != nil {
		zap.L().Warn("从 DB 加载同义词映射失败", zap.Error(err))
		return nil
	}
	sortMappings(ms)
	return ms
}

// sortMappings priority 降序，同优先级按源词长度降序（长词优先，和 Java loadMappings 一致）
func sortMappings(ms []model.TermMappingDO) {
	sort.SliceStable(ms, func(i, j int) bool {
		if ms[i].Priority != ms[j].Priority {
			return ms[i].Priority > ms[j].Priority
		}
		return utf8.RuneCountInString(ms[i].SourceTerm) > utf8.RuneCountInString(ms[j].SourceTerm)
	})
}

// applyMapping 全局子串替换；若当前位置已是 targetTerm 开头则跳过（防重复替换，
// 和 Java QueryTermMappingUtil.applyMapping 一致）
func applyMapping(text, source, target string) string {
	if source == "" || !strings.Contains(text, source) {
		return text
	}
	var b strings.Builder
	i := 0
	for i < len(text) {
		rest := text[i:]
		if target != "" && strings.HasPrefix(rest, target) {
			b.WriteString(target)
			i += len(target)
			continue
		}
		if strings.HasPrefix(rest, source) {
			b.WriteString(target)
			i += len(source)
			continue
		}
		_, size := utf8.DecodeRuneInString(rest)
		b.WriteString(rest[:size])
		i += size
	}
	return b.String()
}
