package genbuild

import (
	"encoding/xml"
	"testing"

	"erdtool/internal/drawio"
)

func TestEmitTableCells_RoundTripsThroughRealParser(t *testing.T) {
	pt := PlacedTable{
		TableDef: TableDef{
			Name: "고객",
			Columns: []ColumnDef{
				{Name: "고객번호", Type: "int", Key: "PK", Nullable: false},
				{Name: "고객명", Type: "char(50)", Key: "", Nullable: true},
			},
		},
		X: 0, Y: 0, Width: minTableWidth, Height: tableHeight(TableDef{Columns: make([]ColumnDef, 2)}),
	}

	cells := emitTableCells(pt)
	root := xmlRoot{Cells: append([]xmlCell{{ID: "0"}, {ID: "1", Parent: "0"}}, cells...)}
	model := xmlGraphModel{Root: root}
	diagram := xmlDiagram{Name: "페이지", ID: "d1", Model: model}
	file := xmlMxFile{Diagrams: []xmlDiagram{diagram}}

	out, err := xml.Marshal(&file)
	if err != nil {
		t.Fatalf("marshal 실패: %v", err)
	}

	diagrams, err := drawio.LoadDiagramsBytes(out)
	if err != nil {
		t.Fatalf("실제 파서가 못 읽는다: %v\n%s", err, out)
	}
	tables := drawio.FindTables(diagrams[0].Cells)
	if len(tables) != 1 {
		t.Fatalf("got %d tables, want 1", len(tables))
	}
	if tables[0].Value != "고객" {
		t.Fatalf("테이블명: got %q", tables[0].Value)
	}

	idx := drawio.BuildIndex(diagrams[0].Cells)
	cols := drawio.ExtractColumns(tables[0], idx)
	if len(cols) != 2 {
		t.Fatalf("got %d columns, want 2", len(cols))
	}
	if cols[0].Name != "고객번호" || cols[0].Type != "int" || !cols[0].IsPrimaryKey() {
		t.Fatalf("컬럼 0: %+v", cols[0])
	}
	if cols[0].Nullable {
		t.Fatalf("고객번호는 NOT NULL이어야 한다: %+v", cols[0])
	}
	if cols[1].Name != "고객명" || cols[1].Type != "char(50)" || !cols[1].Nullable {
		t.Fatalf("컬럼 1: %+v", cols[1])
	}
}

func samplePages() []PageDef {
	return []PageDef{
		{
			Name: "주문정보",
			Tables: []TableDef{
				{Name: "고객", Columns: []ColumnDef{
					{Name: "고객번호", Type: "int", Key: "PK", Nullable: false},
					{Name: "고객명", Type: "char(50)", Nullable: true},
				}},
				{Name: "주문", Columns: []ColumnDef{
					{Name: "주문번호", Type: "int", Key: "PK", Nullable: false},
					{Name: "주문고객번호", Type: "int", Key: "FK1", Nullable: false},
				}},
			},
			Relations: []RelationDef{
				{SourceTable: "고객", SourceColumn: "고객번호", SourceCardinality: "ERone",
					TargetTable: "주문", TargetColumn: "주문고객번호", TargetCardinality: "ERzeroToMany"},
			},
		},
	}
}

func TestBuild_RoundTripsThroughRealParser(t *testing.T) {
	out, err := Build(samplePages())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	diagrams, err := drawio.LoadDiagramsBytes(out)
	if err != nil {
		t.Fatalf("실제 파서가 못 읽는다: %v\n%s", err, out)
	}
	if len(diagrams) != 1 || diagrams[0].Name != "주문정보" {
		t.Fatalf("got %+v", diagrams)
	}

	tables := drawio.FindTables(diagrams[0].Cells)
	if len(tables) != 2 {
		t.Fatalf("got %d tables, want 2", len(tables))
	}

	idx := drawio.BuildIndex(diagrams[0].Cells)
	rels := drawio.ExtractRelationships(diagrams[0].Cells, idx, tables)
	if len(rels) != 1 {
		t.Fatalf("got %d relationships, want 1", len(rels))
	}
	if rels[0].SourceCardinality != "ERone" || rels[0].TargetCardinality != "ERzeroToMany" {
		t.Fatalf("카디널리티: %+v", rels[0])
	}
	if !rels[0].SourceResolved || !rels[0].TargetResolved {
		t.Fatalf("관계 양끝이 실제 테이블/컬럼으로 풀려야 한다: %+v", rels[0])
	}
}

