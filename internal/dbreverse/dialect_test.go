package dbreverse

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

type fakeDialect struct {
	schemas []string
	tables  map[string][]Table
	fks     map[string][]ForeignKey
	err     error
}

func (f fakeDialect) Name() string { return "fake" }
func (f fakeDialect) Schemas(context.Context, *sql.DB) ([]string, error) {
	return f.schemas, f.err
}
func (f fakeDialect) Tables(_ context.Context, _ *sql.DB, s string) ([]Table, error) {
	return f.tables[s], nil
}
func (f fakeDialect) ForeignKeys(_ context.Context, _ *sql.DB, s string) ([]ForeignKey, error) {
	return f.fks[s], nil
}

func TestIntrospect_SortsEverything(t *testing.T) {
	d := fakeDialect{
		schemas: []string{"zoo", "app"},
		tables: map[string][]Table{
			"app": {{Name: "b"}, {Name: "a", Columns: []Column{{Name: "y", Ordinal: 2}, {Name: "x", Ordinal: 1}}}},
			"zoo": {{Name: "c"}},
		},
		fks: map[string][]ForeignKey{},
	}
	got, err := Introspect(context.Background(), d, nil)
	if err != nil {
		t.Fatalf("Introspect: %v", err)
	}
	if len(got) != 2 || got[0].Name != "app" || got[1].Name != "zoo" {
		t.Fatalf("스키마가 이름 오름차순이 아니다: %+v", got)
	}
	if got[0].Tables[0].Name != "a" {
		t.Errorf("테이블이 이름 오름차순이 아니다: %+v", got[0].Tables)
	}
	if got[0].Tables[0].Columns[0].Name != "x" {
		t.Errorf("컬럼이 Ordinal 오름차순이 아니다: %+v", got[0].Tables[0].Columns)
	}
}

func TestIntrospect_PropagatesError(t *testing.T) {
	d := fakeDialect{err: errors.New("boom")}
	if _, err := Introspect(context.Background(), d, nil); err == nil {
		t.Fatal("에러를 삼키면 안 된다")
	}
}

func TestDialectFor_KnownDrivers(t *testing.T) {
	for _, name := range []string{DriverSQLite, DriverPostgres, DriverMySQL} {
		if _, err := DialectFor(name); err != nil {
			t.Errorf("DialectFor(%q): %v", name, err)
		}
	}
	if _, err := DialectFor("oracle"); err == nil {
		t.Error("모르는 드라이버는 에러여야 한다")
	}
}

// TestScanForeignKeyRows_GroupsCompositeConstraints는 PostgreSQL과 MySQL이
// 공유하는 FK 행 스캔을 서버 없이 검사한다. 두 DBMS 테스트는 서버가 없으면
// 건너뛰므로, 이것이 없으면 이 헬퍼에 아무 검사도 닿지 않는다.
// SQLite로 «6열 결과»만 만들어 먹인다 — 스캐너는 어느 DB인지 모른다.
func TestScanForeignKeyRows_GroupsCompositeConstraints(t *testing.T) {
	db, err := sql.Open(DriverSQLite, ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	// (제약이름, 자식테이블, 부모스키마, 부모테이블, 자식컬럼, 부모컬럼)
	rows, err := db.Query(`
		          SELECT 'fk_a' , 'child', 'main', 'parent', 'a1', 'p1'
		UNION ALL SELECT 'fk_a' , 'child', 'main', 'parent', 'a2', 'p2'
		UNION ALL SELECT 'fk_b' , 'child', 'main', 'other' , 'b1', 'q1'`)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()

	fks, err := scanForeignKeyRows(rows)
	if err != nil {
		t.Fatalf("scanForeignKeyRows: %v", err)
	}
	if len(fks) != 2 {
		t.Fatalf("FK 개수 = %d; 제약 이름이 둘이므로 2여야 한다: %+v", len(fks), fks)
	}
	if fks[0].Name != "fk_a" || len(fks[0].Columns) != 2 {
		t.Errorf("복합 FK가 한 제약으로 묶이지 않았다: %+v", fks[0])
	}
	if fks[0].Columns[0].Column != "a1" || fks[0].Columns[1].TargetColumn != "p2" {
		t.Errorf("컬럼 쌍이 순서대로 쌓이지 않았다: %+v", fks[0].Columns)
	}
	if fks[1].Name != "fk_b" || fks[1].TargetTable != "other" {
		t.Errorf("두 번째 제약이 틀렸다: %+v", fks[1])
	}
}
