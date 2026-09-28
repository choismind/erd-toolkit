package report

import (
	_ "embed"

	"github.com/jung-kurt/gofpdf"

	"erdtool/internal/model"
)

// 폰트를 컴파일 시점에 바이너리에 임베드한다 — CWD/실행파일 위치에 상대적인
// 파일 경로로 읽으면 배포된 단일 바이너리를 원래 소스 트리 밖에서 실행하거나
// .exe 파일만 복사했을 때 폰트를 찾지 못해 PDF 생성이 실패한다(스펙: 크로스컴파일
// 단일 바이너리 + "다운로드 → 압축 해제 → 실행" 배포 요구사항과 충돌하는 지점 —
// Task 19 실제 구현 중 리뷰에서 발견됨).
//
//go:embed fonts/NanumGothic.ttf
var nanumGothicTTF []byte

// 열 너비(mm). A4 가로 297에서 좌우 여백 10씩을 뺀 277에 맞춘다.
//
// **세로가 아니라 가로다.** 컬럼 표가 열 개라 세로로는 어느 열도 제 폭을
// 못 갖는다. 예전 PDF는 열 자체가 없이 값을 공백으로 이어 붙인 한 줄이었고,
// 그래서 어느 항목이 비었는지 읽을 수 없었다.
var (
	pdfColumnWidths   = []float64{9, 40, 40, 8, 12, 34, 14, 26, 44, 50}
	pdfListWidths     = []float64{10, 60, 100, 20, 87}
	pdfRelationWidths = []float64{9, 22, 44, 44, 44, 57, 57}
)

func TableDocPDF(doc model.Document, outPath string, opt Options) error {
	pdf := gofpdf.New("L", "mm", "A4", "")
	pdf.AddUTF8FontFromBytes("NanumGothic", "", nanumGothicTTF)
	pdf.SetMargins(10, 10, 10)
	pdf.AddPage()

	pdf.SetFont("NanumGothic", "", 16)
	pdf.CellFormat(0, 10, docTitle, "", 1, "C", false, 0, "")
	pdf.Ln(2)

	pdf.SetFont("NanumGothic", "", 9)
	for _, line := range docHeaderLines(doc, opt) {
		pdf.CellFormat(0, 5, line[0]+": "+line[1], "", 1, "L", false, 0, "")
	}
	pdf.Ln(3)

	pdf.SetFont("NanumGothic", "", 11)
	pdf.CellFormat(0, 7, "테이블 일람", "", 1, "L", false, 0, "")
	pdfTable(pdf, tableListHeaders, tableListRows(doc, opt), pdfListWidths)

	// **테이블 하나가 한 페이지다**(2026-09-04 소유자 결정). 그 테이블의
	// 관계도 같은 페이지 안에 붙는다 — 정의서를 넘기다 어느 테이블에서
	// 멈추면 그 자리에서 컬럼과 관계를 함께 볼 수 있어야 한다.
	//
	// 컬럼이 많아 한 장을 넘기면 그때는 이어서 그린다. 페이지를 나누는
	// 규칙이 «테이블마다»이지 «무조건 한 장»은 아니다.
	eachTableWithRefs(doc, func(d model.Diagram, t model.Table, no int,
		i conflictIndex, refs map[string]model.ColumnRef) {

		pdf.AddPage()
		pdf.SetFont("NanumGothic", "", 12)
		pdf.CellFormat(0, 8, tableHeading(no, t.Name), "", 1, "L", false, 0, "")
		pdf.SetFont("NanumGothic", "", 8)
		for _, note := range tableHeadingLines(d, t, i, opt) {
			pdf.CellFormat(0, 4.5, note, "", 1, "L", false, 0, "")
		}
		pdfTable(pdf, tableDocHeaders, columnRows(t, i, refs), pdfColumnWidths)

		pdf.Ln(4)
		pdf.SetFont("NanumGothic", "", 10)
		pdf.CellFormat(0, 6, "관계", "", 1, "L", false, 0, "")
		if rows := tableRelationRows(d, t, refs); len(rows) == 0 {
			pdf.SetFont("NanumGothic", "", 8)
			pdf.CellFormat(0, 5, noRelationNote, "", 1, "L", false, 0, "")
		} else {
			pdfTable(pdf, relationHeaders, rows, pdfRelationWidths)
		}
	})

	return pdf.OutputFileAndClose(outPath)
}

const (
	headerHeight = 6.0
	rowHeight    = 5.5
)

// bottomLimit는 본문을 그릴 수 있는 아래쪽 한계다.
func bottomLimit(pdf *gofpdf.Fpdf) float64 {
	_, pageH := pdf.GetPageSize()
	_, _, _, bottom := pdf.GetMargins()
	return pageH - bottom
}

// pdfTable은 테두리 있는 표 하나를 찍는다. gofpdf의 CellFormat은 줄바꿈을
// 하지 않으므로, 칸을 넘치는 값은 fitText가 잘라 «…»를 붙인다.
//
// 표가 장을 넘어가면 **머리글을 다시 찍는다.** 안 그러면 둘째 장부터는 어느
// 칸이 무엇인지 알 수 없다 — 열 개짜리 표에서 그것은 문서가 아니다.
func pdfTable(pdf *gofpdf.Fpdf, headers []string, rows [][]string, widths []float64) {
	drawHeader := func() {
		pdf.SetFont("NanumGothic", "", 8)
		pdf.SetFillColor(238, 238, 238)
		for i, h := range headers {
			pdf.CellFormat(widths[i], headerHeight, h, "1", 0, "C", true, 0, "")
		}
		pdf.Ln(-1)
	}

	drawHeader()
	for _, r := range rows {
		if pdf.GetY()+rowHeight > bottomLimit(pdf) {
			pdf.AddPage()
			drawHeader()
		}
		for i, v := range r {
			pdf.CellFormat(widths[i], rowHeight, fitText(pdf, v, widths[i]), "1", 0, "L", false, 0, "")
		}
		pdf.Ln(-1)
	}
}

// fitText는 칸 너비에 맞게 글자를 자른다. **자를 때는 «…»를 붙인다** — 그냥
// 자르면 잘린 값과 원래 짧은 값을 구별할 수 없어서, 문서를 읽는 사람이 틀린
// 이름을 그대로 옮겨 적는다.
func fitText(pdf *gofpdf.Fpdf, s string, width float64) string {
	const padding = 2.0 // CellFormat의 좌우 여백
	limit := width - padding
	if s == "" || pdf.GetStringWidth(s) <= limit {
		return s
	}
	runes := []rune(s)
	for n := len(runes) - 1; n > 0; n-- {
		candidate := string(runes[:n]) + "…"
		if pdf.GetStringWidth(candidate) <= limit {
			return candidate
		}
	}
	return "…"
}
