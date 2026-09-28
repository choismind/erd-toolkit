package annotate

import "erdtool/internal/validate"

// CellMark는 셀 하나에 붙일 진단들이다.
type CellMark struct {
	Messages []string // 이 셀의 진단들. 등장 순
	// OutOfScope는 이 셀에 붙은 진단이 «읽지 않았다»뿐일 때 참이다. 한
	// 셀에 결함이 하나라도 섞이면 거짓이 되어 빨강으로 칠해진다 — 고쳐야
	// 할 것이 회색에 묻히면 안 된다.
	OutOfScope bool
}

// PagePlan은 페이지 하나에 적용할 계획이다.
type PagePlan struct {
	DiagramID string
	Marks     map[string]CellMark // 셀 id -> 마크
	Summary   []string            // 요약 박스에 적을 줄들
}

// BuildPlans는 findings를 페이지별 계획으로 나눈다. XML도 파일도 건드리지
// 않는 순수 함수다 — annotate 파이프라인에서 "무엇을 어디에 쓸지"만
// 결정하고, 실제로 쓰는 일은 뒤 단계(Task 10, 12)가 한다.
//
// order는 페이지 등장 순서(diagram id)다. findings를 diagram id로 묶을 때
// Go map을 그대로 순회해 결과를 만들면 같은 입력에도 실행마다 다른 바이트
// 순서가 나온다 — annotate 명령을 두 번 돌려도 같은 파일이 나와야 한다는
// Phase 3의 계약이 깨진다. 그래서 페이지 순서는 반드시 order를, 한 셀 안의
// 메시지 순서는 반드시 findings의 원래 순서를 따른다.
//
// 마크 대상은 «warning이고 앵커(CellID)가 있는» 것뿐이다. 그리고 그
// 판정보다 **먼저** 페이지가 정해져야 한다 — 페이지를 가로지르는 finding은
// CellID가 있더라도 마크되지 않고 요약에만 실린다. 어느 페이지의 그
// 셀인지가 정해지지 않았는데 마크하면 같은 id를 가진 다른 페이지의 셀을
// 지목하게 되고, 그것은 이 저장소가 페이지 단위화로 없앤 바로 그 사고다.
// 오늘 그런 생산자는 없다(아래 duplicate_table_name은 CellID도 비어
// 있다). 언젠가 생기면 여기가 아니라 **그 생산자**가 페이지를 채워야 한다.
//
// 이유:
//   - duplicate_table_name은 ir.DuplicateNameWarning에서 오는데, 그 값에는
//     페이지 id가 아예 없다. DiagramName·DiagramID·CellID가 모두 비어
//     있고, 어느 페이지들인지는 Message 안에만 적힌다.
//
// 그래서 "셀이 없다"는 사실을 지어내지 않고 요약에만 싣는다. 반대로 요약에는
// 그 페이지에 관련된 진단이 전부 들어간다 — 마크된 것도 포함해서. 마크는
// "어디를 보라"는 화살표일 뿐, 무슨 진단이 났는지 다 읽으려면 요약이
// 필요하다.
//
// 페이지를 가로지르는 finding은 모든 페이지의 요약에 싣는다. 그 신호는
// «DiagramID가 비었다»가 아니라 **DiagramName까지 비었다**이다.
// id 없는 <diagram>의 진단도 DiagramID가 비기 때문이다(validate가 d.ID를
// 그대로 적는다). 둘을 안 가르면 쓸 수도 없는 페이지의 진단이 남은 모든
// 페이지의 요약 박스로 새어 나가 사용자에게 보인다. 그 페이지는 Annotate가
// order에서 빼고 «건너뛴다»고 경고까지 하므로, 여기서는 조용히 버린다 —
// 아래 «order에 없는 DiagramID» 분기가 그 일을 그대로 한다.
func BuildPlans(findings []validate.Finding, order []string) []PagePlan {
	byPage := make(map[string]*PagePlan, len(order))
	for _, id := range order {
		byPage[id] = &PagePlan{DiagramID: id, Marks: map[string]CellMark{}}
	}

	for _, f := range findings {
		if f.DiagramID == "" && f.DiagramName == "" {
			// 페이지를 가로지르는 진단이다 — 모든 페이지의 요약에 싣는다.
			for _, id := range order {
				byPage[id].Summary = append(byPage[id].Summary, f.Message)
			}
			continue
		}

		p, ok := byPage[f.DiagramID]
		if !ok {
			// order는 파싱된 문서에서 온 페이지 목록이다. 그 안에 없는
			// DiagramID는 호출자 쪽 문제지만, 여기서 패닉하거나 존재하지
			// 않는 페이지를 지어내지 않고 조용히 버린다(방어적).
			// id 없는 <diagram>의 진단(DiagramID="", DiagramName≠"")도
			// 여기로 떨어져 버려진다.
			continue
		}

		p.Summary = append(p.Summary, f.Message)

		if f.CellID == "" {
			continue // 앵커 없는 진단은 요약에만 남긴다.
		}
		if f.Severity != validate.SeverityWarning && !f.OutOfScope {
			// 안내는 요약에만 남긴다. 대상 밖은 예외다 — 안내이면서도
			// 회색 테두리를 받아야 하므로 심각도만으로 거르면 안 된다.
			continue
		}
		m, seen := p.Marks[f.CellID]
		if !seen {
			m.OutOfScope = true // 첫 진단이다. 아래에서 실제 값으로 좁힌다.
		}
		m.Messages = append(m.Messages, f.Message)
		m.OutOfScope = m.OutOfScope && f.OutOfScope
		p.Marks[f.CellID] = m
	}

	out := make([]PagePlan, 0, len(order))
	for _, id := range order {
		p := byPage[id]
		if len(p.Summary) == 0 {
			// 진단 없는 페이지엔 빈 PagePlan조차 만들지 않는다 — Task 12가
			// "계획이 있으면 요약 박스를 만든다"고 판단하므로, 빈 계획을
			// 만들면 "이상 없음" 박스가 지어내듯 생겨버린다.
			continue
		}
		out = append(out, *p)
	}
	return out
}
