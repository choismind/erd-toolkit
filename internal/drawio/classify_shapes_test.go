// internal/drawio/classify_shapes_test.go
package drawio

import "testing"

func TestClassifyShape(t *testing.T) {
	cases := []struct {
		name  string
		style string
		want  ShapeClass
	}{
		{"table", "shape=table;childLayout=tableLayout;", ShapeClassParsed},
		{"tableRow", "shape=tableRow;", ShapeClassParsed},
		{"partialRectangle", "shape=partialRectangle;", ShapeClassParsed},
		{"cardinality edge", "edgeStyle=entityRelationEdgeStyle;endArrow=ERone;", ShapeClassParsed},
		{"note", "shape=note;size=20;", ShapeClassIgnored},
		{"entity (chen)", "whiteSpace=wrap;html=1;align=center;", ShapeClassViolation},
		{"relationship diamond", "shape=rhombus;perimeter=rhombusPerimeter;", ShapeClassViolation},
		{"cloud", "ellipse;shape=cloud;", ShapeClassViolation},
		{"draw.io mandatory root/default-parent cell (id=0/1, no style)", "", ShapeClassIgnored},
	}
	for _, tc := range cases {
		got := ClassifyShape(RawCell{Style: tc.style})
		if got != tc.want {
			t.Errorf("%s: expected %v, got %v", tc.name, tc.want, got)
		}
	}
}

func TestShapeName(t *testing.T) {
	// M11: IgnoredShape.Shape / ShapeViolation.Shape에 style 문자열 전체가
	// 그대로 들어가 IR과 검증 리포트에 노출됐다. 도형 이름만 담아야 한다.
	cases := []struct{ style, want string }{
		{"shape=note;whiteSpace=wrap;html=1;", "note"},
		{"shape=table;childLayout=tableLayout;", "table"},
		// shape= 키가 없는 스타일은 draw.io 관례상 첫 토큰이 도형 이름이다.
		{"ellipse;whiteSpace=wrap;html=1;", "ellipse"},
		{"text;html=1;strokeColor=none;", "text"},
		// 엣지는 shape= 대신 edgeStyle=로 종류를 밝힌다.
		{"edgeStyle=entityRelationEdgeStyle;rounded=0;", "entityRelationEdgeStyle"},
		// 스타일 첫 토큰이 key=value뿐이면 이름을 특정할 수 없다.
		{"rounded=0;whiteSpace=wrap;", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := ShapeName(c.style); got != c.want {
			t.Errorf("ShapeName(%q) = %q, want %q", c.style, got, c.want)
		}
	}
}

func TestClassifyAll_ReportsShapeNameNotWholeStyle(t *testing.T) {
	cells := []RawCell{
		{ID: "n1", Style: "shape=note;whiteSpace=wrap;html=1;"},
		{ID: "e1", Style: "ellipse;whiteSpace=wrap;html=1;fillColor=#ff0000;"},
	}
	ignored, violations := ClassifyAll(cells, map[string]bool{})
	if len(ignored) != 1 || ignored[0].Shape != "note" {
		t.Fatalf("expected ignored shape name %q, got %+v", "note", ignored)
	}
	if len(violations) != 1 || violations[0].Shape != "ellipse" {
		t.Fatalf("expected violation shape name %q, got %+v", "ellipse", violations)
	}
}

func TestClassifyAll_OmitsStructuralRootCells(t *testing.T) {
	// draw.io는 페이지마다 style 속성이 없는 구조상 필수 셀 두 개(id="0"
	// 루트, id="1" 기본 부모)를 넣는다. 사용자가 그린 도형이 아니므로
	// 가짜 shape_violation으로 세면 안 되는 건 물론이고, ignored_shapes에
	// {shape:"", style:""} 두 줄로 남겨서도 안 된다 — 페이지마다 IR에
	// 의미 없는 항목이 두 개씩 쌓인다.
	cells := []RawCell{
		{ID: "0"},
		{ID: "1", Parent: "0"},
		{ID: "n1", Style: "shape=note;size=20;", Parent: "1"},
	}
	ignored, violations := ClassifyAll(cells, map[string]bool{})
	if len(violations) != 0 {
		t.Fatalf("structural cells must not be violations, got %+v", violations)
	}
	if len(ignored) != 1 || ignored[0].ID != "n1" {
		t.Fatalf("expected only the note to be reported as ignored, got %+v", ignored)
	}
}
