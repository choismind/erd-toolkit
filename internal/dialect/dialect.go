// internal/dialect/dialect.go
//
// **쓰기 쪽에서 타깃 DBMS를 정한다.** `internal/dbreverse`의 `Dialect`는
// DB에서 «읽을 때» 카탈로그를 어떻게 물어보는가이고, 이쪽은 ERD에서 «쓸 때»
// 어느 DBMS 문법으로 낼 것인가다. 읽기는 예전부터 DBMS를 셋 갈랐는데 쓰기는
// 안 갈랐다 — 그래서 식별자는 ANSI 큰따옴표로 감싸면서 타입은 원본 DB의
// 카탈로그 원문을 그대로 옮겨 적어, 두 DBMS 문법이 한 파일에 섞여
// 나갔다(2026-09-04 실측).
package dialect

import (
	"fmt"
	"sort"
	"strings"
)

// Dialect는 뽑을 SQL이 겨누는 DBMS다.
type Dialect string

const (
	// ANSI는 타깃을 지정하지 않았을 때의 기준이다(2026-09-04 소유자 결정).
	// «아무 데나 맞춘다»가 아니라 «표준을 겨눈다»는 뜻이다.
	ANSI       Dialect = "ansi"
	PostgreSQL Dialect = "postgres"
	MySQL      Dialect = "mysql"
	SQLite     Dialect = "sqlite"
)

// labels는 사람에게 보이는 이름이다. 산출물 머리에 이 이름이 박힌다 —
// 그래야 SQL 파일만 남았을 때도 어느 DB용인지 알 수 있다.
var labels = map[Dialect]string{
	ANSI:       "ANSI (지정 안 함)",
	PostgreSQL: "PostgreSQL",
	MySQL:      "MySQL",
	SQLite:     "SQLite",
}

// aliases는 사용자가 칠 법한 표기를 받아 준다. `postgres`와 `postgresql`을
// 두고 어느 쪽이 맞는지 기억하게 만들 이유가 없다.
var aliases = map[string]Dialect{
	"":           ANSI,
	"ansi":       ANSI,
	"standard":   ANSI,
	"sql":        ANSI,
	"postgres":   PostgreSQL,
	"postgresql": PostgreSQL,
	"pg":         PostgreSQL,
	"mysql":      MySQL,
	"mariadb":    MySQL,
	"sqlite":     SQLite,
	"sqlite3":    SQLite,
}

// Parse는 설정 파일이나 --dialect로 들어온 값을 타깃 DBMS로 바꾼다.
//
// 모르는 값은 **거부한다.** 조용히 ANSI로 떨어뜨리면 오타(`--dialect postgre`)가
// 기본값으로 흘러가 사용자는 자기가 지정한 대로 뽑혔다고 믿는다.
func Parse(s string) (Dialect, error) {
	d, ok := aliases[strings.ToLower(strings.TrimSpace(s))]
	if !ok {
		return "", fmt.Errorf("모르는 타깃 DBMS %q — 쓸 수 있는 값: %s", s, Names())
	}
	return d, nil
}

// Names는 쓸 수 있는 값을 사람이 읽을 한 줄로 만든다. 에러 문구가 «무엇을
// 칠 수 있는지»를 함께 말해야 사용자가 문서를 뒤지지 않는다.
func Names() string {
	seen := map[Dialect]bool{}
	var out []string
	for name, d := range aliases {
		if name == "" || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, string(d))
	}
	sort.Strings(out)
	return strings.Join(out, " / ")
}

// resolve는 제로값을 ANSI로 푼다. `Dialect("")`는 설정 파일의 빈 `dialect:`와
// `Options{}`의 기본값 둘 다에서 나오는데, 그 둘은 «지정 안 함»이지 «타깃 DBMS가
// 없음»이 아니다. 여기서 풀지 않으면 산출물 머리가 빈 칸이 되어, 조용히
// 넘어가지 않겠다는 애초의 목적이 무너진다.
func (d Dialect) resolve() Dialect {
	if d == "" {
		return ANSI
	}
	return d
}

// Label은 산출물에 박을 이름이다.
func (d Dialect) Label() string {
	if l, ok := labels[d.resolve()]; ok {
		return l
	}
	return string(d)
}

// QuoteIdent는 식별자를 그 타깃 DBMS의 방식으로 감싼다.
//
// ERD의 테이블/컬럼명은 사람이 draw.io에서 자유롭게 타이핑한 값이라 한글·공백·
// 예약어가 그대로 들어온다 — quote 없이 찍으면 `CREATE TABLE 주문 정보 (`
// 같은 깨진 DDL이 조용히 생성된다.
//
// 감싸는 문자가 이름 안에 있으면 그 문자를 두 번 반복해 이스케이프한다. 이건
// ANSI 큰따옴표와 MySQL 백틱이 같은 규칙이다.
func (d Dialect) QuoteIdent(name string) string {
	q := d.quoteChar()
	return q + strings.ReplaceAll(name, q, q+q) + q
}

// quoteChar는 그 타깃 DBMS의 식별자 인용 문자다. MySQL만 백틱이고 나머지는 ANSI를
// 따른다 — SQLite는 둘 다 받지만 표준 쪽을 쓴다.
func (d Dialect) quoteChar() string {
	if d.resolve() == MySQL {
		return "`"
	}
	return `"`
}
