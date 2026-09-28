package genbuild

import (
	"reflect"
	"testing"
)

func TestParseTableBlock_GroupsBySeqOrder(t *testing.T) {
	rows := [][]string{
		{"테이블명", "순번", "컬럼명", "컬럼유형", "색인여부", "널허용", "단일값"},
		{"고객", "2", "고객명", "char(50)", "", "False", "False"},
		{"고객", "1", "고객번호", "int", "PK", "False", "False"},
		{"주문", "1", "주문번호", "int", "PK", "False", "False"},
	}

	tables, err := parseTableBlock(rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []TableDef{
		{Name: "고객", Columns: []ColumnDef{
			{Name: "고객번호", Type: "int", Key: "PK", Nullable: false, Unique: false},
			{Name: "고객명", Type: "char(50)", Key: "", Nullable: false, Unique: false},
		}},
		{Name: "주문", Columns: []ColumnDef{
			{Name: "주문번호", Type: "int", Key: "PK", Nullable: false, Unique: false},
		}},
	}
	if !reflect.DeepEqual(tables, want) {
		t.Fatalf("got %+v\nwant %+v", tables, want)
	}
}

func TestParseTableBlock_RejectsWrongHeader(t *testing.T) {
	rows := [][]string{
		{"테이블", "순번", "컬럼명", "컬럼유형", "색인여부", "널허용", "단일값"},
	}
	if _, err := parseTableBlock(rows); err == nil {
		t.Fatal("헤더가 다르면 에러여야 한다")
	}
}

func TestParseTableBlock_RejectsDuplicateColumn(t *testing.T) {
	rows := [][]string{
		{"테이블명", "순번", "컬럼명", "컬럼유형", "색인여부", "널허용", "단일값"},
		{"고객", "1", "고객번호", "int", "PK", "False", "False"},
		{"고객", "2", "고객번호", "int", "", "False", "False"},
	}
	if _, err := parseTableBlock(rows); err == nil {
		t.Fatal("같은 테이블 안에 컬럼명이 중복되면 에러여야 한다")
	}
}

func TestParseTableBlock_RejectsBadBoolean(t *testing.T) {
	rows := [][]string{
		{"테이블명", "순번", "컬럼명", "컬럼유형", "색인여부", "널허용", "단일값"},
		{"고객", "1", "고객번호", "int", "PK", "예", "False"},
	}
	if _, err := parseTableBlock(rows); err == nil {
		t.Fatal("널허용이 True/False가 아니면 에러여야 한다")
	}
}

// 공백이 든 컬럼명은 emit.go columnValue가 공백으로 이어 붙이고
// internal/drawio/tables.go parseColumnValue가 «첫 낱말»을 이름으로 읽으므로,
// 이름 자체에 공백이 있으면 round trip에서 이름·타입 경계가 조용히
// 어긋난다(리뷰에서 발견). 로드 시점에 크게 거부한다.
func TestParseTableBlock_RejectsSpaceInColumnName(t *testing.T) {
	rows := [][]string{
		{"테이블명", "순번", "컬럼명", "컬럼유형", "색인여부", "널허용", "단일값"},
		{"고객", "1", "고객 번호", "int", "PK", "False", "False"},
	}
	if _, err := parseTableBlock(rows); err == nil {
		t.Fatal("컬럼명에 공백이 있으면 에러여야 한다")
	}
}

func TestSplitBlocks_FindsMarker(t *testing.T) {
	rows := [][]string{
		{"테이블명", "순번", "컬럼명", "컬럼유형", "색인여부", "널허용", "단일값"},
		{"고객", "1", "고객번호", "int", "PK", "False", "False"},
		{"## 관계"},
		{"순번", "원천테이블명", "원천컬럼명", "원천카디널리티", "원천색인", "연관명", "목표카디널리티", "목표색인", "목표테이블명", "목표컬럼명"},
		{"1", "고객", "고객번호", "ERone", "PK", "고객주문", "ERzeroToMany", "FK1", "주문", "주문고객번호"},
	}
	tableRows, relRows, err := splitBlocks(rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tableRows) != 2 { // 헤더 + 데이터 1행
		t.Fatalf("tableRows 길이: got %d, want 2", len(tableRows))
	}
	if len(relRows) != 2 { // 헤더 + 데이터 1행
		t.Fatalf("relRows 길이: got %d, want 2", len(relRows))
	}
}

func TestSplitBlocks_MissingMarkerIsError(t *testing.T) {
	rows := [][]string{
		{"테이블명", "순번", "컬럼명", "컬럼유형", "색인여부", "널허용", "단일값"},
		{"고객", "1", "고객번호", "int", "PK", "False", "False"},
	}
	if _, _, err := splitBlocks(rows); err == nil {
		t.Fatal("## 관계 마커가 없으면 에러여야 한다")
	}
}

func TestParseRelationBlock_ParsesRow(t *testing.T) {
	rows := [][]string{
		{"순번", "원천테이블명", "원천컬럼명", "원천카디널리티", "원천색인", "연관명", "목표카디널리티", "목표색인", "목표테이블명", "목표컬럼명"},
		{"1", "고객", "고객번호", "ERone", "PK", "고객주문", "ERzeroToMany", "FK1", "주문", "주문고객번호"},
	}
	rels, err := parseRelationBlock(rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := RelationDef{
		Seq: 1, SourceTable: "고객", SourceColumn: "고객번호", SourceCardinality: "ERone", SourceKey: "PK",
		RelationName: "고객주문", TargetCardinality: "ERzeroToMany", TargetKey: "FK1",
		TargetTable: "주문", TargetColumn: "주문고객번호",
	}
	if len(rels) != 1 || rels[0] != want {
		t.Fatalf("got %+v\nwant %+v", rels, want)
	}
}

func TestParseRelationBlock_RejectsUnknownCardinality(t *testing.T) {
	rows := [][]string{
		{"순번", "원천테이블명", "원천컬럼명", "원천카디널리티", "원천색인", "연관명", "목표카디널리티", "목표색인", "목표테이블명", "목표컬럼명"},
		{"1", "고객", "고객번호", "ERbogus", "PK", "고객주문", "ERzeroToMany", "FK1", "주문", "주문고객번호"},
	}
	if _, err := parseRelationBlock(rows); err == nil {
		t.Fatal("알 수 없는 카디널리티 코드는 에러여야 한다")
	}
}

func TestParseRelationBlock_RejectsBlankColumn(t *testing.T) {
	rows := [][]string{
		{"순번", "원천테이블명", "원천컬럼명", "원천카디널리티", "원천색인", "연관명", "목표카디널리티", "목표색인", "목표테이블명", "목표컬럼명"},
		{"1", "고객", "", "ERone", "PK", "고객주문", "ERzeroToMany", "FK1", "주문", "주문고객번호"},
	}
	if _, err := parseRelationBlock(rows); err == nil {
		t.Fatal("원천컬럼명이 비어 있으면 에러여야 한다(컬럼 레벨 관계만 지원)")
	}
}

func validSheet() [][]string {
	return [][]string{
		{"테이블명", "순번", "컬럼명", "컬럼유형", "색인여부", "널허용", "단일값"},
		{"고객", "1", "고객번호", "int", "PK", "False", "False"},
		{"고객", "2", "고객명", "char(50)", "", "False", "False"},
		{"주문", "1", "주문번호", "int", "PK", "False", "False"},
		{"주문", "2", "주문고객번호", "int", "FK1", "False", "False"},
		{"## 관계"},
		{"순번", "원천테이블명", "원천컬럼명", "원천카디널리티", "원천색인", "연관명", "목표카디널리티", "목표색인", "목표테이블명", "목표컬럼명"},
		{"1", "고객", "고객번호", "ERone", "PK", "고객주문", "ERzeroToMany", "FK1", "주문", "주문고객번호"},
	}
}

func TestParseSheet_ValidSheetRoundTrips(t *testing.T) {
	page, err := ParseSheet("주문정보", validSheet())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Name != "주문정보" {
		t.Errorf("페이지 이름: got %q", page.Name)
	}
	if len(page.Tables) != 2 || len(page.Relations) != 1 {
		t.Fatalf("got %d tables, %d relations", len(page.Tables), len(page.Relations))
	}
}

func TestParseSheet_RejectsUnknownSourceTable(t *testing.T) {
	rows := validSheet()
	rows[7][1] = "없는테이블" // 관계 데이터 행의 원천테이블명
	if _, err := ParseSheet("p", rows); err == nil {
		t.Fatal("존재하지 않는 원천테이블명은 에러여야 한다")
	}
}

func TestParseSheet_RejectsUnknownSourceColumn(t *testing.T) {
	rows := validSheet()
	rows[7][2] = "없는컬럼"
	if _, err := ParseSheet("p", rows); err == nil {
		t.Fatal("존재하지 않는 원천컬럼명은 에러여야 한다")
	}
}

func TestParseSheet_RejectsKeyLabelMismatch(t *testing.T) {
	rows := validSheet()
	rows[7][4] = "FK9" // 원천색인이 테이블 블록의 실제 색인여부(PK)와 다르다
	if _, err := ParseSheet("p", rows); err == nil {
		t.Fatal("원천색인이 테이블 블록과 다르면 에러여야 한다")
	}
}

// TestParseTableBlock_AllowsSpacesInColumnType는 실제 DB가 내는 타입이
// 설계서에도 적힐 수 있어야 함을 고정한다. 컬럼명의 공백 금지는 남는다 —
// drawio.parseColumnValue가 첫 낱말을 이름으로 보기 때문이다.
func TestParseTableBlock_AllowsSpacesInColumnType(t *testing.T) {
	rows := [][]string{
		tableHeader,
		{"주문", "1", "created_at", "timestamp with time zone", "PK", "False", "False"},
	}
	tables, err := parseTableBlock(rows)
	if err != nil {
		t.Fatalf("parseTableBlock: %v", err)
	}
	if got := tables[0].Columns[0].Type; got != "timestamp with time zone" {
		t.Errorf("Type = %q; want %q", got, "timestamp with time zone")
	}
}
