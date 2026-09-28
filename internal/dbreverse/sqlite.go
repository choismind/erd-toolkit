package dbreverse

import (
	"context"
	"database/sql"
	"fmt"
)

// sqliteDialect는 SQLite 카탈로그를 읽는다. 스키마 개념이 없으므로
// 페이지는 언제나 "main" 하나다.
type sqliteDialect struct{}

func (sqliteDialect) Name() string { return "sqlite" }

func (sqliteDialect) Schemas(context.Context, *sql.DB) ([]string, error) {
	return []string{"main"}, nil
}

func (sqliteDialect) Tables(ctx context.Context, db *sql.DB, _ string) ([]Table, error) {
	names, err := sqliteTableNames(ctx, db)
	if err != nil {
		return nil, err
	}
	out := make([]Table, 0, len(names))
	for _, name := range names {
		cols, err := sqliteColumns(ctx, db, name)
		if err != nil {
			return nil, fmt.Errorf("테이블 %q: %w", name, err)
		}
		out = append(out, Table{Name: name, Columns: cols})
	}
	return out, nil
}

// sqliteTableNames는 사용자 테이블 이름을 모은다. sqlite_로 시작하는
// 내부 테이블은 뺀다(스펙 "제외 대상").
func sqliteTableNames(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT name FROM sqlite_master
		  WHERE type='table' AND name NOT LIKE 'sqlite_%'
		  ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

func sqliteColumns(ctx context.Context, db *sql.DB, table string) ([]Column, error) {
	// pragma_table_info는 테이블 값 함수라 바인드 파라미터를 받는다.
	rows, err := db.QueryContext(ctx,
		`SELECT cid, name, type, "notnull", pk FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []Column
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		if err := rows.Scan(&cid, &name, &typ, &notNull, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, Column{
			Name: name, Type: typ, Nullable: notNull == 0,
			Ordinal: cid + 1, PK: pk > 0,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	uniq, err := sqliteSingleColumnUniques(ctx, db, table)
	if err != nil {
		return nil, err
	}
	for i := range cols {
		if uniq[cols[i].Name] {
			cols[i].Unique = true
		}
	}
	return cols, nil
}

// sqliteSingleColumnUniques는 «단일 컬럼» 유일 인덱스가 걸린 컬럼 이름을
// 모은다. 복합 UNIQUE는 컬럼 하나에 표시할 방법이 없으므로 뺀다.
//
// origin='pk'를 빼는 이유: 기본키가 만드는 암묵 인덱스도 unique=1이라 여기에
// 걸린다. 그것까지 세면 DDL에 PRIMARY KEY와 UNIQUE가 겹쳐 나간다. 실측으로
// INTEGER PRIMARY KEY는 rowid 별칭이라 인덱스를 안 만들고, 그 밖의 기본키와
// WITHOUT ROWID 테이블은 origin='pk'인 인덱스를 만든다.
func sqliteSingleColumnUniques(ctx context.Context, db *sql.DB, table string) (map[string]bool, error) {
	idxNames, err := sqliteQueryStrings(ctx, db,
		`SELECT il.name FROM pragma_index_list(?) AS il
		  WHERE il."unique" = 1 AND il.origin <> 'pk'`, table)
	if err != nil {
		return nil, err
	}

	out := map[string]bool{}
	for _, idx := range idxNames {
		cols, err := sqliteQueryStrings(ctx, db, `SELECT name FROM pragma_index_info(?)`, idx)
		if err != nil {
			return nil, err
		}
		if len(cols) == 1 {
			out[cols[0]] = true
		}
	}
	return out, nil
}

// sqliteQueryStrings는 한 컬럼짜리 결과를 모은다. pragma_* 함수를 여러 번
// 부르는 자리마다 같은 열고-읽고-닫는 코드를 되풀이하지 않으려는 것이다.
func sqliteQueryStrings(ctx context.Context, db *sql.DB, query string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (sqliteDialect) ForeignKeys(ctx context.Context, db *sql.DB, _ string) ([]ForeignKey, error) {
	tables, err := sqliteTableNames(ctx, db)
	if err != nil {
		return nil, err
	}
	var out []ForeignKey
	for _, table := range tables {
		fks, err := sqliteForeignKeysOf(ctx, db, table)
		if err != nil {
			return nil, fmt.Errorf("테이블 %q: %w", table, err)
		}
		out = append(out, fks...)
	}
	return out, nil
}

func sqliteForeignKeysOf(ctx context.Context, db *sql.DB, table string) ([]ForeignKey, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, seq, "table", "from", "to" FROM pragma_foreign_key_list(?) ORDER BY id, seq`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byID := map[int]*ForeignKey{}
	var order []int
	type missingTarget struct {
		fk  *ForeignKey
		pos int // fk.Columns 안의 자리
		seq int // 부모 PK의 몇 번째 컬럼인가
	}
	var missing []missingTarget

	for rows.Next() {
		var id, seq int
		var target, from string
		var to sql.NullString
		if err := rows.Scan(&id, &seq, &target, &from, &to); err != nil {
			return nil, err
		}
		fk, ok := byID[id]
		if !ok {
			// SQLite에는 제약 이름이 없다. 정렬 기준이 필요하므로 만들어 붙인다.
			// 테이블 이름을 접두로 둬야 테이블 사이에서도 안정적으로 정렬된다.
			fk = &ForeignKey{
				Name:         fmt.Sprintf("%s_fk%d", table, id),
				Table:        table,
				TargetSchema: "main",
				TargetTable:  target,
			}
			byID[id] = fk
			order = append(order, id)
		}
		fk.Columns = append(fk.Columns, ForeignKeyColumn{Column: from, TargetColumn: to.String})
		if !to.Valid || to.String == "" {
			// REFERENCES parent 처럼 컬럼을 생략하면 부모의 PK를 가리킨다.
			// 여기서 바로 조회하면 rows가 열린 채 같은 커넥션에 두 번째
			// 질의를 걸게 되므로, 읽기가 끝난 뒤로 미룬다.
			missing = append(missing, missingTarget{fk: fk, pos: len(fk.Columns) - 1, seq: seq})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	for _, m := range missing {
		pk, err := sqlitePrimaryKeyColumn(ctx, db, m.fk.TargetTable, m.seq)
		if err != nil {
			return nil, err
		}
		m.fk.Columns[m.pos].TargetColumn = pk
	}

	out := make([]ForeignKey, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

// sqlitePrimaryKeyColumn은 부모 테이블 PK의 seq번째 컬럼 이름을 돌려준다.
func sqlitePrimaryKeyColumn(ctx context.Context, db *sql.DB, table string, seq int) (string, error) {
	names, err := sqliteQueryStrings(ctx, db,
		`SELECT name FROM pragma_table_info(?) WHERE pk > 0 ORDER BY pk`, table)
	if err != nil {
		return "", err
	}
	if seq < 0 || seq >= len(names) {
		return "", fmt.Errorf("부모 %q의 PK %d번째 컬럼을 찾을 수 없다", table, seq)
	}
	return names[seq], nil
}
