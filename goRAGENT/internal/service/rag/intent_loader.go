package rag

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
	"go.uber.org/zap"
)

const (
	// intentTreeCacheKey Redis 缓存 key
	intentTreeCacheKey = "ragent:intent:tree"
	cacheExpire        = 7 * 24 * time.Hour
)

// TreeLoader 意图树加载器：Redis 缓存 → MySQL fallback → 回填缓存
type TreeLoader struct {
	repo repository.IntentNodeRepository
	rdb  *redis.Client
}

func NewTreeLoader(repo repository.IntentNodeRepository, rdb *redis.Client) *TreeLoader {
	return &TreeLoader{repo: repo, rdb: rdb}
}

// Load 加载意图树根节点列表（任一环节失败均降级，不报错中断）
func (l *TreeLoader) Load(ctx context.Context) []*model.IntentNode {
	if roots := l.fromCache(ctx); len(roots) > 0 {
		return roots
	}
	roots := l.fromDB(ctx)
	if len(roots) > 0 {
		l.saveCache(ctx, roots)
	}
	return roots
}

// ClearCache 清除缓存（意图节点增删改后调用）
func (l *TreeLoader) ClearCache(ctx context.Context) {
	if l.rdb == nil {
		return
	}
	if err := l.rdb.Del(ctx, intentTreeCacheKey).Err(); err != nil {
		zap.L().Warn("清除意图树缓存失败", zap.Error(err))
	}
}

func (l *TreeLoader) fromCache(ctx context.Context) []*model.IntentNode {
	if l.rdb == nil {
		return nil
	}
	raw, err := l.rdb.Get(ctx, intentTreeCacheKey).Result()
	if err != nil {
		if err != redis.Nil {
			zap.L().Warn("读取意图树缓存失败，回退 DB", zap.Error(err))
		}
		return nil
	}
	var roots []*model.IntentNode
	if err := json.Unmarshal([]byte(raw), &roots); err != nil {
		zap.L().Warn("意图树缓存反序列化失败，回退 DB", zap.Error(err))
		return nil
	}
	return roots
}

func (l *TreeLoader) saveCache(ctx context.Context, roots []*model.IntentNode) {
	if l.rdb == nil {
		return
	}
	raw, err := json.Marshal(roots)
	if err != nil {
		zap.L().Warn("intent cache marshal failed", zap.Error(err))
		return
	}
	if err := l.rdb.Set(ctx, intentTreeCacheKey, raw, cacheExpire).Err(); err != nil {
		zap.L().Warn("写入意图树缓存失败", zap.Error(err))
	}
}

func (l *TreeLoader) fromDB(ctx context.Context) []*model.IntentNode {
	if l.repo == nil {
		return nil
	}
	dos, err := l.repo.ListActive(ctx)
	if err != nil {
		zap.L().Error("从 DB 加载意图树失败", zap.Error(err))
		return nil
	}
	return buildTree(dos)
}

// buildTree 扁平 DO 列表 → 树（纯函数）
// 根节点判定: parent_code 为空；父节点不存在的孤儿节点提升为根。
// 兄弟节点按 sort_order 升序（相同时按 intent_code），fullPath 递归填充。
func buildTree(dos []model.IntentNodeDO) []*model.IntentNode {
	type entry struct {
		node      *model.IntentNode
		sortOrder int
	}
	id2entry := make(map[string]*entry, len(dos))
	order := make(map[*model.IntentNode]int, len(dos))
	nodes := make([]*model.IntentNode, 0, len(dos))
	for _, d := range dos {
		n := d.ToNode()
		id2entry[n.ID] = &entry{node: n, sortOrder: d.SortOrder}
		order[n] = d.SortOrder
		nodes = append(nodes, n)
	}

	var roots []*model.IntentNode
	for _, n := range nodes {
		if strings.TrimSpace(n.ParentID) == "" {
			roots = append(roots, n)
			continue
		}
		parent, ok := id2entry[n.ParentID]
		if !ok { // 孤儿节点提升为根，避免数据错误导致节点丢失
			roots = append(roots, n)
			continue
		}
		parent.node.Children = append(parent.node.Children, n)
	}

	sortNodes := func(ns []*model.IntentNode) {
		sort.SliceStable(ns, func(i, j int) bool {
			if order[ns[i]] != order[ns[j]] {
				return order[ns[i]] < order[ns[j]]
			}
			return ns[i].ID < ns[j].ID
		})
	}
	sortNodes(roots)

	var fill func(n *model.IntentNode, parentPath string)
	fill = func(n *model.IntentNode, parentPath string) {
		if parentPath == "" {
			n.FullPath = n.Name
		} else {
			n.FullPath = parentPath + " > " + n.Name
		}
		sortNodes(n.Children)
		for _, c := range n.Children {
			fill(c, n.FullPath)
		}
	}
	for _, r := range roots {
		fill(r, "")
	}
	return roots
}

// Leaves 收集所有叶子节点（LLM 分类的候选集）
func Leaves(roots []*model.IntentNode) []*model.IntentNode {
	var leaves []*model.IntentNode
	var walk func(n *model.IntentNode)
	walk = func(n *model.IntentNode) {
		if len(n.Children) == 0 {
			leaves = append(leaves, n)
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, r := range roots {
		walk(r)
	}
	return leaves
}
