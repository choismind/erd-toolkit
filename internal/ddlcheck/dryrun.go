// internal/ddlcheck/dryrun.go
//
// 뽑아낸 DDL을 «SQL을 아는 것»에게 실제로 물어보는 층이다. 이 저장소는 SQL
// 파서를 짜지 않는다 — 대신 이미 들어 있는 순수 Go SQLite(reverse가 쓰던
// 그것)의 메모리 DB에 CREATE TABLE을 실행해 본다. 서버도, 파일도, 새
// 의존성도 필요 없다.
//
// **왜 SQLite인가.** 2026-09-04 기준 이 도구에는 타깃 DBMS라는 개념이 없고,
// 타깃이 없을 때의 기준은 ANSI다(소유자 결정). 그런데 ANSI 문법을 검사하는
// 성숙한 Go 라이브러리는 없다 — 쓸 만한 파서는 전부 특정 DBMS의 파서(MySQL은 TiDB,
// PostgreSQL은 pgparser)다. SQLite는 타입 이름에 관대해서 표준에서 크게 벗어난
// 것만 건다.
//
// **실측 (2026-09-04). 여기 적은 것 말고는 재 보지 않았다.**
//
//	잡는다  — NOT NULL 붙여쓰기(NOTNULL), 타입 괄호 안 닫힘, CHECK 식의 괄호
//	          오류, CHECK가 없는 컬럼을 참조, DEFAULT 따옴표 안 닫힘
//	통과시킨다 — `smallint unsigned`, `바르차`처럼 이름만 표준이 아닌 타입.
//	          그쪽은 non_ansi_type의 몫이다
//	걸린다  — MySQL의 `enum('G','PG')` · `set(...)`. 손대지 않은 sakila
//	          역공학본에서도 film 테이블 한 건이 걸린다
//
// 마지막 줄이 중요하다. **이 검사는 «SQL 일반»이 아니라 «SQLite»를 잰다.**
// enum은 SQL로 아예 못 쓸 것이 아니라 MySQL에만 있는 타입이며, ANSI 기준으로는 표준이
// 아닌 것이 맞아서 여기 걸리는 것 자체는 틀리지 않다. 다만 **왜 걸렸는지를
// 문구가 정직하게 말해야 한다** — 「SQL로 성립하지 않는다」고 하면 사용자는
// 자기 MySQL에서 잘 도는 타입을 두고 도구가 거짓말한다고 여긴다.
//
// 타깃 DBMS가 정해지면 이 층은 그 DBMS의 파서로 갈아끼우는 자리가 된다. 그때
// enum은 MySQL 타깃에서 통과해야 한다.
package ddlcheck

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	_ "modernc.org/sqlite"

	"erdtool/internal/model"
	"erdtool/internal/report"
	"erdtool/internal/validate"
)

// DryRun은 테이블마다 CREATE TABLE을 메모리 DB에 실행해 보고 실패를 진단으로
// 옮긴다.
//
// prior는 이미 나온 진단들이다. **구조가 성립하는 테이블만 문법을 묻는다** —
// 이름 없는 컬럼이나 중복 컬럼은 앞 단계(validate.columnFindings)가 이미
// 사람 말로 짚었고, 여기서 또 걸면 같은 실수가 두 줄로 보고된다. 사용자는
// 두 진단이 같은 것인지 다른 것인지 알 방법이 없다.
func DryRun(doc model.Document, prior []validate.Finding, opt report.Options) []validate.Finding {
	ddls := report.TableDDLs(doc, opt)
	if len(ddls) == 0 {
		return nil
	}

	// 앞 단계가 이미 짚은 테이블 — 페이지가 다르면 이름이 겹쳐도 다른
	// 테이블이므로 페이지 id와 함께 센다.
	flagged := map[string]bool{}
	for _, f := range prior {
		if f.TableName != "" {
			flagged[f.DiagramID+"\x00"+f.TableName] = true
		}
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		// 시험 실행(dry run)을 못 돌린 것을 «문제 없음»으로 삼키지 않는다. 검사가
		// 조용히 죽으면 리포트의 「발견된 문제 없음」이 거짓말이 된다.
		return []validate.Finding{{
			Rule: "ddl_dryrun_unavailable", Severity: validate.SeverityWarning,
			Message: fmt.Sprintf("DDL 문법 검사를 시작하지 못해 건너뜀: %v", err),
			Brief:   fmt.Sprintf("DDL 문법 검사를 시작하지 못해 건너뜀: %v", err),
			Scope:   validate.ScopeFile,
		}}
	}
	defer db.Close()

	var findings []validate.Finding
	for _, td := range ddls {
		if flagged[td.DiagramID+"\x00"+td.TableName] {
			continue
		}
		if _, err := db.Exec(td.SQL); err != nil {
			findings = append(findings, validate.Finding{
				Rule: "ddl_syntax", Severity: validate.SeverityWarning,
				Message: fmt.Sprintf("테이블 %q의 DDL이 SQL 엔진에서 거부됨: %v%s",
					td.TableName, err, columnHint(td, err)),
				Brief: fmt.Sprintf("만든 DDL이 SQL 엔진에서 거부됨: %v%s",
					err, columnHint(td, err)),
				Action:      "가리킨 자리의 정의 셀이나 제약 값을 고친다",
				Scope:       validate.ScopeTable,
				DiagramName: td.DiagramName, DiagramID: td.DiagramID,
				TableName: td.TableName, CellID: td.CellID,
			})
		}
	}
	return findings
}

// nearToken은 SQL 엔진 에러에서 문제가 된 낱말을 뽑는다.
// SQLite는 `near "NOTNULL": syntax error`처럼 낱말을 따옴표로 감싸 알려 준다.
var nearToken = regexp.MustCompile(`near "([^"]+)"|no such column: (\S+)`)

// columnHint는 «어느 컬럼인가»를 덧붙인다. 엔진이 알려 준 낱말이 DDL의 컬럼
// 줄 하나에만 나오면 그 컬럼을 지목하고, 여러 줄에 나오거나 못 찾으면 아무
// 말도 하지 않는다 — 틀린 자리를 짚으면 없느니만 못하다.
func columnHint(td report.TableDDL, err error) string {
	m := nearToken.FindStringSubmatch(err.Error())
	if m == nil {
		return ""
	}
	token := m[1]
	if token == "" {
		token = m[2]
	}
	if token == "" {
		return ""
	}

	var hit []string
	for _, line := range strings.Split(td.SQL, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ","))
		if line == "" || strings.HasPrefix(line, "CREATE TABLE") || line == ");" {
			continue
		}
		if strings.Contains(line, token) {
			hit = append(hit, line)
		}
	}
	if len(hit) != 1 {
		return ""
	}
	return fmt.Sprintf(" — 이 줄이다: %s", hit[0])
}
