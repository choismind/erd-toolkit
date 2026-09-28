package dbreverse

import (
	"context"
	"database/sql"
)

// mysqlDialect는 접속한 데이터베이스 하나만 읽는다. MySQL에서는
// 데이터베이스가 곧 스키마이므로 페이지도 하나다.
type mysqlDialect struct{}

func (mysqlDialect) Name() string { return "mysql" }

func (mysqlDialect) Schemas(ctx context.Context, db *sql.DB) ([]string, error) {
	var name sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&name); err != nil {
		return nil, err
	}
	if !name.Valid || name.String == "" {
		return nil, errNoDatabaseSelected
	}
	return []string{name.String}, nil
}

func (mysqlDialect) Tables(ctx context.Context, db *sql.DB, schema string) ([]Table, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT c.TABLE_NAME, c.COLUMN_NAME, c.COLUMN_TYPE, c.IS_NULLABLE, c.ORDINAL_POSITION
		  FROM information_schema.COLUMNS c
		  JOIN information_schema.TABLES t
		    ON t.TABLE_SCHEMA = c.TABLE_SCHEMA AND t.TABLE_NAME = c.TABLE_NAME
		 WHERE c.TABLE_SCHEMA = ? AND t.TABLE_TYPE = 'BASE TABLE'
		 ORDER BY c.TABLE_NAME, c.ORDINAL_POSITION`, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byTable := map[string][]Column{}
	var order []string
	for rows.Next() {
		var tbl, col, typ, nullable string
		var ord int
		if err := rows.Scan(&tbl, &col, &typ, &nullable, &ord); err != nil {
			return nil, err
		}
		if _, ok := byTable[tbl]; !ok {
			order = append(order, tbl)
		}
		byTable[tbl] = append(byTable[tbl], Column{
			Name: col, Type: typ, Nullable: nullable == "YES", Ordinal: ord,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	pk, uniq, err := mysqlKeyIndexes(ctx, db, schema)
	if err != nil {
		return nil, err
	}

	out := make([]Table, 0, len(order))
	for _, tbl := range order {
		cols := byTable[tbl]
		for i := range cols {
			key := tbl + "\x00" + cols[i].Name
			cols[i].PK = pk[key]
			cols[i].Unique = uniq[key]
		}
		out = append(out, Table{Name: tbl, Columns: cols})
	}
	return out, nil
}

// mysqlKeyIndexes는 PK 참여 컬럼과 «단일 컬럼» 유일 인덱스 컬럼을 모은다.
// information_schema.COLUMNS.COLUMN_KEY를 쓰지 않는 이유: 복합 UNIQUE의
// 첫 컬럼에도 UNI가 붙으므로 «단일 컬럼 유일»과 구분되지 않는다.
func mysqlKeyIndexes(ctx context.Context, db *sql.DB, schema string) (pk, uniq map[string]bool, err error) {
	pk, uniq = map[string]bool{}, map[string]bool{}

	rows, err := db.QueryContext(ctx, `
		SELECT s.TABLE_NAME, s.INDEX_NAME, s.COLUMN_NAME, cnt.n
		  FROM information_schema.STATISTICS s
		  JOIN (SELECT TABLE_NAME, INDEX_NAME, COUNT(*) AS n
		          FROM information_schema.STATISTICS
		         WHERE TABLE_SCHEMA = ? AND NON_UNIQUE = 0
		         GROUP BY TABLE_NAME, INDEX_NAME) cnt
		    ON cnt.TABLE_NAME = s.TABLE_NAME AND cnt.INDEX_NAME = s.INDEX_NAME
		 WHERE s.TABLE_SCHEMA = ? AND s.NON_UNIQUE = 0`, schema, schema)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var tbl, idx, col string
		var n int
		if err := rows.Scan(&tbl, &idx, &col, &n); err != nil {
			return nil, nil, err
		}
		key := tbl + "\x00" + col
		if idx == "PRIMARY" {
			// PRIMARY도 NON_UNIQUE=0이라 이 결과에 함께 실려 온다. 여기서
			// 갈라내지 않으면 단일 컬럼 PK가 uniq에도 들어가고, DDL에
			// PRIMARY KEY와 UNIQUE가 겹쳐 나간다.
			pk[key] = true
			continue
		}
		if n == 1 {
			uniq[key] = true
		}
	}
	return pk, uniq, rows.Err()
}

func (mysqlDialect) ForeignKeys(ctx context.Context, db *sql.DB, schema string) ([]ForeignKey, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT CONSTRAINT_NAME, TABLE_NAME, REFERENCED_TABLE_SCHEMA,
		       REFERENCED_TABLE_NAME, COLUMN_NAME, REFERENCED_COLUMN_NAME
		  FROM information_schema.KEY_COLUMN_USAGE
		 WHERE TABLE_SCHEMA = ? AND REFERENCED_TABLE_NAME IS NOT NULL
		 ORDER BY CONSTRAINT_NAME, TABLE_NAME, ORDINAL_POSITION`, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanForeignKeyRows(rows)
}
