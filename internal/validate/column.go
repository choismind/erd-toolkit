// internal/validate/column.go
package validate

import (
	"fmt"

	"erdtool/internal/model"
)

// columnFindings는 «정의 셀이 덜 채워졌거나 앞뒤가 안 맞는» 경우를 잡는다.
// 셋 다 파서가 이미 아는 사실이라 DBMS 지식도 SQL 지식도 필요 없다 — 그래서
// 언제나 켜져 있다(ANSI 여부를 묻는 non_ansi_type과 달리).
//
// 왜 이 검사가 필요한가: 2026-09-04에 sakila에 사람이 자주 저지르는 실수
// 열 가지를 심어 재 봤더니 다섯이 조용히 통과했고, 그중 셋이 여기서 잡힌다.
// 통과하면 DDL에 `"" /* 타입 없음 */`이나 같은 이름 컬럼 두 줄이 그대로
// 나가는데, 검증 리포트는 「발견된 문제 없음」이라고 말한다.
func columnFindings(d model.Diagram, t model.Table) []Finding {
	var findings []Finding

	// 이름은 «처음 본 행»을 기억해 둔다. 두 번째부터가 중복이다 — 첫 행까지
	// 진단을 내면 사용자가 둘 다 고쳐야 하는 줄 알고 멀쩡한 쪽을 건드린다.
	seen := map[string]bool{}

	for n, c := range t.Columns {
		switch {
		case c.Name == "":
			findings = append(findings, Finding{
				Rule: "missing_column_name", Severity: SeverityWarning,
				Message: fmt.Sprintf("테이블 %q의 %d번째 컬럼에 이름이 없음 (정의 셀 값이 %q)",
					t.Name, n+1, c.RawValue),
				Brief: fmt.Sprintf("%d번째 컬럼에 이름이 없음 (정의 셀 값이 %q)",
					n+1, c.RawValue),
				Action:      "정의 셀에 «컬럼명 타입» 형식으로 적는다",
				Scope:       ScopeTable,
				DiagramName: d.Name, DiagramID: d.ID, TableName: t.Name, CellID: c.ID,
			})
			continue // 이름이 없으면 중복도 타입도 물을 것이 없다
		case c.Type == "":
			findings = append(findings, Finding{
				Rule: "missing_column_type", Severity: SeverityWarning,
				Message: fmt.Sprintf("컬럼 %s.%s에 타입이 없음 (정의 셀 값이 %q)",
					t.Name, c.Name, c.RawValue),
				Brief: fmt.Sprintf("%d번째 컬럼 %s에 타입이 없음 (정의 셀 값이 %q)",
					n+1, c.Name, c.RawValue),
				Action:      "컬럼명 뒤에 타입을 적는다",
				Scope:       ScopeTable,
				DiagramName: d.Name, DiagramID: d.ID, TableName: t.Name, CellID: c.ID,
			})
		}

		if seen[c.Name] {
			findings = append(findings, Finding{
				Rule: "duplicate_column_name", Severity: SeverityWarning,
				Message: fmt.Sprintf("테이블 %q에 컬럼 %q가 둘 이상 있음",
					t.Name, c.Name),
				Brief:       fmt.Sprintf("%d번째 컬럼 %s가 앞에도 있음", n+1, c.Name),
				Action:      "둘 중 하나의 이름을 고친다",
				Scope:       ScopeTable,
				DiagramName: d.Name, DiagramID: d.ID, TableName: t.Name, CellID: c.ID,
			})
		}
		seen[c.Name] = true
	}

	return findings
}

// relationshipFindings는 «그림과 키 표기가 어긋난» 관계를 잡는다.
//
// 2026-09-04에 sakila로 잰 실수 열 가지 중 마지막까지 남았던 것이다. 관계선을
// 그려 놓고 그 컬럼의 키 셀을 비워 두면 정의서의 FK 칸도 「참조」 칸도 비고
// DDL에 FOREIGN KEY도 안 나가는데, 그때까지 아무도 그것을 말하지 않았다 —
// 그림에는 선이 멀쩡히 있으니 사람은 문서가 맞는 줄 안다.
//
// 판정은 model.Diagram.UnmatchedRelationships에 맡긴다. 리포터도 같은 것을
// 쓰므로 «문서에 안 실린 관계»와 «진단이 붙은 관계»가 정확히 같은 집합이 된다.
func relationshipFindings(d model.Diagram) []Finding {
	var findings []Finding
	for _, r := range d.UnmatchedRelationships() {
		findings = append(findings, Finding{
			Rule: "relationship_without_fk", Severity: SeverityWarning,
			Message: fmt.Sprintf("관계 %s는 %s를 잇지만 어느 쪽이 외래키인지 알 수 없음"+
				" (한쪽 키 셀에만 FK를 적어라 — 지금은 정의서의 참조 칸과 DDL의"+
				" FOREIGN KEY에 이 관계가 실리지 않는다)",
				r.ID, relationshipEnds(d, r)),
			Brief: fmt.Sprintf("%s를 잇는데 어느 쪽이 외래키인지 알 수 없음",
				relationshipEnds(d, r)),
			Action:      "자식 쪽 컬럼의 키 셀에만 FK를 적는다",
			Scope:       ScopeRelation,
			DiagramName: d.Name, DiagramID: d.ID, CellID: r.ID,
		})
	}
	return findings
}

// relationshipEnds는 관계의 양 끝을 사람이 읽을 한 조각으로 만든다. 셀 id만
// 주면 사용자가 그림에서 그것을 찾을 방법이 마땅치 않다.
func relationshipEnds(d model.Diagram, r model.Relationship) string {
	name := func(tableID, columnID string) string {
		for _, t := range d.Tables {
			if t.ID != tableID {
				continue
			}
			for _, c := range t.Columns {
				if c.ID == columnID {
					return t.Name + "." + c.Name
				}
			}
			return t.Name
		}
		return "?"
	}
	return fmt.Sprintf("%s ↔ %s",
		name(r.SourceTableID, r.SourceColumnID), name(r.TargetTableID, r.TargetColumnID))
}
