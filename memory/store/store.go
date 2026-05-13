package store

import "context"

// Record 通用记录，key-value 形式，映射到 SQL 行
type Record map[string]interface{}

// Condition 查询条件
type Condition struct {
	Field    string      // 字段名
	Operator string      // "=", "!=", ">", ">=", "<", "<=", "LIKE", "IN"
	Value    interface{} // 值，IN 时为 []interface{}
}

// OrderBy 排序
type OrderBy struct {
	Field string
	Dir   string // "ASC" | "DESC"
}

// Query 结构化查询
type Query struct {
	Table      string
	Conditions []Condition
	OrderBy    []OrderBy
	Limit      int64
	Offset     int64
}

// StructuredStore 结构化存储接口
type StructuredStore interface {
	// Init 初始化存储（建表、迁移等）
	Init(ctx context.Context) error

	// Save 写入一条记录，如果 id 已存在则更新
	Save(ctx context.Context, table string, record Record) error

	// Get 按 ID 取一条记录
	Get(ctx context.Context, table string, id string) (Record, error)

	// Query 条件查询
	Query(ctx context.Context, query Query) ([]Record, error)

	// Delete 按 ID 删除
	Delete(ctx context.Context, table string, id string) error

	// BatchDelete 批量删除
	BatchDelete(ctx context.Context, table string, ids []string) error

	// Exec 执行原始 SQL（用于建关系表等复杂操作）
	Exec(ctx context.Context, sql string, args ...interface{}) error

	// Close 关闭连接
	Close() error
}

// VectorPoint 向量数据点
type VectorPoint struct {
	ID      string                 // 唯一标识，与 SQL 记录的 ID 对应
	Vector  []float64              // 嵌入向量
	Payload map[string]interface{} // 附带元数据，用于过滤
}

// VectorSearchResult 向量检索结果
type VectorSearchResult struct {
	ID      string
	Score   float64
	Payload map[string]interface{}
}

// VectorFilter 向量检索的过滤条件
type VectorFilter struct {
	Field    string      // payload 字段名
	Operator string      // "=", "!=", ">", ">=", "<", "IN"
	Value    interface{} // 值
}

// VectorSearch 向量检索参数
type VectorSearch struct {
	Collection string         // 集合名
	Vector     []float64      // 查询向量
	Limit      int64          // 返回数量
	MinScore   float64        // 最低相似度阈值
	Filters    []VectorFilter // payload 过滤条件
}

// VectorStore 向量存储接口
type VectorStore interface {
	// Init 初始化集合（不存在则创建）
	Init(ctx context.Context, collection string, dimension uint64) error

	// Upsert 写入或更新一个向量点
	Upsert(ctx context.Context, collection string, point VectorPoint) error

	// BatchUpsert 批量写入
	BatchUpsert(ctx context.Context, collection string, points []VectorPoint) error

	// Search 向量检索
	Search(ctx context.Context, search VectorSearch) ([]VectorSearchResult, error)

	// Delete 按 ID 删除向量
	Delete(ctx context.Context, collection string, ids []string) error

	// Close 关闭连接
	Close() error
}

type GraphNode struct {
	ID         string
	Tags       []string
	Properties map[string]interface{}
}

type GraphRelation struct {
	SourceID     string
	TargetID     string
	RelationType string
}

type GraphStore interface {
	Init(ctx context.Context) error

	CreateNode(ctx context.Context, node GraphNode) error
	UpdateNode(ctx context.Context, id string, props map[string]interface{}) error
	GetNode(ctx context.Context, id string) (GraphNode, error)
	DeleteNode(ctx context.Context, id string) error

	CreateRelation(ctx context.Context, rel GraphRelation) error
	GetNeighbors(ctx context.Context, id string) ([]GraphNode, error)
	GetNeighborIDs(ctx context.Context, id string) ([]string, error)

	Query(ctx context.Context, cypher string, params map[string]interface{}) ([]map[string]interface{}, error)

	Close() error
}
