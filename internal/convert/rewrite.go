// internal/convert/rewrite.go
package convert

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"

	"erdtool/internal/drawio"
)

// Edit는 셀 하나에 적용할 변경이다.
//
//	Label       — 새 표시값(맨 mxCell의 value, <object>의 label)
//	LogicalName — 함께 보존할 논리명(한글 원문)
type Edit struct {
	Label       string
	LogicalName string
}

// logicalNameAttr은 논리명을 담는 커스텀 속성 이름이다.
//
// draw.io의 «데이터 편집» 창에 그대로 뜨고, draw.io가 쓰는 예약 속성
// (label, placeholders, link, tooltip)과 겹치지 않는다.
const logicalNameAttr = "logicalName"

// CellEdit는 셀 하나에 적용할 변경이다. 포인터 필드가 nil이면 «안
// 바꾼다»는 뜻이며, 빈 문자열을 넣는 것과 구별된다 — annotate는 label을
// 건드리지 않고 속성만 더해야 하므로 이 구별이 필수다.
//
// 그래서 완전히 빈 CellEdit{}는 «이 셀을 손대지 마라»와 같고, 그대로
// 통과한다. 에러가 아니다: 호출자가 셀별로 편집을 조립하다 결과가
// 비는 것은 정상이고(annotate의 revertMarkEdit가 저장된 BaseStyle이
// 없을 때 Style을 nil로 두는 것이 그 예다), 그때마다 호출자가 맵에서
// 키를 도로 빼게 만들면 «넣을지 말지»의 판단이 두 군데로 갈린다.
// 다만 그 셀은 편집 목록에 이름이 올라 있으므로 재작성기의 감싸기·
// 벗기기 갈래를 «지나가기는 한다» — 결과가 원본과 같을 뿐이다.
type CellEdit struct {
	Label *string           // nil이면 안 바꾼다
	Style *string           // nil이면 안 바꾼다. 안쪽 mxCell의 style을 통째로 교체한다(부분 치환 아님)
	Attrs map[string]string // 커스텀 속성. 값이 ""면 그 속성을 지운다

	// Unwrap이 true고, 이 편집을 다 적용한 뒤 <object>/<UserObject>
	// 래퍼에 label·id 말고 남은 속성이 없으면, 그 래퍼를 벗겨 맨
	// <mxCell>로 되돌린다(rewriteStart 참고).
	//
	// 이 결정은 호출자가 내려야 한다 — convert 자신은 어떤 래퍼가
	// «편집 전에 이미 비어 있었을 뿐인 사용자의 것»인지 «편집으로
	// 방금 비워진 우리 것»인지 구별할 방법이 없다. 예전에는 "편집 후
	// label·id만 남으면 무조건 벗긴다"는 구조만 보는 규칙을 썼는데,
	// 그 규칙은 사용자가 원래부터 label·id만 가진 빈 래퍼를 쓰고
	// 있었을 때도 똑같이 걸려 그 래퍼를 지워버렸다. Unwrap을 호출자
	// opt-in으로 만들면 그 판단(annotate라면 erdtoolWrapped 표를
	// 봤는지)은 호출자 몫이 되고, convert는 "정말 비었을 때만"이라는
	// 구조적 안전장치(onlyLabelAndID)만 최후 방어선으로 유지한다 —
	// 호출자가 Unwrap을 잘못 세워도 다른 정보가 남아 있으면 절대
	// 안 벗긴다.
	Unwrap bool
}

// NewCell은 <root> 끝에 새로 넣을 셀이다.
//
// Attrs가 비어 있으면 예전처럼 맨 mxCell 하나로 나간다. Attrs가 있으면
// rewriteStart의 mxCell 감싸기 갈래와 같은 모양으로 <object>에 싸서
// 낸다 — 그래야 annotate처럼 «이 셀은 내가 만들었다»는 표를 새로 심는
// 셀에도 붙일 수 있다. draw.io는 커스텀 데이터를 <object>/<UserObject>의
// 속성으로만 표현하고 <mxCell>은 원래 그 속성을 못 갖기 때문이다.
//
// 두 모양이 갈리는 기준이 «Attrs가 비었는가» 하나인 이유: 감싸는 것은
// 공짜가 아니다. <object>는 draw.io의 «데이터 편집» 창에 줄을 하나
// 만들고, 커스텀 속성이 하나도 없는 래퍼는 사용자에게 보이는 빈 줄일
// 뿐이다. 실을 것이 있을 때만 감싼다.
//
// Attrs는 «커스텀» 속성만을 위한 것이다. id와 label은 래퍼의 구조적
// 속성이므로 여기 담으면 안 되고, 담으면 encodeNewWrappedCell이 거부한다
// — 조용히 넘기면 id가 두 번 실린 깨진 XML이 나가거나(Go의 인코더는
// 중복 속성을 안 잡는다) label이 Value 대신 갈린다.
type NewCell struct {
	ID, Value, Style, Parent string
	X, Y, Width, Height      float64
	Attrs                    map[string]string
}

// PagePlan은 <diagram> 하나에 적용할 변경이다.
//
// 페이지를 찾는 것은 오로지 RewritePlan.Pages의 첨자다. DiagramID는
// «찾는» 데 안 쓰고 오직 단언에만 쓴다 — 계획을 세운 순회와 그것을
// 적용하는 순회가 어긋났는지 확인하는 용도다.
type PagePlan struct {
	DiagramID string
	Edits     map[string]CellEdit

	// Deletes: 셀 id -> XML 서브트리(자식 요소·mxGeometry·Array as="points"
	// 등 이 요소 안에 «중첩»된 모든 것)를 통째로 지운다.
	//
	// «서브트리»는 XML 중첩만을 뜻하지, draw.io의 셀 그래프를 뜻하지
	// 않는다. draw.io에서 테이블의 행이나 셀 사이 간선은 parent/source/
	// target 속성으로만 다른 셀을 가리키는 평평한 목록이지 그 셀의 XML
	// 자식이 아니다. 그러므로 어떤 id를 Deletes에 넣어도, parent나
	// source/target으로 그 id를 가리키던 다른 셀은 지워지지 않고
	// 매달린 참조로 남는다 — 지운 테이블의 행, 지운 셀을 잇던 간선이
	// 그대로다. 그래프 상 자식까지 지우고 싶은 호출자는 그 자식들의
	// id도 전부 직접 Deletes에 넣어야 한다.
	Deletes map[string]bool

	Inserts []NewCell // <root> 끝에 넣을 셀
}

