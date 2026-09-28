package genbuild

import (
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"
)

// ParseXLSX는 여러 탭을 가진 xlsx를 읽어 탭마다 PageDef 하나를 만든다
// (탭 = drawio 페이지 1:1, 사용자 결정). 탭 하나라도 파싱/교차검증에
// 실패하면 nil, error를 돌려준다 — 부분 페이지를 담은 결과를 반환하지
// 않는다(원장: "xlsx 파일 전체의 실패 단위").
func ParseXLSX(src []byte) ([]PageDef, error) {
	f, err := excelize.OpenReader(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("xlsx 열기 실패: %w", err)
	}
	defer f.Close()

	var pages []PageDef
	for _, name := range f.GetSheetList() {
		rows, err := f.GetRows(name)
		if err != nil {
			return nil, fmt.Errorf("시트 %q 읽기 실패: %w", name, err)
		}
		page, err := ParseSheet(name, rows)
		if err != nil {
			return nil, err
		}
		pages = append(pages, page)
	}
	return pages, nil
}
