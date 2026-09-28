package dbreverse

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
)

// Dialect는 DBMS 하나의 카탈로그 조회 방법이다. **행을 읽어 중간표현을
// 만드는 일만** 하고, 그것을 ERD 의미로 옮기는 판단은 하지 않는다
// (그건 cardinality.go와 topagedef.go의 순수 함수가 한다).
// 이 경계 덕분에 매핑 전체를 서버 없이 테스트할 수 있다.
type Dialect interface {
	// Name은 보고용 이름이다.
	Name() string
	// Schemas는 그릴 대상 스키마를 돌려준다. 시스템 스키마는 제외한다.
	Schemas(ctx context.Context, db *sql.DB) ([]string, error)
	// Tables는 스키마 하나의 BASE TABLE과 그 컬럼을 돌려준다.
	Tables(ctx context.Context, db *sql.DB, schema string) ([]Table, error)
	// ForeignKeys는 스키마 하나가 «가진» FK를 돌려준다(자식 쪽 기준).
	ForeignKeys(ctx context.Context, db *sql.DB, schema string) ([]ForeignKey, error)
}

// DialectFor는 database/sql 드라이버 이름에 맞는 DBMS 구현을 고른다.
func DialectFor(driver string) (Dialect, error) {
	switch driver {
	case DriverSQLite:
		return sqliteDialect{}, nil
	case DriverPostgres:
		return postgresDialect{}, nil
	case DriverMySQL:
		return mysqlDialect{}, nil
	default:
		return nil, fmt.Errorf("지원하지 않는 드라이버: %q (지원: %s, %s, %s)",
			driver, DriverSQLite, DriverPostgres, DriverMySQL)
	}
}

// Introspect는 DBMS 구현 하나로 DB 전체를 읽는다. 정렬은 여기서 한 번에 한다 —
// DBMS마다 따로 하면 한쪽만 빠뜨렸을 때 멱등성이 조용히 깨진다.
func Introspect(ctx context.Context, d Dialect, db *sql.DB) ([]Schema, error) {
	names, err := d.Schemas(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("%s: 스키마 목록: %w", d.Name(), err)
	}
	sort.Strings(names)

	out := make([]Schema, 0, len(names))
	for _, name := range names {
		tables, err := d.Tables(ctx, db, name)
		if err != nil {
			return nil, fmt.Errorf("%s: 스키마 %q 테이블: %w", d.Name(), name, err)
		}
		fks, err := d.ForeignKeys(ctx, db, name)
		if err != nil {
			return nil, fmt.Errorf("%s: 스키마 %q 외래키: %w", d.Name(), name, err)
		}
		s := Schema{Name: name, Tables: tables, ForeignKeys: fks}
		s.Sort()
		out = append(out, s)
	}
	return out, nil
}

var errNoDatabaseSelected = errors.New("DSN에 데이터베이스 이름이 없다 (예: mysql://user@host/DB이름)")

// scanForeignKeyRows는 (제약이름, 자식테이블, 부모스키마, 부모테이블,
// 자식컬럼, 부모컬럼) 6열을 제약 이름 순서로 읽어 ForeignKey로 묶는다.
// 복합 FK는 같은 제약 이름의 행이 여럿이므로 Columns에 쌓인다.
func scanForeignKeyRows(rows *sql.Rows) ([]ForeignKey, error) {
	var out []ForeignKey
	var cur *ForeignKey
	for rows.Next() {
		var name, table, targetSchema, targetTable, col, targetCol string
		if err := rows.Scan(&name, &table, &targetSchema, &targetTable, &col, &targetCol); err != nil {
			return nil, err
		}
		if cur == nil || cur.Name != name || cur.Table != table {
			out = append(out, ForeignKey{
				Name: name, Table: table,
				TargetSchema: targetSchema, TargetTable: targetTable,
			})
			cur = &out[len(out)-1]
		}
		cur.Columns = append(cur.Columns, ForeignKeyColumn{Column: col, TargetColumn: targetCol})
	}
	return out, rows.Err()
}