// RewritePlan은 파일 전체의 계획이다. Pages의 순서가 곧 <diagram>의
// 등장 순서다.
//
// 왜 diagram id가 아니라 순서인가: <diagram>에 id가 없는 파일이 실제로
// 있고(손편집·다른 도구 산출물), id로 키를 잡으면 그런 페이지가 조용히
// 안 바뀐다. 순서로 주소하면 id가 없어도 그대로 닿는다.
//
// 순서 결합의 최악의 실패는 «조용히 한 칸 밀리는 것»이다. 그래서
// RewriteMxFilePlan이 계획 수와 DiagramID를 매번 단언한다. 정렬이
// 성립하는 근거는 구조적이다 — 계획을 세우는 쪽이 쓰는
// drawio.LoadDiagramsBytes는 <diagram>을 하나도 건너뛰지 않는다(셀이
// 0개면 건너뛰는 대신 에러를 낸다) — 단, <diagram>이 루트의 직계 자식이고
// 중첩되지 않는 한이다. LoadDiagramsBytes는 정확히 루트 직계 자식만
// 페이지로 세고, RewriteMxFilePlan의 토큰 순회는 깊이를 안 가려 페이지
// 몸통 안에 중첩된 <diagram>까지 센다 — 그런 손편집 입력에서는 둘의
// 개수가 갈릴 수 있고, 그때는 정렬이 아니라 단언 2·3이 에러로 잡는다
// (경위: 원장의 "이월한 minor" 목록).
type RewritePlan struct {
	Pages []PagePlan
}

// cellEditsFrom은 사전 변환의 Edit 맵을 재작성기가 아는 CellEdit 맵으로
// 옮긴다. LogicalName은 항상 «값이 있으면 채우고 없으면 지운다»는
// 뜻이므로 빈 문자열도 그대로 Attrs에 넣는다 — applyAttrs가 빈 값을
// «지운다»로 해석해 그 뜻과 맞아떨어진다.
//
// 편집이 없으면 nil을 돌려준다. PagePlan.Edits가 nil인 것은 «이 페이지엔
// 손댈 것이 없다»와 같고, RewriteCellsPlan이 그때 순수 왕복을 한다.
func cellEditsFrom(edits map[string]Edit) map[string]CellEdit {
	if len(edits) == 0 {
		return nil
	}
	out := make(map[string]CellEdit, len(edits))
	for id, e := range edits {
		label := e.Label
		out[id] = CellEdit{
			Label: &label,
			Attrs: map[string]string{logicalNameAttr: e.LogicalName},
		}
	}
	return out
}

