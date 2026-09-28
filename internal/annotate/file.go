// internal/annotate/file.go
package annotate

import (
	"bytes"
	"fmt"
	"strings"

	"erdtool/internal/convert"
	"erdtool/internal/drawio"
	"erdtool/internal/model"
	"erdtool/internal/validate"
)

// Result는 파일 하나의 처리 결과다.
type Result struct {
	Marked   int  // 마크한 셀 수
	Findings int  // 표시한 진단 수
	Pages    int  // 요약 박스를 만든 페이지 수
	Changed  bool // 파일 내용이 실제로 달라졌는가

	// Warnings는 «멈출 만큼은 아니지만 사용자가 반드시 알아야 하는»
	// 사실들이다. 한국어 한 줄씩이고, CLI가 결과 줄 밑에 그대로 찍는다.
	//
	// 이 필드가 있는 이유: 이 패키지에는 «에러로 멈춘다»와 «조용히
	// 넘어간다» 사이의 자리가 필요하다. 예를 들어 `Annotate`는 `<diagram>`에
	// id가 없는 페이지를 만나면 그 페이지 전체를 건너뛴다(표식도 요약
	// 박스도 못 넣는다) — 파일 전체를 멈출 이유는 아니지만, 그렇다고
	// 조용히 넘어가면 사용자는 «진단이 없다»와 «이 페이지는 아예 처리되지
	// 않았다»를 구별할 수 없다. 멈추지 않되 말한다 — 그 중간을 이 필드가
	// 맡는다. 로깅 프레임워크를 들이지 않는 것은 의도적이다: 결과
	// 구조체에 실어 CLI까지 그대로 흘리는 것이 이 도구에 필요한 전부다.
	//
	// 2026-08-31 갱신: `Clean`은 이제 어떤 경로로도 이 필드에 쓰지 않는다
	// — `convert.RewritePlan`이 페이지 단위로 주소되면서
	// «충돌 id의 스타일 복원을 포기하고 경고한다»는 갈래 자체가 없어졌다. 유일한 생산자는 위 `Annotate`의
	// id 없는 페이지 건너뛰기다.
	Warnings []string
}

