package dbreverse

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConnectionEntry는 접속 하나다. 지금은 dsn 하나뿐이지만 맵 형태로 둬
// 나중에 항목을 늘릴 자리를 남긴다.
type ConnectionEntry struct {
	DSN string `yaml:"dsn"`
}

// Connections는 접속 설정 파일 전체다. 섹션 이름은 자유이며, 어느 DBMS인지는
// 섹션 이름이 아니라 DSN 스킴으로 정한다 — 그래야 PostgreSQL 인스턴스를
// dev/prod 두 개 둘 수 있다.
type Connections struct {
	Connections map[string]ConnectionEntry `yaml:"connections"`
}

// LoadConnections는 접속 설정 파일을 읽는다. **읽기만 한다** — 이 파일에는
// 비밀번호가 평문으로 있고, erdtool은 그 값을 어디에도 다시 쓰지 않는다.
func LoadConnections(path string) (Connections, error) {
	var c Connections
	data, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := yaml.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("접속 설정 %q: %w", path, err)
	}
	return c, nil
}

// Target은 위치 인자 해석 결과다. Display에는 비밀번호가 없으므로
// 보고와 에러에는 언제나 이쪽을 쓴다.
type Target struct {
	Driver  string
	DSN     string
	Display string
	Label   string
}

// Resolve는 위치 인자 하나를 접속 대상으로 푼다.
//
// 판별 순서: 스킴이 있으면 DSN, 없으면 설정 파일의 섹션 이름, 그것도
// 없으면 SQLite 파일 경로. 어느 것도 아니면 지원 스킴을 나열한 에러를
// 낸다 — 추측하지 않는다.
func Resolve(arg string, conns Connections) (Target, error) {
	if strings.Contains(arg, "://") {
		return fromDSN(arg)
	}
	if entry, ok := conns.Connections[arg]; ok {
		t, err := resolveEntry(entry.DSN)
		if err != nil {
			return Target{}, fmt.Errorf("접속 %q: %w", arg, err)
		}
		return t, nil
	}
	// 스킴도 없고 섹션도 아니면 SQLite 파일이다.
	if t, ok := sqliteTarget(arg); ok {
		return t, nil
	}
	st, _ := os.Stat(arg)
	return Target{}, unresolvableError(arg, conns, st != nil && st.IsDir())
}

// resolveEntry는 접속 설정 섹션의 dsn 값을 푼다. **위치 인자와 같은 규칙**이다 —
// 스킴이 있으면 DSN이고, 없으면 SQLite 파일 경로다.
//
// 규칙을 갈라 두면 사용자설명서가 싣고 있는 «dsn: ./memo.db»가 거짓이 된다.
// 실제로 그랬다(2026-09-22): fromDSN만 부르고 있어서 파일 경로 전체가
// 「지원하지 않는 스킴」으로 찍혔다.
func resolveEntry(dsn string) (Target, error) {
	if strings.Contains(dsn, "://") {
		return fromDSN(dsn)
	}
	if t, ok := sqliteTarget(dsn); ok {
		return t, nil
	}
	// 파일 경로에는 비밀번호가 없으므로 값을 그대로 보여 준다.
	return Target{}, fmt.Errorf("SQLite 파일 경로로 봤지만 %s", missingFileText(dsn))
}

// sqliteTarget은 파일 경로를 SQLite 접속 대상으로 만든다. **있는 파일이어야**
// 한다 — 역공학은 이미 있는 DB를 읽는 일이다. 확인하지 않으면 이름 오타
// 하나가 「없는 이름 -> SQLite 파일로 간주 -> 드라이버가 빈 DB를 새로 만듦
// -> 테이블 0개짜리 산출물」로 흘러가, 툴이 하지도 않은 일을 했다고
// 보고하면서 쓰레기 파일까지 남긴다.
func sqliteTarget(path string) (Target, bool) {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return Target{}, false
	}
	base := filepath.Base(path)
	return Target{
		Driver:  DriverSQLite,
		DSN:     path,
		Display: path,
		Label:   strings.TrimSuffix(base, filepath.Ext(base)),
	}, true
}

// missingFileText는 「왜 파일로 못 봤는지」를 한 조각으로 만든다. 위치 인자와
// 섹션이 같은 문구를 쓰도록 한 자리에 둔다.
func missingFileText(path string) string {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		return "폴더다: " + path
	}
	return "그런 파일이 없다: " + path
}

// unresolvableError는 무엇을 시도했는지 전부 말한다. 추측하게 두지 않는
// 것이 이 저장소의 규칙이다.
func unresolvableError(arg string, conns Connections, isDir bool) error {
	what := "그런 파일이 없다"
	if isDir {
		what = "폴더다"
	}
	names := make([]string, 0, len(conns.Connections))
	for n := range conns.Connections {
		names = append(names, n)
	}
	sort.Strings(names)
	known := "접속 설정에 등록된 이름이 없다"
	if len(names) > 0 {
		known = "설정에 있는 접속 이름: " + strings.Join(names, ", ")
	}
	return fmt.Errorf(
		"접속 대상으로 풀 수 없다: %q (스킴이 없고, %s. SQLite 파일 경로로 봤지만 %s). %s",
		arg, known, what, "DSN을 직접 주거나 --connections로 설정 파일을 지정한다")
}

