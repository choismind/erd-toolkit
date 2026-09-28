package dbreverse

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
)

func openPostgresForTest(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("ERDTOOL_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("ERDTOOL_TEST_PG_DSN이 없다. PostgreSQL 테스트를 건너뛴다.")
	}
	db, err := sql.Open(DriverPostgres, dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	return db
}

const pgDDL = `
DROP SCHEMA IF EXISTS erdtool_probe CASCADE;
CREATE SCHEMA erdtool_probe;
CREATE TABLE erdtool_probe.customer (
  id    integer PRIMARY KEY,
  name  character varying(50) NOT NULL,
  email character varying(200) UNIQUE
);
CREATE TABLE erdtool_probe.orders (
  id          integer PRIMARY KEY,
  customer_id integer NOT NULL REFERENCES erdtool_probe.customer(id),
  created_at  timestamp with time zone
);
`

func TestPostgresDialect_ReadsCatalog(t *testing.T) {
	db := openPostgresForTest(t)
	if _, err := db.Exec(pgDDL); err != nil {
		t.Fatalf("DDL: %v", err)
	}
	t.Cleanup(func() { db.Exec("DROP SCHEMA IF EXISTS erdtool_probe CASCADE") })

	tables, err := postgresDialect{}.Tables(context.Background(), db, "erdtool_probe")
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	if len(tables) != 2 {
		t.Fatalf("테이블 개수 = %d; want 2 (%+v)", len(tables), tables)
	}

	var orders *Table
	for i := range tables {
		if tables[i].Name == "orders" {
			orders = &tables[i]
		}
	}
	if orders == nil {
		t.Fatal("orders를 못 찾았다")
	}
	// 이 두 줄이 작업 1이 존재하는 이유다.
	if got := orders.Columns[2].Type; got != "timestamp with time zone" {
		t.Errorf("created_at 타입 = %q; want timestamp with time zone", got)
	}

	var cust *Table
	for i := range tables {
		if tables[i].Name == "customer" {
			cust = &tables[i]
		}
	}
	if cust == nil {
		t.Fatal("customer를 못 찾았다")
	}
	if got := cust.Columns[1].Type; got != "character varying(50)" {
		t.Errorf("name 타입 = %q; want character varying(50)", got)
	}
	if !cust.Columns[0].PK {
		t.Error("customer.id가 PK로 안 읽혔다")
	}
	// 기본키 제약(contype='p')은 그 자체가 유일하지만 «별도 UNIQUE»가
	// 아니다. 둘을 섞으면 정의 셀에 UNIQUE가 적히고 DDL에 PRIMARY KEY와
	// 겹쳐 나간다.
	if cust.Columns[0].Unique {
		t.Error("customer.id: 기본키 제약을 별도 UNIQUE로 셌다")
	}
	if !cust.Columns[2].Unique {
		t.Error("customer.email의 단일 컬럼 UNIQUE가 안 읽혔다")
	}

	fks, err := postgresDialect{}.ForeignKeys(context.Background(), db, "erdtool_probe")
	if err != nil {
		t.Fatalf("ForeignKeys: %v", err)
	}
	if len(fks) != 1 || fks[0].Table != "orders" || fks[0].TargetTable != "customer" {
		t.Errorf("FK = %+v", fks)
	}
	if fks[0].TargetSchema != "erdtool_probe" {
		t.Errorf("TargetSchema = %q; 스키마 가로지르기 판정에 쓰인다", fks[0].TargetSchema)
	}
}

// 시스템 스키마는 페이지가 되면 안 된다.
func TestPostgresDialect_ExcludesSystemSchemas(t *testing.T) {
	db := openPostgresForTest(t)
	names, err := postgresDialect{}.Schemas(context.Background(), db)
	if err != nil {
		t.Fatalf("Schemas: %v", err)
	}
	for _, n := range names {
		if n == "pg_catalog" || n == "information_schema" || strings.HasPrefix(n, "pg_toast") || strings.HasPrefix(n, "pg_temp") {
			t.Errorf("시스템 스키마 %q가 포함됐다", n)
		}
	}
}