// Annotate는 검증 진단을 .drawio 바이트에 표시해 돌려준다.
//
// 진단이 하나도 없고 파일에 기존 표식도 없으면 입력을 그대로 돌려주고
// Changed=false다. «깨끗한 파일에 돌렸더니 파일이 바뀌었다»는 사람을
// 불안하게 만들고, 그 불안이 이 서브커맨드를 안 쓰게 만든다. 재작성기를
// 아예 타지 않는 것도 중요하다 — 토큰 재인코딩은 편집이 하나도 없어도
// 사람이 짠 원문의 표면적 모양(공백·속성 순서)을 바꿀 수 있으므로, 할 일이
// 정말 없을 때는 재작성기 자체를 건너뛴다.
func Annotate(src []byte, doc model.Document, findings []validate.Finding) ([]byte, Result, error) {
	// id 없는 <diagram>은 계획을 세울 자리가 아예 없다. validate가
	// DiagramID를 d.ID(=빈 문자열)로 적으면 BuildPlans는 그 진단을 전부
	// «페이지를 가로지르는 사실»로 읽어 어느 셀도 마크하지 않고,
	// convert.RewriteMxFilePlan은 id 없는 <diagram>에 Inserts를 아예 안
	// 넘긴다(요약 박스도 안 들어간다). 그 페이지를 order에 그대로 넣으면
	// PagePlan이 하나 생겨 res.Pages만 올라가고, erdtool은 아무것도 못
	// 넣은 채로 「페이지 1개 요약」이라고 보고한다 — 화면의 숫자와 파일
	// 안의 사실이 다르다. 세지 않고 건너뛰되, 건너뛰었다고 말한다.
	//
	// draw.io는 언제나 id를 쓰고 genbuild/emit.go도 page-%d를 박으므로
	// 이 경로에 닿는 것은 손편집 파일이나 다른 도구의 산출물뿐이다 —
	// 이 저장소가 신뢰하지 않기로 한 입력이 정확히 그것이다.
	order := make([]string, 0, len(doc.Diagrams))
	var warnings []string
	for _, d := range doc.Diagrams {
		if d.ID == "" {
			warnings = append(warnings, fmt.Sprintf(
				"페이지 %q에 <diagram> id가 없어 건너뛴다 — 이 페이지에는 표식도 요약 박스도 넣을 수 없다", d.Name))
			continue
		}
		order = append(order, d.ID)
	}

	diagrams, err := drawio.LoadDiagramsBytes(src)
	if err != nil {
		return nil, Result{}, err
	}
	cellsOf := make(map[string][]drawio.RawCell, len(diagrams))
	for _, dg := range diagrams {
		cellsOf[dg.ID] = dg.Cells
	}

	perPage, err := ScanExisting(src)
	if err != nil {
		return nil, Result{}, err
	}

	plans := BuildPlans(findings, order)
	hasResidue := false
	for _, e := range perPage {
		if len(e.ResidueIDs()) > 0 || len(e.Summaries) > 0 {
			hasResidue = true
			break
		}
	}
	if len(plans) == 0 && !hasResidue {
		// Findings는 여기서도 실제 진단 수를 싣는다. 계획이 0개인 것은
		// «진단이 없다»와 같지 않다 — 위에서 건너뛴 id 없는 페이지의
		// 진단이 여기로 온다. 0건이라 말하면 CLI가 "(진단 0건)"이라고
		// 찍고, 사용자는 "이 도면에 고칠 게 없다"로 읽는다.
		return src, Result{Findings: len(findings), Warnings: warnings}, nil
	}

	// 계획은 페이지별로 적용된다(convert.RewritePlan.Pages). 셀 id가 페이지
	// 사이에서 겹쳐도 한쪽의 표시가 다른 쪽에 번지지 않으므로, 예전에 여기
	// 있던 «겹치면 멈춘다» 가드는 필요 없다.
	//
	// 페이지 첨자로 계획을 세운다. diagrams는 drawio.LoadDiagramsBytes가
	// 돌려준 순서 그대로이고, perPage(ScanExisting)도 같은 순서다 —
	// 그 정렬은 convert.RewriteMxFilePlan이 다시 단언한다.
	if len(perPage) != len(diagrams) {
		return nil, Result{}, fmt.Errorf(
			"페이지 스캔은 %d개인데 파싱된 다이어그램은 %d개다 — 두 순회가 어긋났다",
			len(perPage), len(diagrams))
	}
	idxOf := make(map[string]int, len(diagrams))
	pages := make([]convert.PagePlan, len(diagrams))
	marked := make([]map[string]bool, len(diagrams))
	for i, dg := range diagrams {
		pages[i] = convert.PagePlan{
			DiagramID: dg.ID,
			Edits:     map[string]convert.CellEdit{},
			Deletes:   map[string]bool{},
		}
		marked[i] = map[string]bool{}
		if dg.ID != "" {
			idxOf[dg.ID] = i
		}
	}

	// 옛 요약 박스는 전부 지운다. 살아남을 것은 아래에서 다시 넣는다.
	// 진단이 고쳐져 없어진 페이지의 박스가 남아 있으면 툴이 거짓말을 한다.
	for i := range pages {
		for id := range perPage[i].Summaries {
			pages[i].Deletes[id] = true
		}
	}

	var res Result
	for _, p := range plans {
		i, ok := idxOf[p.DiagramID]
		if !ok {
			return nil, Result{}, fmt.Errorf(
				"계획이 가리키는 페이지 %q가 파싱된 다이어그램에 없다 — 두 순회가 어긋났다", p.DiagramID)
		}
		cells := cellsOf[p.DiagramID]
		idx := drawio.BuildIndex(cells)

		for cellID, m := range p.Marks {
			c, ok := idx.ByID[cellID]
			if !ok {
				// 검증이 가리킨 셀이 파일에 없다. 지어내지 않고 건너뛴다 —
				// 이 진단은 요약 박스에 이미 실려 있다.
				continue
			}
			base, had := perPage[i].BaseStyle[cellID]
			if !had {
				// 처음 마크하는 셀이다. 지금 스타일이 원본이다.
				base = c.Style
			}
			// base는 절대 덮어쓰지 않는다. 덮어쓰면 두 번째 실행에서
			// 빨간 테두리가 «원래 스타일»로 굳어 복원이 죽는다.
			style := MarkStyle(base)
			if m.OutOfScope {
				style = MarkStyleOutOfScope(base)
			}
			attrs := map[string]string{
				AttrIssue:     strings.Join(m.Messages, " / "),
				AttrBaseStyle: base,
			}
			if !perPage[i].Wrapped[cellID] || perPage[i].WrappedByUs[cellID] {
				// 이 래퍼는 우리 것이다 — 지금 막 새로 감싸지거나(원래
				// 맨 mxCell이었다), 예전 실행에서 이미 우리가 감쌌다.
				// 표시해 둬야 나중에 진단이 사라졌을 때 안전하게 벗길
				// 수 있다(아래 되돌리기 루프의 Unwrap). 이미 다른
				// 이유로 감싸여 있던 남의 래퍼(perPage[i].Wrapped &&
				// !perPage[i].WrappedByUs)에는 이 표를 절대 안 붙인다 —
				// 우리가 만들지 않은 래퍼를 우리 것이라 지어내면, 진단이
				// 사라질 때 그 래퍼(와 그 안의 사용자 속성)까지
				// 벗겨서 지워버리게 된다(코드리뷰).
				attrs[AttrWrapped] = "1"
			}
			pages[i].Edits[cellID] = convert.CellEdit{
				Style: &style,
				Attrs: attrs,
			}
			marked[i][cellID] = true
		}

		// 옛 박스는 위에서 이미 Deletes에 들어가 있다. 여기서 빼지
		// 않는다 — 언제나 «지우고 다시 넣는다». 재작성기가 삭제를
		// 스트림 순서대로 처리하고 삽입은 </root> 직전에 하므로 같은
		// id가 한 파일에 둘 남는 일이 없다. 빼 버리면 옛 박스가 살아
		// 남아 id가 겹치고, 다시 읽을 때 뒤엣것이 앞엣것을 덮는다.
		nc := SummaryCell(p, cells)
		pages[i].Inserts = append(pages[i].Inserts, nc)
		res.Pages++
	}

	// 이번에 마크 대상이 아닌데 표식이 남은 셀은 되돌린다. 고쳐서 진단이
	// 사라졌는데 빨간 테두리가 남아 있으면 그것도 거짓말이다. Clean(작업 13)이
	// «모든 마크된 셀»에 거는 것과 같은 되돌리기이므로 revertMarkEdit로
	// 뺐다 — 갈라 두면 언젠가 한쪽만 고쳐 --clean이 조용히 진짜 역연산이
	// 아니게 된다.
	//
	// perPage[i].Marked(erdtoolIssue)만 보지 않고 perPage[i].ResidueIDs()
	// 전체를 본다 — 코드리뷰가 잡은 사고: 사람이 draw.io의
	// «데이터 편집» 창에서 erdtoolIssue 하나만 지우면 erdtoolBaseStyle·
	// erdtoolWrapped는 남는데 perPage[i].Marked에는 안 잡힌다. 그러면 이
	// 루프가 그 셀을 그냥 지나치고, 빨간 테두리와 잔여 속성이 다음 실행
	// 이후에도 계속 남는다.
	for i := range pages {
		for cellID := range perPage[i].ResidueIDs() {
			if marked[i][cellID] {
				continue
			}
			pages[i].Edits[cellID] = revertMarkEdit(perPage[i], cellID)
		}
	}

	total := 0
	for _, m := range marked {
		total += len(m)
	}
	res.Marked = total
	res.Findings = len(findings)
	res.Warnings = warnings

	out, err := convert.RewriteMxFilePlan(src, convert.RewritePlan{Pages: pages})
	if err != nil {
		return nil, Result{}, err
	}
	// «원본 바이트 그대로»는 여기서 요구하지 않는다. encoding/xml의
	// Encoder는 self-closing 빈 요소(<mxCell id="0"/>)와 여닫는 쌍
	// (<mxCell id="0"></mxCell>)을 토큰 스트림에서 구별하지 못하므로,
	// 재작성기를 한 번이라도 거치면 그 차이는 원리적으로 되돌릴 수
	// 없다 — 이걸 억지로 되돌리려 한 첫 시도(정규식/스캐너 기반
	// 정규화)가 오히려 새 결함을 낳았다(코드리뷰). annotate가
	// 실제로 지켜야 하는 것은 "자기 표식을 하나도 안 남기고, 그 밖의
	// 무엇도 바꾸지 않는다"이며, 그 기준은 원본 자체가 아니라
	// convert.RewriteMxFilePlan(src, RewritePlan{})(순수 왕복 결과) —
	// convert 자신이 이미 쓰고 있는 정규화 형태다. 두 패키지가 같은
	// 입력에 다른 바이트 형태를 내면 둘을 잇는 작업 16이 매번 이
	// 차이에 걸린다.
	res.Changed = !bytes.Equal(src, out)
	return out, res, nil
}

