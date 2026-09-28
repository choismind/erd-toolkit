// internal/report/validation.go
//
// 검증 리포트를 **페이지와 테이블로 묶어** 찍는다.
//
// 예전에는 진단을 찾은 순서대로 한 줄씩 늘어놓았다. 한 테이블을 고치려면
// 파일을 위아래로 읽어야 했고, 같은 규칙이 여기저기 흩어져 있었다
// (2026-09-04 소유자 지적).
package report

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"erdtool/internal/model"
	"erdtool/internal/validate"
)

// ValidationMarkdown은 검증 리포트를 만든다.
func ValidationMarkdown(doc model.Document, findings []validate.Finding, opt Options) string {
	var b strings.Builder
	b.WriteString("# 검증 리포트\n\n")
	fmt.Fprintf(&b, "- 원본: %s\n", filepath.Base(doc.SourceFile))
	if doc.SourceModified != "" {
		fmt.Fprintf(&b, "- 기준 시각: %s\n", doc.SourceModified)
	}
	fmt.Fprintf(&b, "- 타깃 DBMS: %s\n", opt.Dialect.Label())
	fmt.Fprintf(&b, "- %s\n", countLine(findings))

	if len(findings) == 0 {
		b.WriteString("\n발견된 문제 없음\n")
		return b.String()
	}

	// 그림에서 그 자리를 찾는 방법을 먼저 알려 준다. 진단에 셀 id가 실리지만
	// draw.io에는 id로 도형을 찾는 기능이 없어서, 이 안내가 없으면 사용자는
	// 받은 id로 할 수 있는 일이 없다.
	fmt.Fprintf(&b, "\n> [!TIP] 진단이 붙은 자리를 그림에서 찾으려면\n"+
		"> `erdtool annotate %s`를 돌리면 진단이 붙은 도형에 **빨간** 테두리가 쳐지고\n"+
		"> 페이지마다 요약 상자가 붙는다. 읽지 않은 도형에는 **회색** 테두리가\n"+
		"> 쳐진다 — 고칠 것과 대상 밖을 색으로 가른다.\n"+
		"> `--clean`이 그것을 지운다.\n",
		filepath.Base(doc.SourceFile))

	if hasRule(findings, "ddl_syntax") {
		b.WriteString("\n> [!NOTE] DDL 문법 검사에 대하여\n" +
			"> 만든 SQL이 실행되는지는 메모리 SQLite로 확인한다. 그래서 특정 DBMS에만\n" +
			"> 있는 타입(MySQL의 `enum(...)` 같은 것)은 타깃을 그 DBMS로 줘도 여기서\n" +
			"> 걸릴 수 있다. 그 경우는 고치지 않아도 된다.\n")
	}

	byPage := groupByPage(doc, findings)
	for _, pg := range byPage {
		fmt.Fprintf(&b, "\n## 페이지: %s\n", pg.Name)
		writeGroup(&b, "", pg.Page)
		for _, t := range pg.Tables {
			writeGroup(&b, "테이블 "+t.Name, t.Findings)
		}
		writeGroup(&b, "관계선", pg.Relations)
		// 대상 밖 도형은 맨 뒤에 따로 둔다. 고칠 것과 «읽지 않았다»를
		// 한 목록에 섞으면 무엇을 손봐야 하는지가 묻힌다.
		writeGroup(&b, "대상 아님 — 읽지 않은 도형", pg.OutOfScope)
	}

	var file []validate.Finding
	for _, f := range findings {
		if f.Scope == validate.ScopeFile {
			file = append(file, f)
		}
	}
	if len(file) > 0 {
		b.WriteString("\n## 파일 전체\n")
		writeGroup(&b, "", file)
	}
	return b.String()
}

// countLine은 «몇 건인가»를 심각도별로 센다. 맨 위에서 규모를 먼저 알려 준다.
func countLine(findings []validate.Finding) string {
	// 대상 밖 도형은 «진단»에 넣지 않는다. 고칠 것이 아니라 읽지 않았다는
	// 사실이라, 같이 세면 사용자가 손볼 것이 몇 개인지 알 수 없게 된다.
	warn, info, out := 0, 0, 0
	for _, f := range findings {
		switch {
		case f.OutOfScope:
			out++
		case f.Severity == validate.SeverityInfo:
			info++
		default:
			warn++
		}
	}
	head := "진단 없음"
	switch {
	case warn == 0 && info == 0:
	case info == 0:
		head = fmt.Sprintf("진단 %d건", warn)
	default:
		head = fmt.Sprintf("진단 %d건 (경고 %d · 안내 %d)", warn+info, warn, info)
	}
	if out > 0 {
		head += fmt.Sprintf(" · 읽지 않은 도형 %d개", out)
	}
	return head
}

