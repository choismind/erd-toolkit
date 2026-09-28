package dbreverse

import (
	"database/sql"
	"testing"
)

// TestDriversAreRegistered는 위 상수가 실제 등록 이름과 일치함을 고정한다.
// 이름을 손으로 지어냈다면 여기서 잡힌다.
func TestDriversAreRegistered(t *testing.T) {
	want := map[string]bool{DriverSQLite: false, DriverPostgres: false, DriverMySQL: false}
	for _, name := range sql.Drivers() {
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("드라이버 %q가 등록되지 않았다. sql.Drivers() = %v", name, sql.Drivers())
		}
	}
}

// TestSQLiteRoundTripsInMemory는 modernc.org/sqlite가 이 환경에서 실제로
// 도는지(cgo 없이) 확인한다. 이후 모든 밀폐 테스트가 여기에 기댄다.
func TestSQLiteRoundTripsInMemory(t *testing.T) {
	db, err := sql.Open(DriverSQLite, ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM t`).Scan(&n); err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if n != 0 {
		t.Errorf("count = %d; want 0", n)
	}
}