// RewriteCellsPlan은 mxGraphModel XML을 토큰 단위로 재작성한다.
//
// 왜 토큰 스트리밍인가: 구조체로 읽고 다시 쓰면 «구조체에 없는» 속성과
// 요소가 조용히 사라진다(레이어, mxPoint, Array as="points", 스타일 확장,
// 사용자 커스텀 데이터). 문자열 치환은 이 저장소가 존재하는 이유인 문자열
// 수준 매칭을 다시 들여온다. 토큰을 그대로 흘려보내면 «빠뜨릴 수 있는
// 필드»라는 개념 자체가 없다.
//
// p.Edits에 없는 셀은 손대지 않는다. p가 비어 있으면 순수 왕복이다.
// p.Inserts는 이 페이지의 <root> 끝에 그대로 들어간다 — 예전에는 키 ""를
// 예약어로 쓰는 맵이었는데, 「빈 문자열을 키로도 쓰고 sentinel로도 쓴다」는
// 겹침이 id 없는 <diagram>에 삽입이 새는 사고의 원인이었다. 넣을
// <mxGraphModel><root>가 이 페이지에 아예 없으면 에러다(조용히 버리지
// 않는다 — 아래 inserted 변수의 주석).
//
// 삽입은 멱등이 아니다. 같은 plan을 같은 파일에 두 번 적용하면 같은 셀이
// 두 번 들어간다 — 이 함수는 «무엇이 이미 있는지»를 안 본다. 「지우고 다시
// 넣는다」로 멱등성을 만드는 것은 호출자의 몫이며, annotate.Annotate가
// 옛 요약 박스를 Deletes에 먼저 넣는 것이 바로 그 일이다.
func RewriteCellsPlan(src []byte, p PagePlan) ([]byte, error) {
	dec := xml.NewDecoder(bytes.NewReader(src))
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)

	// depth는 현재 요소 깊이다. closeObjectAt에 담긴 깊이의 요소가 닫힐 때
	// 우리가 새로 연 <object>도 함께 닫는다.
	depth := 0
	closeObjectAt := map[int]bool{}
	// wrapperAt[d]는 «깊이 d의 요소가 <object>/<UserObject>인가»다. 그
	// 바로 아래 <mxCell>은 래퍼의 반쪽이지 독립된 셀이 아니므로 새로
	// 감싸면 안 된다 — 아래 rewriteStart의 mxCell 갈래 참고.
	wrapperAt := map[int]bool{}
	// wrapperIDAt[d]는 «깊이 d의 <object>/<UserObject>가 가진 id»다.
	// 안쪽 mxCell은 자기 id가 없거나 우리 모델에 없는 id를 갖는데,
	// style은 그 안쪽에 있다. 래퍼 id를 들고 내려가지 않으면 감싸인
	// 셀의 스타일에 영영 닿지 못한다.
	wrapperIDAt := map[int]string{}
	// nameAt[d]는 «깊이 d에 지금 열려 있는 요소의 이름»이다. </root>가
	// 닫힐 때 이걸로 «진짜 root인가»(부모, 즉 nameAt[d-1]이
	// mxGraphModel인가)를 가린다 — 깊이만 보면, mxGraphModel 밖에서 온
	// 알 수 없는 <root>라는 이름의 요소(이 재작성기가 «미지의 내용은
	// 그대로 흘려보낸다»고 약속한 바로 그 대상, 예를 들어 어떤 셀의
	// 자식으로 들어온 이름이 겹치는 요소)가 우연히 같은 깊이에 있을 때
	// 삽입이 거기서도 한 번 더 나가는 사고를 막는다.
	nameAt := map[int]string{}
	// skipUntil이 0보다 크면 그 깊이로 돌아올 때까지 모든 토큰을 버린다.
	// 서브트리 통째 삭제이므로 자식·기하·mxPoint가 전부 함께 사라진다 —
	// 부모만 지우고 자식을 남기면 부모 없는 고아 셀이 남는다.
	skipUntil := 0
	// inserted는 «p.Inserts를 실제로 내보냈다»다. mxGraphModel 안의
	// <root>가 아예 없는 입력(손편집·다른 도구 산출물)에서는 아래 </root>
	// 갈래에 한 번도 안 걸리므로 삽입이 통째로 사라지는데, 그것을 그냥
	// 넘기면 이 함수는 nil 에러로 «성공»을 돌려주고 호출자(annotate)는
	// 「페이지 N개 요약」이라고 보고한다 — 파일에는 아무것도 없다.
	// 이 저장소가 존재하는 이유인 «툴은 성공을 보고하는데 산출물이
	// 기대와 다르다»를 재작성기 자신이 저지르는 자리라 끝에서 확인한다.
	inserted := false
	// unwrapAt[d]는 «깊이 d의 <object>/<UserObject>를 벗기는 중»이다.
	// 커스텀 속성이 편집으로 전부 사라져 더는 감쌀 이유가 없어진 래퍼를
	// 원래 모습(맨 mxCell)으로 되돌릴 때 쓴다 — annotate가 진단을 지우면
	// (erdtoolIssue·erdtoolBaseStyle을 ""로 없애면) 마킹 때 새로 씌운
	// <object>가 이 상태가 된다. 그 래퍼의 시작/끝 태그를 아예 안 내보내고,
	// 대신 안쪽 mxCell에 래퍼가 갖고 있던 id/value를 옮겨 붙인다 — 그래야
	// 벗긴 결과가 «처음부터 감싸이지 않았던» 원본과 바이트까지 같아진다
	// (annotate의 Changed=false / 원본 복원 계약).
	unwrapAt := map[int]unwrapInfo{}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode token: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			// 삭제 판정은 이 세 맵(wrapperAt·wrapperIDAt·nameAt)에 아무것도
			// 쓰기 전에 끝내야 한다. 버리는 서브트리의 래퍼·이름 정보가
			// 여기 남으면, 이 깊이가 나중에 «남의» 요소로 다시 채워질 때
			// 지운 서브트리 안에 있었던 것처럼 오염된 상태를 물려받는다
			// (예: 안 지워진 형제가 엉뚱하게 감싸이거나 스타일을 잘못 받는다).
			if skipUntil > 0 {
				continue // 이미 버리는 중이다
			}
			if id, ok := attrValue(t.Attr, "id"); ok && p.Deletes[id] {
				// 삭제가 편집을 이긴다: 여기서 바로 버리므로 아래
				// rewriteStart(안에서 p.Edits를 조회하는 코드)에 아예
				// 도달하지 않는다 — 같은 id가 Edits에도 있어도 적용될
				// 기회 자체가 없다.
				skipUntil = depth
				continue
			}
			nameAt[depth] = t.Name.Local
			insideWrapper := wrapperAt[depth-1]
			wrapperAt[depth] = t.Name.Local == "object" || t.Name.Local == "UserObject"
			if wrapperAt[depth] {
				if id, ok := attrValue(t.Attr, "id"); ok {
					wrapperIDAt[depth] = id
				}
			}

			if u, unwrapping := unwrapAt[depth-1]; insideWrapper && unwrapping && t.Name.Local == "mxCell" {
				// 벗기는 래퍼의 반쪽이다. rewriteStart의 mxCell 갈래(특히
				// «id/value 없이 style만 조회»하는 insideWrapper 분기)를
				// 타면 감싸인 채로 남는 것을 전제한 로직과 뒤섞인다 —
				// 여기서 직접 만든다. 그래도 style 편집(p.Edits[u.id].Style)은
				// 그대로 적용해야 한다 — annotate가 마크를 되돌릴 때 바로 이
				// 경로로 style을 원래 값으로 되돌리기 때문이다. 안 그러면
				// 래퍼는 벗겨져도 빨간 테두리 style만 남는다.
				if err := encodeUnwrappedCellStart(enc, t, u, p); err != nil {
					return nil, err
				}
				continue
			}

			out, openObject, unwrap, err := rewriteStart(t, p, insideWrapper, wrapperIDAt[depth-1])
			if err != nil {
				return nil, err
			}
			if unwrap != nil {
				// 이 <object>/<UserObject> 시작 태그는 내보내지 않는다 —
				// 짝이 되는 EndElement도 아래에서 함께 억누른다. 실제 내용은
				// 바로 아래 mxCell 갈래(위)가 낸다.
				unwrapAt[depth] = *unwrap
				continue
			}
			if openObject != nil {
				if err := enc.EncodeToken(*openObject); err != nil {
					return nil, fmt.Errorf("encode object start: %w", err)
				}
				closeObjectAt[depth] = true
			}
			if err := enc.EncodeToken(out); err != nil {
				return nil, fmt.Errorf("encode start: %w", err)
			}

		case xml.EndElement:
			if skipUntil > 0 {
				// 버리는 서브트리의 끝이다. </root> 삽입도 closeObjectAt도
				// nameAt/wrapperIDAt 정리도 건드리지 않는다 — 이 요소는
				// 애초에 그 어떤 맵에도 기록된 적이 없으니 정리할 것도 없다.
				if depth == skipUntil {
					skipUntil = 0
				}
				depth--
				continue
			}
			if _, unwrapping := unwrapAt[depth]; unwrapping {
				// 벗긴 래퍼의 닫는 태그다. 시작 태그를 안 냈으니 짝이 되는
				// 이 끝 태그도 안 낸다 — root 삽입 판정(다음 블록)도 이
				// 요소 이름이 "object"/"UserObject"라 어차피 걸리지 않지만,
				// closeObjectAt처럼 «우리가 안 연 것을 닫는» 사고를 만들지
				// 않도록 아예 별도 갈래로 뺀다.
				delete(unwrapAt, depth)
				delete(wrapperIDAt, depth)
				delete(nameAt, depth)
				depth--
				continue
			}
			// </root>가 닫히기 직전에 새 셀을 끼워 넣는다. root 뒤에는
			// mxGraphModel의 다른 속성이 없으니 «마지막 자식» 자리가 항상
			// 여기다 — 순서를 바꾸면 draw.io가 읽는 트리는 같아도, 두 번
			// 실행했을 때 삽입 지점이 갈려 멱등성 비교가 깨진다.
			//
			// 이름만 보고 «root»라고 판단하지 않는다: nameAt[depth-1]로
			// 부모가 실제로 mxGraphModel인지까지 본다. 그러지 않으면
			// mxGraphModel 밖에서 온, 우연히 이름이 같은 <root> 요소(이
			// 함수가 «모르는 내용은 그대로 흘려보낸다»고 약속한 대상)를
			// 닫을 때도 걸려 같은 셀을 두 번 내보낸다.
			if t.Name.Local == "root" && nameAt[depth-1] == "mxGraphModel" {
				for _, nc := range p.Inserts {
					if err := encodeNewCell(enc, nc); err != nil {
						return nil, err
					}
				}
				inserted = true
			}
			if err := enc.EncodeToken(t); err != nil {
				return nil, fmt.Errorf("encode end: %w", err)
			}
			if closeObjectAt[depth] {
				delete(closeObjectAt, depth)
				if err := enc.EncodeToken(xml.EndElement{Name: xml.Name{Local: "object"}}); err != nil {
					return nil, fmt.Errorf("encode object end: %w", err)
				}
			}
			delete(wrapperIDAt, depth)
			delete(nameAt, depth)
			depth--

		default:
			if skipUntil > 0 {
				continue // 버리는 서브트리 안의 CharData 등도 함께 버린다
			}
			if err := enc.EncodeToken(tok); err != nil {
				return nil, fmt.Errorf("encode token: %w", err)
			}
		}
	}

	if len(p.Inserts) > 0 && !inserted {
		return nil, fmt.Errorf(
			"셀 %d개를 넣을 <mxGraphModel><root>가 이 페이지에 없다", len(p.Inserts))
	}

	if err := enc.Flush(); err != nil {
		return nil, fmt.Errorf("flush: %w", err)
	}
	return buf.Bytes(), nil
}

