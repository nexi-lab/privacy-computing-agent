package main

import (
	"database/sql"
	"fmt"
	"sync"

	_ "github.com/go-sql-driver/mysql"
)

var (
	db   *sql.DB
	once sync.Once
)

// ================================
// 1. 获取 DB（支持多次调用，只初始化一次）
// ================================
func GetDB(dsn string) (*sql.DB, error) {
	var err error
	once.Do(func() {
		db, err = sql.Open("mysql", dsn)
		if err != nil {
			return
		}
		// 设置连接池
		db.SetMaxOpenConns(20)
		db.SetMaxIdleConns(10)
		err = db.Ping()
	})

	return db, err
}

// ================================
// 2. 判断表是否存在
// ================================
func TableExists(db *sql.DB, database, table string) (bool, error) {
	var exists string
	query := `
		SELECT TABLE_NAME 
		FROM information_schema.TABLES 
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
	`
	err := db.QueryRow(query, database, table).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// ================================
// 3. 获取表数据量
// ================================
func TableCount(db *sql.DB, database, table string) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s.%s", database, table)
	var count int64
	err := db.QueryRow(query).Scan(&count)
	return count, err
}

// ================================
// 4. 获取表内容（limit/offset 可选）
// ================================
func GetTableRows(db *sql.DB, sqlstr string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := db.Query(sqlstr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	result := make([]map[string]interface{}, 0)

	for rows.Next() {
		columnVals := make([]interface{}, len(cols))
		columnPtrs := make([]interface{}, len(cols))

		for i := range columnVals {
			columnPtrs[i] = &columnVals[i]
		}

		err := rows.Scan(columnPtrs...)
		if err != nil {
			return nil, err
		}

		rowMap := map[string]interface{}{}
		for i, col := range cols {
			rowMap[col] = columnVals[i]
		}

		result = append(result, rowMap)
	}

	return result, nil
}

func ExecSQL(db *sql.DB, sqlText string, args ...interface{}) error {
	_, err := db.Exec(sqlText, args...)
	return err
}
