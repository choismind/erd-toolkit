package dbreverse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConns(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "erdtool.connections.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadConnections(t *testing.T) {
	p := writeConns(t, `
connections:
  PostgreSQL:
    dsn: postgres://chois@localhost/mydb
  shop:
    dsn: mysql://chois:pw@localhost/shop
`)
	c, err := LoadConnections(p)
	if err != nil {
		t.Fatalf("LoadConnections: %v", err)
	}
	if c.Connections["PostgreSQL"].DSN != "postgres://chois@localhost/mydb" {
		t.Errorf("PostgreSQL = %+v", c.Connections["PostgreSQL"])
	}
	if len(c.Connections) != 2 {
		t.Errorf("항목 개수 = %d; want 2", len(c.Connections))
	}
}

func TestResolve_SchemeWins(t *testing.T) {
	c := Connections{Connections: map[string]ConnectionEntry{
		"postgres://x": {DSN: "이건 쓰이면 안 된다"},
	}}
	got, err := Resolve("postgres://chois@localhost/mydb", c)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Driver != DriverPostgres {
		t.Errorf("Driver = %q; want %q", got.Driver, DriverPostgres)
	}
	if got.Label != "mydb" {
		t.Errorf("Label = %q; want %q (출력 파일 이름의 바탕)", got.Label, "mydb")
	}
}

func TestResolve_SectionName(t *testing.T) {
	c := Connections{Connections: map[string]ConnectionEntry{
		"shop": {DSN: "mysql://chois:pw@localhost:3306/shopdb"},
	}}
	got, err := Resolve("shop", c)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Driver != DriverMySQL {
		t.Errorf("Driver = %q; 어느 DBMS인지는 섹션 이름이 아니라 DSN 스킴으로 정한다", got.Driver)
	}
	if got.Label != "shopdb" {
		t.Errorf("Label = %q; want %q", got.Label, "shopdb")
	}
	// 보고용 문자열에 비밀번호가 남으면 안 된다.
	if got.Display == got.DSN {
		t.Errorf("Display = %q; 마스킹돼야 한다", got.Display)
	}
	if strings.Contains(got.Display, "pw") {
		t.Errorf("Display = %q; 비밀번호가 남아 있다", got.Display)
	}
}

