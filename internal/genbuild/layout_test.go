package genbuild

import (
	"fmt"
	"math"
	"testing"
)

func TestLayerTables_LinearChain(t *testing.T) {
	// 고객(0) -> 주문(1) -> 선적(2)
	rels := []RelationDef{
		{SourceTable: "고객", TargetTable: "주문"},
		{SourceTable: "주문", TargetTable: "선적"},
	}
	layer := layerTables([]string{"고객", "주문", "선적"}, rels)
	want := map[string]int{"고객": 0, "주문": 1, "선적": 2}
	for k, v := range want {
		if layer[k] != v {
			t.Errorf("layer[%q] = %d, want %d", k, layer[k], v)
		}
	}
}

func TestLayerTables_DiamondTakesMaxIncomingLayer(t *testing.T) {
	// A -> B -> D, A -> C -> D : D는 max(B,C)+1 = 2
	rels := []RelationDef{
		{SourceTable: "A", TargetTable: "B"},
		{SourceTable: "A", TargetTable: "C"},
		{SourceTable: "B", TargetTable: "D"},
		{SourceTable: "C", TargetTable: "D"},
	}
	layer := layerTables([]string{"A", "B", "C", "D"}, rels)
	if layer["D"] != 2 {
		t.Fatalf("layer[D] = %d, want 2", layer["D"])
	}
}

func TestLayerTables_CycleDoesNotInfiniteLoop(t *testing.T) {
	// A -> B -> A (사이클). back-edge를 무시하고 유한한 층을 내야 한다.
	rels := []RelationDef{
		{SourceTable: "A", TargetTable: "B"},
		{SourceTable: "B", TargetTable: "A"},
	}
	layer := layerTables([]string{"A", "B"}, rels)
	if layer["A"] != 0 {
		t.Fatalf("layer[A] = %d, want 0(먼저 방문한 쪽이 root)", layer["A"])
	}
	if layer["B"] != 1 {
		t.Fatalf("layer[B] = %d, want 1", layer["B"])
	}
}

func TestLayout_LinearChainStacksVertically(t *testing.T) {
	p := PageDef{
		Name: "테스트",
		Tables: []TableDef{
			{Name: "고객", Columns: []ColumnDef{{Name: "고객번호"}}},
			{Name: "주문", Columns: []ColumnDef{{Name: "주문번호"}}},
		},
		Relations: []RelationDef{
			{SourceTable: "고객", SourceColumn: "고객번호", TargetTable: "주문", TargetColumn: "주문번호"},
		},
	}
	pl := Layout(p)
	if len(pl.Tables) != 2 {
		t.Fatalf("got %d tables", len(pl.Tables))
	}
	byName := map[string]PlacedTable{}
	for _, t := range pl.Tables {
		byName[t.Name] = t
	}
	if byName["고객"].Y >= byName["주문"].Y {
		t.Fatalf("고객(층0)의 Y가 주문(층1)보다 작아야 한다: 고객.Y=%d, 주문.Y=%d", byName["고객"].Y, byName["주문"].Y)
	}
	if byName["고객"].X != byName["주문"].X {
		t.Fatalf("같은 층에 하나씩만 있으면 X가 같아야 한다(왼쪽 정렬 기준점)")
	}
}

func TestLayout_IsolatedTableGoesBelowGraph(t *testing.T) {
	p := PageDef{
		Name: "테스트",
		Tables: []TableDef{
			{Name: "고객", Columns: []ColumnDef{{Name: "고객번호"}}},
			{Name: "주문", Columns: []ColumnDef{{Name: "주문번호"}}},
			{Name: "고립", Columns: []ColumnDef{{Name: "칼럼"}}},
		},
		Relations: []RelationDef{
			{SourceTable: "고객", SourceColumn: "고객번호", TargetTable: "주문", TargetColumn: "주문번호"},
		},
	}
	pl := Layout(p)
	byName := map[string]PlacedTable{}
	for _, t := range pl.Tables {
		byName[t.Name] = t
	}
	maxGraphBottom := byName["주문"].Y + byName["주문"].Height
	if byName["고립"].Y < maxGraphBottom {
		t.Fatalf("고립 테이블은 그래프 배치 아래쪽에 있어야 한다: 고립.Y=%d, 그래프 최하단=%d", byName["고립"].Y, maxGraphBottom)
	}
}

// chainPage는 A1->A2->…->An으로 이어진 사슬 한 줄을 만든다. 층이 n개 생기고
// 층마다 테이블이 하나씩이라, 접지 않으면 세로로만 길어진다.
func chainPage(n int) PageDef {
	p := PageDef{Name: "사슬"}
	for i := 1; i <= n; i++ {
		p.Tables = append(p.Tables, TableDef{
			Name:    fmt.Sprintf("A%d", i),
			Columns: []ColumnDef{{Name: "번호"}, {Name: "이름"}},
		})
	}
	for i := 1; i < n; i++ {
		p.Relations = append(p.Relations, RelationDef{
			SourceTable: fmt.Sprintf("A%d", i), SourceColumn: "번호",
			TargetTable: fmt.Sprintf("A%d", i+1), TargetColumn: "번호",
		})
	}
	return p
}