// twoPagesWithSameNames는 두 페이지가 테이블명·컬럼명을 그대로 복제한
// 경우다(사람이 탭을 복사해서 만들 때 흔하다). 이름이 같아도 emit이 만드는
// id는 페이지마다 달라야 한다 — CRITICAL 리뷰 발견: 접두 없이는 두 페이지가
// 똑같이 t0/t0_r0/t0_r0d/e0를 써서 internal/convert.File이 "셀 id가
// 페이지 사이에서 겹친다"며 build 산출물 전체를 거부했다.
func twoPagesWithSameNames() []PageDef {
	page := func(name string) PageDef {
		return PageDef{
			Name: name,
			Tables: []TableDef{
				{Name: "고객", Columns: []ColumnDef{
					{Name: "고객번호", Type: "int", Key: "PK", Nullable: false},
				}},
			},
			Relations: nil,
		}
	}
	return []PageDef{page("주문정보"), page("선적정보")}
}

func TestBuild_CellIDsAreUniqueAcrossPages(t *testing.T) {
	out, err := Build(twoPagesWithSameNames())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	diagrams, err := drawio.LoadDiagramsBytes(out)
	if err != nil {
		t.Fatalf("실제 파서가 못 읽는다: %v\n%s", err, out)
	}
	if len(diagrams) != 2 {
		t.Fatalf("got %d diagrams, want 2", len(diagrams))
	}

	seen := map[string]string{} // id -> 처음 본 페이지 이름
	for _, dg := range diagrams {
		for _, c := range dg.Cells {
			// 페이지 루트/기본 레이어("0"/"1")는 draw.io 규약상 페이지마다
			// 재사용되는 게 정상이다 — 충돌 검사에서 뺀다.
			if c.ID == "0" || c.ID == "1" {
				continue
			}
			if prevPage, dup := seen[c.ID]; dup {
				t.Fatalf("셀 id %q가 페이지 %q와 %q 사이에서 겹친다", c.ID, prevPage, dg.Name)
			}
			seen[c.ID] = dg.Name
		}
	}
}

func TestBuild_IsIdempotent(t *testing.T) {
	a, err := Build(samplePages())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, err := Build(samplePages())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(a) != string(b) {
		t.Fatal("같은 입력인데 바이트가 다르다")
	}
}

// 마지막 행은 «테이블 바깥 테두리와 같은 자리»에 선을 그리면 안 된다.
//
// tableRow의 bottom=1은 그 행의 아래 모서리에 선을 그린다. 마지막 행의 아래
// 모서리는 곧 테이블의 바깥 테두리이므로, 거기에 bottom=1을 주면 선이 두 겹이
// 된다. 둘 다 기본색일 때는 겹쳐도 안 보여서 Phase 2a의 육안 확인을 통과했다
// — 하지만 annotate가 진단이 붙은 테이블의 테두리를 빨간 3px로 바꾸면 행이 그린
// 기본색 1px 선이 그 위에 남아 «흰색과 빨간색이 겹친» 아래 모서리가 된다
// (2026-08-31, 소유자가 draw.io에서 발견).
//
// 실물 draw.io 파일도 마지막 행에는 bottom=0을 준다 —
// internal/drawio/testdata/entity_table_basic.drawio의 테이블 둘 다 그렇다.
func TestEmitTableCells_LastRowDrawsNoLineOnTableBorder(t *testing.T) {
	pt := PlacedTable{
		TableDef: TableDef{
			Name: "주문",
			Columns: []ColumnDef{
				{Name: "주문번호", Type: "int", Key: "PK"},
				{Name: "주문일자", Type: "date", Key: ""},
				{Name: "금액", Type: "int", Key: ""},
			},
		},
		X: 0, Y: 0, Width: minTableWidth, Height: tableHeight(TableDef{Columns: make([]ColumnDef, 3)}),
	}

	cells := emitTableCells(pt)

	// 행 셀만 고른다(부모가 테이블인 것). 마지막 행이 검사 대상이다.
	var rows []xmlCell
	for _, c := range cells {
		if drawio.ParseStyle(c.Style)["shape"] == "tableRow" {
			rows = append(rows, c)
		}
	}
	if len(rows) != 3 {
		t.Fatalf("행 %d개; 3개여야 한다", len(rows))
	}

	last := rows[len(rows)-1]
	if got := drawio.ParseStyle(last.Style)["bottom"]; got != "0" {
		t.Errorf("마지막 행의 bottom=%q; \"0\"이어야 한다 — 테이블 바깥 테두리와 겹치는 선을 그린다:\n%s", got, last.Style)
	}
}