// rewriteStart는 StartElement 하나를 손본다.
//
// 두 번째 반환값이 non-nil이면 그 토큰을 «먼저» 내보낸다 — 맨 mxCell을
// <object>로 새로 감쌀 때 쓴다. 세 번째 반환값(*unwrapInfo)이 non-nil이면
// 정반대다 — 이 <object>/<UserObject>는 아예 내보내지 말고, 안쪽 mxCell을
// 그 값(id/value)을 옮겨 붙인 맨 mxCell로 대신 내보내라는 뜻이다(호출자인
// RewriteCellsPlan이 실제 인코딩을 한다 — 이 함수는 순수하게 «무엇을 할지»만
// 정한다).
//
// 감쌀 때의 모양은 draw.io가 실제로 쓰는 것과 같다(실측 확인):
// <object>가 label과 id를 갖고, 안쪽 mxCell은 id도 value도 갖지 않는다.
// 벗길 때는 그 역이다.
//
// insideWrapper는 이 요소의 «바로 위»가 <object>/<UserObject>인지다.
// enclosingID는 그 래퍼의 id다(insideWrapper가 false면 빈 문자열) — 안쪽
// mxCell 자신의 id로는 편집을 조회할 수 없으므로(이중 래핑 사고, 아래
// mxCell 갈래 참고) 래퍼 id를 대신 쓴다.
func rewriteStart(t xml.StartElement, p PagePlan, insideWrapper bool, enclosingID string) (xml.StartElement, *xml.StartElement, *unwrapInfo, error) {
	if len(p.Edits) == 0 {
		return t, nil, nil, nil
	}

	switch t.Name.Local {
	// UserObject도 함께 본다: 파서(internal/drawio/xmlraw.go)가 object와
	// UserObject를 똑같이 «id를 가진 셀»로 평탄화하므로, 여기서 UserObject를
	// 빠뜨리면 그 id에 건 편집이 에러도 없이 사라진다.
	case "object", "UserObject":
		id, ok := attrValue(t.Attr, "id")
		if !ok {
			return t, nil, nil, nil
		}
		e, ok := p.Edits[id]
		if !ok {
			return t, nil, nil, nil
		}
		attrs := make([]xml.Attr, len(t.Attr))
		copy(attrs, t.Attr)
		if e.Label != nil {
			attrs = setAttr(attrs, "label", *e.Label)
		}
		attrs = applyAttrs(attrs, e.Attrs)
		t.Attr = attrs

		// 호출자가 Unwrap을 세우지 않았으면 벗기기는 아예 고려하지 않는다
		// — 이 래퍼가 «편집으로 방금 비었을 뿐인 우리 것»인지 «원래부터
		// 이 모양이던 사용자의 것»인지는 호출자만 안다(annotate라면
		// erdtoolWrapped 표로 안다. CellEdit.Unwrap 주석 참고).
		//
		// Unwrap이 세워졌어도 편집을 다 적용한 뒤 label·id 말고 남은
		// 속성이 있으면(logicalName이나 사용자가 손으로 넣은 값 등)
		// 벗기지 않는다 — 호출자의 판단이 틀렸을 가능성에 대한 마지막
		// 방어선이다. 지어낸 판단으로 남의 것을 지우면 안 된다는
		// 원칙(전역 원칙)이 여기도 그대로 적용된다.
		if e.Unwrap && onlyLabelAndID(attrs) {
			// value 복원 여부는 "label 속성이 있는가"만으로 정한다 —
			// "값이 비었는가"가 아니다. 감싸는 쪽(위 mxCell 갈래)이 이미
			// «value 속성이 원래 있었을 때만 label을 낸다»고 정리해
			// 뒀으므로, 여기서는 그 신호를 곧이곧대로 뒤집기만 하면
			// 된다. label=""도 «있었다»에 속한다 — 값이 빈 문자열인
			// value 속성도 진짜 존재하는 속성이고(이 저장소의
			// fixtureWithMissingPK의 r1k가 실제로 이 모양이다), 벗길 때
			// 그 속성을 지워버리면 그 셀만 순수 왕복 결과와 달라진다
			// (코드리뷰의 재발 — hasLabel && label != "" 로 판정하던
			// 예전 버전이 정확히 이 사고를 냈다).
			label, hasLabel := attrValue(attrs, "label")
			return t, nil, &unwrapInfo{id: id, value: label, hasValue: hasLabel}, nil
		}
		return t, nil, nil, nil

	case "mxCell":
		// <object>/<UserObject>의 바로 아래 mxCell은 «래퍼의 반쪽»이지
		// 독립된 셀이 아니다. 파서(internal/drawio/xmlraw.go)도 둘을 셀
		// 하나로 평탄화하면서 id를 언제나 래퍼 것으로 덮어쓴다 — 안쪽
		// mxCell의 id는 우리 모델에 존재하지 않는 id다. 그러니 그것으로
		// edits를 조회해서는 안 된다.
		//
		// 조회하면 무슨 일이 벌어지는가: <object id="t1">이 안쪽
		// <mxCell id="t1">을 감싸고 있으면 위 갈래가 래퍼를 갱신한 뒤
		// 이 갈래가 같은 id로 또 걸려 안쪽 mxCell을 <object>로 한 번 더
		// 감싼다. 이중 래핑된 결과를 다시 읽으면 바깥 래퍼의 mxCell
		// 자식이 없어 style도 parent도 빈 셀이 되고, 테이블이 파싱
		// 결과에서 통째로 사라진다. 그런데 Stats는 Converted:1 —
		// «조용히 틀린 산출물을 성공으로 보고»다.
		//
		// 예전에는 «id가 없으면 감싸여 들어온 것»이라는 대용 판정을 썼다.
		// 그 판정은 안쪽 mxCell에 id가 붙어 있는 순간 무너진다. 구조를
		// 직접 보는 것이 대용물보다 좁고 정확하다.
		if insideWrapper {
			// 래퍼의 반쪽이다. id로 조회하면 안 되지만(위 이중 래핑 사고),
			// style은 여기 있으므로 «래퍼의 id»로 조회해 스타일만 바꾼다.
			// label·Attrs는 래퍼 쪽 갈래(object/UserObject)가 이미 담당하고
			// 있으니 여기서 손대면 같은 편집을 두 번 적용하는 꼴이 된다.
			//
			// 래퍼를 벗기는 경우는 여기 안 온다: 벗기기로 정해지면 위
			// object/UserObject 갈래가 unwrapInfo를 돌려주고, 호출자
			// (RewriteCellsPlan)가 이 mxCell의 시작 태그를 이 함수를 거치지
			// 않고 직접 만든다 — rewriteStart를 타면 아래 "id 없으면 조회
			// 불가" 판정과 감싸기 재판정이 뒤섞여 다시 감싸는 사고가 난다.
			if enclosingID == "" {
				return t, nil, nil, nil
			}
			e, ok := p.Edits[enclosingID]
			if !ok || e.Style == nil {
				return t, nil, nil, nil
			}
			attrs := make([]xml.Attr, len(t.Attr))
			copy(attrs, t.Attr)
			t.Attr = setAttr(attrs, "style", *e.Style)
			return t, nil, nil, nil
		}
		id, ok := attrValue(t.Attr, "id")
		if !ok {
			return t, nil, nil, nil
		}
		e, ok := p.Edits[id]
		if !ok {
			return t, nil, nil, nil
		}
		// style만 있고 label도 커스텀 속성도 없다면 <object>로 감쌀 이유가
		// 없다. 감싸면 파일이 커지고 diff가 요란해지며, --clean이 원본
		// 바이트를 복원해야 하는 작업 13에도 걸림돌이 된다 — 지워도 되는
		// 래퍼가 새로 생기기 때문이다.
		if e.Label == nil && len(e.Attrs) == 0 {
			attrs := make([]xml.Attr, len(t.Attr))
			copy(attrs, t.Attr)
			if e.Style != nil {
				attrs = setAttr(attrs, "style", *e.Style)
			}
			t.Attr = attrs
			return t, nil, nil, nil
		}
		// 감싸는 순간 안쪽 mxCell의 value를 지운다(아래 removeAttr). Label이
		// nil이면 새 값이 없다는 뜻이지, 표시값을 지운다는 뜻이 아니므로
		// 원래 value를 래퍼의 label로 그대로 옮긴다 — 옮기지 않으면 label이
		// nil인 편집(annotate가 속성만 더할 때)마다 표시 텍스트가 사라진다.
		//
		// label 속성을 낼지 말지는 "값이 비었는가"가 아니라 "value 속성이
		// 원래 있었는가"로 가른다. 원본 mxCell에 value 속성 자체가 없던
		// 셀(행·간선처럼)과 value=""를 명시적으로 갖고 있던 셀(이 저장소의
		// fixtureWithMissingPK 안 r1k가 실제로 이 모양이다)은 서로 다른
		// 사실이다 — "값이 비었다"만 보면 이 둘을 구별 못 하고, 나중에
		// 벗길 때(encodeUnwrappedCellStart) value="" 를 가졌던 셀에서
		// value 속성 자체를 지워버리는 사고(코드리뷰 재발)로 이어진다.
		// e.Label이 세팅돼 있으면(nil이 아니면) 항상 값이 있는 것으로
		// 친다 — 호출자가 명시적으로 표시값을 정한 것이므로 원래 value
		// 속성 유무와 무관하게 label을 내야 한다.
		label := ""
		hadValue := e.Label != nil
		if v, ok := attrValue(t.Attr, "value"); ok {
			label = v
			hadValue = true
		}
		if e.Label != nil {
			label = *e.Label
		}
		// 새로 감쌀 때는 언제나 <object>다. draw.io가 새로 만들 때 쓰는
		// 요소가 그것이고, 우리가 왕복시켜 실측한 모양도 그것이다.
		// <UserObject>는 «이미 그렇게 감싸여 들어온 것을 갱신»할 때만 만난다.
		var wrapperAttrs []xml.Attr
		if hadValue {
			// value 속성이 원래 없던 셀은 label 자체를 안 낸다 — 벗길 때
			// (unwrap) "label 속성이 있는가"만으로 "value 속성이 있었는가"를
			// 되살려야 하므로(hasLabel 참고), 여기서 없는 값을 ""로
			// 지어내면 그 신호가 거짓이 된다.
			wrapperAttrs = append(wrapperAttrs, xml.Attr{Name: xml.Name{Local: "label"}, Value: label})
		}
		wrapperAttrs = applyAttrs(wrapperAttrs, e.Attrs)
		wrapperAttrs = append(wrapperAttrs, xml.Attr{Name: xml.Name{Local: "id"}, Value: id})
		wrapper := xml.StartElement{Name: xml.Name{Local: "object"}, Attr: wrapperAttrs}

		attrs := make([]xml.Attr, len(t.Attr))
		copy(attrs, t.Attr)
		if e.Style != nil {
			attrs = setAttr(attrs, "style", *e.Style)
		}
		attrs = removeAttr(attrs, "id")
		attrs = removeAttr(attrs, "value")
		t.Attr = attrs
		return t, &wrapper, nil, nil
	}

	return t, nil, nil, nil
}

