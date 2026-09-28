// internal/report/xlsx.go
package report

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	"erdtool/internal/model"
)

// TableDocXLSX는 테이블정의서를 엑셀로 만든다.
//
// **테이블 하나에 시트 하나**이고, 맨 앞에 「목차」 시트가 온다. 예전에는 시트
// 하나에 테이블 전부를 세로로 쌓았는데, 그러면 열 너비를 어느 테이블에도 못
// 맞추고 틀 고정도 걸 수 없다(머리글 행이 여럿이라서). 열여섯 테이블짜리
// 파일에서 특정 테이블을 찾으려면 스크롤로 세어 내려가야 했다.
//
// 사내 표준 양식이 없다는 것을 확인하고(2026-09-04 소유자) 관례적인 정의서
// 레이아웃으로 짰다.
func TableDocXLSX(doc model.Document, opt Options) (*excelize.File, error) {
	f := excelize.NewFile()
	const indexSheet = "목차"
	if err := f.SetSheetName("Sheet1", indexSheet); err != nil {
		return nil, err
	}

	styles, err := newXLSXStyles(f)
	if err != nil {
		return nil, err
	}

	// 시트 이름을 먼저 다 정한다. 목차의 링크가 그 이름을 가리켜야 하는데,
	// 이름은 잘리고 겹치면 바뀌므로(엑셀 규칙) 나중에 따로 만들면 링크가
	// 조용히 어긋난다.
	var sheetNames []string
	used := map[string]bool{strings.ToLower(indexSheet): true}
	eachTable(doc, func(_ model.Diagram, t model.Table, _ conflictIndex) {
		sheetNames = append(sheetNames, uniqueSheetName(t.Name, used))
	})

	if err := writeIndexSheet(f, indexSheet, doc, opt, sheetNames, styles); err != nil {
		return nil, err
	}

	idx := 0
	var loopErr error
	eachTableWithRefs(doc, func(d model.Diagram, t model.Table, no int,
		ci conflictIndex, refs map[string]model.ColumnRef) {

		if loopErr != nil {
			return
		}
		loopErr = writeTableSheet(f, sheetNames[idx], no, d, t, ci, refs, opt, styles)
		idx++
	})
	if loopErr != nil {
		return nil, loopErr
	}
	return f, nil
}

// xlsxStyles는 문서 전체가 쓰는 서식 묶음이다.
type xlsxStyles struct {
	title  int
	label  int
	header int
	body   int
	link   int
}

func newXLSXStyles(f *excelize.File) (xlsxStyles, error) {
	var s xlsxStyles
	var err error
	border := []excelize.Border{
		{Type: "left", Color: "999999", Style: 1},
		{Type: "right", Color: "999999", Style: 1},
		{Type: "top", Color: "999999", Style: 1},
		{Type: "bottom", Color: "999999", Style: 1},
	}
	if s.title, err = f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 14},
	}); err != nil {
		return s, err
	}
	if s.label, err = f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Color: "555555"},
	}); err != nil {
		return s, err
	}
	if s.header, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"EEEEEE"}},
		Border:    border,
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	}); err != nil {
		return s, err
	}
	if s.body, err = f.NewStyle(&excelize.Style{
		Border:    border,
		Alignment: &excelize.Alignment{Vertical: "top"},
	}); err != nil {
		return s, err
	}
	if s.link, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Color: "1155CC", Underline: "single"},
		Border:    border,
		Alignment: &excelize.Alignment{Vertical: "top"},
	}); err != nil {
		return s, err
	}
	return s, nil
}

// 열 너비. 엑셀의 «문자 수» 단위다.
var (
	xlsxListWidths   = []float64{5, 24, 40, 8, 16}
	xlsxColumnWidths = []float64{5, 24, 22, 5, 8, 24, 10, 16, 28, 28}
)

func writeIndexSheet(f *excelize.File, sheet string, doc model.Document,
	opt Options, sheetNames []string, styles xlsxStyles) error {

	row := 1
	if err := setCell(f, sheet, 1, row, docTitle, styles.title); err != nil {
		return err
	}
	row += 2
	for _, line := range docHeaderLines(doc, opt) {
		if err := setCell(f, sheet, 1, row, line[0], styles.label); err != nil {
			return err
		}
		if err := setCell(f, sheet, 2, row, line[1], 0); err != nil {
			return err
		}
		row++
	}
	row++

	headerRow := row
	if err := writeSheetRow(f, sheet, row, tableListHeaders, styles.header); err != nil {
		return err
	}
	row++
	for i, r := range tableListRows(doc, opt) {
		if err := writeSheetRow(f, sheet, row, r, styles.body); err != nil {
			return err
		}
		// 테이블명 칸에서 그 테이블의 시트로 건너뛴다. 열여섯 장을 손으로
		// 넘기지 않게 하는 것이 목차의 값이다.
		cell, err := excelize.CoordinatesToCellName(2, row)
		if err == nil && i < len(sheetNames) {
			_ = f.SetCellHyperLink(sheet, cell, "'"+sheetNames[i]+"'!A1", "Location")
			_ = f.SetCellStyle(sheet, cell, cell, styles.link)
		}
		row++
	}

	applyWidths(f, sheet, xlsxListWidths)
	freezeBelow(f, sheet, headerRow)
	return nil
}

