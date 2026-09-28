package annotate

import (
	"testing"

	"erdtool/internal/drawio"
)

func TestMarkStyleOnlyTouchesStrokeKeys(t *testing.T) {
	in := "shape=table;childLayout=tableLayout;fillColor=#FFFFFF;strokeColor=#000000;"
	out := MarkStyle(in)
	got := drawio.ParseStyle(out)

	if got["strokeColor"] != markStrokeColor {
		t.Errorf("strokeColor=%q; %q여야 한다", got["strokeColor"], markStrokeColor)
	}
	if got["strokeWidth"] != markStrokeWidth {
		t.Errorf("strokeWidth=%q; %q여야 한다", got["strokeWidth"], markStrokeWidth)
	}
	// 나머지는 한 글자도 안 바뀐다.
	if got["shape"] != "table" || got["childLayout"] != "tableLayout" {
		t.Errorf("도형 키가 바뀌었다: %v", got)
	}
	if got["fillColor"] != "#FFFFFF" {
		t.Errorf("fillColor=%q; \"#FFFFFF\"가 그대로여야 한다", got["fillColor"])
	}
}

// «shape=table»이 «shape=tableRow»에도 걸리는 부분 문자열 매칭이 이
// 저장소의 존재 이유다. 마크가 그 실수를 되들이지 않는지 본다.
func TestMarkStyleDoesNotCorruptSimilarKeys(t *testing.T) {
	in := "shape=tableRow;strokeColor=#AAAAAA;"
	got := drawio.ParseStyle(MarkStyle(in))
	if got["shape"] != "tableRow" {
		t.Errorf("shape=%q; \"tableRow\"여야 한다", got["shape"])
	}
}

// 마크는 멱등이어야 한다 — 이미 마크된 스타일에 또 걸어도 같아야 한다.
func TestMarkStyleIsIdempotent(t *testing.T) {
	in := "shape=table;strokeColor=#000000;"
	once := MarkStyle(in)
	if twice := MarkStyle(once); twice != once {
		t.Errorf("두 번 마크하면 달라진다:\n1회: %s\n2회: %s", once, twice)
	}
}

// draw.io 스타일 문자열의 첫 토큰은 "="이 없는 맨 도형 이름일 수 있다
// (예: text, ellipse, swimlane). 이 저장소 fixture에도 실제로 있는
// 모양이다. 키를 정렬해 재조립하면 이 토큰이 뒤로 밀려
// "align=left;html=1;text;"가 되고, draw.io는 다른 도형을 그린다.
// MarkStyle은 원래 순서를 그대로 걷기 때문에 첫 토큰이 첫 자리에
// 남아야 한다.
func TestMarkStyleKeepsBareFirstToken(t *testing.T) {
	in := "text;html=1;align=left"

	// 0번 위치만 고정하면 뒤쪽 세그먼트가 통째로 재배치돼도 초록이다.
	// 이 함수가 지키는 것은 «첫 토큰의 자리»가 아니라 «입력 순서 전체»
	// 이므로 출력 문자열을 통으로 못박는다. 없던 두 키는 걷기가 끝난
	// 뒤에만 붙으므로 언제나 맨 뒤다.
	want := "text;html=1;align=left;strokeColor=" + markStrokeColor +
		";strokeWidth=" + markStrokeWidth + ";"
	if out := MarkStyle(in); out != want {
		t.Errorf("출력이 다르다:\n얻음: %s\n기대: %s", out, want)
	}
}

// 이미 있는 strokeColor/strokeWidth는 «제자리에서» 값만 바뀐다 — 지우고
// 뒤에 다시 붙이는 구현으로 바뀌면 순서가 갈리는데, 위 테스트는 그 두
// 키가 원래 없는 입력만 쓰므로 그것을 못 본다.
func TestMarkStyleReplacesStrokeKeysInPlace(t *testing.T) {
	in := "strokeColor=#000000;shape=table;strokeWidth=1;fillColor=#FFF"

	want := "strokeColor=" + markStrokeColor + ";shape=table;strokeWidth=" +
		markStrokeWidth + ";fillColor=#FFF;"
	if out := MarkStyle(in); out != want {
		t.Errorf("출력이 다르다:\n얻음: %s\n기대: %s", out, want)
	}
}

// 빈 세그먼트(";;"나 맨 끝의 ";")와 세그먼트 앞뒤 공백은 정규화된다.
// 손편집 파일과 일부 도구가 실제로 이런 스타일을 낸다. 지금까지 테스트도
// 문서도 없어서, 정규화를 지우거나 반대로 공백을 보존하도록 바꿔도
// 아무 단언이 걸리지 않았다 — 그러면 --clean이 복원한 스타일이 원본과
// 바이트가 달라져 멱등성 비교가 조용히 어긋난다.
func TestMarkStyleNormalizesBlankSegmentsAndSpaces(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a=1;;b=2", "a=1;b=2;"},
		{"a=1;b=2;", "a=1;b=2;"},
		{" a=1 ; b=2 ", "a=1;b=2;"},
		{";;;", ""},
	}
	suffix := "strokeColor=" + markStrokeColor + ";strokeWidth=" + markStrokeWidth + ";"
	for _, c := range cases {
		if out := MarkStyle(c.in); out != c.want+suffix {
			t.Errorf("MarkStyle(%q)=%q; %q여야 한다", c.in, out, c.want+suffix)
		}
	}
}

