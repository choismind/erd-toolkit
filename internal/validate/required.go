// internal/validate/required.go
package validate

import (
	"fmt"
	"strings"

	"erdtool/internal/drawio"
	"erdtool/internal/ir"
	"erdtool/internal/model"
)

type Severity string

const (
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Scope는 이 진단이 «무엇에 대한 것인가»다. 검증 리포트가 페이지와 테이블로
// 묶어 보여 줄 때 쓴다. 예전에는 진단을 찾은 순서대로 늘어놓아서, 한 테이블을
// 고치려면 파일을 위아래로 읽어야 했다.
type Scope string

const (
	ScopeTable    Scope = "table"    // 테이블과 그 컬럼
	ScopeRelation Scope = "relation" // 관계선
	ScopePage     Scope = "page"     // 페이지 전체
	ScopeFile     Scope = "file"     // 페이지를 가로지르는 사실
)

type Finding struct {
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	// Message는 **혼자서도 뜻이 통하는** 문장이다. annotate가 이것을 그림의
	// 요약 상자에 그대로 쓰기 때문에, 테이블 이름 같은 맥락이 들어 있어야 한다.
	Message string `json:"message"`
	// Brief는 검증 리포트에서 쓰는 짧은 문장이다. 리포트는 테이블 이름을 제목으로
	// 이미 보여 주므로 그것을 다시 말할 필요가 없다. 비어 있으면 Message를 쓴다.
	Brief string `json:"brief,omitempty"`
	// Action은 «그래서 무엇을 하면 되는가»다. 이것이 없으면 사용자는 문제만
	// 알고 다음 걸음을 모른다.
	Action string `json:"action,omitempty"`
	Scope  Scope  `json:"scope,omitempty"`
	// OutOfScope는 «잘못»이 아니라 «이 도구가 다루지 않는 것»이라는 표시다.
	// 이 도구는 논리·물리설계만 읽는다. 개념설계 도형과 낙서는 고칠 것이
	// 아니라 읽지 않았다는 사실을 알려 줄 대상이다. 그림에서는 회색으로,
	// 리포트에서는 결함과 다른 절로 나뉜다.
	OutOfScope bool `json:"out_of_scope,omitempty"`

	DiagramName string `json:"diagram_name,omitempty"`
	// DiagramID는 어느 페이지의 셀인가다. CellID만으로는 삽입 대상
	// 페이지를 못 고른다 — 요약 박스가 페이지마다 하나이기 때문이다.
	DiagramID string `json:"diagram_id,omitempty"`

	TableName string `json:"table_name,omitempty"`
	// CellID는 이 진단을 붙일 .drawio 셀 id다. 빈 문자열이면 «붙일 셀이
	// 없다»는 뜻이며, 그때는 마크하지 않고 요약 박스에만 싣는다.
	// 지어내지 않는다 — duplicate_table_name이 실제로 그런 경우다.
	CellID string `json:"cell_id,omitempty"`
}

// Required는 스펙 "검증 리포트"의 필수 검사 4종 + 정보성 안내를 수행한다.
func Required(doc model.Document, dups []ir.DuplicateNameWarning) []Finding {
	var findings []Finding

	for _, d := range doc.Diagrams {

		for _, v := range d.Violations {
			// 도형 이름이 없을 때 원본 style을 대신 싣던 자리다. 그러면
			// annotate가 그 셀에 테두리 색을 더한 뒤 다시 돌릴 때 문구가
			// 달라져 **같은 파일이 두 번 다르게 나온다**(2026-09-06에
			// 실측). 문구에는 변하지 않는 것만 담고, 원본 style은 IR의
			// shape_violations[].style에 그대로 남는다.
			// 이름을 못 뽑으면 Brief에도 id를 싣는다. 리포트는 Brief를
			// 찍는데, 이름 없는 도형이 셋이면 세 줄이 글자까지 똑같아져
			// 어느 것이 어느 것인지 구별되지 않는다.
			what := "이름을 알 수 없는 도형"
			brief := fmt.Sprintf("이름을 알 수 없는 도형(id=%s)", v.ID)
			if v.Shape != "" {
				what = v.Shape + " 도형"
				brief = what
			}
			findings = append(findings, Finding{
				// 잘못이 아니라 이 도구가 안 읽은 것이라 «안내»다. 리포트의
				// 줄머리와 건수는 이 값이 아니라 OutOfScope가 가른다.
				Rule: "shape_violation", Severity: SeverityInfo,
				Message: fmt.Sprintf("논리·물리설계 대상이 아닌 %s이라 읽지 않음(id=%s)",
					what, v.ID),
				Brief:       brief + "은 읽지 않았다",
				Action:      "맞으면 그대로 둔다. 테이블로 읽혀야 할 것이면 ER 도형으로 다시 그린다",
				Scope:       ScopePage,
				OutOfScope:  true,
				DiagramName: d.Name, DiagramID: d.ID, CellID: v.ID,
			})
		}

		// erd_* 커스텀 속성의 진단 둘. 순서는 ir이 이미 정렬해 둔 것을
		// 그대로 따른다(annotate가 이 순서로 요약 박스를 쓴다).
		for _, c := range d.AttrConflicts {
			findings = append(findings, Finding{
				Rule: "attr_conflict", Severity: SeverityWarning,
				Message: fmt.Sprintf("%s에 %s가 둘 이상 붙었는데 값이 서로 다름: %s"+
					" (어느 쪽도 쓰지 않는다 — 하나만 남겨라)",
					attrTarget(c), c.Name, quoteValues(c.Values)),
				Brief: fmt.Sprintf("%s의 %s 값이 둘인데 서로 다름: %s",
					attrColumnLabel(c), c.Name, quoteValues(c.Values)),
				Action:      "둘 중 하나를 지운다. 지금은 어느 쪽도 쓰이지 않는다",
				Scope:       ScopeTable,
				DiagramName: d.Name, DiagramID: d.ID, TableName: c.TableName, CellID: c.CellID,
			})
		}
		for _, u := range d.UnknownAttrs {
			findings = append(findings, Finding{
				Rule: "unknown_attr", Severity: SeverityWarning,
				Message: fmt.Sprintf("%s에 붙은 %q는 이 도구가 읽지 않는 이름이라 값이 버려짐"+
					" (오타인지 보라 — 읽는 이름은 %s)",
					attrTarget(u), u.Name, knownAttrList(u)),
				Brief: fmt.Sprintf("%s에 붙은 %q는 읽지 않는 이름이라 값이 버려짐",
					attrColumnLabel(u), u.Name),
				Action:      fmt.Sprintf("오타인지 보라. 읽는 이름은 %s", knownAttrList(u)),
				Scope:       ScopeTable,
				DiagramName: d.Name, DiagramID: d.ID, TableName: u.TableName, CellID: u.CellID,
			})
		}

		for _, t := range d.Tables {
			if len(t.Columns) == 0 {
				findings = append(findings, Finding{
					Rule: "empty_table", Severity: SeverityWarning,
					Message:     fmt.Sprintf("테이블 %q에 컬럼이 없음", t.Name),
					Brief:       "컬럼이 하나도 없음",
					Action:      "행을 넣거나, 쓰지 않는 도형이면 지운다",
					Scope:       ScopeTable,
					DiagramName: d.Name, DiagramID: d.ID, TableName: t.Name, CellID: t.ID,
				})
				continue
			}
			hasPK := false
			for _, c := range t.Columns {
				if c.IsPrimaryKey() {
					hasPK = true
					break
				}
			}
			if !hasPK {
				findings = append(findings, Finding{
					Rule: "missing_primary_key", Severity: SeverityWarning,
					Message:     fmt.Sprintf("테이블 %q에 PK가 없음", t.Name),
					Brief:       "기본키 표기가 없음",
					Action:      "기본키 컬럼의 키 셀에 PK를 적는다",
					Scope:       ScopeTable,
					DiagramName: d.Name, DiagramID: d.ID, TableName: t.Name, CellID: t.ID,
				})
			}
			findings = append(findings, columnFindings(d, t)...)
		}

		findings = append(findings, relationshipFindings(d)...)

		// 주의: 여기서 "알려진 테이블 ID 집합에 있는지" 로 판정하지 않는다.
		// SourceTableID/TargetTableID는 resolveEnd가 실패하면 애초에 빈
		// 문자열로 남으므로, 그 값을 known 맵에 다시 물어보는 방식은 항상
		// 통과해버려 이 검사가 죽은 코드가 된다(끊어진 관계가 실제 프로덕션
		// 경로에서 한 번도 못 걸린 원인). 대신 drawio.ExtractRelationships가
		// 남겨준 명시적 Resolved 플래그로만 판정한다.
		//
		// Resolved=false만으로는 부족하다: raw id가 idx.ByID에 실존하는
		// 셀을 가리키지만 이 파서가 행으로 인식 못하는 도형 변형(예: 구버전
		// 2단 테이블의 partialRectangle 직속 행 — out_of_order_cells.drawio)도
		// Resolved=false가 되기 때문이다. 그 경우는 진짜 dangling reference가
		// 아니라 알려진 미지원 케이스이므로, Exists=false(idx.ByID에 그
		// id 자체가 없음)일 때만 broken_reference로 플래그한다.
		for _, r := range d.Relationships {
			if r.SourceRawID != "" && !r.SourceResolved && !r.SourceExists {
				findings = append(findings, Finding{
					Rule: "broken_reference", Severity: SeverityWarning,
					Message: fmt.Sprintf("관계 %s의 시작점(id=%s)이 이 페이지의 테이블이나 행을 가리키지 않음 (반대쪽은 %s)",
						r.ID, r.SourceRawID, endLabel(d, r.TargetTableID, r.TargetColumnID, r.TargetResolved)),
					Brief: fmt.Sprintf("시작점이 없는 도형(id=%s)을 가리킴. 반대쪽 끝은 %s",
						r.SourceRawID, endLabel(d, r.TargetTableID, r.TargetColumnID, r.TargetResolved)),
					Action:      "관계선의 시작점을 다시 잇는다",
					Scope:       ScopeRelation,
					DiagramName: d.Name, DiagramID: d.ID, CellID: r.ID,
				})
			}
			if r.TargetRawID != "" && !r.TargetResolved && !r.TargetExists {
				findings = append(findings, Finding{
					Rule: "broken_reference", Severity: SeverityWarning,
					Message: fmt.Sprintf("관계 %s의 끝점(id=%s)이 이 페이지의 테이블이나 행을 가리키지 않음 (반대쪽은 %s)",
						r.ID, r.TargetRawID, endLabel(d, r.SourceTableID, r.SourceColumnID, r.SourceResolved)),
					Brief: fmt.Sprintf("끝점이 없는 도형(id=%s)을 가리킴. 반대쪽 끝은 %s",
						r.TargetRawID, endLabel(d, r.SourceTableID, r.SourceColumnID, r.SourceResolved)),
					Action:      "관계선의 끝점을 다시 잇는다",
					Scope:       ScopeRelation,
					DiagramName: d.Name, DiagramID: d.ID, CellID: r.ID,
				})
			}
		}

		// 아무것도 잇지 않았는데 그림에서는 테이블에 닿아 있는 관계선.
		// broken_reference와 달리 가리키던 id조차 없어서, 이 검사가 없으면
		// 관계 하나가 정의서에서 아무 말 없이 사라진다.
		for _, f := range d.FloatingRelations {
			name := f.Label
			if name == "" {
				name = "id=" + f.ID
			}
			where := tableNames(d, f.NearTableIDs)
			findings = append(findings, Finding{
				Rule: "floating_relationship", Severity: SeverityWarning,
				// 이름 뒤에 «의»를 쓴다. «이/가»는 앞 글자의 받침에 따라
				// 갈려서, 사용자가 붙인 이름에 그대로 이으면 틀린다.
				Message: fmt.Sprintf("관계선 %s의 양 끝이 어느 테이블·행에도 연결되어 있지 않음 (그림에서는 %s에 닿아 있음)",
					name, where),
				Brief: fmt.Sprintf("%s의 양 끝이 어느 테이블·행에도 연결되어 있지 않음. 그림에서는 %s에 닿아 있다",
					name, where),
				Action:      "선 끝을 테이블의 행 위로 끌어 연결점이 잡히는 것을 보고 놓는다",
				Scope:       ScopeRelation,
				DiagramName: d.Name, DiagramID: d.ID, CellID: f.ID,
			})
		}
	}

	for _, dup := range dups {
		findings = append(findings, Finding{
			Rule: "duplicate_table_name", Severity: SeverityWarning,
			Message:   duplicateNameMessage(dup),
			Brief:     duplicateNameMessage(dup),
			Action:    "어느 쪽이 맞는지 정하고 나머지 이름을 고친다",
			Scope:     ScopeFile,
			TableName: dup.TableName,
		})
	}

	return findings
}

// duplicateNameMessage는 중복 이름 하나를 사람이 읽을 한 줄로 만든다.
//
// 「몇 개 페이지」와 「몇 곳」을 가르는 이유: 예전에는 출현 수를 그대로
// «N개 페이지»라고 불러, 페이지가 둘뿐인 파일이 "6개 페이지에 존재함"으로
// 보고됐다. 목록에 같은 페이지 이름이 세 번 찍혀 사람은 눈치챌 수 있었지만,
// 숫자만 세는 쪽은 그대로 속는다 — 화면의 숫자와 파일 안의 사실이 다른,
// 이 저장소가 존재하는 이유인 그 실패다.
//
// 한 페이지 안에서만 겹치는 경우도 보고한다(같은 이름 테이블 둘 중 어느
// 쪽이 진짜인지는 사람이 정해야 한다). 그때는 «페이지»라는 말 자체가
// 오해를 부르므로 문구를 아예 가른다.
func duplicateNameMessage(dup ir.DuplicateNameWarning) string {
	switch {
	case len(dup.PageNames) == 1:
		return fmt.Sprintf("동일 이름 테이블 %q이 페이지 %q 안에 %d개 있음",
			dup.TableName, dup.PageNames[0], dup.Occurrences)
	case dup.Occurrences > len(dup.PageNames):
		return fmt.Sprintf("동일 이름 테이블 %q이 %d개 페이지에 걸쳐 %d곳 존재함: %v",
			dup.TableName, len(dup.PageNames), dup.Occurrences, dup.PageNames)
	default:
		return fmt.Sprintf("동일 이름 테이블 %q이 %d개 페이지에 존재함: %v",
			dup.TableName, len(dup.PageNames), dup.PageNames)
	}
}

// quoteValues는 충돌한 값들을 문구에 넣는다. Go의 %v로 슬라이스를 그대로
// 찍으면 `[LENGTH(email) >= 5 email LIKE '%@%']`가 되어 값의 경계가
// 사라진다 — 값 자체에 공백이 흔한 SQL 식이라 더 그렇다.
func quoteValues(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = fmt.Sprintf("%q", v)
	}
	return strings.Join(quoted, " / ")
}

