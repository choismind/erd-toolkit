package drawio

import (
	"os"
	"testing"
)

// 실제 draw.io로 각 픽스처를 열어 직접 센 값. 파서 출력의 정답으로 쓴다.
const (
	entityTableBasicTableCount = 2 // "Orders", "Shipments"류 2개 테이블

	relationshipLogicalDiagramCount = 2
	relationshipLogicalTableCount   = 6 // 페이지당 3개 x 2페이지: 고객/주문/선적, 고객2/주문2/선적2
	relationshipLogicalColumnCount  = 16
	relationshipLogicalRelCount     = 4

	tableLevelRelationsTableCount = 2
	tableLevelRelationsColCount   = 8
	// table_level_relations.drawio ("ERD_기본도형")의 entityRelationEdgeStyle
	// 엣지 16개는 draw.io ER 도형 라이브러리 참고 페이지의 장식용 화살표라
	// source/target 속성 자체가 없다(작업 10 재조사로 확인, 원장 정정).
	// 따라서 ExtractRelationships가 해석 가능한 관계는 0개다.
	tableLevelRelationsRelCount = 0

	rowspanExampleTableCount  = 1
	rowspanExampleColumnCount = 4

	irregularRowsTableCount = 2 // 마지막 행 근처 구조가 불규칙 (작업 8에서 방어)

	outOfOrderCellsTableCount = 6 // 테이블 셀 바로 다음이 tableRow가 아닌 실제 사례
)

func TestFixturesExist(t *testing.T) {
	files := []string{
		"testdata/entity_table_basic.drawio",
		"testdata/relationship_logical.drawio",
		"testdata/relationship_physical.drawio",
		"testdata/table_level_relations.drawio",
		"testdata/rowspan_example.xml",
		"testdata/irregular_rows.drawio",
		"testdata/out_of_order_cells.drawio",
	}
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("fixture missing: %s (%v)", f, err)
		}
	}
}