// 스타일이 없는 셀(빈 문자열)도 유효한 스타일 문자열을 내놓아야 한다 —
// 맨 앞에 ";"가 남으면 draw.io가 그 빈 세그먼트를 빈 키로 오해한다.
func TestMarkStyleOnEmptyStyle(t *testing.T) {
	out := MarkStyle("")

	if len(out) > 0 && out[0] == ';' {
		t.Errorf("빈 스타일을 마크하면 앞에 \";\"가 남는다: %q", out)
	}

	got := drawio.ParseStyle(out)
	if got["strokeColor"] != markStrokeColor {
		t.Errorf("strokeColor=%q; %q여야 한다", got["strokeColor"], markStrokeColor)
	}
	if got["strokeWidth"] != markStrokeWidth {
		t.Errorf("strokeWidth=%q; %q여야 한다", got["strokeWidth"], markStrokeWidth)
	}
}

// strokeColor가 이미 마크 색과 같은 셀(가령 두 번째 실행)은 손대지
// 않아도 결과가 그대로여야 한다 — TestMarkStyleIsIdempotent가 실제로
// 뭔가를 검증하고 있다는 근거다.
func TestMarkStyleRoundTripsAlreadyMarkedColor(t *testing.T) {
	in := "shape=table;strokeColor=#FF3333;strokeWidth=3;"
	out := MarkStyle(in)
	if out != in {
		t.Errorf("이미 마크된 스타일이 바뀌었다:\n입력: %s\n출력: %s", in, out)
	}
}

// 행에 붙인 표식이 화면에 안 나온 적이 있다(2026-09-22). 파일에는
// strokeColor=#FF3333이 멀쩡히 들어 있는데 draw.io가 아무것도 안 그렸다 —
// shape=tableRow의 top/left/right/bottom이 넷 다 0이라 «그릴 변»이 없었다.
//
// 실물 draw.io가 저장한 ERD도 마지막 행만 bottom=1이고 나머지는 넷 다 0이다.
// 그러니 손으로 그린 그림에서도 같은 일이 난다.
func TestMarkStyleTurnsOnEdgesSoTheStrokeIsVisible(t *testing.T) {
	// internal/genbuild/emit.go가 내는 행 스타일 그대로다.
	in := "shape=tableRow;horizontal=0;startSize=0;swimlaneHead=0;swimlaneBody=0;" +
		"fillColor=none;collapsible=0;dropTarget=0;points=[[0,0.5],[1,0.5]];" +
		"portConstraint=eastwest;top=0;left=0;right=0;bottom=0;"
	got := drawio.ParseStyle(MarkStyle(in))

	for _, k := range []string{"top", "left", "right", "bottom"} {
		if got[k] != "1" {
			t.Errorf("%s=%q; 마크한 행은 네 변을 다 그려야 보인다", k, got[k])
		}
	}
	// 색과 굵기는 그대로 얹힌다.
	if got["strokeColor"] != markStrokeColor || got["strokeWidth"] != markStrokeWidth {
		t.Errorf("테두리가 안 얹혔다: %v", got)
	}
	// 나머지는 안 건드린다.
	if got["shape"] != "tableRow" || got["fillColor"] != "none" {
		t.Errorf("도형 키가 바뀌었다: %v", got)
	}
}

// 모서리 플래그를 안 쓰는 도형에는 그 키를 새로 만들지 않는다. 원본에
// 없던 키가 늘면 «손 안 댄 곳은 그대로»라는 계약이 흐려진다.
func TestMarkStyleDoesNotAddEdgesToOtherShapes(t *testing.T) {
	in := "shape=table;startSize=30;container=1;childLayout=tableLayout;"
	got := drawio.ParseStyle(MarkStyle(in))

	for _, k := range []string{"top", "left", "right", "bottom"} {
		if _, ok := got[k]; ok {
			t.Errorf("%s가 새로 생겼다; shape=table은 이 키를 안 쓴다: %v", k, got)
		}
	}
}

// 대상 밖(회색)도 같은 규칙을 탄다 — 안 보이면 색을 가른 뜻이 없다.
func TestMarkStyleOutOfScopeTurnsOnEdgesToo(t *testing.T) {
	in := "shape=partialRectangle;top=0;left=0;right=0;bottom=0;"
	got := drawio.ParseStyle(MarkStyleOutOfScope(in))

	if got["top"] != "1" || got["bottom"] != "1" {
		t.Errorf("회색 표식도 변을 켜야 한다: %v", got)
	}
	if got["strokeColor"] != outOfScopeStrokeColor {
		t.Errorf("strokeColor=%q; %q여야 한다", got["strokeColor"], outOfScopeStrokeColor)
	}
}
