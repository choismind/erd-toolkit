package genbuild

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// 아래 스타일 상수는 internal/drawio/testdata/entity_table_basic.drawio에서
// 그대로 옮긴 문자열이다(프로젝트 원칙 3번 — 손으로 지어내지 않는다).
// TestEmitTableCells_RoundTripsThroughRealParser가 drawio.LoadDiagramsBytes로
// 되읽어 이 문자열이 실제로 인식됨을 검증한다.
const (
	styleTable   = "shape=table;startSize=30;container=1;collapsible=1;childLayout=tableLayout;fixedRows=1;rowLines=0;fontStyle=1;align=center;resizeLast=1;html=1;"
	styleKeyCell = "shape=partialRectangle;connectable=0;fillColor=none;top=0;left=0;bottom=0;right=0;fontStyle=1;overflow=hidden;whiteSpace=wrap;html=1;"
	styleDefCell = "shape=partialRectangle;connectable=0;fillColor=none;top=0;left=0;bottom=0;right=0;align=left;spacingLeft=6;overflow=hidden;whiteSpace=wrap;html=1;"
)

// styleRow는 행 셀의 스타일이다. bottom은 언제나 0이다.
//
// tableRow의 bottom=1은 그 행의 «아래 모서리»에 선을 그린다. genbuild가
// 내는 테이블은 행 사이 구분선을 쓰지 않으므로 그릴 자리가 없고, 특히
// 마지막 행의 아래 모서리는 곧 테이블의 바깥 테두리라 거기에 선을 그리면
// 두 겹이 된다.
//
// 예전에는 마지막 행에 bottom=1을 줬다. 둘 다 기본색일 때는 겹쳐도 안
// 보여서 Phase 2a의 육안 확인을 통과했지만, annotate가 진단이 붙은 테이블의
// 테두리를 빨간 3px로 바꾸자 행이 그린 기본색 1px 선이 그 위에 남아
// «흰색과 빨간색이 겹친» 아래 모서리로 드러났다(2026-08-31, 소유자가
// draw.io에서 발견). 실물 draw.io 파일도 마지막 행에는 bottom=0을 준다.
//
// 원장이 적어 둔 «마지막 키 행(PK/비PK 경계)에 구분선을 준다»는 실물
// draw.io의 관례는 여전히 구현돼 있지 않다 — 그것은 이 결함과 별개의
// 개선이고 이월 목록에 남아 있다.
func styleRow() string {
	return "shape=tableRow;horizontal=0;startSize=0;swimlaneHead=0;swimlaneBody=0;fillColor=none;collapsible=0;dropTarget=0;points=[[0,0.5],[1,0.5]];portConstraint=eastwest;top=0;left=0;right=0;bottom=0;"
}

type xmlGeometry struct {
	X        int    `xml:"x,attr,omitempty"`
	Y        int    `xml:"y,attr,omitempty"`
	Width    int    `xml:"width,attr,omitempty"`
	Height   int    `xml:"height,attr,omitempty"`
	Relative string `xml:"relative,attr,omitempty"`
	As       string `xml:"as,attr"`
}

type xmlCell struct {
	XMLName  xml.Name     `xml:"mxCell"`
	ID       string       `xml:"id,attr"`
	Value    string       `xml:"value,attr,omitempty"`
	Style    string       `xml:"style,attr,omitempty"`
	Parent   string       `xml:"parent,attr,omitempty"`
	Vertex   string       `xml:"vertex,attr,omitempty"`
	Edge     string       `xml:"edge,attr,omitempty"`
	Source   string       `xml:"source,attr,omitempty"`
	Target   string       `xml:"target,attr,omitempty"`
	Geometry *xmlGeometry `xml:"mxGeometry"`
}

type xmlRoot struct {
	Cells []xmlCell `xml:"mxCell"`
}

type xmlGraphModel struct {
	XMLName    xml.Name `xml:"mxGraphModel"`
	Dx         int      `xml:"dx,attr"`
	Dy         int      `xml:"dy,attr"`
	Grid       int      `xml:"grid,attr"`
	GridSize   int      `xml:"gridSize,attr"`
	Guides     int      `xml:"guides,attr"`
	Tooltips   int      `xml:"tooltips,attr"`
	Connect    int      `xml:"connect,attr"`
	Arrows     int      `xml:"arrows,attr"`
	Fold       int      `xml:"fold,attr"`
	Page       int      `xml:"page,attr"`
	PageScale  int      `xml:"pageScale,attr"`
	PageWidth  int      `xml:"pageWidth,attr"`
	PageHeight int      `xml:"pageHeight,attr"`
	Math       int      `xml:"math,attr"`
	Shadow     int      `xml:"shadow,attr"`
	Root       xmlRoot  `xml:"root"`
}

