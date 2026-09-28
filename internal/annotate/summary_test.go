package annotate

import (
	"strings"
	"testing"

	"erdtool/internal/convert"
	"erdtool/internal/drawio"
)

func TestSummaryCellPlacedRightOfBounds(t *testing.T) {
	cells := []drawio.RawCell{
		{ID: "0"}, {ID: "1", Parent: "0"},
		{ID: "t1", Parent: "1", Geometry: &drawio.RawGeometry{X: 80, Y: 40, Width: 200, Height: 100}},
	}
	p := PagePlan{DiagramID: "pg1", Summary: []string{"가", "나"}}
	got := SummaryCell(p, cells)

	if got.X != 320 { // 80 + 200 + 40
		t.Errorf("X=%v; 320이어야 한다", got.X)
	}
	if got.Y != 40 {
		t.Errorf("Y=%v; 40이어야 한다", got.Y)
	}
	if drawio.ParseStyle(got.Style)["shape"] != "note" {
		t.Errorf("shape=%q; \"note\"여야 한다 — 다른 도형은 shape_violation이 된다",
			drawio.ParseStyle(got.Style)["shape"])
	}
	if got.ID != SummaryCellID("pg1") {
		t.Errorf("ID=%q; %q여야 한다", got.ID, SummaryCellID("pg1"))
	}
}

// 사람이 박스를 옮겼으면 그 자리를 존중한다. 매번 제자리로 되돌리면
// 사람이 옮긴 뜻이 지워진다.
func TestSummaryCellReusesExistingPosition(t *testing.T) {
	cells := []drawio.RawCell{
		{ID: "0"}, {ID: "1", Parent: "0"},
		{ID: "t1", Parent: "1", Geometry: &drawio.RawGeometry{X: 80, Y: 40, Width: 200, Height: 100}},
		{ID: SummaryCellID("pg1"), Parent: "1",
			Geometry: &drawio.RawGeometry{X: 999, Y: 777, Width: 260, Height: 60}},
	}
	got := SummaryCell(PagePlan{DiagramID: "pg1", Summary: []string{"가"}}, cells)
	if got.X != 999 || got.Y != 777 {
		t.Errorf("좌표=(%v,%v); 기존 자리 (999,777)을 써야 한다", got.X, got.Y)
	}
}

// 기하를 가진 최상위 셀이 하나도 없어도 죽지 않는다.
func TestSummaryCellWithNoGeometry(t *testing.T) {
	cells := []drawio.RawCell{{ID: "0"}, {ID: "1", Parent: "0"}}
	got := SummaryCell(PagePlan{DiagramID: "pg1", Summary: []string{"가"}}, cells)
	if got.X != 0 || got.Y != 0 {
		t.Errorf("좌표=(%v,%v); (0,0)이어야 한다", got.X, got.Y)
	}
}

// SummaryCellID는 정확히 이 형태여야 한다 — 작업 12가 «이전 실행의 박스»를
// 이 id로 찾아 지운다. 타임스탬프나 uuid를 섞으면 실행마다 id가 달라져
// 옛 박스를 못 찾고, 지우는 대신 매번 새 박스가 쌓인다.
func TestSummaryCellIDIsPinnedLiteral(t *testing.T) {
	got := SummaryCellID("pg1")
	const want = "erdtool-summary-pg1"
	if got != want {
		t.Errorf("SummaryCellID(%q)=%q; %q여야 한다", "pg1", got, want)
	}
}

// 줄이 늘면 높이도 늘어야 박스 안에서 글이 잘리지 않는다. 다만 아무리
// 줄이 적어도(0줄 포함) summaryMinHeight 밑으로는 안 내려간다 — 너무
// 작은 박스는 draw.io 화면에서 찾기 힘들다.
func TestSummaryCellHeightGrowsWithLines(t *testing.T) {
	few := SummaryCell(PagePlan{DiagramID: "pg1", Summary: []string{"가"}}, nil)
	if few.Height < summaryMinHeight {
		t.Errorf("줄이 적을 때 Height=%v; summaryMinHeight(%v) 밑으로 내려가면 안 된다", few.Height, summaryMinHeight)
	}

	var many []string
	for i := 0; i < 20; i++ {
		many = append(many, "줄")
	}
	got := SummaryCell(PagePlan{DiagramID: "pg1", Summary: many}, nil)
	if got.Height <= few.Height {
		t.Errorf("줄 20개일 때 Height=%v; 줄 1개일 때(%v)보다 커야 한다", got.Height, few.Height)
	}
}

