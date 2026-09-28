package dbreverse

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
)

// openMySQLForTest는 ERDTOOL_TEST_MYSQL_DSN이 있을 때만 연다. 없으면
// 건너뛴다 — 서버 없이도 go test ./...가 깨지지 않아야 한다.
func openMySQLForTest(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("ERDTOOL_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("ERDTOOL_TEST_MYSQL_DSN이 없다. MySQL 테스트를 건너뛴다.")
	}
	db, err := sql.Open(DriverMySQL, dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	return db
}

const mysqlDDL = `
DROP TABLE IF EXISTS erdtool_orders;
DROP TABLE IF EXISTS erdtool_customer;
CREATE TABLE erdtool_customer (
  id    INT PRIMARY KEY,
  name  VARCHAR(50) NOT NULL,
  email VARCHAR(200) UNIQUE
);
CREATE TABLE erdtool_orders (
  id          INT PRIMARY KEY,
  customer_id INT NOT NULL,
  qty         INT UNSIGNED,
  CONSTRAINT erdtool_orders_fk FOREIGN KEY (customer_id) REFERENCES erdtool_customer(id)
);
`

func TestMySQLDialect_ReadsCatalog(t *testing.T) {
	db := openMySQLForTest(t)
	for _, stmt := range splitSQL(mysqlDDL) {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("DDL %q: %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		db.Exec("DROP TABLE IF EXISTS erdtool_orders")
		db.Exec("DROP TABLE IF EXISTS erdtool_customer")
	})

	schemas, err := Introspect(context.Background(), mysqlDialect{}, db)
	if err != nil {
		t.Fatalf("Introspect: %v", err)
	}
	if len(schemas) != 1 {
		t.Fatalf("MySQL은 접속한 DB 하나가 한 페이지다: %+v", schemas)
	}

	var orders *Table
	for i := range schemas[0].Tables {
		if schemas[0].Tables[i].Name == "erdtool_orders" {
			orders = &schemas[0].Tables[i]
		}
	}
	if orders == nil {
		t.Fatal("erdtool_orders를 못 찾았다")
	}
	// 공백 든 타입이 원문 그대로 와야 한다. Task 1이 이걸 위해 있다.
	var qty *Column
	for i := range orders.Columns {
		if orders.Columns[i].Name == "qty" {
			qty = &orders.Columns[i]
		}
	}
	if qty == nil || qty.Type != "int unsigned" {
		t.Errorf("qty 타입 = %+v; want int unsigned", qty)
	}

	var cust *Table
	for i := range schemas[0].Tables {
		if schemas[0].Tables[i].Name == "erdtool_customer" {
			cust = &schemas[0].Tables[i]
		}
	}
	if cust == nil {
		t.Fatal("erdtool_customer를 못 찾았다")
	}
	if !cust.Columns[0].PK {
		t.Error("erdtool_customer.id가 PK로 안 읽혔다")
	}
	// PRIMARY 인덱스는 NON_UNIQUE=0이라 «단일 컬럼 유일 인덱스» 조건에
	// 그대로 걸린다. 그것까지 세면 DDL에 PRIMARY KEY와 UNIQUE가 겹쳐 나간다.
	if cust.Columns[0].Unique {
		t.Error("erdtool_customer.id: PRIMARY 인덱스를 별도 UNIQUE로 셌다")
	}
	if !cust.Columns[2].Unique {
		t.Error("erdtool_customer.email의 단일 컬럼 UNIQUE가 안 읽혔다")
	}

	var fk *ForeignKey
	for i := range schemas[0].ForeignKeys {
		if schemas[0].ForeignKeys[i].Name == "erdtool_orders_fk" {
			fk = &schemas[0].ForeignKeys[i]
		}
	}
	if fk == nil {
		t.Fatalf("FK를 못 찾았다: %+v", schemas[0].ForeignKeys)
	}
	if fk.Table != "erdtool_orders" || fk.TargetTable != "erdtool_customer" {
		t.Errorf("FK 방향이 틀렸다: %+v", fk)
	}
}

// splitSQL은 세미콜론으로 끊는다. 테스트 DDL 전용이며 문자열 안의
// 세미콜론을 다루지 않는다 — 위 DDL에는 없다.
func splitSQL(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ";") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
