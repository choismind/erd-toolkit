package drawio

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadDiagrams_MultiPageCompressed(t *testing.T) {
	diagrams, err := LoadDiagrams("testdata/relationship_logical.drawio")
	if err != nil {
		t.Fatalf("LoadDiagrams failed: %v", err)
	}
	if len(diagrams) != relationshipLogicalDiagramCount {
		t.Fatalf("expected %d diagrams, got %d", relationshipLogicalDiagramCount, len(diagrams))
	}
	found := false
	for _, d := range diagrams {
		for _, c := range d.Cells {
			if c.Style != "" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no cells with style parsed from any diagram")
	}
}

func TestLoadDiagrams_SingleUncompressedXML(t *testing.T) {
	diagrams, err := LoadDiagrams("testdata/rowspan_example.xml")
	if err != nil {
		t.Fatalf("LoadDiagrams failed: %v", err)
	}
	if len(diagrams) != 1 {
		t.Fatalf("expected 1 diagram, got %d", len(diagrams))
	}
}

// TestLoadDiagrams_ObjectWrappedCellsFlattened은 회귀 테스트다: draw.io는
// 셀에 커스텀 데이터/플레이스홀더/링크가 붙으면 <object label="…" id="…">
// (구버전) 또는 <UserObject label="…" id="…">(신버전)로 <mxCell>을 감싸며,
// 이때 id/표시값은 래퍼 요소에만 있고 안쪽 mxCell엔 없다. rawRoot가 <root>의
// 직계 mxCell만 보던 시절엔 이런 셀이 통째로 사라졌다 — 그 셀이 테이블이면
// 0개 테이블·빈 리포트·exit 0으로 조용히 실패하는 회귀였다(golden fixture
// entity_table_basic.drawio 3페이지의 "도메인-1" 앵커가 실제 사례).
// 이 테스트는 <object>/<UserObject>로 감싼 테이블 둘 다 실제로 파싱되고,
// id/label이 래퍼 쪽 값으로 올바르게 채워지는지 확인한다.
func TestLoadDiagrams_ObjectWrappedCellsFlattened(t *testing.T) {
	diagrams, err := LoadDiagrams("testdata/object_wrapped_table.drawio")
	if err != nil {
		t.Fatalf("LoadDiagrams failed: %v", err)
	}
	if len(diagrams) != 1 {
		t.Fatalf("expected 1 diagram, got %d", len(diagrams))
	}
	cells := diagrams[0].Cells

	byID := map[string]RawCell{}
	for _, c := range cells {
		byID[c.ID] = c
	}

	objCell, ok := byID["objwrap-table-1"]
	if !ok {
		t.Fatalf("<object>-wrapped table cell not found by its object id — cell was dropped")
	}
	if objCell.Value != "객체테이블" {
		t.Fatalf("expected <object> label to populate RawCell.Value, got %q", objCell.Value)
	}
	if objCell.Parent != "1" {
		t.Fatalf("expected inner mxCell's parent to carry over, got %q", objCell.Parent)
	}
	style := ParseStyle(objCell.Style)
	if style["shape"] != "table" || style["childLayout"] != "tableLayout" {
		t.Fatalf("expected inner mxCell's style to carry over, got %q", objCell.Style)
	}

	userObjCell, ok := byID["userobjwrap-table-1"]
	if !ok {
		t.Fatalf("<UserObject>-wrapped table cell not found by its object id — cell was dropped")
	}
	if userObjCell.Value != "UserObject테이블" {
		t.Fatalf("expected <UserObject> label to populate RawCell.Value, got %q", userObjCell.Value)
	}

	tables := FindTables(cells)
	if len(tables) != 2 {
		t.Fatalf("expected both object-wrapped and UserObject-wrapped tables to be found, got %d tables", len(tables))
	}
}

// TestRawCellReadsGeometry는 <mxCell>의 <mxGeometry> 자식이 RawCell.Geometry로
// 읽히는지, 그리고 기하가 없는 셀(루트 id="0", 기본 부모 id="1")은 Geometry가
// nil로 남는지 확인한다. Task 3의 bounding box 계산이 "기하 없음"과 "원점의
// 0크기 도형"을 구별해야 하므로 이 nil 대비가 핵심이다.
func TestRawCellReadsGeometry(t *testing.T) {
	p := filepath.Join(t.TempDir(), "g.drawio")
	content := `<mxfile><diagram name="p" id="p"><mxGraphModel><root>
  <mxCell id="0"/>
  <mxCell id="1" parent="0"/>
  <mxCell id="t1" value="고객" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1">
    <mxGeometry x="80" y="120" width="240" height="90" as="geometry"/>
  </mxCell>
</root></mxGraphModel></diagram></mxfile>`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	ds, err := LoadDiagrams(p)
	if err != nil {
		t.Fatalf("LoadDiagrams: %v", err)
	}
	idx := BuildIndex(ds[0].Cells)

	got := idx.ByID["t1"].Geometry
	if got == nil {
		t.Fatal("t1의 Geometry가 nil이다")
	}
	want := RawGeometry{X: 80, Y: 120, Width: 240, Height: 90}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("Geometry=%+v; %+v여야 한다", *got, want)
	}
	if idx.ByID["1"].Geometry != nil {
		t.Error("기하 없는 셀의 Geometry는 nil이어야 한다")
	}
}

// TestLoadDiagrams_ObjectWrappedGeometryCarriesOver는
// TestLoadDiagrams_ObjectWrappedCellsFlattened과 같은 계열의 회귀 테스트다:
// <object>/<UserObject> 래퍼 평탄화 경로에서 안쪽 <mxCell>의 <mxGeometry>도
// 함께 옮겨지는지 확인한다. id/label/style/parent만 옮기고 Geometry를
// 빠뜨리면, 래퍼로 감싼 테이블마다 요약 박스 계산이 원점(0,0)을 가리키는
// 조용한 회귀가 된다.
func TestLoadDiagrams_ObjectWrappedGeometryCarriesOver(t *testing.T) {
	diagrams, err := LoadDiagrams("testdata/object_wrapped_table.drawio")
	if err != nil {
		t.Fatalf("LoadDiagrams failed: %v", err)
	}
	byID := map[string]RawCell{}
	for _, c := range diagrams[0].Cells {
		byID[c.ID] = c
	}

	objCell, ok := byID["objwrap-table-1"]
	if !ok {
		t.Fatalf("<object>-wrapped table cell not found")
	}
	if objCell.Geometry == nil {
		t.Fatal("<object> 래퍼 안쪽 mxCell의 Geometry가 옮겨지지 않았다")
	}
	wantObj := RawGeometry{X: 40, Y: 40, Width: 180, Height: 90}
	if !reflect.DeepEqual(*objCell.Geometry, wantObj) {
		t.Errorf("objwrap-table-1 Geometry=%+v; %+v여야 한다", *objCell.Geometry, wantObj)
	}

	userObjCell, ok := byID["userobjwrap-table-1"]
	if !ok {
		t.Fatalf("<UserObject>-wrapped table cell not found")
	}
	if userObjCell.Geometry == nil {
		t.Fatal("<UserObject> 래퍼 안쪽 mxCell의 Geometry가 옮겨지지 않았다")
	}
	wantUserObj := RawGeometry{X: 280, Y: 40, Width: 180, Height: 90}
	if !reflect.DeepEqual(*userObjCell.Geometry, wantUserObj) {
		t.Errorf("userobjwrap-table-1 Geometry=%+v; %+v여야 한다", *userObjCell.Geometry, wantUserObj)
	}
}
