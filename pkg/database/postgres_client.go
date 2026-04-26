package database

import (
	"context"
	"database/sql"
)

// =============================================================================
// 统一 PostgreSQL 客户端接口
// =============================================================================

// PostgreSQLClient 统一数据库操作接口。
// 同时支持 database/sql 标准库和自定义风格的调用，
// 为 agent/persistence 和 workflow/core 提供一致的抽象。
type PostgreSQLClient interface {
	// Exec 执行写操作（INSERT/UPDATE/DELETE），不返回行数据。
	Exec(ctx context.Context, query string, args ...any) error

	// Query 执行查询，返回多行结果。
	Query(ctx context.Context, query string, args ...any) (Rows, error)

	// QueryRow 执行查询，返回单行结果。
	QueryRow(ctx context.Context, query string, args ...any) Row
}

// Row 数据库单行接口，抽象 database/sql.Row 和自定义实现。
type Row interface {
	// Scan 将行数据扫描到目标变量。
	Scan(dest ...any) error
}

// Rows 数据库行迭代器接口，抽象 database/sql.Rows 和自定义实现。
type Rows interface {
	// Next 准备下一行数据，返回是否还有更多行。
	Next() bool

	// Scan 将当前行数据扫描到目标变量。
	Scan(dest ...any) error

	// Close 释放资源。
	Close() error
}

// =============================================================================
// SQLDBAdapter - 适配 *sql.DB
// =============================================================================

// SQLDBAdapter 将 *sql.DB 适配为 PostgreSQLClient 接口。
// 用于将标准库 database/sql.DB 转换为统一接口。
type SQLDBAdapter struct {
	db *sql.DB
}

// NewSQLDBAdapter 创建 *sql.DB 的适配器。
func NewSQLDBAdapter(db *sql.DB) *SQLDBAdapter {
	return &SQLDBAdapter{db: db}
}

// Exec 实现 PostgreSQLClient.Exec。
func (a *SQLDBAdapter) Exec(ctx context.Context, query string, args ...any) error {
	_, err := a.db.ExecContext(ctx, query, args...)
	return err
}

// Query 实现 PostgreSQLClient.Query。
func (a *SQLDBAdapter) Query(ctx context.Context, query string, args ...any) (Rows, error) {
	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &sqlRowsAdapter{rows: rows}, nil
}

// QueryRow 实现 PostgreSQLClient.QueryRow。
func (a *SQLDBAdapter) QueryRow(ctx context.Context, query string, args ...any) Row {
	return &sqlRowAdapter{row: a.db.QueryRowContext(ctx, query, args...)}
}

// DB 返回底层 *sql.DB，用于需要直接访问的场景。
func (a *SQLDBAdapter) DB() *sql.DB {
	return a.db
}

// =============================================================================
// SQLTxAdapter - 适配 *sql.Tx
// =============================================================================

// SQLTxAdapter 将 *sql.Tx 适配为 PostgreSQLClient 接口。
// 用于事务内的数据库操作。
type SQLTxAdapter struct {
	tx *sql.Tx
}

// NewSQLTxAdapter 创建 *sql.Tx 的适配器。
func NewSQLTxAdapter(tx *sql.Tx) *SQLTxAdapter {
	return &SQLTxAdapter{tx: tx}
}

// Exec 实现 PostgreSQLClient.Exec。
func (a *SQLTxAdapter) Exec(ctx context.Context, query string, args ...any) error {
	_, err := a.tx.ExecContext(ctx, query, args...)
	return err
}

// Query 实现 PostgreSQLClient.Query。
func (a *SQLTxAdapter) Query(ctx context.Context, query string, args ...any) (Rows, error) {
	rows, err := a.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &sqlRowsAdapter{rows: rows}, nil
}

// QueryRow 实现 PostgreSQLClient.QueryRow。
func (a *SQLTxAdapter) QueryRow(ctx context.Context, query string, args ...any) Row {
	return &sqlRowAdapter{row: a.tx.QueryRowContext(ctx, query, args...)}
}

// Tx 返回底层 *sql.Tx，用于需要直接访问的场景。
func (a *SQLTxAdapter) Tx() *sql.Tx {
	return a.tx
}

// =============================================================================
// 内部适配器类型
// =============================================================================

// sqlRowsAdapter 适配 *sql.Rows 到 Rows 接口。
type sqlRowsAdapter struct {
	rows *sql.Rows
}

func (a *sqlRowsAdapter) Next() bool {
	return a.rows.Next()
}

func (a *sqlRowsAdapter) Scan(dest ...any) error {
	return a.rows.Scan(dest...)
}

func (a *sqlRowsAdapter) Close() error {
	return a.rows.Close()
}

// sqlRowAdapter 适配 *sql.Row 到 Row 接口。
type sqlRowAdapter struct {
	row *sql.Row
}

func (a *sqlRowAdapter) Scan(dest ...any) error {
	return a.row.Scan(dest...)
}

// =============================================================================
// DBClient 兼容接口 - 兼容 workflow/core.DBClient
// =============================================================================

// DBClient 兼容接口，提供 database/sql 标准风格的数据库操作。
// 该接口与 workflow/core.DBClient 完全兼容，用于统一的类型定义。
// 优先使用 PostgreSQLClient 接口（更简洁的错误处理），
// 此接口保留用于需要 sql.Result 或 *sql.Rows 类型的场景。
type DBClient interface {
	// ExecContext 执行写操作（INSERT/UPDATE/DELETE），返回影响行数等信息。
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	// QueryContext 执行查询，返回多行结果。
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	// QueryRowContext 执行查询，返回单行结果。
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// SQLDBClientCompat 实现 DBClient 接口，提供 workflow/core.DBClient 兼容实现。
// 该适配器返回 sql.Result 和 *sql.Rows/*sql.Row，用于需要这些类型的场景。
type SQLDBClientCompat struct {
	db *sql.DB
}

// NewSQLDBClientCompat 创建兼容 workflow/core.DBClient 的适配器。
func NewSQLDBClientCompat(db *sql.DB) *SQLDBClientCompat {
	return &SQLDBClientCompat{db: db}
}

// ExecContext 实现 workflow/core.DBClient.ExecContext。
func (c *SQLDBClientCompat) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return c.db.ExecContext(ctx, query, args...)
}

// QueryContext 实现 workflow/core.DBClient.QueryContext。
func (c *SQLDBClientCompat) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return c.db.QueryContext(ctx, query, args...)
}

// QueryRowContext 实现 workflow/core.DBClient.QueryRowContext。
func (c *SQLDBClientCompat) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return c.db.QueryRowContext(ctx, query, args...)
}

// DB 返回底层 *sql.DB。
func (c *SQLDBClientCompat) DB() *sql.DB {
	return c.db
}

// =============================================================================
// 类型断言检查
// =============================================================================

var (
	_ PostgreSQLClient = (*SQLDBAdapter)(nil)
	_ PostgreSQLClient = (*SQLTxAdapter)(nil)
	_ DBClient         = (*SQLDBClientCompat)(nil)
	_ Row              = (*sqlRowAdapter)(nil)
	_ Rows             = (*sqlRowsAdapter)(nil)
)
