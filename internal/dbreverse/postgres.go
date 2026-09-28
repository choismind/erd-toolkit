package dbreverse

import (
	"context"
	"database/sql"
)

// postgresDialect는 pg_catalog를 직접 읽는다. information_schema보다
// format_type()으로 «카탈로그 원문 그대로의 타입»을 얻기 쉽다.
type postgresDialect struct{}

func (postgresDialect) Name() string { return "postgres" }

// Schemas는 «보통 테이블이 실제로 들어 있는» 사용자 스키마만 돌려준다.
// 빈 스키마를 페이지로 만들면 테이블 0개짜리 빈 페이지가 나온다.
func (postgresDialect) Schemas(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT n.nspname
		  FROM pg_namespace n
		 WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
		   AND n.nspname NOT LIKE 'pg\_toast%'
		   AND n.nspname NOT LIKE 'pg\_temp%'
		   AND EXISTS (SELECT 1 FROM pg_class c
		                WHERE c.relnamespace = n.oid AND c.relkind = 'r')
		 ORDER BY n.nspname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (postgresDialect) Tables(ctx context.Context, db *sql.DB, schema string) ([]Table, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT c.relname, a.attname,
		       format_type(a.atttypid, a.atttypmod),
		       a.attnotnull, a.attnum
		  FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		  JOIN pg_attribute a ON a.attrelid = c.oid
		 WHERE n.nspname = $1 AND c.relkind = 'r'
		   AND a.attnum > 0 AND NOT a.attisdropped
		 ORDER BY c.relname, a.attnum`, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byTable := map[string][]Column{}
	var order []string
	for rows.Next() {
		var tbl, col, typ string
		var notNull bool
		var attnum int
		if err := rows.Scan(&tbl, &col, &typ, &notNull, &attnum); err != nil {
			return nil, err
		}
		if _, ok := byTable[tbl]; !ok {
			order = append(order, tbl)
		}
		byTable[tbl] = append(byTable[tbl], Column{
			Name: col, Type: typ, Nullable: !notNull, Ordinal: attnum,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	pk, uniq, err := postgresKeyConstraints(ctx, db, schema)
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

// postgresKeyConstraints는 PK 참여 컬럼과 «단일 컬럼» 유일 제약 컬럼을 모은다.
func postgresKeyConstraints(ctx context.Context, db *sql.DB, schema string) (pk, uniq map[string]bool, err error) {
	pk, uniq = map[string]bool{}, map[string]bool{}
	rows, err := db.QueryContext(ctx, `
		SELECT c.relname, a.attname, k.contype, array_length(k.conkey, 1)
		  FROM pg_constraint k
		  JOIN pg_class c      ON c.oid = k.conrelid
		  JOIN pg_namespace n  ON n.oid = c.relnamespace
		  JOIN unnest(k.conkey) AS ck(attnum) ON true
		  JOIN pg_attribute a  ON a.attrelid = c.oid AND a.attnum = ck.attnum
		 WHERE n.nspname = $1 AND k.contype IN ('p', 'u')`, schema)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var tbl, col, contype string
		var n int
		if err := rows.Scan(&tbl, &col, &contype, &n); err != nil {
			return nil, nil, err
		}
		key := tbl + "\x00" + col
		if contype == "p" {
			// 기본키 제약은 그 자체로 유일하지만 «별도 UNIQUE»가 아니다.
			// 여기서 갈라내지 않으면 단일 컬럼 PK가 uniq에도 들어가고,
			// DDL에 PRIMARY KEY와 UNIQUE가 겹쳐 나간다.
			pk[key] = true
			continue
		}
		if n == 1 {
			uniq[key] = true
		}
	}
	return pk, uniq, rows.Err()
}

// ForeignKeys는 scanForeignKeyRows가 읽을 수 있도록 MySQL과 «같은 6개 열을
// 같은 순서로» 돌려준다.
func (postgresDialect) ForeignKeys(ctx context.Context, db *sql.DB, schema string) ([]ForeignKey, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT k.conname, c.relname, tn.nspname, tc.relname, a.attname, ta.attname
		  FROM pg_constraint k
		  JOIN pg_class c       ON c.oid = k.conrelid
		  JOIN pg_namespace n   ON n.oid = c.relnamespace
		  JOIN pg_class tc      ON tc.oid = k.confrelid
		  JOIN pg_namespace tn  ON tn.oid = tc.relnamespace
		  JOIN LATERAL unnest(k.conkey, k.confkey) WITH ORDINALITY AS u(child, parent, ord) ON true
		  JOIN pg_attribute a   ON a.attrelid = c.oid  AND a.attnum = u.child
		  JOIN pg_attribute ta  ON ta.attrelid = tc.oid AND ta.attnum = u.parent
		 WHERE n.nspname = $1 AND k.contype = 'f'
		 ORDER BY k.conname, c.relname, u.ord`, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanForeignKeyRows(rows)
}