func TestResolve_FilePathIsSQLite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "app.db")
	if err := os.WriteFile(p, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(p, Connections{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Driver != DriverSQLite {
		t.Errorf("Driver = %q; want %q", got.Driver, DriverSQLite)
	}
	if got.Label != "app" {
		t.Errorf("Label = %q; want %q", got.Label, "app")
	}
}

func TestResolve_UnknownSchemeIsRefusedWithList(t *testing.T) {
	_, err := Resolve("oracle://scott@localhost/orcl", Connections{})
	if err == nil {
		t.Fatal("모르는 스킴은 거부해야 한다")
	}
	// 지원 목록을 알려줘야 한다 — 추측하게 두면 안 된다.
	for _, want := range []string{"postgres", "mysql"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("에러 문구에 %q가 없다: %v", want, err)
		}
	}
}

func TestResolve_ErrorNeverLeaksPassword(t *testing.T) {
	c := Connections{Connections: map[string]ConnectionEntry{
		"bad": {DSN: "oracle://scott:tiger@localhost/orcl"},
	}}
	_, err := Resolve("bad", c)
	if err == nil {
		t.Fatal("모르는 스킴은 거부해야 한다")
	}
	if strings.Contains(err.Error(), "tiger") {
		t.Errorf("에러에 비밀번호가 새어 나왔다: %v", err)
	}
}

// mysql:// URL을 네이티브 DSN으로 옮기는 규칙을 고정한다. 사용자에게는 세
// DBMS가 같은 모양으로 보여야 하는데 그 드라이버만 URL을 안 받는다.
func TestMySQLNativeDSN(t *testing.T) {
	cases := []struct{ in, want string }{
		{"mysql://chois:pw@localhost:3306/shop", "chois:pw@tcp(localhost:3306)/shop"},
		{"mysql://chois@localhost/shop", "chois@tcp(localhost:3306)/shop"},
		{"mysql://chois:pw@localhost/shop?parseTime=true", "chois:pw@tcp(localhost:3306)/shop?parseTime=true"},
	}
	for _, c := range cases {
		got, err := mysqlNativeDSN(c.in)
		if err != nil {
			t.Errorf("mysqlNativeDSN(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("mysqlNativeDSN(%q) = %q; want %q", c.in, got, c.want)
		}
	}
	if _, err := mysqlNativeDSN("mysql://chois@localhost"); err == nil {
		t.Error("DB 이름이 없으면 거부해야 한다 — 어느 DB를 그릴지 추측할 수 없다")
	}
}

// 섹션 이름을 오타 내면 «스킴도 섹션도 아니니 SQLite 파일이겠지»가 되어,
// 없는 파일을 새 DB로 만들어 놓고 테이블 0개짜리 산출물을 성공이라고
// 보고했다. 역공학은 «이미 있는» DB를 읽는 일이므로 없는 파일은 거부한다.
func TestResolve_NonexistentFileIsRefused(t *testing.T) {
	c := Connections{Connections: map[string]ConnectionEntry{
		"PostgreSQL": {DSN: "postgres://chois@localhost/mydb"},
	}}
	_, err := Resolve("PostgresQL", c) // 대소문자를 틀린 오타
	if err == nil {
		t.Fatal("없는 파일을 SQLite DB로 만들어 내면 안 된다")
	}
	// 무엇을 시도했는지 말해야 한다 — 추측하게 두지 않는다.
	if !strings.Contains(err.Error(), "PostgreSQL") {
		t.Errorf("에러가 쓸 수 있는 접속 이름을 알려주지 않는다: %v", err)
	}
}

// 폴더를 주면 거부한다. sql.Open은 폴더에도 성공하고 조회할 때에야 깨진다.
func TestResolve_DirectoryIsRefused(t *testing.T) {
	if _, err := Resolve(t.TempDir(), Connections{}); err == nil {
		t.Fatal("폴더는 DB 파일이 아니다")
	}
}

// mysql:// URL의 사용자정보는 URL 규칙대로 «퍼센트 디코딩»해야 한다.
// 비밀번호에 :나 @가 들어가면 URL 문법상 이스케이프가 강제되고, PostgreSQL
// 쪽은 pgx가 이미 디코딩한다 — 두 DBMS가 같은 모양으로 보여야 하므로
// MySQL만 원문을 넘기면 안 된다. 실제로 비밀번호에 *가 든 계정으로
// 「Access denied」가 났다(2026-08-28).
func TestMySQLNativeDSN_PercentDecodesUserinfo(t *testing.T) {
	cases := []struct{ in, want string }{
		{"mysql://u:p%2Aw@localhost:3306/db", "u:p*w@tcp(localhost:3306)/db"},
		{"mysql://u:p%40ss@localhost:3306/db", "u:p@ss@tcp(localhost:3306)/db"},
		{"mysql://u:p%3Aw@localhost:3306/db", "u:p:w@tcp(localhost:3306)/db"},
		{"mysql://us%65r@localhost:3306/db", "user@tcp(localhost:3306)/db"},
		// 이스케이프가 없으면 예전과 한 글자도 다르지 않아야 한다.
		{"mysql://chois:pw@localhost:3306/shop", "chois:pw@tcp(localhost:3306)/shop"},
	}
	for _, c := range cases {
		got, err := mysqlNativeDSN(c.in)
		if err != nil {
			t.Errorf("mysqlNativeDSN(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("mysqlNativeDSN(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

// 망가진 이스케이프는 조용히 넘기지 않는다. 그대로 넘기면 「Access denied」
// 라는, 원인을 알 수 없는 에러로 나타난다.
func TestMySQLNativeDSN_RejectsBadEscapeWithoutLeakingIt(t *testing.T) {
	_, err := mysqlNativeDSN("mysql://u:pa%zzss@localhost/db")
	if err == nil {
		t.Fatal("올바르지 않은 퍼센트 이스케이프는 거부해야 한다")
	}
	if strings.Contains(err.Error(), "pa%zzss") {
		t.Errorf("에러에 사용자정보가 새어 나왔다: %v", err)
	}
	// 어떻게 고치는지 알려줘야 한다.
	if !strings.Contains(err.Error(), "%25") {
		t.Errorf("에러가 고치는 법을 알려주지 않는다: %v", err)
	}
}

// 사용자설명서가 접속 설정에 SQLite를 «dsn: ./memo.db»로 적는 예를 싣고
// 있다. 섹션의 dsn을 위치 인자와 같은 규칙으로 풀지 않으면 그 예가 거짓이
// 된다 — 파일 경로 전체가 「스킴」으로 찍히며 거부됐다(2026-09-22 실측).
func TestResolve_SectionCanHoldSQLitePath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "memo.db")
	if err := os.WriteFile(p, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	c := Connections{Connections: map[string]ConnectionEntry{
		"쪽지": {DSN: p},
	}}
	got, err := Resolve("쪽지", c)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Driver != DriverSQLite {
		t.Errorf("Driver = %q; want %q", got.Driver, DriverSQLite)
	}
	if got.Label != "memo" {
		t.Errorf("Label = %q; want %q (출력 파일 이름의 바탕)", got.Label, "memo")
	}
	if got.Display != p {
		t.Errorf("Display = %q; want %q", got.Display, p)
	}
}

// 섹션이 가리킨 SQLite 파일이 없으면, 무엇을 시도했는지 말해야 한다.
// 「지원하지 않는 스킴」은 여기서 나올 문구가 아니다.
func TestResolve_SectionWithMissingSQLiteFileSaysSo(t *testing.T) {
	c := Connections{Connections: map[string]ConnectionEntry{
		"쪽지": {DSN: filepath.Join(t.TempDir(), "없는것.db")},
	}}
	_, err := Resolve("쪽지", c)
	if err == nil {
		t.Fatal("없는 파일은 거부해야 한다")
	}
	if strings.Contains(err.Error(), "스킴") {
		t.Errorf("파일 경로를 스킴이라 부르고 있다: %v", err)
	}
	if !strings.Contains(err.Error(), "쪽지") {
		t.Errorf("어느 접속인지 말해야 한다: %v", err)
	}
}
