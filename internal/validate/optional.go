// internal/validate/optional.go
package validate

import (
	"fmt"
	"regexp"
	"strings"

	"erdtool/internal/config"
	"erdtool/internal/model"
)

var ansiSQLTypes = map[string]bool{
	"CHAR": true, "VARCHAR": true, "TEXT": true, "CLOB": true,
	"INT": true, "INTEGER": true, "SMALLINT": true, "BIGINT": true,
	"DECIMAL": true, "NUMERIC": true, "FLOAT": true, "REAL": true, "DOUBLE": true,
	"DATE": true, "TIME": true, "TIMESTAMP": true,
	"BOOLEAN": true, "BOOL": true, "BLOB": true, "VARBINARY": true, "BINARY": true,

	// 표준 타입 중 «낱말이 여럿»인 것들. 이 줄들이 없으면 역공학한
	// PostgreSQL DB의 거의 모든 컬럼에 오경보가 붙는다 — 그 DBMS는
	// character varying / timestamp with time zone을 카탈로그 원문으로 낸다.
	// Phase 2b 전에는 파서가 첫 낱말만 읽어 여기 닿지 않던 값들이다.
	"CHARACTER": true, "CHARACTER VARYING": true, "CHARACTER LARGE OBJECT": true,
	"NATIONAL CHARACTER": true, "NATIONAL CHARACTER VARYING": true,
	"BIT": true, "BIT VARYING": true,
	"BINARY VARYING": true, "BINARY LARGE OBJECT": true,
	"DOUBLE PRECISION":         true,
	"TIMESTAMP WITH TIME ZONE": true, "TIMESTAMP WITHOUT TIME ZONE": true,
	"TIME WITH TIME ZONE": true, "TIME WITHOUT TIME ZONE": true,
}

// snakeCasePattern은 소문자 스네이크(cust_no)와 대문자 스네이크(CUST_NO)를
// 둘 다 통과시키고, 섞어 쓰는 표기(customerName/CustomerName)만 잡는다.
// 규칙의 뜻은 "표기를 일관되게 쓰라"이지 "소문자를 쓰라"가 아니다 — 한국
// 공공/기업 물리 ERD는 대문자 스네이크가 관례라, 소문자만 허용하면 규칙을
// 켜는 순간 모든 컬럼에 warning이 붙어 리포트가 통째로 쓸모없어진다(실제
// 픽스처를 돌려보고 확인). 한 이름 안에서 대소문자를 섞지 않는 것이 통과
// 조건이므로 두 갈래를 or로 둔다.
var snakeCasePattern = regexp.MustCompile(`^([a-z][a-z0-9_]*|[A-Z][A-Z0-9_]*)$`)

// typeBaseName은 타입 문자열에서 «이름 부분»만 남긴다. 괄호로 묶인 인자는
// 어디에 있든 걷어내고, 낱말 사이 공백은 하나로 모은다 — `character  varying`과
// `character varying`이 한 형태가 돼야 표준 타입 목록과 대조할 수 있다.
//
// **괄호 «앞»까지만 자르지 않는다.** 예전에는 `strings.SplitN(t, "(", 2)[0]`으로
// 앞부분만 봤는데, 그러면 `char(20) NOTNULL`이 `CHAR`로 줄어 목록을 통과했다 —
// NOT NULL을 붙여 쓴 오타가 이 검사를 그대로 빠져나갔다(2026-09-04 실측).
// 괄호 뒤에 남은 낱말은 타입 이름의 일부로 쳐서 목록에 없게 만든다.
//
// 괄호가 안 닫혔으면 걷어낼 범위를 알 수 없으므로 원문을 그대로 돌려준다.
// 목록에 있을 리 없으니 진단으로 올라가고, 정확히 무엇이 잘못됐는지는
// DDL 시험 실행(dry run)이 말해 준다.
func typeBaseName(t string) string {
	var b strings.Builder
	depth := 0
	for _, r := range t {
		switch r {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return normalizeWords(t) // 여는 괄호 없이 닫혔다
			}
			depth--
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}
	if depth != 0 {
		return normalizeWords(t) // 안 닫혔다
	}
	return normalizeWords(b.String())
}

func normalizeWords(s string) string {
	return strings.ToUpper(strings.Join(strings.Fields(s), " "))
}

// Optional은 원장의 옵션 검사 2종을 페이지별 활성화 여부(config.EffectiveValidation)에 따라 수행한다.
func Optional(doc model.Document, cfg config.Config) []Finding {
	var findings []Finding

	for _, d := range doc.Diagrams {
		ansiOn, namingOn := cfg.EffectiveValidation(d.Name)
		for _, t := range d.Tables {
			for _, c := range t.Columns {
				if c.Type == "" {
					continue
				}
				if ansiOn {
					base := typeBaseName(c.Type)
					if !ansiSQLTypes[base] {
						findings = append(findings, Finding{
							Rule: "non_ansi_type", Severity: SeverityWarning,
							Message:     fmt.Sprintf("컬럼 %s.%s의 타입 %q이 ANSI SQL 표준이 아님", t.Name, c.Name, c.Type),
							Brief:       fmt.Sprintf("컬럼 %s의 타입 %q이 표준이 아님", c.Name, c.Type),
							Action:      "표준 타입으로 바꾸거나, 이 검사를 설정에서 끈다",
							Scope:       ScopeTable,
							DiagramName: d.Name, DiagramID: d.ID, TableName: t.Name, CellID: c.ID,
						})
					}
				}
				if namingOn && c.Name != "" && !snakeCasePattern.MatchString(c.Name) {
					findings = append(findings, Finding{
						Rule: "naming_convention", Severity: SeverityWarning,
						Message:     fmt.Sprintf("컬럼명 %q이 스네이크 표기(cust_no 또는 CUST_NO)를 따르지 않음", c.Name),
						Brief:       fmt.Sprintf("컬럼명 %q이 스네이크 표기를 따르지 않음", c.Name),
						Action:      "cust_no 또는 CUST_NO처럼 한 이름 안에서 대소문자를 섞지 않는다",
						Scope:       ScopeTable,
						DiagramName: d.Name, DiagramID: d.ID, TableName: t.Name, CellID: c.ID,
					})
				}
			}
		}
	}
	return findings
}
