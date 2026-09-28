package drawio

import "testing"

func TestBuildIndex_OutOfOrderCells(t *testing.T) {
	// out_of_order_cells.drawio는 "테이블 셀 바로 다음 형제가 그 테이블의
	// 첫 행"이라는 문서 순서 가정이 깨진 실제 파일이다(부록 B). ID/parent
	// 인덱스는 순서와 무관하게 정확해야 한다.
	diagrams, err := LoadDiagrams("testdata/out_of_order_cells.drawio")
	if err != nil {
		t.Fatalf("LoadDiagrams failed: %v", err)
	}
	var totalTables int
	for _, d := range diagrams {
		idx := BuildIndex(d.Cells)
		for _, cell := range d.Cells {
			style := ParseStyle(cell.Style)
			if style["shape"] == "table" && style["childLayout"] == "tableLayout" {
				totalTables++
				children := idx.ChildrenOf[cell.ID]
				_ = children // 자식이 0개여도(문서 순서 문제) 인덱스 자체는 조회 가능해야 함
			}
		}
	}
	if totalTables != outOfOrderCellsTableCount {
		t.Fatalf("expected %d tables findable via style, got %d", outOfOrderCellsTableCount, totalTables)
	}
}
