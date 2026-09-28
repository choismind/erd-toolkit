package drawio

import (
	"os"
	"testing"
)

func TestExtractRelationships_ColumnLevel(t *testing.T) {
	diagrams, err := LoadDiagrams("testdata/relationship_logical.drawio")
	if err != nil {
		t.Fatalf("LoadDiagrams failed: %v", err)
	}
	total := 0
	for _, d := range diagrams {
		idx := BuildIndex(d.Cells)
		tables := FindTables(d.Cells)
		rels := ExtractRelationships(d.Cells, idx, tables)
		total += len(rels)
		for _, r := range rels {
			if !r.ColumnLevel {
				t.Fatalf("expected column-level relationship in relationship_logical fixture")
			}
			if r.SourceTableID == "" || r.TargetTableID == "" {
				t.Fatalf("relationship missing table id: %+v", r)
			}
		}
	}
	if total != relationshipLogicalRelCount {
		t.Fatalf("expected %d relationships, got %d", relationshipLogicalRelCount, total)
	}
}

func TestExtractRelationships_TableDirectConnection(t *testing.T) {
	// testdata/table_direct_relation.drawio: 실제 테이블 2개(entity_table_basic.drawio
	// 에서 그대로 가져옴) + entityRelationEdgeStyle 엣지 1개, source/target이
	// 행이 아니라 두 테이블의 셀 ID를 직접 가리킨다. 원본(Python) 구현은
	// 이 케이스를 처리하지 못해 첫 관계에서 크래시하거나 이전 관계 값을
	// 재사용했다(부록 B). 여기서는 ColumnLevel=false, 두 ColumnID 모두
	// 빈 문자열, 두 TableID 모두 채워진 상태로 정확히 기록되어야 한다.
	if _, err := os.Stat("testdata/table_direct_relation.drawio"); err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	diagrams, err := LoadDiagrams("testdata/table_direct_relation.drawio")
	if err != nil {
		t.Fatalf("LoadDiagrams failed: %v", err)
	}
	if len(diagrams) != 1 {
		t.Fatalf("expected 1 diagram, got %d", len(diagrams))
	}
	d := diagrams[0]
	idx := BuildIndex(d.Cells)
	tables := FindTables(d.Cells)
	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}
	rels := ExtractRelationships(d.Cells, idx, tables)
	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(rels))
	}
	r := rels[0]
	if r.ColumnLevel {
		t.Fatalf("expected ColumnLevel=false for a table-direct connection, got true: %+v", r)
	}
	if r.SourceColumnID != "" || r.TargetColumnID != "" {
		t.Fatalf("table-direct relationship must have empty column ids: %+v", r)
	}
	if r.SourceTableID == "" || r.TargetTableID == "" {
		t.Fatalf("table-direct relationship must resolve both table ids: %+v", r)
	}
}

func TestExtractRelationships_BothEndsDangling(t *testing.T) {
	// C2 회귀: 양쪽 다 존재하지 않는 id를 가리키는 엣지는 예전엔 continue로
	// 완전히 버려져 findings가 0건이었다. 지금은 관계로 기록하되
	// SourceResolved/TargetResolved가 둘 다 false여야 한다 — 검증 단계가 이
	// 신호로 broken_reference를 잡는다.
	diagrams, err := LoadDiagrams("testdata/broken_reference_both_dangling.drawio")
	if err != nil {
		t.Fatalf("LoadDiagrams failed: %v", err)
	}
	d := diagrams[0]
	idx := BuildIndex(d.Cells)
	tables := FindTables(d.Cells)
	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}
	rels := ExtractRelationships(d.Cells, idx, tables)
	if len(rels) != 1 {
		t.Fatalf("expected the dangling edge to still be recorded as 1 relationship, got %d", len(rels))
	}
	r := rels[0]
	if r.SourceResolved || r.TargetResolved {
		t.Fatalf("expected both ends unresolved, got %+v", r)
	}
	if r.SourceRawID != "bd-ghost-source" || r.TargetRawID != "bd-ghost-target" {
		t.Fatalf("expected raw ids preserved even when unresolved, got %+v", r)
	}
	if r.SourceTableID != "" || r.TargetTableID != "" {
		t.Fatalf("expected empty resolved table ids when unresolved, got %+v", r)
	}
}

func TestExtractRelationships_OneEndDangling(t *testing.T) {
	// C2 회귀: 한쪽만 존재하지 않는 id를 가리키는 엣지는 예전엔 조용히
	// append되긴 했지만 끊어진 쪽 필드가 전부 빈 문자열이라 검증기의
	// `!= ""` 가드에 걸려 findings가 0건이었다. 지금은 정상 쪽은
	// Resolved=true로, 끊어진 쪽은 Resolved=false + 원본 id 보존으로
	// 기록되어야 한다.
	diagrams, err := LoadDiagrams("testdata/broken_reference_one_dangling.drawio")
	if err != nil {
		t.Fatalf("LoadDiagrams failed: %v", err)
	}
	d := diagrams[0]
	idx := BuildIndex(d.Cells)
	tables := FindTables(d.Cells)
	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}
	rels := ExtractRelationships(d.Cells, idx, tables)
	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(rels))
	}
	r := rels[0]
	if !r.SourceResolved {
		t.Fatalf("expected source to resolve to the real table, got %+v", r)
	}
	if r.SourceTableID != "od-table-1" {
		t.Fatalf("expected source table id od-table-1, got %+v", r)
	}
	if r.TargetResolved {
		t.Fatalf("expected target to remain unresolved, got %+v", r)
	}
	if r.TargetRawID != "od-ghost-target" {
		t.Fatalf("expected target raw id preserved, got %+v", r)
	}
	if r.TargetTableID != "" {
		t.Fatalf("expected empty resolved target table id, got %+v", r)
	}
}

