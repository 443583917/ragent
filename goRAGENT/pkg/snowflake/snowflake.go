package snowflake

import (
	"sync"

	sf "github.com/bwmarrin/snowflake"
)

var (
	node *sf.Node
	once sync.Once
)

// Init 初始化 Snowflake 节点（服务启动时调用一次）
func Init(workerID int64) error {
	var err error
	once.Do(func() {
		n, e := sf.NewNode(workerID)
		if e != nil {
			err = e
			return
		}
		node = n
	})
	return err
}

// NextID 生成下一个 Snowflake ID
func NextID() string {
	if node == nil {
		panic("snowflake 未初始化，请先调用 Init()")
	}
	return node.Generate().String()
}

// NextIDInt64 生成 int64 格式的 Snowflake ID
func NextIDInt64() int64 {
	if node == nil {
		panic("snowflake 未初始化，请先调用 Init()")
	}
	return node.Generate().Int64()
}
