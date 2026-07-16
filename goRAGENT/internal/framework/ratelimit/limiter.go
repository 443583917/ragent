package ratelimit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Lua 脚本：原子入队/出队
// KEYS[1] = queue key  ARGV[1] = taskID  ARGV[2] = maxConcurrent
const luaEnqueue = `
local queue = KEYS[1]
local taskID = ARGV[1]
local max = tonumber(ARGV[2])
local len = redis.call('LLEN', queue)
local pos = len + 1
for i = 0, len - 1 do
  if redis.call('LINDEX', queue, i) == taskID then
    return {-1, 0}
  end
end
if len < max then
  return {0, 0}
end
redis.call('RPUSH', queue, taskID)
return {pos, len + 1}
`

const luaDequeue = `
local queue = KEYS[1]
local taskID = ARGV[1]
redis.call('LREM', queue, 0, taskID)
local len = redis.call('LLEN', queue)
return len
`

const luaPoll = `
local queue = KEYS[1]
local taskID = ARGV[1]
local max = tonumber(ARGV[2])
local len = redis.call('LLEN', queue)
for i = 0, len - 1 do
  if redis.call('LINDEX', queue, i) == taskID then
    if i < max then
      return {0, i + 1}
    else
      return {i + 1, len}
    end
  end
end
return {0, 0}
`

const queueKey = "ragent:queue:chat"
const pubsubChannel = "ragent:queue:notify"

// Config 限流配置
type Config struct {
	Enabled        bool
	MaxConcurrent  int
	MaxWaitSeconds int
	LeaseSeconds   int
	PollIntervalMs int
}

// Limiter 公平分布式限流器（对齐 Java FairDistributedRateLimiter）
type Limiter struct {
	rdb *redis.Client
	cfg Config
}

func NewLimiter(rdb *redis.Client, cfg Config) *Limiter {
	return &Limiter{rdb: rdb, cfg: cfg}
}

// TryAcquire 尝试获取执行槽位。
// 返回 (position, totalWait, ok)：
//
//	position=0  → 立即执行
//	position>0  → 排队中，第 position 位，需等待
//	ok=false    → 队列已满或重复
func (l *Limiter) TryAcquire(taskID string) (position int, totalWait int, ok bool) {
	if !l.cfg.Enabled {
		return 0, 0, true
	}
	ctx := context.Background()
	res, err := l.rdb.Eval(ctx, luaEnqueue, []string{queueKey}, taskID, l.cfg.MaxConcurrent).Result()
	if err != nil {
		zap.L().Error("限流 Lua 脚本失败", zap.Error(err))
		return 0, 0, true // Redis 故障降级放行
	}
	arr := res.([]interface{})
	pos := int(arr[0].(int64))
	total := int(arr[1].(int64))
	if pos == -1 {
		return 0, 0, false // 重复 taskID
	}
	if pos == 0 {
		return 0, total, true // 立即执行
	}
	return pos, total, true
}

// PollPosition 轮询排队位置
func (l *Limiter) PollPosition(taskID string) (position int, total int) {
	ctx := context.Background()
	res, err := l.rdb.Eval(ctx, luaPoll, []string{queueKey}, taskID, l.cfg.MaxConcurrent).Result()
	if err != nil {
		return 0, 0
	}
	arr := res.([]interface{})
	return int(arr[0].(int64)), int(arr[1].(int64))
}

// Release 释放槽位，通知下一个排队者
func (l *Limiter) Release(taskID string) {
	ctx := context.Background()
	l.rdb.Eval(ctx, luaDequeue, []string{queueKey}, taskID)
	l.rdb.Publish(ctx, pubsubChannel, "slot_free")
}

// WaitForSlot 带超时的排队等待（使用轮询 + pub/sub 通知）
func (l *Limiter) WaitForSlot(ctx context.Context, taskID string) (acquired bool) {
	if !l.cfg.Enabled {
		return true
	}

	maxWait := time.Duration(l.cfg.MaxWaitSeconds) * time.Second
	pollInterval := time.Duration(l.cfg.PollIntervalMs) * time.Millisecond
	if pollInterval <= 0 {
		pollInterval = 200 * time.Millisecond
	}

	deadline := time.Now().Add(maxWait)
	sub := l.rdb.Subscribe(ctx, pubsubChannel)
	defer sub.Close()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		pos, _ := l.PollPosition(taskID)
		if pos == 0 {
			return true
		}

		select {
		case <-ctx.Done():
			l.Release(taskID)
			return false
		case <-ticker.C:
		case <-sub.Channel():
		}

		if time.Now().After(deadline) {
			l.Release(taskID)
			return false
		}
	}
}

// RejectPayload SSE reject 事件载荷
type RejectPayload struct {
	Message  string `json:"message"`
	Position int    `json:"position,omitempty"`
	WaitSec  int    `json:"waitSec,omitempty"`
}

func (p RejectPayload) JSON() string {
	b, _ := json.Marshal(p)
	return string(b)
}

// FormatPosition 返回排队提醒文本
func FormatPosition(position int) string {
	return fmt.Sprintf("当前排队人数较多，您排在第 %d 位，请耐心等待。", position)
}