func newGraphModel(cells []xmlCell) xmlGraphModel {
	return xmlGraphModel{
		Dx: 800, Dy: 600, Grid: 1, GridSize: 10, Guides: 1, Tooltips: 1,
		Connect: 1, Arrows: 1, Fold: 1, Page: 1, PageScale: 1,
		PageWidth: 850, PageHeight: 1100, Math: 0, Shadow: 0,
		Root: xmlRoot{Cells: cells},
	}
}

type xmlDiagram struct {
	XMLName xml.Name `xml:"diagram"`
	Name    string   `xml:"name,attr"`
	ID      string   `xml:"id,attr"`
	Model   xmlGraphModel
}

type xmlMxFile struct {
	XMLName  xml.Name     `xml:"mxfile"`
	Diagrams []xmlDiagram `xml:"diagram"`
}

// rowCellID는 페이지 인덱스·테이블 인덱스·컬럼 인덱스로 행 셀 id를
// 만든다. 관계 엣지가 (테이블명, 컬럼명)으로 같은 id를 다시 찾아야 하므로
// 이 규칙은 emit 안에서 한 곳에만 있어야 한다.
//
// pageIdx를 접두에 넣는 이유: internal/convert.File은 파일 전체에 한 벌의
// id 맵을 적용하며 페이지 사이에 같은 id가 겹치면 하드 에러를 낸다(한쪽
// 편집이 다른 쪽에 조용히 적용되는 것을 막기 위해서다). 접두 없이는 모든
// 페이지가 t0/t0_r0 같은 id를 재사용해 build가 만든 다중 페이지 파일을
// convert가 통째로 거부했다.
func rowCellID(pageIdx, tableIdx, colIdx int) string {
	return fmt.Sprintf("p%d_t%d_r%d", pageIdx, tableIdx, colIdx)
}

func tableCellID(pageIdx, tableIdx int) string {
	return fmt.Sprintf("p%d_t%d", pageIdx, tableIdx)
}

func columnValue(c ColumnDef) string {
	v := c.Name
	if c.Type != "" {
		v += " " + c.Type
	}
	var flags []string
	if !c.Nullable {
		flags = append(flags, "NOT NULL")
	}
	if c.Unique {
		flags = append(flags, "UNIQUE")
	}
	if len(flags) > 0 {
		v += " " + strings.Join(flags, " ")
	}
	return v
}

// emitTableCells는 테이블 하나를 table/tableRow/partialRectangle×2 셀
// 목록으로 만든다. tableIdx는 이 페이지 안에서 이 테이블의 순서(0부터) —
// rowCellID의 유일성을 페이지 단위로 보장하는 데 쓴다.
func emitTableCells(t PlacedTable) []xmlCell {
	return emitTableCellsIndexed(t, 0, 0)
}

func styleEdge(sourceCardinality, targetCardinality string) string {
	return fmt.Sprintf("edgeStyle=entityRelationEdgeStyle;endArrow=%s;startArrow=%s;", targetCardinality, sourceCardinality)
}

