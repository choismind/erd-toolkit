package drawio

import (
	"path/filepath"
	"reflect"
	"testing"
)

func cellWith(id string, attrs map[string]string) RawCell {
	return RawCell{ID: id, Attrs: attrs}
}

func TestColumnErdAttrs_행과키셀과정의셀을모두받는다(t *testing.T) {
	got := ColumnErdAttrs(
		cellWith("row", map[string]string{AttrComment: "주문일자"}),
		cellWith("key", map[string]string{AttrCheck: "x > 0"}),
		cellWith("def", map[string]string{AttrDefault: "'미상'"}),
	)
	want := map[string]string{AttrComment: "주문일자", AttrCheck: "x > 0", AttrDefault: "'미상'"}
	if !reflect.DeepEqual(got.Values, want) {
		t.Fatalf("세 자리의 값이 모두 모여야 한다: got %v want %v", got.Values, want)
	}
	if len(got.Conflicts) != 0 || len(got.Unknown) != 0 {
		t.Fatalf("진단이 없어야 한다: %+v", got)
	}
}

func TestColumnErdAttrs_같은이름같은값은통과(t *testing.T) {
	got := ColumnErdAttrs(
		cellWith("row", nil),
		cellWith("key", map[string]string{AttrCheck: "x > 0"}),
		cellWith("def", map[string]string{AttrCheck: "x > 0"}),
	)
	if got.Get(AttrCheck) != "x > 0" {
		t.Fatalf("중복일 뿐 뜻이 갈리지 않으므로 통과해야 한다: %v", got.Values)
	}
	if len(got.Conflicts) != 0 {
		t.Fatalf("충돌이 아니어야 한다: %+v", got.Conflicts)
	}
}

func TestColumnErdAttrs_값이다르면어느쪽도고르지않는다(t *testing.T) {
	got := ColumnErdAttrs(
		cellWith("row", nil),
		cellWith("key", map[string]string{AttrCheck: "x > 0"}),
		cellWith("def", map[string]string{AttrCheck: "x >= 1"}),
	)
	// 우선순위를 정해 조용히 하나를 고르면 사용자는 잃은 값을 끝까지 모른다.
	if got.Get(AttrCheck) != "" {
		t.Fatalf("충돌한 값을 채택하면 안 된다: %q", got.Get(AttrCheck))
	}
	if !got.HasConflict(AttrCheck) {
		t.Fatal("충돌로 보고해야 한다")
	}
	want := []string{"x > 0", "x >= 1"} // 정렬된 순서
	if !reflect.DeepEqual(got.Conflicts[0].Values, want) {
		t.Fatalf("두 값이 모두 남아야 하고 순서가 고정이어야 한다: %v", got.Conflicts[0].Values)
	}
}

func TestColumnErdAttrs_모르는이름은진단으로올린다(t *testing.T) {
	got := ColumnErdAttrs(
		cellWith("row", nil),
		cellWith("key", nil),
		cellWith("def", map[string]string{"erd_chek": "x > 0"}),
	)
	if len(got.Unknown) != 1 || got.Unknown[0] != "erd_chek" {
		t.Fatalf("오타가 조용히 사라지면 안 된다: %v", got.Unknown)
	}
}

func TestErdAttrs_접두어없는이름은건드리지않는다(t *testing.T) {
	// 사용자들이 자기 용도로 이미 쓰는 데이터다. 우리 것이 아니다.
	got := ColumnErdAttrs(
		cellWith("row", nil),
		cellWith("key", nil),
		cellWith("def", map[string]string{"check": "남의 데이터", "placeholders": "1"}),
	)
	if len(got.Values) != 0 || len(got.Unknown) != 0 || len(got.Conflicts) != 0 {
		t.Fatalf("erd_ 접두어가 없는 것은 무시해야 한다: %+v", got)
	}
}

func TestTableErdAttrs_컬럼전용이름은테이블에서모르는이름이다(t *testing.T) {
	got := TableErdAttrs(cellWith("t", map[string]string{AttrDefault: "0", AttrComment: "고객"}))
	if got.Get(AttrComment) != "고객" {
		t.Fatalf("테이블 설명은 받아야 한다: %v", got.Values)
	}
	if len(got.Unknown) != 1 || got.Unknown[0] != AttrDefault {
		t.Fatalf("기본값은 컬럼의 성질이라 테이블에서는 모르는 이름이다: %v", got.Unknown)
	}
}

// 아래는 실제 draw.io 저장본을 프로그램으로 변형해 만든 픽스처를 통과시킨다.
// 손으로 적은 XML은 파서만 읽고 draw.io는 못 그리는 일이 반복됐다(설계 원칙 3).
func TestExtractTable_실제픽스처에서제약을읽는다(t *testing.T) {
	diagrams, err := LoadDiagrams(filepath.Join("testdata", "erd_attrs.drawio"))
	if err != nil {
		t.Fatalf("픽스처 로드 실패: %v", err)
	}
	idx := BuildIndex(diagrams[0].Cells)

	var cust, ordr TableExtract
	for _, tc := range FindTables(diagrams[0].Cells) {
		switch tc.Value {
		case "CUST":
			cust = ExtractTable(tc, idx)
		case "ORDR":
			ordr = ExtractTable(tc, idx)
		}
	}

	if cust.Table.Comment != "고객" {
		t.Errorf("테이블 설명: got %q", cust.Table.Comment)
	}
	if cust.Table.Check != "CUST_NO > 0" {
		t.Errorf("테이블 CHECK: got %q", cust.Table.Check)
	}

	byName := map[string]int{}
	for i, c := range cust.Table.Columns {
		byName[c.Name] = i
	}
	nm := cust.Table.Columns[byName["CUST_NM"]]
	if nm.Default != "'미상'" || nm.Comment != "고객명" {
		t.Errorf("정의 셀에 붙인 컬럼 제약: %+v", nm)
	}
	no := cust.Table.Columns[byName["CUST_NO"]]
	if no.Check != "" {
		t.Errorf("충돌한 값을 채택하면 안 된다: %q", no.Check)
	}
	if len(cust.Conflicts) != 1 || cust.Conflicts[0].ColumnName != "CUST_NO" {
		t.Fatalf("CUST_NO 충돌 하나가 보고돼야 한다: %+v", cust.Conflicts)
	}
	// 진단은 행(tableRow) 셀에 붙는다 — annotate가 그 id로 테두리를 친다(I7).
	if cust.Conflicts[0].CellID != no.ID {
		t.Errorf("진단이 행 셀을 가리켜야 한다: got %q want %q", cust.Conflicts[0].CellID, no.ID)
	}

	ymd := ordr.Table.Columns[len(ordr.Table.Columns)-1]
	if ymd.Name != "ORDR_YMD" {
		t.Fatalf("픽스처 전제가 바뀌었다: 마지막 컬럼이 %q", ymd.Name)
	}
	// 행 셀에 붙인 것도 그 컬럼의 제약으로 받는다.
	if ymd.Comment != "주문일자" {
		t.Errorf("행 셀에 붙인 설명: got %q", ymd.Comment)
	}
	if len(ordr.Unknown) != 2 {
		t.Fatalf("테이블의 erd_default와 컬럼의 erd_chek 둘이 잡혀야 한다: %+v", ordr.Unknown)
	}
}
