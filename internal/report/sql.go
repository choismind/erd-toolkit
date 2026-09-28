package report

import (
	"fmt"
	"path/filepath"
	"strings"

	"erdtool/internal/drawio"
	"erdtool/internal/model"
)

// SQLDDL은 물리 ERD로부터 CREATE TABLE 문을 생성한다.
//
// 타입이 비어 있는 컬럼("UniqueID"처럼 이름만 적힌 셀)은 타입을 추측해
// 채우지 않는다 — 이 프로젝트는 값을 플레이스홀더로 대체하지 않는다는
// 원칙을 지킨다(원장, 이전 구현의 "컬럼유형 없음" 결함). 대신
// 그 줄에 주석으로 표시를 남겨, 생성된 DDL을 실행하면 정확히 그 지점에서
// 걸리도록 한다. 조용히 빈 칸으로 두면 문법만 깨진 채 원인을 못 찾는다.
func SQLDDL(doc model.Document, opt Options) string {
	var b strings.Builder
	// SQL 파일만 남았을 때도 어느 DB용인지 알 수 있어야 한다. `--`는 SQL
	// 표준 주석이라 네 타깃 어디서나 통한다.
	fmt.Fprintf(&b, "-- 타깃 DBMS: %s\n-- 원본: %s\n\n",
		opt.Dialect.Label(), filepath.Base(doc.SourceFile))
	for _, td := range TableDDLs(doc, opt) {
		if td.Comment != "" {
			fmt.Fprintf(&b, "/* %s: %s */\n", td.TableName, td.Comment)
		}
		b.WriteString(td.SQL)
		b.WriteString("\n\n")
	}
	return b.String()
}

// TableDDL은 테이블 하나의 CREATE TABLE 문과 «그것이 어디서 나왔는가»다.
//
// 자리 정보를 함께 드는 이유는 이 문장을 실제로 실행해 보는 쪽
// (internal/ddlcheck)이 «어느 테이블이 걸렸는가»를 검증 진단으로 옮겨야
// 하기 때문이다. SQL 엔진은 문장만 알지 그것이 그림의 어느 셀에서 나왔는지
// 모른다.
type TableDDL struct {
	DiagramID   string
	DiagramName string
	TableName   string
	// CellID는 테이블 셀 id다. annotate가 이 id로 테두리를 친다.
	CellID string
	// Comment는 테이블 설명이다. SQL 문장에는 넣지 않는다 — 주석은 DBMS마다
	// 표기가 갈리고, 시험 실행(dry run)에 굳이 태울 것도 아니다.
	Comment string
	SQL     string
}

