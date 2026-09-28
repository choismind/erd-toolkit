package dbreverse

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"erdtool/internal/drawio"
	"erdtool/internal/genbuild"
)

const sqliteDDL = `
CREATE TABLE customer (
  id   INTEGER PRIMARY KEY,
  name VARCHAR(50) NOT NULL,
  email VARCHAR(200) UNIQUE
);
CREATE TABLE orders (
  id          INTEGER PRIMARY KEY,
  customer_id INTEGER NOT NULL REFERENCES customer(id),
  memo        TEXT
);
`

func newSQLiteDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "probe.db")
	db, err := sql.Open(DriverSQLite, path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(sqliteDDL); err != nil {
		t.Fatalf("DDL: %v", err)
	}
	return db
}

func TestSQLiteDialect_ReadsColumnsAndKeys(t *testing.T) {
	db := newSQLiteDB(t)
	schemas, err := Introspect(context.Background(), sqliteDialect{}, db)
	if err != nil {
		t.Fatalf("Introspect: %v", err)
	}
	if len(schemas) != 1 || schemas[0].Name != "main" {
		t.Fatalf("스키마 = %+v; SQLite는 main 하나여야 한다", schemas)
	}
	tables := schemas[0].Tables
	if len(tables) != 2 || tables[0].Name != "customer" || tables[1].Name != "orders" {
		t.Fatalf("테이블 = %+v", tables)
	}
	cust := tables[0]
	if !cust.Columns[0].PK {
		t.Error("customer.id가 PK로 안 읽혔다")
	}
	if cust.Columns[1].Nullable {
		t.Error("customer.name은 NOT NULL이다")
	}
	if !cust.Columns[2].Unique {
		t.Error("customer.email의 단일 컬럼 UNIQUE가 안 읽혔다")
	}
	if got := cust.Columns[1].Type; got != "VARCHAR(50)" {
		t.Errorf("타입 = %q; 카탈로그 원문 그대로여야 한다", got)
	}
}

func TestSQLiteDialect_ReadsForeignKeys(t *testing.T) {
	db := newSQLiteDB(t)
	fks, err := sqliteDialect{}.ForeignKeys(context.Background(), db, "main")
	if err != nil {
		t.Fatalf("ForeignKeys: %v", err)
	}
	if len(fks) != 1 {
		t.Fatalf("FK 개수 = %d; want 1 (%+v)", len(fks), fks)
	}
	fk := fks[0]
	if fk.Table != "orders" || fk.TargetTable != "customer" {
		t.Errorf("FK 방향이 틀렸다: %+v", fk)
	}
	if len(fk.Columns) != 1 || fk.Columns[0].Column != "customer_id" || fk.Columns[0].TargetColumn != "id" {
		t.Errorf("FK 컬럼 쌍이 틀렸다: %+v", fk.Columns)
	}
	if fk.Name == "" {
		t.Error("SQLite는 제약 이름이 없으므로 만들어 붙여야 한다(정렬 기준이다)")
	}
}

