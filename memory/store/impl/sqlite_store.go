package impl

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/LiuAnzheng/memoria/memory/store"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db     *sql.DB
	dbPath string
}

func NewSQLiteStore(options map[string]interface{}) *SQLiteStore {
	dbPath := "./data/memory.db"
	if v, ok := options["db_path"]; ok {
		if s, ok := v.(string); ok && s != "" {
			dbPath = s
		}
	}
	return &SQLiteStore{dbPath: dbPath}
}

// Init 打开数据库连接，配置运行参数
func (s *SQLiteStore) Init(ctx context.Context) error {
	e := os.MkdirAll(filepath.Dir(s.dbPath), 0755)
	if e != nil {
		return fmt.Errorf("fail to create database path %s: %v", s.dbPath, e)
	}
	db, err := sql.Open("sqlite", s.dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite %s: %w", s.dbPath, err)
	}
	s.db = db

	// 并发安全配置
	pragmas := []string{
		"PRAGMA journal_mode=WAL",   // 读写不互斥
		"PRAGMA foreign_keys=ON",    // 启用外键约束
		"PRAGMA busy_timeout=5000",  // 写冲突时等 5s
		"PRAGMA synchronous=NORMAL", // WAL 下安全且快
	}
	for _, p := range pragmas {
		if _, err := s.db.ExecContext(ctx, p); err != nil {
			return fmt.Errorf("set pragma %s: %w", p, err)
		}
	}
	return nil
}

// Save 写入一条记录，存在则替换（INSERT OR REPLACE）
func (s *SQLiteStore) Save(ctx context.Context, table string, record store.Record) error {
	if err := validateName(table); err != nil {
		return err
	}
	keys := make([]string, 0, len(record))
	placeholders := make([]string, 0, len(record))
	values := make([]interface{}, 0, len(record))

	for k, v := range record {
		if err := validateName(k); err != nil {
			return fmt.Errorf("invalid column %q: %w", k, err)
		}
		serialized, err := serializeValue(v)
		if err != nil {
			return fmt.Errorf("serialize column %q: %w", k, err)
		}
		keys = append(keys, k)
		placeholders = append(placeholders, "?")
		values = append(values, serialized)
	}

	query := fmt.Sprintf(
		"INSERT OR REPLACE INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(keys, ", "),
		strings.Join(placeholders, ", "),
	)
	_, err := s.db.ExecContext(ctx, query, values...)
	return err
}

// Get 按 ID 取一条记录
func (s *SQLiteStore) Get(ctx context.Context, table string, id string) (store.Record, error) {
	if err := validateName(table); err != nil {
		return nil, err
	}
	query := fmt.Sprintf("SELECT * FROM %s WHERE id = ?", table)
	rows, err := s.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, rows.Err() // 不存在返回 nil，不报错
	}
	return scanRecord(rows)
}

// Query 条件查询
func (s *SQLiteStore) Query(ctx context.Context, q store.Query) ([]store.Record, error) {
	if err := validateName(q.Table); err != nil {
		return nil, err
	}

	var sb strings.Builder
	sb.WriteString("SELECT * FROM ")
	sb.WriteString(q.Table)

	args := make([]interface{}, 0)
	if len(q.Conditions) > 0 {
		where, whereArgs := buildWhereClause(q.Conditions)
		sb.WriteString(" WHERE ")
		sb.WriteString(where)
		args = append(args, whereArgs...)
	}

	if len(q.OrderBy) > 0 {
		sb.WriteString(" ORDER BY ")
		for i, ob := range q.OrderBy {
			if i > 0 {
				sb.WriteString(", ")
			}
			if err := validateName(ob.Field); err != nil {
				return nil, err
			}
			sb.WriteString(ob.Field)
			sb.WriteString(" ")
			sb.WriteString(ob.Dir) // ASC / DESC，由调用方保证合法
		}
	}

	if q.Limit > 0 {
		sb.WriteString(" LIMIT ?")
		args = append(args, q.Limit)
	}
	if q.Offset > 0 {
		sb.WriteString(" OFFSET ?")
		args = append(args, q.Offset)
	}

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]store.Record, 0)
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// Delete 按 ID 删除
func (s *SQLiteStore) Delete(ctx context.Context, table string, id string) error {
	if err := validateName(table); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = ?", table), id)
	return err
}

