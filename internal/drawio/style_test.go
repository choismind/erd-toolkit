package drawio

import "testing"

func TestParseStyle_ExactMatch(t *testing.T) {
	tableStyle := "shape=table;startSize=30;container=1;childLayout=tableLayout;"
	rowStyle := "shape=tableRow;horizontal=0;startSize=0;"

	tableMap := ParseStyle(tableStyle)
	rowMap := ParseStyle(rowStyle)

	if tableMap["shape"] != "table" {
		t.Fatalf("expected shape=table, got %q", tableMap["shape"])
	}
	if tableMap["childLayout"] != "tableLayout" {
		t.Fatalf("expected childLayout=tableLayout, got %q", tableMap["childLayout"])
	}
	// 회귀 테스트: 기존 4개 구현체를 전부 깨뜨린 버그 — "shape=table" 부분
	// 문자열 매칭이 "shape=tableRow"에도 걸리던 것. 정확 비교라면 걸리면 안 된다.
	if rowMap["shape"] == "table" {
		t.Fatalf("style parser must not confuse tableRow with table (exact match required)")
	}
	if rowMap["shape"] != "tableRow" {
		t.Fatalf("expected shape=tableRow, got %q", rowMap["shape"])
	}
}

func TestParseStyle_ValuelessFlag(t *testing.T) {
	// draw.io 스타일에는 값 없는 플래그도 등장한다 (예: "rounded;whiteSpace=wrap;")
	m := ParseStyle("rounded;whiteSpace=wrap;")
	if _, ok := m["rounded"]; !ok {
		t.Fatalf("expected 'rounded' key present with empty value")
	}
	if m["whiteSpace"] != "wrap" {
		t.Fatalf("expected whiteSpace=wrap")
	}
}