func hasRule(findings []validate.Finding, rule string) bool {
	for _, f := range findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

type tableGroup struct {
	Name     string
	Findings []validate.Finding
}

type pageGroup struct {
	Name      string
	Page      []validate.Finding // 페이지 전체에 대한 것
	Relations []validate.Finding
	// OutOfScope는 «읽지 않은 도형»이다. 고칠 것이 아니라 알려 줄 것이라
	// 페이지 맨 뒤에 따로 찍는다.
	OutOfScope []validate.Finding
	Tables     []tableGroup
}

// groupByPage는 진단을 페이지 → 테이블로 묶는다.
//
// 페이지와 테이블의 순서는 **그림에 있는 순서**를 따른다. 진단이 나온 순서로
// 두면 같은 파일을 두 번 처리한 리포트가 서로 다른 순서로 나올 수 있다.
func groupByPage(doc model.Document, findings []validate.Finding) []pageGroup {
	// 페이지 순서와 그 안의 테이블 순서를 문서에서 가져온다.
	order := map[string]int{}
	tableOrder := map[string]int{}
	for i, d := range doc.Diagrams {
		order[d.Name] = i
		for j, t := range d.Tables {
			tableOrder[d.Name+"\x00"+t.Name] = j
		}
	}

	pages := map[string]*pageGroup{}
	tables := map[string]*tableGroup{}

	for _, f := range findings {
		if f.Scope == validate.ScopeFile {
			continue // 페이지에 속하지 않는다
		}
		pg, ok := pages[f.DiagramName]
		if !ok {
			pg = &pageGroup{Name: f.DiagramName}
			pages[f.DiagramName] = pg
		}
		switch {
		case f.OutOfScope:
			pg.OutOfScope = append(pg.OutOfScope, f)
		case f.Scope == validate.ScopeRelation:
			pg.Relations = append(pg.Relations, f)
		case f.TableName != "":
			key := f.DiagramName + "\x00" + f.TableName
			tg, ok := tables[key]
			if !ok {
				tg = &tableGroup{Name: f.TableName}
				tables[key] = tg
				pg.Tables = append(pg.Tables, tableGroup{})
			}
			tg.Findings = append(tg.Findings, f)
		default:
			pg.Page = append(pg.Page, f)
		}
	}

	var out []pageGroup
	for name, pg := range pages {
		// 이 페이지의 테이블 묶음을 그림 순서로 모은다.
		var tgs []tableGroup
		for key, tg := range tables {
			if strings.HasPrefix(key, name+"\x00") {
				tgs = append(tgs, *tg)
			}
		}
		sort.Slice(tgs, func(i, j int) bool {
			a := tableOrder[name+"\x00"+tgs[i].Name]
			b := tableOrder[name+"\x00"+tgs[j].Name]
			if a == b {
				return tgs[i].Name < tgs[j].Name
			}
			return a < b
		})
		out = append(out, pageGroup{
			Name: name, Page: pg.Page, Relations: pg.Relations,
			OutOfScope: pg.OutOfScope, Tables: tgs,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		a, aok := order[out[i].Name]
		b, bok := order[out[j].Name]
		if aok && bok && a != b {
			return a < b
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// writeGroup은 묶음 하나를 찍는다. 비어 있으면 아무것도 안 찍는다.
func writeGroup(b *strings.Builder, heading string, findings []validate.Finding) {
	if len(findings) == 0 {
		return
	}
	if heading != "" {
		fmt.Fprintf(b, "\n### %s\n\n", heading)
	} else {
		b.WriteString("\n")
	}
	for _, f := range findings {
		text := f.Brief
		if text == "" {
			text = f.Message
		}
		label := severityLabel(f.Severity)
		if f.OutOfScope {
			// «경고»가 아니다. 잘못이 아니라 이 도구가 안 읽은 것이다.
			label = "대상 밖"
		}
		fmt.Fprintf(b, "- **%s** · %s", label, text)
		if f.Action != "" {
			fmt.Fprintf(b, "\n  → %s", f.Action)
		}
		fmt.Fprintf(b, "\n  <sub>%s</sub>\n", f.Rule)
	}
}

// severityLabel은 심각도를 한글로 보인다. `warning`은 규칙 이름 옆에 영어로
// 남아 있어도 되지만, 줄 맨 앞은 사람이 읽는 자리다.
func severityLabel(s validate.Severity) string {
	if s == validate.SeverityInfo {
		return "안내"
	}
	return "경고"
}