func writeTableSheet(f *excelize.File, sheet string, no int, d model.Diagram,
	t model.Table, ci conflictIndex, refs map[string]model.ColumnRef,
	opt Options, styles xlsxStyles) error {

	if _, err := f.NewSheet(sheet); err != nil {
		return err
	}

	row := 1
	if err := setCell(f, sheet, 1, row, tableHeading(no, t.Name), styles.title); err != nil {
		return err
	}
	row++
	for _, note := range tableHeadingLines(d, t, ci, opt) {
		if err := setCell(f, sheet, 1, row, note, styles.label); err != nil {
			return err
		}
		row++
	}
	row++

	headerRow := row
	if err := writeSheetRow(f, sheet, row, tableDocHeaders, styles.header); err != nil {
		return err
	}
	row++
	for _, r := range columnRows(t, ci, refs) {
		if err := writeSheetRow(f, sheet, row, r, styles.body); err != nil {
			return err
		}
		row++
	}

	// 관계는 **같은 탭 안**에 이어 붙인다(2026-09-04 소유자 결정). 별도
	// 시트로 빼면 테이블 하나를 보려고 탭 둘을 오가야 한다.
	row += 2
	if err := setCell(f, sheet, 1, row, "관계", styles.title); err != nil {
		return err
	}
	row++
	if rows := tableRelationRows(d, t, refs); len(rows) == 0 {
		if err := setCell(f, sheet, 1, row, noRelationNote, styles.label); err != nil {
			return err
		}
	} else {
		if err := writeSheetRow(f, sheet, row, relationHeaders, styles.header); err != nil {
			return err
		}
		row++
		for _, r := range rows {
			if err := writeSheetRow(f, sheet, row, r, styles.body); err != nil {
				return err
			}
			row++
		}
	}

	applyWidths(f, sheet, xlsxColumnWidths)
	freezeBelow(f, sheet, headerRow)
	return nil
}

func setCell(f *excelize.File, sheet string, col, row int, value string, style int) error {
	cell, err := excelize.CoordinatesToCellName(col, row)
	if err != nil {
		return err
	}
	if err := f.SetCellValue(sheet, cell, value); err != nil {
		return err
	}
	if style != 0 {
		return f.SetCellStyle(sheet, cell, cell, style)
	}
	return nil
}

// writeSheetRow는 한 줄을 A열부터 채운다. 열 이름을 "A"+i로 손계산하지
// 않는다 — 열이 늘어 Z를 넘어가면 조용히 틀린 자리에 쓴다.
func writeSheetRow(f *excelize.File, sheet string, row int, values []string, style int) error {
	for i, v := range values {
		if err := setCell(f, sheet, i+1, row, v, style); err != nil {
			return err
		}
	}
	return nil
}

func applyWidths(f *excelize.File, sheet string, widths []float64) {
	for i, w := range widths {
		name, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			continue
		}
		_ = f.SetColWidth(sheet, name, name, w)
	}
}

// freezeBelow는 머리글 행까지를 고정한다. 서식은 산출물의 «있으면 좋은»
// 부분이라 실패해도 문서를 못 내보내는 이유로 삼지 않는다.
func freezeBelow(f *excelize.File, sheet string, headerRow int) {
	top := fmt.Sprintf("A%d", headerRow+1)
	_ = f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, YSplit: headerRow, TopLeftCell: top, ActivePane: "bottomLeft",
	})
}

// sheetNameForbidden은 엑셀이 시트 이름에 허용하지 않는 문자다.
var sheetNameForbidden = strings.NewReplacer(
	":", "_", "\\", "_", "/", "_", "?", "_", "*", "_", "[", "_", "]", "_")

// uniqueSheetName은 테이블 이름을 엑셀 시트 이름으로 바꾼다. 규칙은 엑셀의
// 것이다 — 31자 이하, 금지 문자 없음, 빈 이름 불가, 대소문자 구분 없이 중복
// 불가. 겹치면 뒤에 번호를 붙인다(같은 이름 테이블이 여러 페이지에 있을 수
// 있고, 그것은 경고 대상이지 산출물을 못 내는 이유가 아니다).
func uniqueSheetName(tableName string, used map[string]bool) string {
	base := strings.TrimSpace(sheetNameForbidden.Replace(tableName))
	if base == "" {
		base = "이름없음"
	}
	base = truncateRunes(base, 31)

	name := base
	for n := 2; used[strings.ToLower(name)]; n++ {
		suffix := fmt.Sprintf("_%d", n)
		name = truncateRunes(base, 31-len([]rune(suffix))) + suffix
	}
	used[strings.ToLower(name)] = true
	return name
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