// BatchDelete 批量删除
func (s *SQLiteStore) BatchDelete(ctx context.Context, table string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := validateName(table); err != nil {
		return err
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf("DELETE FROM %s WHERE id IN (%s)", table, strings.Join(placeholders, ", "))
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

// Exec 执行原始 SQL
func (s *SQLiteStore) Exec(ctx context.Context, sqlStr string, args ...interface{}) error {
	_, err := s.db.ExecContext(ctx, sqlStr, args...)
	return err
}

// Close 关闭连接
func (s *SQLiteStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// validateName 防注入：只允许字母、数字、下划线
func validateName(name string) error {
	for _, ch := range name {
		if !(('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') ||
			('0' <= ch && ch <= '9') || ch == '_') {
			return fmt.Errorf("invalid name %q: only alphanumeric and underscore allowed", name)
		}
	}
	return nil
}

// serializeValue 处理 Record 中的复杂类型
// map/slice → JSON 字符串，其余原样
func serializeValue(v interface{}) (interface{}, error) {
	switch val := v.(type) {
	case nil:
		return nil, nil
	case string, int, int64, float64, bool, []byte:
		return val, nil
	case map[string]string, map[string]interface{}, []interface{}, []string:
		bytes, err := json.Marshal(val)
		if err != nil {
			return nil, fmt.Errorf("json marshal: %w", err)
		}
		return string(bytes), nil
	default:
		bytes, err := json.Marshal(val)
		if err != nil {
			return nil, fmt.Errorf("json marshal: %w", err)
		}
		return string(bytes), nil
	}
}

// buildWhereClause 将 Condition 列表转为 SQL WHERE 子句
func buildWhereClause(conditions []store.Condition) (string, []interface{}) {
	clauses := make([]string, 0, len(conditions))
	args := make([]interface{}, 0)

	for _, c := range conditions {
		_ = validateName(c.Field) // 调用方已校验，这里忽略

		switch strings.ToUpper(c.Operator) {
		case "IN":
			// Value 必须是 slice
			items, ok := c.Value.([]interface{})
			if !ok {
				// 尝试 []string
				if strItems, ok := c.Value.([]string); ok {
					items = make([]interface{}, len(strItems))
					for i, s := range strItems {
						items[i] = s
					}
				}
			}
			if len(items) == 0 {
				clauses = append(clauses, "1=0")
				break
			}
			placeholders := strings.Repeat("?,", len(items))
			placeholders = placeholders[:len(placeholders)-1] // 去掉末尾逗号
			clauses = append(clauses, fmt.Sprintf("%s IN (%s)", c.Field, placeholders))
			for _, item := range items {
				args = append(args, item)
			}
		default:
			clauses = append(clauses, fmt.Sprintf("%s %s ?", c.Field, c.Operator))
			args = append(args, c.Value)
		}
	}

	return strings.Join(clauses, " AND "), args
}

// scanRecord 将 sql.Rows 当前行扫描为 Record
func scanRecord(rows *sql.Rows) (store.Record, error) {
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}

	values := make([]interface{}, len(colTypes))
	valuePtrs := make([]interface{}, len(colTypes))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	if err := rows.Scan(valuePtrs...); err != nil {
		return nil, err
	}

	record := make(store.Record, len(colTypes))
	for i, ct := range colTypes {
		val := values[i]
		// SQLite 驱动返回 []byte 或 nil，需要类型转换
		switch v := val.(type) {
		case []byte:
			record[ct.Name()] = string(v)
		default:
			record[ct.Name()] = v
		}
	}
	return record, nil
}