// revertMarkEdit는 마크된 셀 하나를 원래 모습으로 되돌리는 CellEdit를
// 만든다. style을 저장된 BaseStyle로 되돌리고, erdtoolIssue·erdtoolBaseStyle을
// 지운다(빈 값 = 지운다, RewriteCellsPlan.applyAttrs). 래퍼를 벗기는
// 것(Unwrap)은 erdtoolWrapped 표가 있는 셀에만 요청한다 — 그 표가 없으면
// 이 래퍼는 우리 것이 아니다(사용자의 래퍼거나, 이 표가 생기기 전 옛
// 버전이 마크한 셀이거나). 우리가 지어낸 적 없는 소유권을 근거로 남의
// 래퍼(와 그 안의 남의 속성)를 지우지 않는다는 뜻이다.
//
// Style은 existing.BaseStyle에 **값이 실제로 있을 때만** 채운다(코드리뷰)
// . map의 콤마-ok 조회를 쓰는 이유: erdtoolBaseStyle이 없는
// 마크된 셀(사람이 손으로 erdtoolBaseStyle만 지운 경우)에서 존재하지 않는
// 값을 ""로 읽어 CellEdit.Style에 넣으면, style="" 로 실제 모양을 지워
// 버린다 — "저장된 원래 스타일이 빈 문자열이었다"와 "저장된 원래 스타일
// 자체가 없다"는 서로 다른 사실이고(작업 12에서 value=""를 놓고 이미
// 배운 것과 같은 구별), 후자에는 Style을 nil로 둬(CellEdit 주석의 "안
// 바꾼다") 지금 스타일(빨간 테두리 포함)을 그대로 둔다 — 지어낸 빈
// 스타일보다는 훨씬 낫다.
//
// 다만 오늘 그 두 사실은 **실제로는 갈리지 않는다**. ScanExisting이
// erdtoolBaseStyle=""를 «없는 것»으로 읽어 BaseStyle 맵에 아예 안 넣기
// 때문에(existing.go의 scanGraphModel), 콤마-ok가 false를 내는 경우에
// «빈 저장 스타일»과 «저장 자체가 없음»이 함께 들어온다. 지금은 둘 다
// «지금 스타일을 그대로 둔다»가 정답이라 구별이 필요 없다. 언젠가
// 스타일이 진짜로 빈 셀을 복원해야 하면 그 두 자리를 함께 고쳐야 한다 —
// 여기만 고치면 ScanExisting이 그 사실을 이미 지운 뒤라 닿지 않는다.
//
// Annotate는 이번 실행에서 더는 대상이 아닌 마크된 셀 하나하나에 이것을
// 걸고, Clean은 지금 잔재가 남은 셀 전부(ResidueIDs)에 같은 것을 건다 —
// «진단이 사라진 셀을 되돌린다»와 «--clean이 전부 되돌린다」는 같은
// 연산을 다른 범위에 적용하는 것뿐이다. 이 함수 하나로 묶어 두지 않으면
// 언젠가 한쪽만 고쳐 --clean이 조용히 Annotate의 진짜 역연산이 아니게
// 된다.
func revertMarkEdit(existing Existing, cellID string) convert.CellEdit {
	attrs := map[string]string{AttrIssue: "", AttrBaseStyle: ""}
	unwrap := existing.WrappedByUs[cellID]
	if unwrap {
		attrs[AttrWrapped] = ""
	}
	edit := convert.CellEdit{
		Attrs:  attrs,
		Unwrap: unwrap,
	}
	if base, had := existing.BaseStyle[cellID]; had {
		edit.Style = &base
	}
	return edit
}