func TestExtractRelationships_UnsupportedRowShapeExistsButUnresolved(t *testing.T) {
	// 회귀: out_of_order_cells.drawio는 draw.io의 구버전 2단 테이블 형식을
	// 쓴다 — 행이 shape=tableRow가 아니라 shape=partialRectangle로 테이블에
	// 직접 매달린다. resolveEnd는 이 행 변형을 인식하지 못해 Resolved=false
	// 가 되지만, 그 raw id 자체는 idx.ByID에 실존하는 진짜 셀을 가리키므로
	// Exists=true여야 한다 — "진짜 없는 id"(dangling)와 구분되어야
	// broken_reference 오탐(fix wave 재검토에서 발견된 회귀)을 막을 수 있다.
	diagrams, err := LoadDiagrams("testdata/out_of_order_cells.drawio")
	if err != nil {
		t.Fatalf("LoadDiagrams failed: %v", err)
	}
	if len(diagrams) == 0 {
		t.Fatalf("expected at least 1 diagram")
	}
	total := 0
	for _, d := range diagrams {
		idx := BuildIndex(d.Cells)
		tables := FindTables(d.Cells)
		rels := ExtractRelationships(d.Cells, idx, tables)
		for _, r := range rels {
			total++
			if r.SourceResolved || r.TargetResolved {
				t.Fatalf("expected both ends unresolved (unrecognized row shape), got %+v", r)
			}
			if !r.SourceExists || !r.TargetExists {
				t.Fatalf("expected both raw ids to exist in idx.ByID (they are real cells, just an unrecognized shape), got %+v", r)
			}
		}
	}
	if total == 0 {
		t.Fatalf("expected at least 1 relationship across pages in out_of_order_cells.drawio")
	}
}

func TestExtractRelationships_UnresolvableEdgesSkippedNotCrashed(t *testing.T) {
	// testdata/table_level_relations.drawio ("ERD_기본도형")의 엣지 16개는
	// source/target 속성 자체가 없다(draw.io ER 도형 라이브러리 참고
	// 페이지로 추정). 이런 엣지는 크래시하지 않고 조용히 건너뛰어야 한다 —
	// 관계로 잘못 기록해서도 안 되고(참조 대상이 없으므로), panic해서도
	// 안 된다.
	diagrams, err := LoadDiagrams("testdata/table_level_relations.drawio")
	if err != nil {
		t.Fatalf("LoadDiagrams failed: %v", err)
	}
	total := 0
	for _, d := range diagrams {
		idx := BuildIndex(d.Cells)
		tables := FindTables(d.Cells)
		rels := ExtractRelationships(d.Cells, idx, tables)
		total += len(rels)
	}
	if total != tableLevelRelationsRelCount {
		t.Fatalf("expected %d relationships (none resolvable), got %d", tableLevelRelationsRelCount, total)
	}
}

// floating_relation.drawio는 relationship_physical.drawio에서 첫 페이지의
// 관계선 하나(고객주문)의 source/target 속성만 떼고, 그 자리에 CUST의
// 오른쪽 변(x=340)과 ORDR의 왼쪽 변(x=440)에 닿는 끝점 좌표를 넣은 것이다.
// 손으로 적지 않고 실제 draw.io 저장본을 변형해 만들었다. 그림으로는 두
// 테이블이 이어져 보이지만 파일이 말하는 연결은 없다.
func TestExtractFloatingRelations_TouchingTablesIsReported(t *testing.T) {
	ds, err := LoadDiagrams("testdata/floating_relation.drawio")
	if err != nil {
		t.Fatalf("LoadDiagrams: %v", err)
	}
	d := ds[0]
	idx := BuildIndex(d.Cells)
	tables := FindTables(d.Cells)

	floating := ExtractFloatingRelations(d.Cells, tables)
	if len(floating) != 1 {
		t.Fatalf("floating=%d건; 1건이어야 한다: %+v", len(floating), floating)
	}
	if floating[0].Label != "고객주문" {
		t.Errorf("Label=%q; \"고객주문\"이어야 한다", floating[0].Label)
	}
	if len(floating[0].NearTableIDs) != 2 {
		t.Errorf("NearTableIDs=%v; 끝점이 CUST와 ORDR 둘에 닿으므로 2개여야 한다",
			floating[0].NearTableIDs)
	}

	// 같은 페이지의 나머지 관계선은 그대로 관계로 읽혀야 한다.
	rels := ExtractRelationships(d.Cells, idx, tables)
	if len(rels) != 1 {
		t.Errorf("rels=%d건; 멀쩡한 관계선 1건은 그대로 읽혀야 한다", len(rels))
	}
}

// draw.io의 ER 도형 라이브러리를 붙여 둔 참고용 화살표는 source/target이
// 없지만 테이블에서 멀리 떨어져 있다. 이것까지 진단으로 올리면 그런 페이지를
// 가진 사람은 매번 가짜 진단을 열여섯 건씩 본다.
func TestExtractFloatingRelations_DecorativeArrowsAreSilent(t *testing.T) {
	for _, name := range []string{
		"testdata/table_level_relations.drawio",
		"testdata/entity_table_basic.drawio",
	} {
		ds, err := LoadDiagrams(name)
		if err != nil {
			t.Fatalf("LoadDiagrams(%s): %v", name, err)
		}
		for _, d := range ds {
			tables := FindTables(d.Cells)
			if got := ExtractFloatingRelations(d.Cells, tables); len(got) != 0 {
				t.Errorf("%s 페이지 %q: floating=%+v; 0건이어야 한다", name, d.Name, got)
			}
		}
	}
}
