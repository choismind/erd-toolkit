// internal/report/html.go
package report

import (
	"fmt"
	"html"
	"strings"

	"erdtool/internal/model"
)

func TableDocHTML(doc model.Document, opt Options) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<html><head><meta charset=\"utf-8\"><title>%s</title>%s</head><body>\n",
		html.EscapeString(docTitle), htmlStyle)
	fmt.Fprintf(&b, "<h1>%s</h1>\n<dl>\n", html.EscapeString(docTitle))
	for _, line := range docHeaderLines(doc, opt) {
		fmt.Fprintf(&b, "<dt>%s</dt><dd>%s</dd>\n",
			html.EscapeString(line[0]), html.EscapeString(line[1]))
	}
	b.WriteString("</dl>\n<h2>테이블 일람</h2>\n")
	writeHTMLTable(&b, tableListHeaders, tableListRows(doc, opt))

	eachTableWithRefs(doc, func(d model.Diagram, t model.Table, no int,
		i conflictIndex, refs map[string]model.ColumnRef) {

		fmt.Fprintf(&b, "<h2>%s</h2>\n", html.EscapeString(tableHeading(no, t.Name)))
		for _, note := range tableHeadingLines(d, t, i, opt) {
			fmt.Fprintf(&b, "<p>%s</p>\n", html.EscapeString(note))
		}
		writeHTMLTable(&b, tableDocHeaders, columnRows(t, i, refs))

		b.WriteString("<h3>관계</h3>\n")
		if rows := tableRelationRows(d, t, refs); len(rows) == 0 {
			fmt.Fprintf(&b, "<p>%s</p>\n", html.EscapeString(noRelationNote))
		} else {
			writeHTMLTable(&b, relationHeaders, rows)
		}
	})
	b.WriteString("</body></html>")
	return b.String()
}

// htmlStyle은 표가 문서로 보이게 하는 최소한이다. 외부 파일을 참조하지
// 않는다 — 산출물은 메일에 붙거나 폴더째 옮겨 다니므로 한 파일로 완결돼야
// 한다.
const htmlStyle = `<style>
body{font-family:"맑은 고딕","Malgun Gothic",sans-serif;font-size:13px;margin:24px}
h1{font-size:20px;border-bottom:2px solid #333;padding-bottom:6px}
h2{font-size:15px;margin-top:28px}
dl{margin:0 0 8px}dt{float:left;clear:left;width:80px;color:#555}dd{margin:0 0 2px 88px}
table{border-collapse:collapse;margin:6px 0 4px}
th,td{border:1px solid #999;padding:3px 8px;text-align:left;vertical-align:top}
th{background:#eee;white-space:nowrap}
p{margin:2px 0;color:#333}
</style>`

// writeHTMLTable은 머리글과 본문으로 표 하나를 찍는다. 일람과 컬럼 표가
// 같은 함수를 쓴다.
func writeHTMLTable(b *strings.Builder, headers []string, rows [][]string) {
	b.WriteString("<table>\n<tr>")
	for _, h := range headers {
		fmt.Fprintf(b, "<th>%s</th>", html.EscapeString(h))
	}
	b.WriteString("</tr>\n")
	for _, r := range rows {
		b.WriteString("<tr>")
		for _, v := range r {
			fmt.Fprintf(b, "<td>%s</td>", html.EscapeString(v))
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</table>\n")
}
