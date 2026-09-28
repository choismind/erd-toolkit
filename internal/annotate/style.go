package annotate

import "strings"

const (
	// markStrokeColor는 «고쳐야 할 것»이다.
	markStrokeColor = "#FF3333"
	// outOfScopeStrokeColor는 «이 도구가 읽지 않은 것»이다. 잘못이 아니라
	// 대상 밖이므로 빨강과 확실히 갈리는 회색을 쓴다 — 색만 보고 둘을
	// 가를 수 있어야 한다.
	outOfScopeStrokeColor = "#999999"
	markStrokeWidth       = "3"
)

// MarkStyle은 진단이 붙은 셀의 스타일에 «빨간 굵은 테두리»만 얹는다.
//
// drawio.ParseStyle이 주는 map은 순회 순서가 없다 — 맵에서 다시 조립하면
// 같은 입력이 실행마다 다른 바이트 순서로 나올 위험이 있는 것과는 별개로,
// draw.io 스타일 문자열의 첫 토큰은 "="이 없는 맨 도형 이름(text, ellipse,
// swimlane...)일 수 있다. 이 저장소의 fixture
// (internal/drawio/testdata/table_level_relations.drawio 등)에 실제로
// "text;strokeColor=none;..." 꼴이 있다. 이 토큰을 알파벳순으로 정렬해
// 재배치하면 "align=left;html=1;text;"가 되어 draw.io가 다른 도형을
// 그린다. 그래서 여기서는 map을 만들지 않고 원본 문자열을 ";"로 나눈
// 순서 그대로 걸으며 strokeColor/strokeWidth 두 세그먼트만 제자리에서
// 바꾸고, 나머지는 원래 자리에 원래 모습으로 둔다. 없던 키는 걷기가
// 끝난 뒤에만 새로 덧붙인다. 입력 순서 자체가 결정적이므로 정렬 없이도
// 멱등성(TestMarkStyleIsIdempotent)은 그대로 성립한다.
//
// 키 비교는 이 저장소의 규칙대로 "=" 앞부분을 정확히(==) 비교한다.
// "shape=table"에 ";strokeColor=..."를 그냥 이어 붙이거나 부분 문자열
// 매칭으로 지우려 들면 "shape=table"이 "shape=tableRow"까지 건드리는,
// 이 저장소가 존재하는 이유인 그 실수가 재발한다.
//
// 출력은 순서를 빼면 정규화된다 — 세그먼트 앞뒤 공백은 지워지고, 빈
// 세그먼트(연속된 ";;", 맨 끝의 ";")는 버려지며, 결과는 언제나 ";"로
// 끝난다(예: " a=1 ;;b=2 " -> "a=1;b=2;strokeColor=...;strokeWidth=...;").
// 그래서 이 함수는 «원본 바이트 보존»이 아니라 «순서 보존»을 약속한다.
// 손편집 파일과 일부 도구가 실제로 저런 스타일을 내는데, 그것을 그대로
// 두면 마크한 셀과 안 한 셀의 스타일 표기가 한 파일 안에서 갈린다.
// 복원(--clean)이 되돌리는 기준은 erdtoolBaseStyle에 «마크 전 그대로»
// 저장해 둔 문자열이므로 이 정규화는 복원에 영향을 주지 않는다.
func MarkStyle(style string) string {
	return markStyleWith(style, markStrokeColor)
}

// MarkStyleOutOfScope는 «읽지 않은 도형»에 회색 테두리를 얹는다. 하는 일은
// MarkStyle과 같고 색만 다르다.
func MarkStyleOutOfScope(style string) string {
	return markStyleWith(style, outOfScopeStrokeColor)
}

// edgeFlags는 «어느 모서리에 선을 그릴지»를 정하는 키들이다. draw.io는
// shape=tableRow와 shape=partialRectangle에서 이 넷을 보고 그릴 변을
// 고른다 — 넷 다 0이면 strokeColor를 줘도 **그릴 변이 없어서 아무것도
// 안 보인다.**
//
// 이것 때문에 행에 붙인 표식이 화면에 안 나온 적이 있다(2026-09-22).
// 진단 셋을 심은 그림을 annotate로 마크했더니 테이블에 붙은 것 하나만
// 빨갛고 행에 붙은 둘은 보이지 않았다. 파일에는 strokeColor=#FF3333이
// 멀쩡히 들어 있었다 — 리포트와 그림이 어긋나 보이는데 원인이 파일에
// 안 드러나는, 이 저장소가 가장 싫어하는 모양이었다.
//
// 실물 draw.io가 저장한 ERD도 «마지막 행»만 bottom=1이고 나머지 행은
// 넷 다 0이다. 그러니 손으로 그린 그림에서도 같은 일이 난다.
var edgeFlags = map[string]bool{"top": true, "left": true, "right": true, "bottom": true}

func markStyleWith(style, strokeColor string) string {
	segments := strings.Split(style, ";")

	// 모서리 플래그를 켤지는 «이 셀이 그것을 쓰는 도형인지»로 정한다.
	// 쓰지 않는 도형에 top=1 따위를 덧붙이면 draw.io가 모르는 키가 하나
	// 늘 뿐이지만, 그래도 안 붙이는 편이 원본에 덜 손대는 것이다.
	usesEdgeFlags := false
	for _, raw := range segments {
		switch strings.TrimSpace(raw) {
		case "shape=tableRow", "shape=partialRectangle":
			usesEdgeFlags = true
		}
	}

	out := make([]string, 0, len(segments)+2)
	sawStrokeColor := false
	sawStrokeWidth := false

	for _, raw := range segments {
		seg := strings.TrimSpace(raw)
		if seg == "" {
			// 빈 세그먼트(연속된 ";;", 맨 끝의 ";")는 정보가 없으니 버린다.
			// 어차피 마지막에 하나로 다시 붙이므로 지워도 순서에 영향이 없다.
			continue
		}

		key := seg
		if idx := strings.Index(seg, "="); idx >= 0 {
			key = seg[:idx]
		}

		switch {
		case key == "strokeColor":
			out = append(out, "strokeColor="+strokeColor)
			sawStrokeColor = true
		case key == "strokeWidth":
			out = append(out, "strokeWidth="+markStrokeWidth)
			sawStrokeWidth = true
		case usesEdgeFlags && edgeFlags[key]:
			// 제자리에서 1로 바꾼다. 원래 값은 erdtoolBaseStyle에 그대로
			// 있으므로 --clean이 정확히 되돌린다.
			out = append(out, key+"=1")
		default:
			// 도형 키가 아니면 손대지 않는다 — bare 세그먼트(예: "text")도
			// "="을 안 붙였으니 그대로 bare로 남는다.
			out = append(out, seg)
		}
	}

	if !sawStrokeColor {
		out = append(out, "strokeColor="+strokeColor)
	}
	if !sawStrokeWidth {
		out = append(out, "strokeWidth="+markStrokeWidth)
	}

	// 빈 스타일이어도 out은 strokeColor/strokeWidth 두 개는 반드시 갖고
	// 있으므로 join 결과 맨 앞에 ";"가 남을 일이 없다.
	return strings.Join(out, ";") + ";"
}