// emitPage는 PageLayout 하나를 xmlDiagram으로 만든다. pageIdx는 이
// 파일(mxfile) 안에서 이 페이지의 순서(0부터) — 모든 생성 id(t.../e...)에
// 접두로 붙여 페이지 사이 id 충돌을 막는다(rowCellID 주석 참고).
//
// id="0"/id="1"(페이지 루트/기본 레이어)은 예외다: draw.io는 각 <diagram>이
// 자신의 mxGraphModel 안에 독립된 "0"/"1" 루트 쌍을 갖기를 기대하고
// (internal/drawio/testdata의 실제 다중 페이지 픽스처도 페이지마다 이
// 둘을 그대로 재사용한다), internal/convert의 id 충돌 검사는 실제로
// 값을 바꿔 쓰는 셀(테이블/컬럼/관계)만 편집 맵에 올리므로 이 둘은 애초에
// 그 검사에 걸리지 않는다. 그래서 이 둘만 접두 없이 그대로 둔다.
func emitPage(pageIdx int, pl PageLayout) (xmlDiagram, error) {
	var cells []xmlCell
	cells = append(cells, xmlCell{ID: "0"}, xmlCell{ID: "1", Parent: "0"})

	rowIDOf := map[string]map[string]string{}
	for ti, t := range pl.Tables {
		cells = append(cells, emitTableCellsIndexed(t, pageIdx, ti)...)
		rowIDOf[t.Name] = map[string]string{}
		for ci, col := range t.Columns {
			rowIDOf[t.Name][col.Name] = rowCellID(pageIdx, ti, ci)
		}
	}

	for ri, r := range pl.Relations {
		srcRow, ok := rowIDOf[r.SourceTable][r.SourceColumn]
		if !ok {
			return xmlDiagram{}, fmt.Errorf("페이지 %q: 관계 %d: 원천 %q.%q의 행을 찾을 수 없다(내부 불변식 위반)", pl.Name, ri, r.SourceTable, r.SourceColumn)
		}
		tgtRow, ok := rowIDOf[r.TargetTable][r.TargetColumn]
		if !ok {
			return xmlDiagram{}, fmt.Errorf("페이지 %q: 관계 %d: 목표 %q.%q의 행을 찾을 수 없다(내부 불변식 위반)", pl.Name, ri, r.TargetTable, r.TargetColumn)
		}
		cells = append(cells, xmlCell{
			ID: fmt.Sprintf("p%d_e%d", pageIdx, ri), Value: r.RelationName,
			Style:  styleEdge(r.SourceCardinality, r.TargetCardinality),
			Parent: "1", Edge: "1", Source: srcRow, Target: tgtRow,
			Geometry: &xmlGeometry{Relative: "1", As: "geometry"},
		})
	}

	return xmlDiagram{
		Name: pl.Name,
		// 이름이 아니라 인덱스로 짓는다: 두 페이지가 같은 이름을 쓰면(사람이
		// 탭 이름을 복사해 만들었을 때 흔하다) "page-"+이름도 겹친다.
		ID:    fmt.Sprintf("page-%d", pageIdx),
		Model: newGraphModel(cells),
	}, nil
}

// Build는 파싱된 페이지들(genbuild.ParseXLSX/ParseCSV의 결과)로 .drawio
// 파일 바이트를 만든다. 압축하지 않는다(사용자 결정 — diff·검사가 쉽고
// draw.io가 평문도 정상적으로 연다).
func Build(pages []PageDef) ([]byte, error) {
	diagrams := make([]xmlDiagram, 0, len(pages))
	for pi, p := range pages {
		pl := Layout(p)
		d, err := emitPage(pi, pl)
		if err != nil {
			return nil, err
		}
		diagrams = append(diagrams, d)
	}
	out, err := xml.Marshal(&xmlMxFile{Diagrams: diagrams})
	if err != nil {
		return nil, fmt.Errorf("drawio xml 조립 실패: %w", err)
	}
	return out, nil
}

func emitTableCellsIndexed(t PlacedTable, pageIdx, tableIdx int) []xmlCell {
	tableID := tableCellID(pageIdx, tableIdx)
	cells := []xmlCell{{
		ID: tableID, Value: t.Name, Style: styleTable, Parent: "1", Vertex: "1",
		Geometry: &xmlGeometry{X: t.X, Y: t.Y, Width: t.Width, Height: t.Height, As: "geometry"},
	}}
	for ci, col := range t.Columns {
		rID := rowCellID(pageIdx, tableIdx, ci)
		cells = append(cells,
			xmlCell{
				ID: rID, Style: styleRow(), Parent: tableID, Vertex: "1",
				Geometry: &xmlGeometry{Y: headerHeight + rowHeight*ci, Width: t.Width, Height: rowHeight, As: "geometry"},
			},
			xmlCell{
				ID: rID + "k", Value: col.Key, Style: styleKeyCell, Parent: rID, Vertex: "1",
				Geometry: &xmlGeometry{Width: keyColWidth, Height: rowHeight, As: "geometry"},
			},
			xmlCell{
				ID: rID + "d", Value: columnValue(col), Style: styleDefCell, Parent: rID, Vertex: "1",
				Geometry: &xmlGeometry{X: keyColWidth, Width: t.Width - keyColWidth, Height: rowHeight, As: "geometry"},
			},
		)
	}
	return cells
}