// 기존 요약 박스가 있고, 페이지에 다른 기하 있는 셀도 있을 때 — 재사용
// 좌표가 계산된 좌표를 이긴다는 것을 증명한다. 계산된 좌표(bounding box +
// gap)가 기존 좌표와 겹치지 않게 일부러 멀리 떨어뜨려서, 결과가 계산값이
// 아니라 재사용값이라는 걸 확실히 가른다.
func TestSummaryCellReuseWinsOverComputedPosition(t *testing.T) {
	cells := []drawio.RawCell{
		{ID: "0"}, {ID: "1", Parent: "0"},
		{ID: "t1", Parent: "1", Geometry: &drawio.RawGeometry{X: 5000, Y: 5000, Width: 200, Height: 100}},
		{ID: SummaryCellID("pg1"), Parent: "1",
			Geometry: &drawio.RawGeometry{X: 10, Y: 20, Width: 260, Height: 60}},
	}
	got := SummaryCell(PagePlan{DiagramID: "pg1", Summary: []string{"가"}}, cells)
	if got.X != 10 || got.Y != 20 {
		t.Errorf("좌표=(%v,%v); 기존 자리 (10,20)을 써야 한다 — 계산된 자리로 밀리면 안 된다", got.X, got.Y)
	}
}

// summaryValue는 이스케이프하지 않는다 — XML 인코딩은
// convert.encodeNewCell(작업 6)이 책임진다. 여기서 또 이스케이프하면
// &amp;lt; 처럼 이중으로 걸린 텍스트가 사용자 눈에 보인다. 실제
// RewriteMxFilePlan을 한 번 태워서 <, &, "가 문자 그대로 살아 돌아오는지
// 본다 — 이 테스트가 미래의 «잘 몰라서 고친» 이스케이프 추가를 막는
// 그물이다.
func TestSummaryCellValueSurvivesConvertEncoding(t *testing.T) {
	msg := `이상한 값 <table> & "인용부호"`
	p := PagePlan{DiagramID: "pg1", Summary: []string{msg}}
	nc := SummaryCell(p, nil)

	src := []byte(`<mxfile>
  <diagram name="p" id="pg1"><mxGraphModel><root>
    <mxCell id="0"/><mxCell id="1" parent="0"/>
  </root></mxGraphModel></diagram>
</mxfile>`)

	out, err := convert.RewriteMxFilePlan(src, convert.RewritePlan{Pages: []convert.PagePlan{
		{DiagramID: "pg1", Inserts: []convert.NewCell{nc}},
	}})
	if err != nil {
		t.Fatalf("RewriteMxFilePlan: %v", err)
	}

	ds, err := drawio.LoadDiagramsBytes(out)
	if err != nil {
		t.Fatalf("LoadDiagramsBytes: %v", err)
	}
	idx := drawio.BuildIndex(ds[0].Cells)
	cell, ok := idx.ByID[nc.ID]
	if !ok {
		t.Fatalf("삽입한 셀 %q를 못 찾았다", nc.ID)
	}
	for _, sub := range []string{"<table>", "&", `"인용부호"`} {
		if !strings.Contains(cell.Value, sub) {
			t.Errorf("value=%q; 원문(%q)이 이스케이프 한 겹만 거쳐 그대로 살아있어야 한다 (%q 없음)", cell.Value, msg, sub)
		}
	}
}

// 기본 레이어 id가 "1"이 아닌 파일에서도 요약 박스가 실제로 존재하는
// 부모에 붙어야 한다.
//
// draw.io는 보통 기본 레이어에 id="1"을 주지만, 레이어를 지웠다 다시
// 만들거나 다른 도구가 만든 파일은 그렇지 않다. 예전에는 Parent를 "1"로
// 박아 뒀는데, 그러면 존재하지 않는 셀을 부모로 가리키는 박스가 나가고
// erdtool은 「페이지 1개 요약」이라고 성공을 보고한다 — 사람이 draw.io로
// 열면 그 박스가 안 보인다. 도구가 한 일과 다른 말을 하는 것이라
// 이 저장소가 가장 경계하는 실패 유형이다.
func TestSummaryCellParentComesFromPageNotHardcodedOne(t *testing.T) {
	cells := []drawio.RawCell{
		{ID: "0"},
		{ID: "LAYER-A", Parent: "0"},
		{ID: "t1", Parent: "LAYER-A", Geometry: &drawio.RawGeometry{X: 80, Y: 40, Width: 200, Height: 100}},
		{ID: "r1", Parent: "t1", Geometry: &drawio.RawGeometry{Y: 30, Width: 200, Height: 30}},
	}
	got := SummaryCell(PagePlan{DiagramID: "pg1", Summary: []string{"가"}}, cells)
	if got.Parent != "LAYER-A" {
		t.Errorf("Parent=%q; 이 페이지에 실제로 있는 레이어 %q여야 한다", got.Parent, "LAYER-A")
	}
}

