package glossary

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

// writeTestDict는 실제 xlsx 파일을 만든다.
//
// 바이너리 픽스처를 커밋하면 리뷰어가 내용을 읽을 수 없고, 외부 드라이브의
// 실물 사전(E:\devol\...)에 의존하면 다른 기계에서 테스트가 깨진다. 실제
// xlsx를 만들면 Load를 포맷째로 검사하면서 픽스처 내용도 코드로 읽힌다.
func writeTestDict(t *testing.T, terms, words [][2]string) string {
	t.Helper()

	f := excelize.NewFile()
	t.Cleanup(func() { _ = f.Close() })

	fill := func(sheet string, header [2]string, rows [][2]string) {
		if _, err := f.NewSheet(sheet); err != nil {
			t.Fatalf("NewSheet(%q): %v", sheet, err)
		}
		must := func(err error) {
			if err != nil {
				t.Fatalf("SetCellStr: %v", err)
			}
		}
		must(f.SetCellStr(sheet, "A1", header[0]))
		must(f.SetCellStr(sheet, "B1", header[1]))
		for i, r := range rows {
			must(f.SetCellStr(sheet, fmt.Sprintf("A%d", i+2), r[0]))
			must(f.SetCellStr(sheet, fmt.Sprintf("B%d", i+2), r[1]))
		}
	}

	fill("공통표준용어", [2]string{"공통표준용어명", "공통표준용어영문약어명"}, terms)
	fill("공통표준단어", [2]string{"공통표준단어명", "공통표준단어영문약어명"}, words)
	if err := f.DeleteSheet("Sheet1"); err != nil {
		t.Fatalf("DeleteSheet: %v", err)
	}

	path := filepath.Join(t.TempDir(), "표준용어사전.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	return path
}

func TestLoad_ReadsBothSheets(t *testing.T) {
	p := writeTestDict(t,
		[][2]string{{"고객번호", "CUST_NO"}},
		[][2]string{{"고객", "CUST"}, {"번호", "NO"}, {"1회섭취참고량", "RAFOS"}},
	)

	d, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, ok := d.Term("고객번호"); !ok || got != "CUST_NO" {
		t.Fatalf("Term(고객번호) = %q, %v; want CUST_NO, true", got, ok)
	}
	if got, ok := d.Word("고객"); !ok || got != "CUST" {
		t.Fatalf("Word(고객) = %q, %v; want CUST, true", got, ok)
	}
	if _, ok := d.Term("없는용어"); ok {
		t.Fatal("없는 용어가 있다고 나왔다")
	}
	if got := d.MaxWordRunes(); got != 7 {
		t.Fatalf("MaxWordRunes = %d; want 7 (1회섭취참고량)", got)
	}
}

func TestLoad_DedupKeepsFirstOccurrence(t *testing.T) {
	p := writeTestDict(t,
		[][2]string{{"고객번호", "CUST_NO"}, {"고객번호", "CUSTOMER_NUMBER"}},
		[][2]string{{"고객", "CUST"}, {"고객", "CUSTOMER"}},
	)

	d, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, ok := d.Term("고객번호"); !ok || got != "CUST_NO" {
		t.Fatalf("Term(고객번호) = %q, %v; want CUST_NO(앞 행), true — 중복 행에서 뒤 행이 이겼다", got, ok)
	}
	if got, ok := d.Word("고객"); !ok || got != "CUST" {
		t.Fatalf("Word(고객) = %q, %v; want CUST(앞 행), true — 중복 행에서 뒤 행이 이겼다", got, ok)
	}
}

func TestLoad_RejectsTooFewSheets(t *testing.T) {
	f := excelize.NewFile()
	t.Cleanup(func() { _ = f.Close() })
	path := filepath.Join(t.TempDir(), "한장뿐.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("시트가 둘 미만이면 에러여야 한다")
	}
}