// attrTarget은 진단이 가리키는 자리를 사람 말로 만든다. 컬럼이면
// «테이블.컬럼», 테이블 제약이면 «테이블»이다 — 붙인 자리를 잘못 짚으면
// 사용자가 엉뚱한 셀을 열어 본다.
func attrTarget(a model.AttrIssue) string {
	if a.ColumnName == "" {
		return fmt.Sprintf("테이블 %q", a.TableName)
	}
	return fmt.Sprintf("컬럼 %s.%s", a.TableName, a.ColumnName)
}

// knownAttrList는 그 자리에서 읽는 이름들을 문구에 넣는다. 컬럼과 테이블이
// 다르므로(erd_default는 컬럼뿐) 자리에 맞는 목록을 보여준다 — 전체 목록을
// 보여주면 «테이블에 erd_default를 붙이면 되겠구나»로 읽힌다.
func knownAttrList(a model.AttrIssue) string {
	if a.ColumnName == "" {
		return drawio.AttrCheck + " / " + drawio.AttrComment
	}
	return drawio.AttrCheck + " / " + drawio.AttrDefault + " / " + drawio.AttrComment
}

// attrColumnLabel은 제약 진단이 가리키는 자리를 «리포트 안에서» 부르는 말이다.
// 리포트는 테이블 이름을 제목으로 이미 보여 주므로 컬럼 이름만 말한다.
func attrColumnLabel(a model.AttrIssue) string {
	if a.ColumnName == "" {
		return "테이블 자체"
	}
	return "컬럼 " + a.ColumnName
}