func fromDSN(dsn string) (Target, error) {
	scheme := dsn
	if i := strings.Index(dsn, "://"); i >= 0 {
		scheme = dsn[:i]
	}
	switch scheme {
	case "postgres", "postgresql":
		return Target{
			Driver: DriverPostgres, DSN: dsn,
			Display: MaskDSN(dsn), Label: databaseNameOf(dsn),
		}, nil
	case "mysql":
		native, err := mysqlNativeDSN(dsn)
		if err != nil {
			return Target{}, err
		}
		return Target{
			Driver: DriverMySQL, DSN: native,
			Display: MaskDSN(dsn), Label: databaseNameOf(dsn),
		}, nil
	default:
		// 비밀번호가 들어 있을 수 있으므로 DSN 원문을 에러에 넣지 않는다.
		return Target{}, fmt.Errorf(
			"지원하지 않는 스킴 %q (지원: postgres://, postgresql://, mysql://, 또는 SQLite 파일 경로)",
			scheme)
	}
}

// decodeUserinfo는 URL 사용자정보를 퍼센트 디코딩한다.
//
// URL 문법에서 `:`와 `@`는 구분자이므로 비밀번호에 그것들을 쓰려면
// 퍼센트 이스케이프가 강제된다. PostgreSQL 쪽은 pgx가 URL을 직접 파싱하며
// 이미 디코딩하므로, MySQL만 원문을 넘기면 «같은 모양으로 보여야 한다»는
// 이 서브커맨드의 전제가 깨진다. 실제로 비밀번호에 *가 든 계정이
// `%2A`로 적혀 「Access denied」가 났다(2026-08-28 실측).
//
// 사용자와 비밀번호를 «첫 콜론»으로 갈라 각각 디코딩한다. 통째로 디코딩하면
// 비밀번호 안의 `%3A`가 구분자로 되살아나 경계가 어긋난다.
func decodeUserinfo(userinfo string) (string, error) {
	user, pass, hasPass := strings.Cut(userinfo, ":")
	du, err := url.PathUnescape(user)
	if err != nil {
		return "", errBadUserinfoEscape
	}
	if !hasPass {
		return du, nil
	}
	dp, err := url.PathUnescape(pass)
	if err != nil {
		return "", errBadUserinfoEscape
	}
	return du + ":" + dp, nil
}

// errBadUserinfoEscape는 값을 담지 않는다 — 그 값이 비밀번호다.
var errBadUserinfoEscape = errors.New(
	"DSN의 사용자정보에 올바르지 않은 퍼센트 이스케이프가 있다 " +
		"(비밀번호에 %를 쓰려면 %25로, :는 %3A로, @는 %40으로 적는다)")

// databaseNameOf는 URL의 경로 부분에서 DB 이름을 뽑는다. 출력 파일 이름의
// 바탕이 된다.
func databaseNameOf(dsn string) string {
	rest := dsn
	if i := strings.Index(dsn, "://"); i >= 0 {
		rest = dsn[i+3:]
	}
	if i := strings.Index(rest, "?"); i >= 0 {
		rest = rest[:i]
	}
	slash := strings.LastIndex(rest, "/")
	if slash < 0 || slash+1 >= len(rest) {
		return "database"
	}
	return rest[slash+1:]
}

// mysqlNativeDSN은 mysql:// URL을 go-sql-driver/mysql의 네이티브 DSN으로
// 옮긴다. 사용자에게는 세 DBMS가 같은 모양(스킴 있는 URL)으로 보여야 하는데,
// 그 드라이버만 URL을 안 받기 때문이다.
//
//	mysql://user:pw@localhost:3306/shop?parseTime=true
//	-> user:pw@tcp(localhost:3306)/shop?parseTime=true
func mysqlNativeDSN(dsn string) (string, error) {
	rest := strings.TrimPrefix(dsn, "mysql://")

	params := ""
	if i := strings.Index(rest, "?"); i >= 0 {
		params = rest[i:]
		rest = rest[:i]
	}

	at := strings.LastIndex(rest, "@")
	userinfo := ""
	hostpath := rest
	if at >= 0 {
		decoded, err := decodeUserinfo(rest[:at])
		if err != nil {
			return "", err
		}
		userinfo = decoded
		hostpath = rest[at+1:]
	}

	slash := strings.Index(hostpath, "/")
	if slash < 0 {
		return "", errNoDatabaseSelected
	}
	host := hostpath[:slash]
	dbname := hostpath[slash+1:]
	if dbname == "" {
		return "", errNoDatabaseSelected
	}
	if host == "" {
		host = "localhost:3306"
	} else if !strings.Contains(host, ":") {
		host += ":3306"
	}
	if userinfo != "" {
		userinfo += "@"
	}
	return fmt.Sprintf("%stcp(%s)/%s%s", userinfo, host, dbname, params), nil
}
