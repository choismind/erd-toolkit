package convert

import (
	"strings"
	"testing"

	"erdtool/internal/drawio"
	"erdtool/internal/glossary"
)

func planTestDict(t *testing.T) *glossary.Dict {
	t.Helper()
	// glossary 패키지의 테스트 헬퍼는 쓸 수 없으므로(비공개), 여기서는
	// 실물 사전 대신 아주 작은 사전을 만든다.
	return glossary.MustLoadForTest(map[string]string{
		"고객번호": "CUST_NO",
	}, map[string]string{
		"고객": "CUST", "번호": "NO", "주문": "ORDR",
	})
}

func planCells() []drawio.RawCell {
	return []drawio.RawCell{
		{ID: "0"},
		{ID: "1", Parent: "0"},
		{ID: "t1", Parent: "1", Value: "고객", Style: "shape=table;childLayout=tableLayout;"},
		{ID: "r1", Parent: "t1", Style: "shape=tableRow;"},
		{ID: "r1k", Parent: "r1", Value: "PK", Style: "shape=partialRectangle;"},
		{ID: "r1d", Parent: "r1", Value: "고객번호 int NOT NULL", Style: "shape=partialRectangle;"},
		{ID: "e1", Parent: "1", Value: "주문한다", Style: "edgeStyle=entityRelationEdgeStyle;"},
	}
}

func TestBuildEdits_TableAndColumnOnly(t *testing.T) {
	edits, stats := buildEdits(planCells(), planTestDict(t), nil)

	if got, ok := edits["t1"]; !ok || got.Label != "CUST" || got.LogicalName != "고객" {
		t.Fatalf("테이블 편집이 틀렸다: %+v (ok=%v)", got, ok)
	}
	if got, ok := edits["r1d"]; !ok || got.Label != "CUST_NO int NOT NULL" || got.LogicalName != "고객번호" {
		t.Fatalf("컬럼 편집이 틀렸다: %+v (ok=%v)", got, ok)
	}
	if _, ok := edits["r1k"]; ok {
		t.Fatal("키 셀을 건드렸다")
	}
	if _, ok := edits["e1"]; ok {
		t.Fatal("관계선 라벨을 건드렸다")
	}
	if stats.Total != 2 {
		t.Fatalf("Total = %d; want 2", stats.Total)
	}
	if stats.Converted != 2 {
		t.Fatalf("Converted = %d; want 2", stats.Converted)
	}
	if len(stats.Unmatched) != 0 {
		t.Fatalf("Unmatched = %v; want none", stats.Unmatched)
	}
}

func TestBuildEdits_KeepsTypeAndSpacing(t *testing.T) {
	cells := planCells()
	for i := range cells {
		if cells[i].ID == "r1d" {
			cells[i].Value = "고객번호   int  NOT NULL"
		}
	}
	edits, _ := buildEdits(cells, planTestDict(t), nil)
	if got := edits["r1d"].Label; got != "CUST_NO   int  NOT NULL" {
		t.Fatalf("Label = %q; 첫 필드만 갈고 나머지 공백은 그대로여야 한다", got)
	}
}

func TestBuildEdits_UnmatchedReported(t *testing.T) {
	cells := planCells()
	for i := range cells {
		if cells[i].ID == "t1" {
			cells[i].Value = "약어"
		}
	}
	_, stats := buildEdits(cells, planTestDict(t), nil)
	if len(stats.Unmatched) == 0 {
		t.Fatal("미매칭이 보고되지 않았다")
	}
}

func TestBuildEdits_LogicalNameIsTheSource(t *testing.T) {
	// 이미 변환된 파일: label은 CUST, logicalName은 고객.
	// 사람이 label을 CUSTOMER로 고쳤어도 소스는 언제나 logicalName이다.
	cells := planCells()
	for i := range cells {
		if cells[i].ID == "t1" {
			cells[i].Value = "CUSTOMER"
		}
	}
	edits, _ := buildEdits(cells, planTestDict(t), map[string]string{"t1": "고객"})

	if got := edits["t1"].Label; got != "CUST" {
		t.Fatalf("Label = %q; logicalName(고객)에서 다시 변환했어야 한다", got)
	}
	if got := edits["t1"].LogicalName; got != "고객" {
		t.Fatalf("LogicalName = %q; want 고객", got)
	}
}

func TestBuildEdits_SkipsBlankValues(t *testing.T) {
	cells := planCells()
	for i := range cells {
		if cells[i].ID == "t1" {
			cells[i].Value = "   "
		}
	}
	edits, stats := buildEdits(cells, planTestDict(t), nil)
	if _, ok := edits["t1"]; ok {
		t.Fatal("값이 공백뿐인 셀을 건드렸다")
	}
	if stats.Total != 1 {
		t.Fatalf("Total = %d; want 1", stats.Total)
	}
}

