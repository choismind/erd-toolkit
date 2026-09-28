package report

import (
	"fmt"
	"strings"

	"erdtool/internal/model"
)

// TableDocMarkdown은 테이블정의서를 렌더링한다. pageAsDomain이 true면
// 각 테이블 제목 옆에 "도메인"(페이지명)을 표시한다(원장: 다중 페이지
// 처리 — page_as_domain 옵션, 기본값 꺼짐).
func TableDocMarkdown(doc model.Document, opt Options) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", docTitle)
	for _, line := range docHeaderLines(doc, opt) {
		fmt.Fprintf(&b, "- **%s**: %s\n", line[0], line[1])
	}
	b.WriteString("\n## 테이블 일람\n\n")
	writeMarkdownTable(&b, tableListHeaders, tableListRows(doc, opt))

	// PDF와 같은 구성이다 — 테이블 하나가 한 절이고, 그 테이블의 관계가
	// 같은 절 안에 붙는다(2026-09-04 소유자 결정).
	eachTableWithRefs(doc, func(d model.Diagram, t model.Table, no int,
		i conflictIndex, refs map[string]model.ColumnRef) {

		fmt.Fprintf(&b, "\n## %s\n\n", tableHeading(no, t.Name))
		for _, note := range tableHeadingLines(d, t, i, opt) {
			fmt.Fprintf(&b, "%s\n\n", note)
		}
		writeMarkdownTable(&b, tableDocHeaders, columnRows(t, i, refs))

		b.WriteString("\n**관계**\n\n")
		if rows := tableRelationRows(d, t, refs); len(rows) == 0 {
			fmt.Fprintf(&b, "%s\n", noRelationNote)
		} else {
			writeMarkdownTable(&b, relationHeaders, rows)
		}
	})
	return b.String()
}

// writeMarkdownTable은 머리글과 본문으로 Markdown 표 하나를 찍는다. 일람과
// 컬럼 표가 같은 함수를 쓴다 — 구분선 개수를 열 수와 따로 세면 열이 늘어난
// 날 표가 깨진다.
func writeMarkdownTable(b *strings.Builder, headers []string, rows [][]string) {
	fmt.Fprintf(b, "| %s |\n", strings.Join(headers, " | "))
	b.WriteString("|" + strings.Repeat("---|", len(headers)) + "\n")
	for _, r := range rows {
		fmt.Fprintf(b, "| %s |\n", strings.Join(r, " | "))
	}
}

// RelationDocMarkdown은 관계정의서를 렌더링한다. 예전에는 테이블명 대신
// 원시 셀 id를 그대로 찍고, 컬럼명은 아예 빼먹고, 다른 리포터들과 달리
// conceptual 페이지도 걸러내지 않았다 — 사람이 읽을 수 없는 문서였다.
//
// 해석되지 않은 끝점은 빈 칸으로 삼키지 않고 "(미해석: <원본 id>)"로
// 남긴다. 조용히 비면 검증 리포트의 broken_reference와 대조할 단서가
// 사라진다.
func RelationDocMarkdown(doc model.Document) string {
	var b strings.Builder
	b.WriteString("# 관계정의서\n\n")
	b.WriteString("| 페이지 | 원천 테이블 | 원천 컬럼 | 카디널리티(원천) | 카디널리티(목표) | 목표 컬럼 | 목표 테이블 |\n")
	b.WriteString("|---|---|---|---|---|---|---|\n")
	for _, d := range doc.Diagrams {
		// 컬럼 조회는 Column.ID(= 행 셀 id)를 키로 쓴다. Relationship의
		// Source/TargetColumnID도 같은 행 id라서 그대로 조인된다.
		tables := map[string]model.Table{}
		columns := map[string]model.Column{}
		for _, t := range d.Tables {
			tables[t.ID] = t
			for _, c := range t.Columns {
				columns[c.ID] = c
			}
		}
		for _, r := range d.Relationships {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n",
				d.Name,
				endName(tables[r.SourceTableID].Name, r.SourceResolved, r.SourceRawID),
				columns[r.SourceColumnID].Name,
				cardinalityLabel(r.SourceCardinality), cardinalityLabel(r.TargetCardinality),
				columns[r.TargetColumnID].Name,
				endName(tables[r.TargetTableID].Name, r.TargetResolved, r.TargetRawID))
		}
	}
	return b.String()
}

// cardinalityNames는 draw.io의 화살표 코드를 사람 말로 옮긴다. 실측 6종이
// 전부이며(scripts/doclint.py의 ARROWS), 그 밖의 값은 도구가 모르는
// 것이므로 원문을 그대로 둔다.
//
// 예전에는 코드를 그대로 찍었다 — 관계정의서의 카디널리티 칸이
// `ERmandOne` · `ERzeroToMany`였다. 그것은 draw.io 스타일 값이지 사람이 읽는
// 말이 아니고, 정의서를 받는 쪽은 그 낱말이 무슨 뜻인지 알 길이 없다.
var cardinalityNames = map[string]string{
	"ERone":        "1 (정확히 하나)",
	"ERmandOne":    "1 (필수, 하나)",
	"ERmany":       "N (여럿)",
	"ERoneToMany":  "1..N (하나 이상)",
	"ERzeroToOne":  "0..1 (없거나 하나)",
	"ERzeroToMany": "0..N (없거나 여럿)",
}

// cardinalityLabel은 «사람 말 (원본 코드)» 꼴로 만든다. 원본을 함께 두는
// 이유는 그림과 대조할 단서를 남기기 위해서다 — 문서만 보고 스타일 값을
// 되짚을 수 있어야 어느 선을 고쳐야 할지 안다.
func cardinalityLabel(code string) string {
	if code == "" {
		return ""
	}
	name, ok := cardinalityNames[code]
	if !ok {
		return code
	}
	return fmt.Sprintf("%s [%s]", name, code)
}

// endName은 관계 끝점의 표시 이름을 고른다. 해석에 실패했으면 원본 id를
// 드러내서 어떤 셀이 문제인지 추적할 수 있게 한다.
func endName(name string, resolved bool, rawID string) string {
	if name != "" {
		return name
	}
	if !resolved && rawID != "" {
		return fmt.Sprintf("(미해석: %s)", rawID)
	}
	return ""
}
