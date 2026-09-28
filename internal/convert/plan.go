// internal/convert/plan.go
package convert

import (
	"strings"

	"erdtool/internal/drawio"
	"erdtool/internal/glossary"
)

// Detail은 이름 하나의 변환 내역이다. --dry-run 표가 이걸 찍는다.
type Detail struct {
	Kind     string // "table" | "column"
	Logical  string
	Physical string
	Parts    []glossary.Part
	Route    glossary.Route
}

// Stats는 파일 하나의 변환 요약이다.
type Stats struct {
	Total     int      // 변환을 시도한 이름 개수
	Converted int      // 실제로 값이 달라진 개수
	Unmatched []string // 사전에 없던 낱말 (중복 제거, 등장 순)
	Details   []Detail
}

// buildEdits는 셀 목록에서 «무엇을 어떻게 바꿀지»를 정한다.
//
// 바꾸는 지점은 둘뿐이다:
//   - 테이블 셀(shape=table + childLayout=tableLayout)의 value 전체
//   - 정의 셀(행의 partialRectangle 자식 중 둘째)의 value 첫 필드
//
// 키 셀·행·엣지·노트·자유도형은 건드리지 않는다.
//
// 페이지 레벨(relational/conceptual)을 보지 않는다. 현재 판별은 "이 파서가
// 인식하는 도형 수 vs 아닌 도형 수"라서 테이블 1개와 장식 도형 5개가 있는
// 페이지가 conceptual로 떨어진다 — 레벨을 조건으로 걸면 그 테이블이 조용히
// 변환에서 빠진다.
//
// logicalOf는 «셀 id -> 이미 저장돼 있던 logicalName»이다. 값이 있으면 그것이
// 변환 소스다. 현재 label을 소스로 삼으면, 사람이 물리명을 손으로 고쳐둔
// 파일을 다시 돌리는 순간 그 손댄 값이 소스가 되어 원래 한글이 영원히
// 사라진다. 논리명이 언제나 진실의 출처다.
func buildEdits(cells []drawio.RawCell, d *glossary.Dict, logicalOf map[string]string) (map[string]Edit, Stats) {
	idx := drawio.BuildIndex(cells)
	edits := map[string]Edit{}
	var stats Stats
	seenUnmatched := map[string]bool{}

	record := func(kind, id, logical, newValue string, res glossary.Result) {
		stats.Total++
		stats.Details = append(stats.Details, Detail{
			Kind:     kind,
			Logical:  logical,
			Physical: res.Physical,
			Parts:    res.Parts,
			Route:    res.Route,
		})
		for _, p := range res.Parts {
			// 공백뿐이거나 빈 조각은 미매칭 목록에 싣지 않는다. 이 목록의
			// 쓸모는 «사용자가 눈으로 읽어 손볼 자리를 찾는» 것인데, 보이지
			// 않는 낱말은 "미매칭 낱말 1개( )"라는 말만 남기고 아무것도
			// 보여주지 않는다 — 뭔가 실패했다고 알리면서 그게 무엇인지는
			// 끝내 말하지 않는 셈이다. glossary가 구분자를 제대로 쪼개므로
			// 지금은 나오지 않지만, 목록의 계약은 여기서 지킨다.
			if p.Matched || strings.TrimSpace(p.Source) == "" || seenUnmatched[p.Source] {
				continue
			}
			seenUnmatched[p.Source] = true
			stats.Unmatched = append(stats.Unmatched, p.Source)
		}
		edits[id] = Edit{Label: newValue, LogicalName: logical}
	}

	for _, table := range drawio.FindTables(cells) {
		logical := source(table.ID, table.Value, logicalOf)
		if strings.TrimSpace(logical) != "" {
			res := d.Convert(strings.TrimSpace(logical))
			record("table", table.ID, strings.TrimSpace(logical), res.Physical, res)
			if res.Physical != table.Value {
				stats.Converted++
			}
		}

		for _, row := range idx.ChildrenOf[table.ID] {
			if drawio.ParseStyle(row.Style)["shape"] != "tableRow" {
				continue
			}
			_, defCell, ok := drawio.RowBoxes(row, idx)
			if !ok {
				continue
			}
			// 이름은 «논리명 우선»에서, 나머지(타입·NOT NULL·공백)는 언제나
			// «현재 셀 값»에서 온다. logicalName에는 이름만 담기므로
			// (논리명은 이름이지 타입이 아니다) 그것을 raw 전체로 취급하면
			// 2회차 실행에서 타입이 에러 없이 통째로 사라진다.
			name, _ := splitFirstField(source(defCell.ID, defCell.Value, logicalOf))
			_, rest := splitFirstField(defCell.Value)
			if name == "" {
				continue
			}
			res := d.Convert(name)
			newValue := res.Physical + rest
			record("column", defCell.ID, name, newValue, res)
			if newValue != defCell.Value {
				stats.Converted++
			}
		}
	}
	return edits, stats
}

// source는 변환의 출처를 고른다. 저장된 논리명이 있으면 그것이 이긴다.
func source(id, current string, logicalOf map[string]string) string {
	if logical, ok := logicalOf[id]; ok && strings.TrimSpace(logical) != "" {
		return logical
	}
	return current
}

// splitFirstField는 "컬럼명 타입 [NOT NULL]"에서 첫 필드와 «나머지 원문»을
// 가른다. 나머지는 공백까지 그대로 보존한다.
//
// 레퍼런스는 value.replace(colName, physical)을 썼는데, 타입 문자열에
// 컬럼명과 같은 글자가 들어 있으면 타입까지 오염된다.
func splitFirstField(raw string) (name, rest string) {
	lead := len(raw) - len(strings.TrimLeft(raw, " \t"))
	body := raw[lead:]
	end := strings.IndexAny(body, " \t")
	if end < 0 {
		return strings.TrimSpace(body), ""
	}
	return body[:end], body[end:]
}