// tableNames는 테이블 도형 id 목록을 사람이 읽는 이름으로 바꾼다. 이름을
// 못 찾은 id는 id 그대로 남긴다 — 지어내지 않는다.
func tableNames(d model.Diagram, ids []string) string {
	if len(ids) == 0 {
		return "어느 테이블도 아님"
	}
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		name := id
		for _, t := range d.Tables {
			if t.ID == id {
				name = t.Name
				break
			}
		}
		names = append(names, name)
	}
	return "테이블 " + strings.Join(names, "·")
}

// endLabel은 관계선 한쪽 끝을 사람이 찾을 수 있는 말로 만든다.
//
// 끊어진 쪽의 셀 id만 알려 주면 사용자는 그 선을 그림에서 찾을 방법이 없다 —
// draw.io에는 id로 도형을 찾는 기능이 없기 때문이다. 붙어 있는 반대쪽은
// 이름을 알 수 있으므로 그것을 함께 준다.
func endLabel(d model.Diagram, tableID, columnID string, resolved bool) string {
	if !resolved {
		return "그쪽도 해석되지 않음"
	}
	for _, t := range d.Tables {
		if t.ID != tableID {
			continue
		}
		for _, c := range t.Columns {
			if c.ID == columnID {
				return t.Name + "." + c.Name
			}
		}
		return "테이블 " + t.Name
	}
	return "알 수 없음"
}