// Clean은 erdtool이 .drawio에 남긴 표식을 전부 지우고 스타일을 원래대로
// 되돌린다 — 잔재가 남은 셀마다 revertMarkEdit를 걸고, erdtool이 만든
// 요약 박스를 전부 지운다. Annotate의 역연산이다.
//
// 지울 것이 하나도 없으면(잔재도 요약 박스도) 입력을 그대로 돌려주고
// Changed=false다 — Annotate가 «진단 0건이면 재작성기를 아예 안 탄다»는
// 것과 같은 이유다: 재작성기는 편집이 없어도 self-closing 태그를 펼치는
// 등 표면 모양을 바꿀 수 있으므로, 할 일이 정말 없을 때는 재작성기 자체를
// 건너뛴다.
//
// 「지울 것」의 판정은 existing.Marked(erdtoolIssue) 하나만 보지 않고
// existing.ResidueIDs()를 쓴다(코드리뷰). draw.io의 «데이터
// 편집» 창에서 erdtoolIssue 속성 하나만 지우는 것은 사람이 얼마든지 할 수
// 있는 일이고, 그러면 erdtoolBaseStyle·erdtoolWrapped와 빨간 테두리
// style은 그대로 남는데 existing.Marked는 비어 있다. 그 상태에서 옛
// 코드처럼 existing.Marked만 보면 --clean이 Changed=false·바이트 무변화로
// «성공」을 보고하면서 실제로는 아무것도 안 지운다 — 이 저장소가 가장
// 경계하는 실패 유형(조용히 틀린 성공)을 --clean 자신이 저지르는 셈이다.
//
// Result.Pages는 지운 요약 박스 수다(코드리뷰). Marked만 보고
// 있으면 「요약 박스만 남고 마크된 셀은 하나도 없는」 파일에서
// Marked=0인데 실제로는 파일이 바뀌는 경우를 사용자에게 「0개 되돌림」으로
// 잘못 보고하게 된다.
//
// 셀 id가 페이지 사이에서 겹쳐도 그냥 복원한다. 계획이 페이지별로
// 적용되므로 한 페이지의 복원이 다른 페이지의 동명 셀에 닿지 않는다.
// 예전에는 스타일 복원을 포기하고 erdtoolBaseStyle을 남긴 뒤 사용자에게
// «직접 되돌려라»고 경고했는데(원장의 판정 7과 확장 17), 그 둘은 편집이
// 파일 전체에 적용된다는 전제 위에서만 뜻이 있었다. 전제가 사라져 질문
// 자체가 없어졌다.
//
// 이미 그 경로로 erdtoolBaseStyle이 박힌 파일은 저절로 낫는다 —
// ResidueIDs()가 그 속성을 잔재로 세므로, 다음 --clean이 그것을 근거로
// 스타일을 복원하고 속성을 지운다.
//
// 계약(원장의 "멱등성·안전 계약" 절):
//
//	Clean(Annotate(x)) == convert.RewriteMxFilePlan(x, convert.RewritePlan{})
//
// 「원본 바이트와 완전히 같다」가 아니다 — Go의 encoding/xml은
// self-closing 빈 요소(<a/>)와 여닫는 쌍(<a></a>)을 토큰 수준에서
// 구별하지 못해, 재작성기를 한 번이라도 거치면 그 차이는 원리적으로
// 되돌릴 수 없다. annotate가 실제로 지키는 것은 "자기 표식을 하나도 안
// 남기고, 그 밖의 무엇도 안 바꾼다"이며, 그 기준선은 원본 자체가 아니라
// convert 자신이 이미 쓰고 있는 정규화 왕복형이다(Annotate 위 주석 참고).
func Clean(src []byte) ([]byte, Result, error) {
	perPage, err := ScanExisting(src)
	if err != nil {
		return nil, Result{}, err
	}
	total, summaries := 0, 0
	for _, e := range perPage {
		total += len(e.ResidueIDs())
		summaries += len(e.Summaries)
	}
	if total == 0 && summaries == 0 {
		return src, Result{}, nil
	}

	// DiagramID는 비운다 — Clean은 LoadDiagramsBytes를 안 타므로 페이지
	// id를 모르고, 빈 값은 단언을 건너뛴다. 첨자 정렬은 ScanExisting이
	// CountDiagrams로 슬라이스 길이를 잡는 것으로 보장된다.
	pages := make([]convert.PagePlan, len(perPage))
	for i, e := range perPage {
		pages[i] = convert.PagePlan{
			Edits:   map[string]convert.CellEdit{},
			Deletes: map[string]bool{},
		}
		for id := range e.Summaries {
			pages[i].Deletes[id] = true
		}
		for id := range e.ResidueIDs() {
			pages[i].Edits[id] = revertMarkEdit(e, id)
		}
	}

	out, err := convert.RewriteMxFilePlan(src, convert.RewritePlan{Pages: pages})
	if err != nil {
		return nil, Result{}, err
	}
	return out, Result{
		Marked:  total,
		Pages:   summaries,
		Changed: !bytes.Equal(src, out),
	}, nil
}
