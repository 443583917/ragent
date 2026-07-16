package intent

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	// intentTreeCacheKey Redis 缓存 key（和 Java IntentTreeCacheManager 一致）
	intentTreeCacheKey = "ragent:intent:tree"
	cacheExpire        = 7 * 24 * time.Hour
)

// TreeLoader 意图树加载器：Redis 缓存 → MySQL fallback → 回填缓存
type TreeLoader struct {
	db  *gorm.DB
	rdb *redis.Client
}

func NewTreeLoader(db *gorm.DB, rdb *redis.Client) *TreeLoader {
	return &TreeLoader{db: db, rdb: rdb}
}

// Load 加载意图树根节点列表（任一环节失败均降级，不报错中断）
func (l *TreeLoader) Load(ctx context.Context) []*IntentNode {
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

func (l *TreeLoader) fromCache(ctx context.Context) []*IntentNode {
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
	var roots []*IntentNode
	if err := json.Unmarshal([]byte(raw), &roots); err != nil {
		zap.L().Warn("意图树缓存反序列化失败，回退 DB", zap.Error(err))
		return nil
	}
	return roots
}

func (l *TreeLoader) saveCache(ctx context.Context, roots []*IntentNode) {
	if l.rdb == nil {
		return
	}
	raw, err := json.Marshal(roots)
	if err != nil {
		return
	}
	if err := l.rdb.Set(ctx, intentTreeCacheKey, raw, cacheExpire).Err(); err != nil {
		zap.L().Warn("写入意图树缓存失败", zap.Error(err))
	}
}

func (l *TreeLoader) fromDB(ctx context.Context) []*IntentNode {
	if l.db == nil {
		return nil
	}
	var dos []IntentNodeDO
	if err := l.db.WithContext(ctx).
		Where("deleted = 0 AND enabled = 1").
		Find(&dos).Error; err != nil {
		zap.L().Error("从 DB 加载意图树失败", zap.Error(err))
		return nil
	}
	return buildTree(dos)
}

// buildTree 扁平 DO 列表 → 树（纯函数）
// 根节点判定: parent_code 为空；父节点不存在的孤儿节点提升为根。
// 兄弟节点按 sort_order 升序（相同时按 intent_code），fullPath 递归填充。
func buildTree(dos []IntentNodeDO) []*IntentNode {
	type entry struct {
		node      *IntentNode
		sortOrder int
	}
	id2entry := make(map[string]*entry, len(dos))
	order := make(map[*IntentNode]int, len(dos))
	nodes := make([]*IntentNode, 0, len(dos))
	for _, d := range dos {
		n := d.toNode()
		id2entry[n.ID] = &entry{node: n, sortOrder: d.SortOrder}
		order[n] = d.SortOrder
		nodes = append(nodes, n)
	}

	var roots []*IntentNode
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

	sortNodes := func(ns []*IntentNode) {
		sort.SliceStable(ns, func(i, j int) bool {
			if order[ns[i]] != order[ns[j]] {
				return order[ns[i]] < order[ns[j]]
			}
			return ns[i].ID < ns[j].ID
		})
	}
	sortNodes(roots)

	var fill func(n *IntentNode, parentPath string)
	fill = func(n *IntentNode, parentPath string) {
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
func Leaves(roots []*IntentNode) []*IntentNode {
	var leaves []*IntentNode
	var walk func(n *IntentNode)
	walk = func(n *IntentNode) {
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