func TestBuildEdits_ColumnLogicalNameKeepsTypeAndNotNull(t *testing.T) {
	// 2회차 실행 상황: 1회차가 정의 셀 label을 "CUST_NO int NOT NULL"로 바꾸고
	// logicalName에는 «이름만»("고객번호") 저장해 두었다.
	//
	// 논리명은 이름이지 타입이 아니다. 그러므로 이름은 logicalName에서,
	// 타입·NOT NULL·공백은 «현재 셀 값»에서 와야 한다. 논리명을 raw 전체로
	// 취급하면 타입이 에러 없이 통째로 사라진다.
	cells := planCells()
	for i := range cells {
		if cells[i].ID == "r1d" {
			cells[i].Value = "CUST_NO int NOT NULL"
		}
	}
	edits, _ := buildEdits(cells, planTestDict(t), map[string]string{"r1d": "고객번호"})

	if got := edits["r1d"].Label; got != "CUST_NO int NOT NULL" {
		t.Fatalf("Label = %q; 타입과 NOT NULL이 그대로 보존돼야 한다", got)
	}
	if got := edits["r1d"].LogicalName; got != "고객번호" {
		t.Fatalf("LogicalName = %q; want 고객번호", got)
	}
}

func TestBuildEdits_ColumnLogicalNameKeepsSpacing(t *testing.T) {
	// 2회차에도 원래 공백은 바이트 그대로여야 한다.
	cells := planCells()
	for i := range cells {
		if cells[i].ID == "r1d" {
			cells[i].Value = "CUST_NO   int  NOT NULL"
		}
	}
	edits, _ := buildEdits(cells, planTestDict(t), map[string]string{"r1d": "고객번호"})
	if got := edits["r1d"].Label; got != "CUST_NO   int  NOT NULL" {
		t.Fatalf("Label = %q; 나머지 원문은 공백까지 그대로여야 한다", got)
	}
}

func TestBuildEdits_UnmatchedIsDedupedInOrder(t *testing.T) {
	// "갑"과 "을"은 사전에 없다. "갑"은 테이블명과 둘째 컬럼에 두 번 나온다.
	// Unmatched는 «중복 제거, 등장 순»이라고 plan.go가 못박은 계약이다.
	cells := []drawio.RawCell{
		{ID: "0"},
		{ID: "1", Parent: "0"},
		{ID: "t1", Parent: "1", Value: "갑고객", Style: "shape=table;childLayout=tableLayout;"},
		{ID: "r1", Parent: "t1", Style: "shape=tableRow;"},
		{ID: "r1k", Parent: "r1", Value: "PK", Style: "shape=partialRectangle;"},
		{ID: "r1d", Parent: "r1", Value: "을번호 int", Style: "shape=partialRectangle;"},
		{ID: "r2", Parent: "t1", Style: "shape=tableRow;"},
		{ID: "r2k", Parent: "r2", Value: "", Style: "shape=partialRectangle;"},
		{ID: "r2d", Parent: "r2", Value: "갑주문 varchar", Style: "shape=partialRectangle;"},
	}
	_, stats := buildEdits(cells, planTestDict(t), nil)

	want := []string{"갑", "을"}
	if len(stats.Unmatched) != len(want) {
		t.Fatalf("Unmatched = %v; want %v (중복 제거, 등장 순)", stats.Unmatched, want)
	}
	for i := range want {
		if stats.Unmatched[i] != want[i] {
			t.Fatalf("Unmatched = %v; want %v (중복 제거, 등장 순)", stats.Unmatched, want)
		}
	}
	// 매칭된 낱말은 섞여 들어오지 않는다.
	if stats.Total != 3 {
		t.Fatalf("Total = %d; want 3", stats.Total)
	}
}

// TestBuildEdits_SpacedTableNameIsUsable는 공백이 든 테이블 이름이 쓸 수
// 있는 식별자로 나오고, 미매칭 보고가 «보이는 낱말»만 담는지 본다.
//
// 손으로 그린 논리 ERD는 테이블 이름을 "고객 정보"처럼 띄어 쓴다. 테이블
// 셀은 값 «전체»를 변환하므로 그 공백이 변환 엔진까지 그대로 들어온다.
// 공백을 낱말로 취급하면 label이 "CUST_ _정보"가 되어(원문도 아니고
// 식별자도 아니다) generate의 CREATE TABLE까지 흘러가고, 미매칭 목록에는
// " "가 실려 "미매칭 낱말 2개(  , 정보)"처럼 보이지 않는 항목이 찍힌다.
func TestBuildEdits_SpacedTableNameIsUsable(t *testing.T) {
	cells := planCells()
	for i := range cells {
		if cells[i].ID == "t1" {
			cells[i].Value = "고객 정보"
		}
	}
	edits, stats := buildEdits(cells, planTestDict(t), nil)

	// "정보"는 사전에 없으므로 한글 원문이 남는다. 공백은 밑줄로 바뀐다.
	if got := edits["t1"].Label; got != "CUST_정보" {
		t.Fatalf("t1.Label = %q; want CUST_정보", got)
	}
	if got := edits["t1"].LogicalName; got != "고객 정보" {
		t.Fatalf("t1.LogicalName = %q; 논리명은 원문 그대로여야 한다", got)
	}
	// 미매칭 목록은 사람이 눈으로 찾을 수 있는 낱말만 담아야 한다.
	for _, u := range stats.Unmatched {
		if strings.TrimSpace(u) == "" {
			t.Fatalf("미매칭 목록에 보이지 않는 항목이 있다: %q (전체 %q)", u, stats.Unmatched)
		}
	}
	if len(stats.Unmatched) != 1 || stats.Unmatched[0] != "정보" {
		t.Fatalf("stats.Unmatched = %q; want [정보]", stats.Unmatched)
	}
}