// unwrapInfo는 <object>/<UserObject> 래퍼를 벗길 때, 그 반쪽인 mxCell에
// 옮겨 붙일 id/value다. hasValue가 false면 value 속성 자체를 안 낸다 —
// 아래 encodeUnwrappedCellStart 주석 참고.
type unwrapInfo struct {
	id       string
	value    string
	hasValue bool
}

// onlyLabelAndID는 «label과 id 말고 다른 속성이 없는가»를 본다. 있으면
// 그 래퍼는 다른 누군가의 정보(draw.io 예약 속성이든 사용자 값이든)를 들고
// 있다는 뜻이므로 벗기면 안 된다.
func onlyLabelAndID(attrs []xml.Attr) bool {
	for _, a := range attrs {
		if a.Name.Local != "label" && a.Name.Local != "id" {
			return false
		}
	}
	return true
}

// encodeUnwrappedCellStart는 벗기는 래퍼의 반쪽인 mxCell을 원래 모습(맨
// mxCell)으로 낸다. id/value를 앞자리에 두고, 이 mxCell이 원래 갖고 있던
// 나머지 속성(style/vertex/parent/source/target...)은 순서를 바꾸지 않고
// 그대로 잇는다.
//
// 이 mxCell 자신이 (드물게도) id/value를 이미 갖고 있을 수 있다 — 우리가
// 직접 감쌀 때는 항상 지우지만(위 mxCell 갈래의 wrap 코드), 이 파일이
// 우리 손을 거치지 않고 이미 이 모양(<object id="t1"><mxCell id="t1"
// value="주문" …>)으로 들어왔을 수도 있다. 먼저 지우지 않고 앞에 새로
// 붙이면 id/value가 중복되어 XML 1.0 Unique Attribute Spec을 어긴다 —
// Go의 디코더는 조용히 받아주지만(그래서 이 저장소의 어떤 테스트도 못
// 잡았다) draw.io의 DOMParser는 거부한다(코드리뷰).
//
// hasValue가 false면(래퍼에 label 속성 자체가 없었다면) value 속성을
// 아예 안 낸다. tableRow·edge처럼 애초에 value 속성이 없던 셀은 그대로
// «value 속성 없음»으로 돌아가야 한다. label="" (값은 비었지만 속성은
// 있다 — 원래 value=""였던 셀, 이 저장소의 fixtureWithMissingPK의
// r1k가 실제로 이 모양이다)와는 다르다: 그때는 hasValue가 true이므로
// value=""를 낸다. "값이 비었는가"로 판정하면 이 둘을 못 가르고, 원래
// value=""였던 셀에서까지 속성을 지워버려 그 셀만 순수 왕복 결과와
// 달라진다(코드리뷰 — 처음 고침이 "hasLabel && label != \"\""로
// 판정해 바로 이 사고를 냈다. 지금은 hasLabel 단독으로만 판정한다).
//
// 속성 순서는 id, value, (원래 순서 그대로인 나머지) 순이다. draw.io가
// bare mxCell을 쓸 때의 실측 순서(id, value, style, ...)가 이것이므로
// 대개의 실사용 파일에서는 벗긴 결과가 원본과 순서까지 같다. 다만 이
// 함수는 «벗기기 전 원본의 진짜 속성 순서»를 어디에도 저장해 두지
// 않으므로(erdtoolBaseStyle은 style 문자열만 기억한다), 손으로
// style을 parent보다 뒤에 적은 것처럼 draw.io의 관례를 벗어난 순서로
// 쓰인 파일이라면 벗긴 뒤 속성의 "집합과 값"은 정확히 같아도 "순서"는
// 원본과 달라질 수 있다. 이 한계는 의도적으로 안 고친다 — 벗기기 전
// 순서를 통째로 기억해 두는 장치를 새로 만드는 비용이 이 저장소가
// 실제로 만나는 파일(전부 draw.io 산출물)에 비해 과하다(코드리뷰,
// 다운그레이드됨). `TestRewriteCellsPlanUnwrapPreservesAttributeSetRegardlessOfOrder`가
// "순서가 달라도 집합과 값은 정확히 같다"는 것만 못박는다.
//
// p.Edits[u.id].Style도 여기서 함께 적용한다. 이 mxCell은 rewriteStart의
// insideWrapper 갈래(호출자가 일부러 건너뛰었다)를 대신하는 자리이므로,
// 그 갈래가 원래 하던 style 교체를 여기서 대신 해야 한다 — 안 하면
// 래퍼는 벗겨지는데 마크 때 씌운 빨간 테두리 style은 그대로 남는다.
func encodeUnwrappedCellStart(enc *xml.Encoder, t xml.StartElement, u unwrapInfo, p PagePlan) error {
	rest := removeAttr(t.Attr, "id")
	rest = removeAttr(rest, "value")

	attrs := make([]xml.Attr, 0, len(rest)+2)
	attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "id"}, Value: u.id})
	if u.hasValue {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "value"}, Value: u.value})
	}
	attrs = append(attrs, rest...)
	if e, ok := p.Edits[u.id]; ok && e.Style != nil {
		attrs = setAttr(attrs, "style", *e.Style)
	}
	t.Attr = attrs
	if err := enc.EncodeToken(t); err != nil {
		return fmt.Errorf("encode unwrapped cell: %w", err)
	}
	return nil
}