// 이 테스트가 이 Phase 전체의 끝에서 끝까지 검증이다:
// 진짜 DB -> 역공학 -> genbuild.Build -> 실제 drawio 파서로 되읽기.
func TestSQLite_EndToEndRoundTripThroughRealParser(t *testing.T) {
	db := newSQLiteDB(t)
	schemas, err := Introspect(context.Background(), sqliteDialect{}, db)
	if err != nil {
		t.Fatalf("Introspect: %v", err)
	}
	out, err := genbuild.Build(ToPageDefs(schemas).Pages)
	if err != nil {
		t.Fatalf("genbuild.Build: %v", err)
	}

	diags, err := drawio.LoadDiagramsBytes(out)
	if err != nil {
		t.Fatalf("LoadDiagramsBytes: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("페이지 개수 = %d; want 1", len(diags))
	}
	idx := drawio.BuildIndex(diags[0].Cells)
	tables := drawio.FindTables(diags[0].Cells)
	if len(tables) != 2 {
		t.Fatalf("되읽은 테이블 개수 = %d; want 2", len(tables))
	}

	got := map[string][]string{}
	for _, tb := range tables {
		var names []string
		for _, c := range drawio.ExtractColumns(tb, idx) {
			names = append(names, c.Name+" "+c.Type)
		}
		got[tb.Value] = names
	}
	want := map[string][]string{
		"customer": {"id INTEGER", "name VARCHAR(50)", "email VARCHAR(200)"},
		"orders":   {"id INTEGER", "customer_id INTEGER", "memo TEXT"},
	}
	for name, cols := range want {
		if len(got[name]) != len(cols) {
			t.Fatalf("%s 컬럼 = %v; want %v", name, got[name], cols)
		}
		for i := range cols {
			if got[name][i] != cols[i] {
				t.Errorf("%s 컬럼 %d = %q; want %q", name, i, got[name][i], cols[i])
			}
		}
	}

	rels := drawio.ExtractRelationships(diags[0].Cells, idx, tables)
	if len(rels) != 1 {
		t.Fatalf("되읽은 관계 개수 = %d; want 1", len(rels))
	}
}

// 멱등성: 같은 DB를 두 번 읽으면 바이트가 같아야 한다.
func TestSQLite_IsIdempotent(t *testing.T) {
	db := newSQLiteDB(t)
	build := func() []byte {
		schemas, err := Introspect(context.Background(), sqliteDialect{}, db)
		if err != nil {
			t.Fatalf("Introspect: %v", err)
		}
		out, err := genbuild.Build(ToPageDefs(schemas).Pages)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		return out
	}
	if string(build()) != string(build()) {
		t.Error("같은 DB를 두 번 읽었는데 바이트가 다르다")
	}
}

// REFERENCES에 컬럼을 생략하면 pragma_foreign_key_list의 "to"가 NULL이고,
// 부모의 PK를 따로 찾아야 한다. 그 조회는 FK 읽기가 «끝난 뒤»에 해야 한다 —
// 같은 커넥션에서 rows를 열어 둔 채 두 번째 질의를 걸 수 없기 때문이다.
func TestSQLiteDialect_ForeignKeyWithoutTargetColumnResolvesToPK(t *testing.T) {
	path := filepath.Join(t.TempDir(), "implicit.db")
	db, err := sql.Open(DriverSQLite, path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE parent (pid INTEGER PRIMARY KEY, tag TEXT);
		CREATE TABLE child (
		  id INTEGER PRIMARY KEY,
		  parent_id INTEGER NOT NULL REFERENCES parent
		);
	`); err != nil {
		t.Fatalf("DDL: %v", err)
	}

	fks, err := sqliteDialect{}.ForeignKeys(context.Background(), db, "main")
	if err != nil {
		t.Fatalf("ForeignKeys: %v", err)
	}
	if len(fks) != 1 || len(fks[0].Columns) != 1 {
		t.Fatalf("FK = %+v", fks)
	}
	if got := fks[0].Columns[0].TargetColumn; got != "pid" {
		t.Errorf("TargetColumn = %q; 생략된 대상은 부모의 PK여야 한다", got)
	}
}

// TestSQLiteDialect_PKIndexIsNotUnique는 기본키가 만드는 암묵 인덱스를
// «단일 컬럼 UNIQUE»로 세지 않는지 본다. 그것까지 세면 정의 셀에
// "code VARCHAR(10) NOT NULL UNIQUE"가 적히고 DDL에 PRIMARY KEY와 UNIQUE가
// 겹쳐 나간다.
//
// 별도 DB를 쓰는 이유: 위 sqliteDDL의 id는 INTEGER PRIMARY KEY라 rowid
// 별칭이고, SQLite는 그 경우 인덱스를 아예 만들지 않아 이 결함이 안 드러난다
// (실측: pragma_index_list가 빈 결과). 인덱스가 생기는 두 모양으로 잰다.
func TestSQLiteDialect_PKIndexIsNotUnique(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pk.db")
	db, err := sql.Open(DriverSQLite, path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`
CREATE TABLE code_pk (code VARCHAR(10) PRIMARY KEY, x INT);
CREATE TABLE no_rowid (k INT PRIMARY KEY, y INT) WITHOUT ROWID;
`); err != nil {
		t.Fatalf("DDL: %v", err)
	}

	schemas, err := Introspect(context.Background(), sqliteDialect{}, db)
	if err != nil {
		t.Fatalf("Introspect: %v", err)
	}
	for _, tbl := range schemas[0].Tables {
		c := tbl.Columns[0]
		if !c.PK {
			t.Errorf("%s.%s가 PK로 안 읽혔다", tbl.Name, c.Name)
		}
		if c.Unique {
			t.Errorf("%s.%s: 기본키의 암묵 인덱스를 별도 UNIQUE로 셌다", tbl.Name, c.Name)
		}
	}
}
