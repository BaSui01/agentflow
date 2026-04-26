package database

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// =============================================================================
// SQLDBAdapter 测试
// =============================================================================

func TestSQLDBAdapter_Exec(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	adapter := NewSQLDBAdapter(db)

	mock.ExpectExec("INSERT INTO test").
		WithArgs(1, "test").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = adapter.Exec(context.Background(), "INSERT INTO test (id, name) VALUES (?, ?)", 1, "test")
	if err != nil {
		t.Errorf("Exec failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestSQLDBAdapter_Exec_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	adapter := NewSQLDBAdapter(db)

	expectedErr := errors.New("exec error")
	mock.ExpectExec("INSERT INTO test").
		WillReturnError(expectedErr)

	err = adapter.Exec(context.Background(), "INSERT INTO test (id) VALUES (?)", 1)
	if err == nil {
		t.Error("expected error, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestSQLDBAdapter_Query(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	adapter := NewSQLDBAdapter(db)

	rows := sqlmock.NewRows([]string{"id", "name"}).
		AddRow(1, "test1").
		AddRow(2, "test2")

	mock.ExpectQuery("SELECT id, name FROM test").
		WillReturnRows(rows)

	resultRows, err := adapter.Query(context.Background(), "SELECT id, name FROM test")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer resultRows.Close()

	var count int
	for resultRows.Next() {
		var id int
		var name string
		if err := resultRows.Scan(&id, &name); err != nil {
			t.Errorf("Scan failed: %v", err)
		}
		count++
	}

	if count != 2 {
		t.Errorf("expected 2 rows, got %d", count)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestSQLDBAdapter_Query_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	adapter := NewSQLDBAdapter(db)

	expectedErr := errors.New("query error")
	mock.ExpectQuery("SELECT").
		WillReturnError(expectedErr)

	_, err = adapter.Query(context.Background(), "SELECT * FROM test")
	if err == nil {
		t.Error("expected error, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestSQLDBAdapter_QueryRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	adapter := NewSQLDBAdapter(db)

	rows := sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "test")
	mock.ExpectQuery("SELECT id, name FROM test WHERE id = ?").
		WithArgs(1).
		WillReturnRows(rows)

	row := adapter.QueryRow(context.Background(), "SELECT id, name FROM test WHERE id = ?", 1)

	var id int
	var name string
	if err := row.Scan(&id, &name); err != nil {
		t.Errorf("Scan failed: %v", err)
	}

	if id != 1 || name != "test" {
		t.Errorf("expected id=1, name=test, got id=%d, name=%s", id, name)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestSQLDBAdapter_DB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	adapter := NewSQLDBAdapter(db)

	if adapter.DB() != db {
		t.Error("DB() did not return the underlying db")
	}

	// Use mock to avoid linter warning
	_ = mock
}

// =============================================================================
// SQLTxAdapter 测试
// =============================================================================

func TestSQLTxAdapter_Exec(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO test").
		WithArgs(1, "test").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}

	adapter := NewSQLTxAdapter(tx)

	err = adapter.Exec(context.Background(), "INSERT INTO test (id, name) VALUES (?, ?)", 1, "test")
	if err != nil {
		t.Errorf("Exec failed: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Errorf("Commit failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestSQLTxAdapter_Query(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	rows := sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "test")
	mock.ExpectQuery("SELECT").
		WillReturnRows(rows)
	mock.ExpectCommit()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}

	adapter := NewSQLTxAdapter(tx)

	resultRows, err := adapter.Query(context.Background(), "SELECT id, name FROM test")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer resultRows.Close()

	if !resultRows.Next() {
		t.Error("expected one row")
	}

	var id int
	var name string
	if err := resultRows.Scan(&id, &name); err != nil {
		t.Errorf("Scan failed: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Errorf("Commit failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestSQLTxAdapter_QueryRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	rows := sqlmock.NewRows([]string{"count"}).AddRow(42)
	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(rows)
	mock.ExpectCommit()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}

	adapter := NewSQLTxAdapter(tx)

	row := adapter.QueryRow(context.Background(), "SELECT COUNT(*) FROM test")

	var count int
	if err := row.Scan(&count); err != nil {
		t.Errorf("Scan failed: %v", err)
	}

	if count != 42 {
		t.Errorf("expected count=42, got %d", count)
	}

	if err := tx.Commit(); err != nil {
		t.Errorf("Commit failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestSQLTxAdapter_Tx(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectRollback()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}

	adapter := NewSQLTxAdapter(tx)

	if adapter.Tx() != tx {
		t.Error("Tx() did not return the underlying transaction")
	}

	_ = tx.Rollback()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// =============================================================================
// SQLDBClientCompat 测试
// =============================================================================

func TestSQLDBClientCompat_ExecContext(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	compat := NewSQLDBClientCompat(db)

	mock.ExpectExec("INSERT INTO test").
		WithArgs(1, "test").
		WillReturnResult(sqlmock.NewResult(1, 1))

	result, err := compat.ExecContext(context.Background(), "INSERT INTO test (id, name) VALUES (?, ?)", 1, "test")
	if err != nil {
		t.Errorf("ExecContext failed: %v", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected != 1 {
		t.Errorf("expected 1 row affected, got %d", rowsAffected)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestSQLDBClientCompat_QueryContext(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	compat := NewSQLDBClientCompat(db)

	rows := sqlmock.NewRows([]string{"id"}).AddRow(1).AddRow(2)
	mock.ExpectQuery("SELECT id FROM test").
		WillReturnRows(rows)

	resultRows, err := compat.QueryContext(context.Background(), "SELECT id FROM test")
	if err != nil {
		t.Fatalf("QueryContext failed: %v", err)
	}
	defer resultRows.Close()

	var count int
	for resultRows.Next() {
		var id int
		if err := resultRows.Scan(&id); err != nil {
			t.Errorf("Scan failed: %v", err)
		}
		count++
	}

	if count != 2 {
		t.Errorf("expected 2 rows, got %d", count)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestSQLDBClientCompat_QueryRowContext(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	compat := NewSQLDBClientCompat(db)

	rows := sqlmock.NewRows([]string{"name"}).AddRow("test")
	mock.ExpectQuery("SELECT name FROM test WHERE id = ?").
		WithArgs(1).
		WillReturnRows(rows)

	row := compat.QueryRowContext(context.Background(), "SELECT name FROM test WHERE id = ?", 1)

	var name string
	if err := row.Scan(&name); err != nil {
		t.Errorf("Scan failed: %v", err)
	}

	if name != "test" {
		t.Errorf("expected name=test, got %s", name)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestSQLDBClientCompat_DB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	compat := NewSQLDBClientCompat(db)

	if compat.DB() != db {
		t.Error("DB() did not return the underlying db")
	}

	// Use mock to avoid linter warning
	_ = mock
}

// =============================================================================
// 接口兼容性测试
// =============================================================================

func TestPostgreSQLClient_Interface_Compatibility(t *testing.T) {
	// 验证 SQLDBAdapter 实现了 PostgreSQLClient 接口
	var _ PostgreSQLClient = NewSQLDBAdapter(nil)

	// 验证 SQLTxAdapter 实现了 PostgreSQLClient 接口
	var _ PostgreSQLClient = NewSQLTxAdapter(nil)

	// 由于 sql.DB 和 sql.Tx 不能为 nil，这里只做类型检查
	t.Log("interface compatibility verified")
}

func TestSQLDBClientCompat_ReturnsSqlTypes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	compat := NewSQLDBClientCompat(db)

	// 测试 ExecContext 返回 sql.Result
	mock.ExpectExec("DELETE").WillReturnResult(sqlmock.NewResult(0, 5))
	result, err := compat.ExecContext(context.Background(), "DELETE FROM test")
	if err != nil {
		t.Errorf("ExecContext failed: %v", err)
	}
	if result == nil {
		t.Error("expected sql.Result, got nil")
	}

	// 测试 QueryContext 返回 *sql.Rows
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	rows, err := compat.QueryContext(context.Background(), "SELECT 1")
	if err != nil {
		t.Errorf("QueryContext failed: %v", err)
	}
	if rows == nil {
		t.Error("expected *sql.Rows, got nil")
	}
	_ = rows.Close()

	// 测试 QueryRowContext 返回 *sql.Row
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	row := compat.QueryRowContext(context.Background(), "SELECT 1")
	if row == nil {
		t.Error("expected *sql.Row, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}
