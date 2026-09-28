package pipeline

import (
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"erdtool/internal/dbreverse"
)

func makeSQLiteFile(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "app.db")
	db, err := sql.Open(dbreverse.DriverSQLite, p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE customer (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE orders (id INTEGER PRIMARY KEY,
		                     customer_id INTEGER NOT NULL REFERENCES customer(id));
	`); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReverse_SQLiteFileWritesDrawio(t *testing.T) {
	dir := t.TempDir()
	dbPath := makeSQLiteFile(t, dir)

	res, err := Reverse(dbPath, "", "")
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	want := filepath.Join(dir, "app.drawio")
	if res.OutputPath != want {
		t.Errorf("OutputPath = %q; want %q", res.OutputPath, want)
	}
	if res.Pages != 1 || res.Tables != 2 || res.Relations != 1 {
		t.Errorf("통계 = {Pages:%d Tables:%d Relations:%d}; want {1 2 1}", res.Pages, res.Tables, res.Relations)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("산출물이 없다: %v", err)
	}
}

// SQLite 파일 위에 덮어쓰면 DB가 사라진다. build가 설계서를 지키려고 넣은
// 가드와 같은 이유로 막는다.
func TestReverse_RefusesToOverwriteInputDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := makeSQLiteFile(t, dir)
	before, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Reverse(dbPath, dbPath, ""); err == nil {
		t.Fatal("입력 DB 위에 덮어쓰는 것을 막아야 한다")
	}
	after, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("막았다고 하면서 파일이 바뀌었다")
	}
}

// 산출물에 접속 정보가 들어가면 안 된다. 산출물은 커밋되고 공유된다.
func TestReverse_OutputContainsNoConnectionInfo(t *testing.T) {
	dir := t.TempDir()
	dbPath := makeSQLiteFile(t, dir)
	res, err := Reverse(dbPath, "", "")
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	out, err := os.ReadFile(res.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), dbPath) {
		t.Error("산출물에 입력 경로가 박혔다")
	}
}

// 같은 DB를 두 번 역공학하면 바이트가 같아야 한다.
func TestReverse_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := makeSQLiteFile(t, dir)

	res1, err := Reverse(dbPath, "", "")
	if err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(res1.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Reverse(dbPath, "", ""); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(res1.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Error("두 번 돌렸는데 바이트가 다르다")
	}
}

// 한국어 로케일 PostgreSQL은 인증 실패 문구를 UTF-8이 아닌 서버 로케일
// 인코딩(CP949)으로 보낸다. 접속에 실패한 순간이 사용자에게 가장 중요한
// 순간이므로, 그 문구가 스크롤백에 깨져 나오면 안 된다.
//
// 영어 로케일 서버에서는 문구가 ASCII라 이 테스트가 그냥 통과한다.
// 그래도 «사용자에게 나가는 에러는 유효한 UTF-8이다»라는 성질 자체는
// 어느 서버에서든 옳으므로 그대로 검사한다.
func TestReverse_ConnectionErrorIsValidUTF8(t *testing.T) {
	dsn := os.Getenv("ERDTOOL_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("ERDTOOL_TEST_PG_DSN이 없다. PostgreSQL 접속 실패 문구 테스트를 건너뛴다.")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("ERDTOOL_TEST_PG_DSN을 URL로 못 읽었다: %v", err)
	}
	// 반드시 인증에 실패하도록 없는 계정으로 바꿔 붙는다.
	u.User = url.UserPassword("erdtool_no_such_user", "wrong")

	_, err = Reverse(u.String(), filepath.Join(t.TempDir(), "out.drawio"), "")
	if err == nil {
		t.Fatal("없는 계정으로 붙었는데 성공했다")
	}
	if !utf8.ValidString(err.Error()) {
		t.Errorf("접속 실패 문구가 유효한 UTF-8이 아니다: %q", err.Error())
	}
}
