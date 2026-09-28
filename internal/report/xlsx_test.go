// internal/report/xlsx_test.go
package report

import (
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"erdtool/internal/model"
)

// 레이아웃이 바뀌었다(2026-09-04): 시트 하나에 전 테이블을 쌓던 것을 «목차
// 시트 + 테이블당 시트 하나»로 바꿨다. 아래 테스트들은 그 구조를 지킨다.

func TestTableDocXLSX_목차와테이블시트가생긴다(t *testing.T) {
	f, err := TableDocXLSX(sampleDoc(), Options{})
	if err != nil {
		t.Fatalf("TableDocXLSX failed: %v", err)
	}
	defer f.Close()

	if first := f.GetSheetName(0); first != "목차" {
		t.Fatalf("첫 시트는 목차여야 한다: %q", first)
	}
	if title, _ := f.GetCellValue("목차", "A1"); title != docTitle {
		t.Errorf("목차 A1: got %q", title)
	}
	if idx, _ := f.GetSheetIndex("Orders"); idx < 0 {
		t.Fatalf("테이블 시트가 없다: %v", f.GetSheetList())
	}
}

func TestTableDocXLSX_테이블시트에머리와컬럼이있다(t *testing.T) {
	f, err := TableDocXLSX(sampleDoc(), Options{})
	if err != nil {
		t.Fatalf("TableDocXLSX failed: %v", err)
	}
	defer f.Close()

	if head, _ := f.GetCellValue("Orders", "A1"); !strings.Contains(head, "Orders") {
		t.Errorf("시트 제목에 테이블명이 있어야 한다: %q", head)
	}
	// 머리글 행의 자리는 «페이지/설명» 줄 수에 따라 달라지므로 찾아서 본다.
	row := findHeaderRow(t, f, "Orders")
	if got, _ := f.GetCellValue("Orders", cellAt(t, 2, row)); got != "컬럼명" {
		t.Errorf("머리글 두 번째 칸: got %q", got)
	}
	if got, _ := f.GetCellValue("Orders", cellAt(t, 2, row+1)); got != "order_id" {
		t.Errorf("첫 컬럼: got %q", got)
	}
	if got, _ := f.GetCellValue("Orders", cellAt(t, 1, row+1)); got != "1" {
		t.Errorf("순번이 1부터 붙어야 한다: got %q", got)
	}
}

func TestTableDocXLSX_페이지표기가옵션을따른다(t *testing.T) {
	// 같은 실행의 산출물들이 서로 다른 제목을 달고 나가면 안 된다.
	// 엑셀도 나머지 셋과 같은 규칙(pageLabel)을 써야 한다.
	domain, err := TableDocXLSX(sampleDoc(), Options{PageAsDomain: true})
	if err != nil {
		t.Fatalf("TableDocXLSX failed: %v", err)
	}
	defer domain.Close()
	if !sheetContains(t, domain, "Orders", "페이지-1") || !sheetContains(t, domain, "Orders", "도메인") {
		t.Errorf("page_as_domain이 켜지면 «도메인»으로 불러야 한다")
	}

	plain, err := TableDocXLSX(sampleDoc(), Options{})
	if err != nil {
		t.Fatalf("TableDocXLSX failed: %v", err)
	}
	defer plain.Close()
	if !sheetContains(t, plain, "Orders", "페이지-1") {
		t.Errorf("꺼져 있어도 페이지명은 보여야 한다")
	}
	if sheetContains(t, plain, "Orders", "도메인") {
		t.Errorf("꺼져 있으면 «도메인»이라 부르지 않는다")
	}
}

func TestUniqueSheetName_엑셀규칙을지킨다(t *testing.T) {
	used := map[string]bool{}
	cases := []struct{ in, want string }{
		{"주문/선적", "주문_선적"},                                 // 금지 문자
		{"", "이름없음"},                                       // 빈 이름
		{"주문/선적", "주문_선적_2"},                               // 중복
		{strings.Repeat("가", 40), strings.Repeat("가", 31)}, // 31자
	}
	for _, c := range cases {
		if got := uniqueSheetName(c.in, used); got != c.want {
			t.Errorf("uniqueSheetName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTableDocXLSX_같은이름테이블도시트가겹치지않는다(t *testing.T) {
	// 같은 이름 테이블은 경고 대상이지(duplicate_table_name) 산출물을 못
	// 내는 이유가 아니다.
	doc := model.Document{Diagrams: []model.Diagram{{
		ID: "p1", Name: "물리",
		Tables: []model.Table{
			{ID: "t1", Name: "고객", Columns: []model.Column{{ID: "r1", Name: "A", Type: "int"}}},
			{ID: "t2", Name: "고객", Columns: []model.Column{{ID: "r2", Name: "B", Type: "int"}}},
		},
	}}}
	f, err := TableDocXLSX(doc, Options{})
	if err != nil {
		t.Fatalf("TableDocXLSX failed: %v", err)
	}
	defer f.Close()
	if len(f.GetSheetList()) != 3 {
		t.Fatalf("목차 + 테이블 둘이어야 한다: %v", f.GetSheetList())
	}
}

// --- 아래는 위 테스트들이 쓰는 도우미다 ---

func cellAt(t *testing.T, col, row int) string {
	t.Helper()
	name, err := excelize.CoordinatesToCellName(col, row)
	if err != nil {
		t.Fatalf("CoordinatesToCellName(%d,%d): %v", col, row, err)
	}
	return name
}

// findHeaderRow는 컬럼 표의 머리글 행을 찾는다. 그 자리는 테이블에 붙은
// 설명/제약 줄 수에 따라 달라지므로 상수로 박지 않는다.
func findHeaderRow(t *testing.T, f *excelize.File, sheet string) int {
	t.Helper()
	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	for i, r := range rows {
		if len(r) > 1 && r[0] == tableDocHeaders[0] && r[1] == tableDocHeaders[1] {
			return i + 1
		}
	}
	t.Fatalf("머리글 행을 못 찾았다: %v", rows)
	return 0
}

func sheetContains(t *testing.T, f *excelize.File, sheet, needle string) bool {
	t.Helper()
	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	for _, r := range rows {
		for _, v := range r {
			if strings.Contains(v, needle) {
				return true
			}
		}
	}
	return false
}

func TestTableDocXLSX_관계가같은탭에붙는다(t *testing.T) {
	// 별도 시트로 빼면 테이블 하나를 보려고 탭 둘을 오가야 한다
	// (2026-09-04 소유자 결정).
	f, err := TableDocXLSX(refDoc(), Options{})
	if err != nil {
		t.Fatalf("TableDocXLSX failed: %v", err)
	}
	defer f.Close()
	for _, want := range []string{"관계", "방향", "상대 테이블"} {
		if !sheetContains(t, f, "주문", want) {
			t.Errorf("주문 탭에 %q가 없다", want)
		}
	}
	// 관계 전용 시트를 따로 만들지 않는다.
	if len(f.GetSheetList()) != 3 {
		t.Errorf("목차 + 테이블 둘이어야 한다: %v", f.GetSheetList())
	}
}