// bounds는 놓인 테이블 전체를 감싸는 사각형의 가로·세로를 돌려준다.
func bounds(pl PageLayout) (w, h int) {
	x0, y0 := math.MaxInt, math.MaxInt
	x1, y1 := math.MinInt, math.MinInt
	for _, t := range pl.Tables {
		x0, y0 = min(x0, t.X), min(y0, t.Y)
		x1, y1 = max(x1, t.X+t.Width), max(y1, t.Y+t.Height)
	}
	return x1 - x0, y1 - y0
}

// overlaps는 서로 넓이가 겹치는 테이블 쌍의 수를 센다.
func overlaps(pl PageLayout) int {
	n := 0
	for i := 0; i < len(pl.Tables); i++ {
		for j := i + 1; j < len(pl.Tables); j++ {
			a, b := pl.Tables[i], pl.Tables[j]
			ox := min(a.X+a.Width, b.X+b.Width) - max(a.X, b.X)
			oy := min(a.Y+a.Height, b.Y+b.Height) - max(a.Y, b.Y)
			if ox > 0 && oy > 0 {
				n++
			}
		}
	}
	return n
}

// 층이 많아 세로로만 길어지면 층 묶음을 여러 열로 접어야 한다. 접지 않으면
// A4로 여러 장에 걸쳐 띠처럼 늘어나 읽기 나쁘다(2026-09-21 소유자).
func TestLayout_TallStackFoldsIntoColumns(t *testing.T) {
	pl := Layout(chainPage(12))
	w, h := bounds(pl)
	if h > w*2 {
		t.Fatalf("세로가 가로의 두 배를 넘으면 안 된다: 가로 %d, 세로 %d (비 1:%.2f)", w, h, float64(h)/float64(w))
	}
	xs := map[int]bool{}
	for _, tb := range pl.Tables {
		xs[tb.X] = true
	}
	if len(xs) < 2 {
		t.Fatalf("열이 둘 이상으로 접혀야 한다: 쓰인 X가 %d가지뿐이다", len(xs))
	}
	if n := overlaps(pl); n != 0 {
		t.Fatalf("접은 뒤에도 테이블이 겹치면 안 된다: 겹친 쌍 %d", n)
	}
}

// 짧은 사슬은 접지 않는다 — 접을 이유가 없는데 접으면 「부모가 위, 자식이
// 아래」라는 읽는 순서만 끊긴다.
func TestLayout_ShortStackDoesNotFold(t *testing.T) {
	pl := Layout(chainPage(3))
	xs := map[int]bool{}
	for _, tb := range pl.Tables {
		xs[tb.X] = true
	}
	if len(xs) != 1 {
		t.Fatalf("짧은 사슬은 한 열에 머물러야 한다: 쓰인 X가 %d가지다", len(xs))
	}
}

// 접은 열 안에서는 층 차례가 그대로 지켜져야 한다.
func TestLayout_FoldKeepsLayerOrderInsideColumn(t *testing.T) {
	pl := Layout(chainPage(12))
	byName := map[string]PlacedTable{}
	for _, tb := range pl.Tables {
		byName[tb.Name] = tb
	}
	for i := 1; i < 12; i++ {
		a, b := byName[fmt.Sprintf("A%d", i)], byName[fmt.Sprintf("A%d", i+1)]
		if a.X == b.X && a.Y >= b.Y {
			t.Fatalf("같은 열에 있으면 뒤 층이 아래여야 한다: A%d.Y=%d, A%d.Y=%d", i, a.Y, i+1, b.Y)
		}
	}
}

func TestLayout_IsDeterministic(t *testing.T) {
	p := PageDef{
		Name: "테스트",
		Tables: []TableDef{
			{Name: "고객", Columns: []ColumnDef{{Name: "고객번호"}}},
			{Name: "주문", Columns: []ColumnDef{{Name: "주문번호"}}},
		},
		Relations: []RelationDef{
			{SourceTable: "고객", SourceColumn: "고객번호", TargetTable: "주문", TargetColumn: "주문번호"},
		},
	}
	a := Layout(p)
	b := Layout(p)
	for i := range a.Tables {
		if a.Tables[i].X != b.Tables[i].X || a.Tables[i].Y != b.Tables[i].Y {
			t.Fatalf("같은 입력인데 좌표가 다르다: %+v vs %+v", a.Tables[i], b.Tables[i])
		}
	}
}
