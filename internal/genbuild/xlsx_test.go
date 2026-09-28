package genbuild

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

func writeSheet(f *excelize.File, name string, rows [][]string) {
	f.NewSheet(name)
	for r, row := range rows {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
			f.SetCellValue(name, cell, v)
		}
	}
}

func TestParseXLSX_OneSheetPerPage(t *testing.T) {
	f := excelize.NewFile()
	writeSheet(f, "주문정보", validSheet())
	f.DeleteSheet("Sheet1")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("xlsx 작성 실패: %v", err)
	}

	pages, err := ParseXLSX(buf.Bytes())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pages) != 1 || pages[0].Name != "주문정보" {
		t.Fatalf("got %+v", pages)
	}
}

func TestParseXLSX_OneBadSheetFailsWholeFile(t *testing.T) {
	f := excelize.NewFile()
	writeSheet(f, "정상", validSheet())
	bad := validSheet()
	bad[7][1] = "없는테이블"
	writeSheet(f, "비정상", bad)
	f.DeleteSheet("Sheet1")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("xlsx 작성 실패: %v", err)
	}

	pages, err := ParseXLSX(buf.Bytes())
	if err == nil {
		t.Fatal("한 시트가 실패하면 파일 전체가 에러여야 한다")
	}
	if pages != nil {
		t.Fatal("실패 시 부분 결과를 돌려주면 안 된다")
	}
}