// TableDDLs는 테이블마다 CREATE TABLE 문 하나씩을 만든다. SQLDDL이 이것을
// 이어 붙인다 — 문장을 만드는 규칙이 두 벌이 되면 «파일로 뽑은 DDL»과
// «검사한 DDL»이 조용히 갈린다.
func TableDDLs(doc model.Document, opt Options) []TableDDL {
	var out []TableDDL
	eachTable(doc, func(d model.Diagram, t model.Table, i conflictIndex) {
		var b strings.Builder
		fmt.Fprintf(&b, "CREATE TABLE %s (\n", opt.Dialect.QuoteIdent(t.Name))
		var lines []string
		var pkCols []string
		for _, c := range t.Columns {
			line := "  " + opt.Dialect.QuoteIdent(c.Name)
			if c.Type == "" {
				line += " /* 타입 없음: ERD 셀 값이 " + fmt.Sprintf("%q", c.RawValue) + " */"
			} else {
				line += " " + c.Type
			}
			// DEFAULT와 CHECK의 값은 사람이 draw.io에 손으로 쓴 SQL 식이다.
			// 따옴표를 붙이거나 고쳐 쓰지 않고 **그대로 흘린다** — 이 도구는
			// 그 식이 무슨 뜻인지 모르며, 지레 감싸면 now() 같은 함수 호출이
			// 문자열이 되어 조용히 뜻이 바뀐다.
			line += attrClause(i, c.ID, drawio.AttrDefault, c.Default, "DEFAULT %s")
			if !c.Nullable {
				line += " NOT NULL"
			}
			if c.Unique {
				line += " UNIQUE"
			}
			line += attrClause(i, c.ID, drawio.AttrCheck, c.Check, "CHECK (%s)")
			if v := attrCell(i, c.ID, drawio.AttrComment, c.Comment); v != "" {
				line += " /* " + v + " */"
			}
			lines = append(lines, line)
			if c.IsPrimaryKey() {
				pkCols = append(pkCols, opt.Dialect.QuoteIdent(c.Name))
			}
		}
		if len(pkCols) > 0 {
			lines = append(lines, fmt.Sprintf("  PRIMARY KEY (%s)", strings.Join(pkCols, ", ")))
		}
		lines = append(lines, foreignKeyClauses(d, t, opt)...)
		if clause := attrClause(i, t.ID, drawio.AttrCheck, t.Check, "CHECK (%s)"); clause != "" {
			lines = append(lines, " "+clause)
		}
		b.WriteString(strings.Join(lines, ",\n"))
		b.WriteString("\n);")

		out = append(out, TableDDL{
			DiagramID: d.ID, DiagramName: d.Name, TableName: t.Name, CellID: t.ID,
			Comment: attrCell(i, t.ID, drawio.AttrComment, t.Comment),
			SQL:     b.String(),
		})
	})
	return out
}

// foreignKeyClauses는 테이블 하나의 FOREIGN KEY 절을 만든다.
//
// 예전에는 이 파일에 그 낱말이 한 번도 안 나왔다 — 관계는 IR에도 그림에도
// 관계정의서에도 살아 있는데 DDL 렌더러만 안 썼고, 그래서 `reverse` →
// `generate --sql` 왕복이 참조무결성을 잃었다(2026-09-04 소유자 지적).
//
// 참조 컬럼을 모르면(관계선이 행이 아니라 테이블에 직접 연결된 경우) 컬럼
// 목록 없이 `REFERENCES 부모`만 쓴다. ANSI에서 그것은 «그 테이블의 기본키»를
// 뜻한다 — 없는 컬럼 이름을 지어내는 대신 표준이 이미 가진 뜻에 맡긴다.
func foreignKeyClauses(d model.Diagram, t model.Table, opt Options) []string {
	refs := d.ColumnReferences()
	var out []string
	for _, c := range t.Columns {
		ref, ok := refs[c.ID]
		if !ok || ref.TableName == "" {
			continue
		}
		clause := fmt.Sprintf("  FOREIGN KEY (%s) REFERENCES %s",
			opt.Dialect.QuoteIdent(c.Name), opt.Dialect.QuoteIdent(ref.TableName))
		if ref.ColumnName != "" {
			clause += fmt.Sprintf(" (%s)", opt.Dialect.QuoteIdent(ref.ColumnName))
		}
		out = append(out, clause)
	}
	return out
}

// attrClause는 제약 하나를 DDL 조각으로 만든다. 값이 없으면 빈 문자열이다.
//
// 충돌한 자리는 **빈 칸으로 지나가지 않고 주석을 남긴다** — 타입이 비었을 때
// `/* 타입 없음: … */`을 남겨 실행하면 그 지점에서 걸리게 하는 것과 같은
// 처리다(작업 기록이 정한 세 갈래 중 둘째). 조용히 빠지면 사용자는
// 자기가 쓴 제약이 사라진 것을 끝까지 모른다.
func attrClause(i conflictIndex, cellID, name, value, format string) string {
	if i.has(cellID, name) {
		return fmt.Sprintf(" /* %s 충돌: 값을 고르지 않음 — ERD에서 하나만 남겨라 */", name)
	}
	if value == "" {
		return ""
	}
	return " " + fmt.Sprintf(format, value)
}