// 기하를 가진 최상위 셀이 하나도 없는 페이지에서도 존재하는 레이어를
// 찾아낸다 — 앵커 없는 진단(duplicate_table_name 등)만 나온 페이지가
// 실제로 이 모양이 될 수 있다.
func TestSummaryCellParentFallsBackToFirstLayer(t *testing.T) {
	cells := []drawio.RawCell{{ID: "0"}, {ID: "LAYER-A", Parent: "0"}}
	got := SummaryCell(PagePlan{DiagramID: "pg1", Summary: []string{"가"}}, cells)
	if got.Parent != "LAYER-A" {
		t.Errorf("Parent=%q; 유일한 레이어 %q여야 한다", got.Parent, "LAYER-A")
	}
}

// 사람이 요약 박스를 다른 레이어로 옮겼으면 그 레이어를 존중한다 —
// 좌표를 존중하는 것과 같은 이유이고, 안 그러면 실행마다 박스가 원래
// 레이어로 되돌아가 멱등성이 깨진다.
func TestSummaryCellReusesExistingParent(t *testing.T) {
	cells := []drawio.RawCell{
		{ID: "0"}, {ID: "1", Parent: "0"}, {ID: "LAYER-B", Parent: "0"},
		{ID: "t1", Parent: "1", Geometry: &drawio.RawGeometry{X: 80, Y: 40, Width: 200, Height: 100}},
		{ID: SummaryCellID("pg1"), Parent: "LAYER-B",
			Geometry: &drawio.RawGeometry{X: 999, Y: 777, Width: 260, Height: 60}},
	}
	got := SummaryCell(PagePlan{DiagramID: "pg1", Summary: []string{"가"}}, cells)
	if got.Parent != "LAYER-B" {
		t.Errorf("Parent=%q; 사람이 옮겨 둔 레이어 %q여야 한다", got.Parent, "LAYER-B")
	}
}

// 기존 박스의 부모가 파일에 실제로 없는(매달린 참조) 셀이면 그것을 그대로
// 쓰면 안 된다. summaryParent가 처음 만드는 박스에서 일부러 거르는 바로
// 그 상태이고(레이어를 지웠다 다시 만든 파일에서 생긴다), 재사용 경로가
// 그것을 통과시키면 존재하지 않는 셀을 부모로 가리키는 박스가 나간다 —
// erdtool은 「페이지 N개 요약」이라고 성공을 보고하는데 draw.io로 열면
// 그 박스가 없다. 고치려던 결함(코드리뷰)을 재사용 경로로 재현하는
// 것이라 좌표만 존중하고 부모는 살아 있는 것으로 물러난다.
func TestSummaryCellDoesNotReuseDanglingParent(t *testing.T) {
	cells := []drawio.RawCell{
		{ID: "0"}, {ID: "1", Parent: "0"},
		{ID: "t1", Parent: "1", Geometry: &drawio.RawGeometry{X: 80, Y: 40, Width: 200, Height: 100}},
		{ID: SummaryCellID("pg1"), Parent: "지워진레이어",
			Geometry: &drawio.RawGeometry{X: 999, Y: 777, Width: 260, Height: 60}},
	}
	got := SummaryCell(PagePlan{DiagramID: "pg1", Summary: []string{"가"}}, cells)

	if got.Parent == "지워진레이어" {
		t.Errorf("부모=%q; 파일에 없는 셀을 부모로 쓰면 draw.io에서 박스가 안 보인다", got.Parent)
	}
	if got.Parent != "1" {
		t.Errorf("부모=%q; 살아 있는 레이어 %q로 물러나야 한다", got.Parent, "1")
	}
	// 부모만 물러나고 좌표는 그대로 존중한다 — 사람이 옮긴 자리다.
	if got.X != 999 || got.Y != 777 {
		t.Errorf("좌표=(%v,%v); 기존 자리 (999,777)은 그대로 써야 한다", got.X, got.Y)
	}
}

// 기존 박스가 있어도 Geometry가 nil이면 재사용 후보가 아니다 — 쓸 좌표가
// 없으므로 bounding box 계산으로 넘어가야 한다. 이 분기를 조용히 뒤집으면
// 좌표가 (0,0)으로 굳어 박스가 그림 위에 겹쳐 놓인다.
func TestSummaryCellIgnoresExistingBoxWithoutGeometry(t *testing.T) {
	cells := []drawio.RawCell{
		{ID: "0"}, {ID: "1", Parent: "0"},
		{ID: "t1", Parent: "1", Geometry: &drawio.RawGeometry{X: 80, Y: 40, Width: 200, Height: 100}},
		{ID: SummaryCellID("pg1"), Parent: "1"}, // Geometry 없음
	}
	got := SummaryCell(PagePlan{DiagramID: "pg1", Summary: []string{"가"}}, cells)

	if got.X != 320 || got.Y != 40 { // 80 + 200 + 40
		t.Errorf("좌표=(%v,%v); bounding box 계산 결과 (320,40)이어야 한다", got.X, got.Y)
	}
	if got.Parent != "1" {
		t.Errorf("부모=%q; %q여야 한다", got.Parent, "1")
	}
}