// encodeNewCell은 셀 하나를 mxCell(+ 필요하면 <object> 래퍼) + mxGeometry로
// 내보낸다.
//
// 문자열을 이어 붙이지 않고 토큰으로 내보내는 이유: value에 사용자
// 테이블 이름이 들어가는데 거기에 <나 &가 있으면 문자열 조립은 깨진
// XML을 만든다. 인코더가 이스케이프를 책임진다.
func encodeNewCell(enc *xml.Encoder, nc NewCell) error {
	if len(nc.Attrs) > 0 {
		return encodeNewWrappedCell(enc, nc)
	}
	cell := xml.StartElement{
		Name: xml.Name{Local: "mxCell"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "id"}, Value: nc.ID},
			{Name: xml.Name{Local: "value"}, Value: nc.Value},
			{Name: xml.Name{Local: "style"}, Value: nc.Style},
			{Name: xml.Name{Local: "vertex"}, Value: "1"},
			{Name: xml.Name{Local: "parent"}, Value: nc.Parent},
		},
	}
	if err := enc.EncodeToken(cell); err != nil {
		return fmt.Errorf("encode new cell: %w", err)
	}
	if err := encodeNewGeometry(enc, nc); err != nil {
		return err
	}
	return enc.EncodeToken(xml.EndElement{Name: cell.Name})
}

// encodeNewWrappedCell은 Attrs가 있는 NewCell을 <object>로 감싸 낸다.
// 모양은 rewriteStart의 mxCell 감싸기 갈래(id 없는 새 <object>)와
// 같다 — id/value는 래퍼가 갖고, 안쪽 mxCell은 style/vertex/parent만
// 갖는다. 커스텀 속성(annotate의 erdtoolAnnotation 등)은 <mxCell>이 아니라
// <object>에만 실을 수 있는 draw.io의 제약이 이 모양을 강제한다.
func encodeNewWrappedCell(enc *xml.Encoder, nc NewCell) error {
	// applyAttrs는 id를 append하기 «전»에 돈다(순서를 바꾸면 이미 나간
	// 파일들과 속성 순서가 갈려 멱등성 비교가 한 번 깨진다). 그래서
	// Attrs에 id가 오면 setAttr이 못 찾아 그대로 두 번 실리고, XML은
	// 같은 속성을 두 번 가질 수 없으므로 draw.io가 못 여는 파일이 나간다
	// — Go의 encoding/xml은 그것을 에러로 잡지 않는다. label은 덮어써서
	// Value가 조용히 사라진다. 둘 다 호출자의 프로그래밍 실수이므로
	// 시끄럽게 멈춘다.
	for _, reserved := range []string{"id", "label"} {
		if _, ok := nc.Attrs[reserved]; ok {
			return fmt.Errorf(
				"NewCell.Attrs에 구조적 속성 %q가 있다 — 커스텀 속성만 담을 수 있다", reserved)
		}
	}

	wrapperAttrs := []xml.Attr{{Name: xml.Name{Local: "label"}, Value: nc.Value}}
	wrapperAttrs = applyAttrs(wrapperAttrs, nc.Attrs)
	wrapperAttrs = append(wrapperAttrs, xml.Attr{Name: xml.Name{Local: "id"}, Value: nc.ID})
	wrapper := xml.StartElement{Name: xml.Name{Local: "object"}, Attr: wrapperAttrs}
	if err := enc.EncodeToken(wrapper); err != nil {
		return fmt.Errorf("encode new wrapped cell: %w", err)
	}

	cell := xml.StartElement{
		Name: xml.Name{Local: "mxCell"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "style"}, Value: nc.Style},
			{Name: xml.Name{Local: "vertex"}, Value: "1"},
			{Name: xml.Name{Local: "parent"}, Value: nc.Parent},
		},
	}
	if err := enc.EncodeToken(cell); err != nil {
		return fmt.Errorf("encode new cell: %w", err)
	}
	if err := encodeNewGeometry(enc, nc); err != nil {
		return err
	}
	if err := enc.EncodeToken(xml.EndElement{Name: cell.Name}); err != nil {
		return err
	}
	return enc.EncodeToken(xml.EndElement{Name: wrapper.Name})
}

