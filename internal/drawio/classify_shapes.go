// internal/drawio/classify_shapes.go
package drawio

import "strings"

type ShapeClass int

const (
	ShapeClassParsed ShapeClass = iota
	ShapeClassIgnored
	ShapeClassViolation
)

// ClassifyShape은 원장의 "허용 도형 세트" 3분류를 그대로 구현한다:
//   - Parsed: 테이블 계열(table/tableRow/partialRectangle) + 카디널리티 엣지
//   - Ignored: Note — 구조 데이터 아니지만 경고 없음(어디서나 쓸 수 있는 범용 주석)
//   - Violation: 그 외 전부(Chen 계열, Hierarchy, Cloud, 자유도형)
//
// 이 함수는 스타일만으로 판단 가능한 셀(테이블/행/키정의셀/엣지/노트)을
// 다룬다. Note를 제외한 "값 있는 텍스트 상자" 등은 기본적으로 Violation으로
// 분류되며, 실제 배치(테이블 자식인지 등)에 따른 예외는 ClassifyAll에서
// 처리한다.
func ClassifyShape(cell RawCell) ShapeClass {
	if strings.TrimSpace(cell.Style) == "" {
		// draw.io는 모든 페이지에 style 속성이 아예 없는 구조상 필수 셀
		// (id="0" 루트, id="1" 기본 부모)을 반드시 넣는다. 사용자가 그린
		// 도형이 아니므로 허용 도형 세트 판정 대상이 아니다. 이걸 그냥
		// 두면 스타일 없음 -> 빈 맵 -> 어떤 조건에도 안 걸림 ->
		// ShapeClassViolation으로 떨어져서, 페이지마다 매번 가짜
		// shape_violation 2건이 검증 리포트 맨 위에 뜬다.
		return ShapeClassIgnored
	}
	style := ParseStyle(cell.Style)

	if style["shape"] == "table" && style["childLayout"] == "tableLayout" {
		return ShapeClassParsed
	}
	if style["shape"] == "tableRow" {
		return ShapeClassParsed
	}
	if style["shape"] == "partialRectangle" {
		return ShapeClassParsed
	}
	if style["edgeStyle"] == "entityRelationEdgeStyle" {
		return ShapeClassParsed
	}
	if style["shape"] == "note" {
		return ShapeClassIgnored
	}
	return ShapeClassViolation
}

// ShapeName은 style 문자열에서 도형 이름만 뽑는다. style 문자열 전체를
// "도형"이라고 부르며 IR과 검증 리포트에 그대로 흘려보내면, 사용자는
// "shape=note;whiteSpace=wrap;html=1;rounded=0;..." 같은 한 줄을 읽고 무슨
// 도형인지 스스로 골라내야 한다.
//
// 우선순위는 draw.io가 스타일을 쓰는 방식 그대로다:
//  1. shape=<이름>  — 대부분의 도형
//  2. edgeStyle=<이름> — 엣지는 shape= 대신 이걸 쓴다
//  3. 첫 토큰이 값 없는 플래그면 그게 도형 이름이다("ellipse;whiteSpace=wrap")
//
// 셋 다 아니면(순수 key=value만 있는 스타일) 이름을 특정할 수 없으므로 빈
// 문자열을 반환한다 — 이 경우를 위해 원본 style은 Style 필드에 따로 남긴다.
func ShapeName(style string) string {
	parsed := ParseStyle(style)
	if name := parsed["shape"]; name != "" {
		return name
	}
	if name := parsed["edgeStyle"]; name != "" {
		return name
	}
	for _, part := range strings.Split(style, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.Contains(part, "=") {
			return part
		}
		break
	}
	return ""
}

// ClassifiedShape은 Parsed로 소비되지 않은 셀 하나를 나타낸다. Shape은 도형
// 이름, Style은 원본 스타일 문자열 전체다 — 이름을 특정할 수 없는 스타일도
// 있으므로 원본을 버리지 않는다.
type ClassifiedShape struct {
	ID     string
	Shape  string
	Style  string
	Reason string
}

// ClassifyAll은 다이어그램의 모든 셀을 읽어 Ignored/Violation 목록을 만든다.
// tableIDs/rowIDs/cellIDs에 속한 셀(이미 Parsed로 소비된 것)은 제외한다.
func ClassifyAll(cells []RawCell, consumedIDs map[string]bool) (ignored, violations []ClassifiedShape) {
	for _, c := range cells {
		if consumedIDs[c.ID] {
			continue
		}
		if strings.TrimSpace(c.Style) == "" {
			// draw.io가 페이지마다 넣는 구조상 필수 셀(id="0" 루트, id="1"
			// 기본 부모)이다. 사용자가 그린 도형이 아니므로 가짜 violation은
			// 물론이고 ignored_shapes 목록에도 올리지 않는다 — 올리면 IR에
			// 페이지마다 {shape:"", style:""} 두 줄이 의미 없이 쌓인다.
			continue
		}
		shape := ClassifiedShape{ID: c.ID, Shape: ShapeName(c.Style), Style: c.Style}
		switch ClassifyShape(c) {
		case ShapeClassIgnored:
			ignored = append(ignored, shape)
		case ShapeClassViolation:
			shape.Reason = "허용 도형 세트(A/C군) 밖의 도형"
			violations = append(violations, shape)
		}
	}
	return ignored, violations
}
