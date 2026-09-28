// internal/ir/assemble.go
package ir

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"erdtool/internal/drawio"
	"erdtool/internal/model"
)

const currentIRVersion = "1.0"

type DuplicateNameWarning struct {
	TableName string

	// PageNames는 이 이름을 쓰는 테이블이 **있는 페이지들**이다. 중복
	// 없이, 페이지 등장 순.
	//
	// 예전에는 테이블 하나마다 페이지 이름을 밀어 넣어 같은 페이지가
	// 여러 번 들어 있었다. 그것을 세는 쪽(validate.Required)이 «N개
	// 페이지»라고 말했으므로, 한 페이지에 같은 이름 테이블이 셋 있으면
	// 2페이지짜리 파일이 "6개 페이지에 존재함"이라고 보고됐다 — 파일에
	// 없는 숫자가 사용자에게 보이는, 이 저장소가 가장 경계하는 실패
	// 유형이다. 몇 «곳»에 있는지는 아래 Occurrences가 따로 든다.
	PageNames []string

	// Occurrences는 이 이름을 쓰는 테이블의 총 개수다. 언제나
	// len(PageNames) 이상이며, 같으면 «페이지마다 하나씩»이라는 뜻이다.
	Occurrences int
}

// Assemble은 파일 하나를 파싱해 model.Document로 조립한다. 테이블 식별은
// (페이지, 셀 ID) 기준이며, 동일 이름 테이블이 여러 페이지에 있어도 자동
// 병합하지 않고 DuplicateNameWarning으로만 보고한다(스펙: 다중 페이지 처리).
//
// 설정(config.Config)을 받지 않는다. 예전에는 페이지 성격 판정이 여기에
// 있어 페이지별 오버라이드를 봐야 했지만 그 판정 자체가 없어졌다
// (2026-09-06). 조립은 파일에 적힌 것만 보고, 설정이 갈라놓는 것은
// 검증과 리포터 쪽에서 갈린다.
func Assemble(path string) (model.Document, []DuplicateNameWarning, error) {
	rawDiagrams, err := drawio.LoadDiagrams(path)
	if err != nil {
		return model.Document{}, nil, err
	}

	doc := model.Document{IRVersion: currentIRVersion, SourceFile: path}
	// 원본의 수정 시각은 정의서 머리의 «기준 시각»이 된다. 못 읽어도 파싱은
	// 이미 끝난 뒤이므로 실패로 만들지 않는다 — 머리 한 줄이 비는 것과
	// 산출물이 아예 안 나오는 것은 무게가 다르다.
	if st, statErr := os.Stat(path); statErr == nil {
		doc.SourceModified = st.ModTime().Format("2006-01-02 15:04")
	}
	// nameToPages는 테이블 이름 -> 그 이름이 있는 «페이지» 목록(중복 없이,
	// 등장 순)이고, nameToCount는 같은 이름 -> 테이블 총 개수다. 둘을
	// 가르지 않으면 «몇 개 페이지»와 «몇 곳»이 한 숫자로 뭉개진다.
	nameToPages := map[string][]string{}
	nameToCount := map[string]int{}
	seenOnPage := map[string]bool{} // "페이지첨자\x00테이블이름"

	for _, rd := range rawDiagrams {
		idx := drawio.BuildIndex(rd.Cells)
		tableCells := drawio.FindTables(rd.Cells)

		consumed := map[string]bool{}
		var tables []model.Table
		var attrConflicts, unknownAttrs []model.AttrIssue
		for _, tc := range tableCells {
			consumed[tc.ID] = true
			ext := drawio.ExtractTable(tc, idx)
			for _, row := range idx.ChildrenOf[tc.ID] {
				consumed[row.ID] = true
				for _, child := range idx.ChildrenOf[row.ID] {
					consumed[child.ID] = true
				}
			}
			tables = append(tables, ext.Table)
			attrConflicts = append(attrConflicts, ext.Conflicts...)
			unknownAttrs = append(unknownAttrs, ext.Unknown...)
			nameToCount[tc.Value]++
			// 페이지 이름은 겹칠 수 있으므로(draw.io가 막지 않는다) 첨자로
			// 가른다 — 이름으로 가르면 이름이 같은 두 페이지가 한 페이지로
			// 뭉개져 PageNames가 하나만 갖는다.
			key := fmt.Sprintf("%d\x00%s", len(doc.Diagrams), tc.Value)
			if !seenOnPage[key] {
				seenOnPage[key] = true
				nameToPages[tc.Value] = append(nameToPages[tc.Value], rd.Name)
			}
		}

		rels := drawio.ExtractRelationships(rd.Cells, idx, tableCells)
		for _, r := range rels {
			consumed[r.ID] = true
		}

		// 아무것도 잇지 않았지만 그림에서는 테이블에 닿아 있는 관계선.
		// rels에 없으므로 consumed에도 넣어 «분류 안 된 도형»으로 다시
		// 세지 않는다.
		floating := drawio.ExtractFloatingRelations(rd.Cells, tableCells)
		for _, f := range floating {
			consumed[f.ID] = true
		}

		ignoredRaw, violationsRaw := drawio.ClassifyAll(rd.Cells, consumed)
		var ignored []model.IgnoredShape
		for _, ig := range ignoredRaw {
			ignored = append(ignored, model.IgnoredShape{ID: ig.ID, Shape: ig.Shape, Style: ig.Style})
		}
		var violations []model.ShapeViolation
		for _, v := range violationsRaw {
			violations = append(violations, model.ShapeViolation{
				ID: v.ID, Shape: v.Shape, Style: v.Style, Reason: v.Reason})
		}

		doc.Diagrams = append(doc.Diagrams, model.Diagram{
			ID:                rd.ID,
			Name:              rd.Name,
			Tables:            tables,
			Relationships:     rels,
			FloatingRelations: floating,
			Ignored:           ignored,
			Violations:        violations,
			AttrConflicts:     attrConflicts,
			UnknownAttrs:      unknownAttrs,
		})
	}

	// 판정은 «출현이 둘 이상»이다 — 한 페이지 안의 중복도 보고할 값이
	// 있다(같은 이름 테이블 둘은 어느 쪽이 진짜인지 사람이 정해야 한다).
	// 그 둘을 문구에서 가르는 것은 validate.Required가 한다.
	var dups []DuplicateNameWarning
	for name, pages := range nameToPages {
		if nameToCount[name] > 1 {
			dups = append(dups, DuplicateNameWarning{
				TableName: name, PageNames: pages, Occurrences: nameToCount[name]})
		}
	}
	// map 순회 순서는 실행마다 다르다. 이 순서는 validate.Required의
	// finding 순서 → 검증 리포트 줄 순서 → annotate가 요약 박스에 쓰는 줄
	// 순서로 그대로 내려가므로, 정렬하지 않으면 annotate를 두 번 돌려도
	// 같은 바이트가 나와야 한다는 Phase 3의 계약이 여기서 깨진다.
	sort.Slice(dups, func(i, j int) bool { return dups[i].TableName < dups[j].TableName })

	return doc, dups, nil
}

func Write(doc model.Document, path string) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