// encodeNewGeometry는 새 셀의 mxGeometry를 낸다. 감싼 모양과 안 감싼 모양이
// 이 부분만은 완전히 같으므로 따로 뺐다.
func encodeNewGeometry(enc *xml.Encoder, nc NewCell) error {
	geo := xml.StartElement{
		Name: xml.Name{Local: "mxGeometry"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "x"}, Value: formatCoord(nc.X)},
			{Name: xml.Name{Local: "y"}, Value: formatCoord(nc.Y)},
			{Name: xml.Name{Local: "width"}, Value: formatCoord(nc.Width)},
			{Name: xml.Name{Local: "height"}, Value: formatCoord(nc.Height)},
			{Name: xml.Name{Local: "as"}, Value: "geometry"},
		},
	}
	if err := enc.EncodeToken(geo); err != nil {
		return fmt.Errorf("encode new geometry: %w", err)
	}
	return enc.EncodeToken(xml.EndElement{Name: geo.Name})
}

// formatCoord는 좌표를 문자열로 만든다. 정수면 소수점을 안 붙인다 —
// draw.io가 쓰는 모양이 그것이고, 같은 그림에 다른 바이트가 나오면
// 멱등성 테스트가 무의미해진다.
func formatCoord(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// applyAttrs는 커스텀 속성을 적용한다. 값이 빈 문자열이면 «지운다»는
// 뜻이다 — --clean이 표식을 없앨 때 이 규칙을 쓴다. 빈 값을 남기면
// draw.io의 «데이터 편집» 창에 빈 줄이 계속 뜨고, 두 번째 실행에서
// «표식이 있다»로 오판된다.
func applyAttrs(attrs []xml.Attr, kv map[string]string) []xml.Attr {
	names := make([]string, 0, len(kv))
	for k := range kv {
		names = append(names, k)
	}
	// 맵 순회 순서가 실행마다 다르면 같은 입력에 다른 바이트가 나온다.
	// 멱등성이 이 서브커맨드의 계약이므로 정렬한다. (ATTRIBUTE 방출
	// 순서에 대한 얘기이고, style 문자열 안의 키 순서와는 무관하다.)
	sort.Strings(names)
	for _, k := range names {
		if kv[k] == "" {
			attrs = removeAttr(attrs, k)
			continue
		}
		attrs = setAttr(attrs, k, kv[k])
	}
	return attrs
}

func attrValue(attrs []xml.Attr, name string) (string, bool) {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}

func setAttr(attrs []xml.Attr, name, value string) []xml.Attr {
	for i, a := range attrs {
		if a.Name.Local == name {
			attrs[i].Value = value
			return attrs
		}
	}
	return append(attrs, xml.Attr{Name: xml.Name{Local: name}, Value: value})
}

// removeAttr는 속성 하나를 뺀 «새» 슬라이스를 돌려준다.
//
// attrs[:0]으로 백킹 배열을 재사용하지 않는 이유: 그러면 입력 슬라이스의
// 원소가 앞으로 밀려 원본이 조용히 망가진다. [id value style parent]에서
// value를 빼면 백킹 배열이 [id style parent parent]가 되고, 반환 슬라이스만
// 보면 맞지만 원본 슬라이스를 계속 들고 있는 호출자는 깨진 것을 보게 된다.
func removeAttr(attrs []xml.Attr, name string) []xml.Attr {
	out := make([]xml.Attr, 0, len(attrs))
	for _, a := range attrs {
		if a.Name.Local == name {
			continue
		}
		out = append(out, a)
	}
	return out
}

// RewriteMxFilePlan은 .drawio 파일 전체를 재작성한다. 각 diagram의 내용만
// RewriteCellsPlan에 넘기고, 압축 여부는 입력을 그대로 따른다.
//
// 우리가 임의로 평문으로 펴면 사용자가 draw.io에서 다시 저장할 때 또
// 달라져서, 어느 쪽이 우리 탓인지 알 수 없게 된다.
//
// p.Pages[i]가 i번째 <diagram>에 적용된다. 편집·삭제·삽입이 전부 그
// 페이지 안에서만 일어나므로, 셀 id가 페이지 사이에서 겹쳐도 한쪽의
// 편집이 다른 쪽에 번지지 않는다.
func RewriteMxFilePlan(src []byte, p RewritePlan) ([]byte, error) {
	dec := xml.NewDecoder(bytes.NewReader(src))
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)

	pageIdx := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode mxfile token: %w", err)
		}

		start, isStart := tok.(xml.StartElement)
		if !isStart || start.Name.Local != "diagram" {
			if err := enc.EncodeToken(tok); err != nil {
				return nil, fmt.Errorf("encode mxfile token: %w", err)
			}
			continue
		}

		var pp PagePlan
		if len(p.Pages) > 0 {
			if pageIdx >= len(p.Pages) {
				return nil, fmt.Errorf(
					"계획은 페이지 %d개인데 파일의 <diagram>이 더 많다 — 계획을 세운 순회와 적용하는 순회가 어긋났다",
					len(p.Pages))
			}
			pp = p.Pages[pageIdx]
			if pp.DiagramID != "" {
				if id, hasID := attrValue(start.Attr, "id"); hasID && id != pp.DiagramID {
					return nil, fmt.Errorf(
						"계획 %d번은 페이지 %q용인데 그 자리의 <diagram> id는 %q다 — 계획을 세운 순회와 적용하는 순회가 어긋났다",
						pageIdx, pp.DiagramID, id)
				}
			}
		}
		pageIdx++

		inner, hasElement, err := captureInner(dec)
		if err != nil {
			return nil, err
		}
		replacement, err := rewriteDiagramInner(inner, hasElement, pp)
		if err != nil {
			return nil, err
		}

		if err := enc.EncodeToken(start); err != nil {
			return nil, fmt.Errorf("encode diagram start: %w", err)
		}
		if hasElement {
			// 평문 mxGraphModel: 인코더를 비운 뒤 원시 바이트를 그대로 쓴다.
			// EncodeToken으로 다시 흘리면 이미 재작성한 결과를 두 번 처리하게 된다.
			if err := enc.Flush(); err != nil {
				return nil, fmt.Errorf("flush before raw: %w", err)
			}
			buf.Write(replacement)
		} else {
			if err := enc.EncodeToken(xml.CharData(replacement)); err != nil {
				return nil, fmt.Errorf("encode diagram text: %w", err)
			}
		}
		if err := enc.EncodeToken(xml.EndElement{Name: start.Name}); err != nil {
			return nil, fmt.Errorf("encode diagram end: %w", err)
		}
	}

	if len(p.Pages) > 0 && pageIdx != len(p.Pages) {
		return nil, fmt.Errorf(
			"계획은 페이지 %d개인데 파일의 <diagram>은 %d개다 — 짧은 쪽에 맞춰 자르면 나머지 페이지가 침묵 속에서 계획을 잃는다",
			len(p.Pages), pageIdx)
	}

	if err := enc.Flush(); err != nil {
		return nil, fmt.Errorf("flush mxfile: %w", err)
	}
	return buf.Bytes(), nil
}

// captureInner는 현재 열린 요소의 내용을 끝까지 읽어 원시 바이트로 돌려준다.
// hasElement는 그 안에 자식 «요소»가 있었는지다 — 압축된 diagram은 텍스트
// 뿐이고, 평문 diagram은 mxGraphModel 요소를 갖는다. 이 판정을 문자열의
// 첫 글자로 하지 않는 이유는, 그것이 이 저장소가 반복해서 데인 문자열 수준
// 판정이기 때문이다.
func captureInner(dec *xml.Decoder) (inner []byte, hasElement bool, err error) {
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	var text bytes.Buffer
	depth := 0

	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, false, fmt.Errorf("capture diagram inner: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			hasElement = true
			depth++
			if err := enc.EncodeToken(t); err != nil {
				return nil, false, err
			}
		case xml.EndElement:
			if depth == 0 {
				if err := enc.Flush(); err != nil {
					return nil, false, err
				}
				if hasElement {
					return buf.Bytes(), true, nil
				}
				return bytes.TrimSpace(text.Bytes()), false, nil
			}
			depth--
			if err := enc.EncodeToken(t); err != nil {
				return nil, false, err
			}
		case xml.CharData:
			if hasElement {
				if err := enc.EncodeToken(t); err != nil {
					return nil, false, err
				}
			} else {
				text.Write(t)
			}
		default:
			if hasElement {
				if err := enc.EncodeToken(tok); err != nil {
					return nil, false, err
				}
			}
		}
	}
}

// rewriteDiagramInner는 diagram 하나의 내용을 재작성한다. 압축이었으면
// 풀어서 처리하고 다시 같은 방식으로 압축한다.
func rewriteDiagramInner(inner []byte, hasElement bool, p PagePlan) ([]byte, error) {
	if hasElement {
		return RewriteCellsPlan(inner, p)
	}
	if len(inner) == 0 {
		return inner, nil
	}
	plain, err := drawio.Decompress(string(inner))
	if err != nil {
		return nil, fmt.Errorf("decompress diagram: %w", err)
	}
	out, err := RewriteCellsPlan([]byte(plain), p)
	if err != nil {
		return nil, err
	}
	packed, err := drawio.Compress(string(out))
	if err != nil {
		return nil, fmt.Errorf("compress diagram: %w", err)
	}
	return []byte(packed), nil
}

// CountDiagrams는 파일의 <diagram> 수다. RewritePlan.Pages의 길이를
// 맞춰야 하는 호출자를 위한 것이다.
//
// 이 함수의 순회 규칙은 RewriteMxFilePlan의 것과 같아야 한다 —
// <diagram> 시작 토큰을 조건 없이 전부 센다.
func CountDiagrams(src []byte) (int, error) {
	dec := xml.NewDecoder(bytes.NewReader(src))
	n := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return n, nil
		}
		if err != nil {
			return 0, fmt.Errorf("count diagrams: %w", err)
		}
		if st, ok := tok.(xml.StartElement); ok && st.Name.Local == "diagram" {
			n++
		}
	}
}

// ForEachGraphModel은 .drawio의 각 diagram 내용을 «평문 mxGraphModel»
// 바이트로 넘겨 준다. 압축된 페이지는 풀어서 넘긴다.
//
// 파서(drawio.LoadDiagrams)가 커스텀 속성을 버리므로, 그것을 읽어야 하는
// 쪽은 원문을 다시 읽을 수밖에 없다. collectLogicalNames가 바로 그 이유로
// 이미 이 읽기를 하고 있고, annotate.ScanExisting(작업 11)도 같은 것이
// 필요하다. 패키지마다 새로 쓰지 않도록 여기서 한 번만 만든다.
func ForEachGraphModel(src []byte, fn func(pageIdx int, diagramID string, graphModel []byte) error) error {
	dec := xml.NewDecoder(bytes.NewReader(src))
	pageIdx := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("scan mxfile: %w", err)
		}
		st, ok := tok.(xml.StartElement)
		if !ok || st.Name.Local != "diagram" {
			continue
		}
		// 첨자는 <diagram>을 만날 때마다 무조건 증가시킨다 — 아래 continue
		// (내용이 빈 압축 diagram)로 콜백이 안 불려도 마찬가지다. 그래야
		// 첨자가 CountDiagrams·LoadDiagramsBytes와 계속 정렬된다.
		idx := pageIdx
		pageIdx++
		id, _ := attrValue(st.Attr, "id")
		inner, hasElement, err := captureInner(dec)
		if err != nil {
			return err
		}
		graphModel := inner
		if !hasElement {
			// 압축된 diagram이다. 실사용 .drawio는 대부분 이 모양이므로,
			// 풀지 않고 그대로 넘기면 호출자는 base64 텍스트를 mxGraphModel
			// XML로 착각해 파싱에서 항상 «아무것도 없음»을 얻는다.
			if len(inner) == 0 {
				continue
			}
			plain, err := drawio.Decompress(string(inner))
			if err != nil {
				return fmt.Errorf("decompress diagram: %w", err)
			}
			graphModel = []byte(plain)
		}
		if err := fn(idx, id, graphModel); err != nil {
			return err
		}
	}
}
