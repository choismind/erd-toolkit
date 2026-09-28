package drawio

import (
	"erdtool/internal/model"
)

type resolvedEnd struct {
	tableID  string
	columnID string
	found    bool
	// exists는 raw id가 idx.ByID에 "어떤 셀로든" 존재하는지를 나타낸다.
	// found=false인데 exists=true인 경우는 진짜 dangling reference가
	// 아니라, 이 파서가 행으로 인식하지 못하는 도형 변형(예: 구버전
	// 2단 테이블의 partialRectangle 직속 행)을 가리킨 것이다 — 셀 자체는
	// 실존한다. 진짜 dangling은 found=false && exists=false뿐이다.
	exists bool
}

// resolveEnd는 관계선의 source/target id가 가리키는 대상을 찾는다.
//  1. 그 id가 어떤 테이블의 tableRow(행)라면 -> 그 행이 속한 테이블 +
//     해당 행에 대응하는 컬럼(정의 셀)을 반환한다.
//  2. 그 id가 테이블 자체라면 -> 컬럼 없이 테이블만 반환한다(스펙:
//     "관계선이 행이 아니라 테이블에 직접 연결된 경우").
//  3. 어느 쪽도 아니면 found=false. 이때 id가 idx.ByID에 실존하는 셀을
//     가리키고 있었다면(그저 tableRow로 인식되지 않았거나 parent 테이블을
//     못 찾은 것뿐이라면) exists=true로 남겨 "진짜 없는 id"와 구분한다.
func resolveEnd(id string, idx CellIndex, tables []RawCell) resolvedEnd {
	if id == "" {
		return resolvedEnd{}
	}
	for _, t := range tables {
		if t.ID == id {
			return resolvedEnd{tableID: t.ID, found: true, exists: true}
		}
	}
	cell, ok := idx.ByID[id]
	if !ok {
		return resolvedEnd{}
	}
	style := ParseStyle(cell.Style)
	if style["shape"] != "tableRow" {
		return resolvedEnd{exists: true}
	}
	// 이 행이 속한 테이블을 parent로 거슬러 올라가 찾는다.
	parentID := cell.Parent
	for _, t := range tables {
		if t.ID == parentID {
			return resolvedEnd{tableID: t.ID, columnID: id, found: true, exists: true}
		}
	}
	return resolvedEnd{exists: true}
}

// ExtractRelationships는 edgeStyle=entityRelationEdgeStyle 엣지를 찾아
// source/target을 해석한다. 매칭 실패 시 이전 관계의 값을 재사용하거나
// 초기화되지 않은 값을 쓰지 않는다 — 매 관계마다 완전히 새 값으로 계산한다
// (원본 gen_tablespec.py의 UnboundLocalError/조용한 오염 버그의 원인).
//
// source/target 속성이 둘 다 아예 없는 엣지(예: draw.io ER 도형 라이브러리
// 참고 페이지의 장식용 화살표 — 아무 것에도 연결된 적이 없다)만 완전히
// 건너뛴다. 반면 속성은 있는데(연결을 "시도"했는데) 이 다이어그램의
// 테이블/행으로 해석되지 않는 경우 — 한쪽만이든 양쪽 다든 — 는 조용히
// 버리지 않고 SourceResolved/TargetResolved=false로 기록해 남긴다. 검증
// 단계(broken_reference)가 바로 이 플래그로 끊어진 관계를 잡는다.
func ExtractRelationships(cells []RawCell, idx CellIndex, tables []RawCell) []model.Relationship {
	var rels []model.Relationship
	for _, cell := range cells {
		style := ParseStyle(cell.Style)
		if style["edgeStyle"] != "entityRelationEdgeStyle" {
			continue
		}

		if cell.Source == "" && cell.Target == "" {
			continue // 애초에 아무 것에도 연결을 시도하지 않은 장식용 엣지
		}

		src := resolveEnd(cell.Source, idx, tables)
		tgt := resolveEnd(cell.Target, idx, tables)

		rels = append(rels, model.Relationship{
			ID:                cell.ID,
			Label:             cell.Value,
			SourceTableID:     src.tableID,
			SourceColumnID:    src.columnID,
			SourceCardinality: style["startArrow"],
			SourceRawID:       cell.Source,
			SourceResolved:    src.found,
			SourceExists:      src.exists,
			TargetTableID:     tgt.tableID,
			TargetColumnID:    tgt.columnID,
			TargetCardinality: style["endArrow"],
			TargetRawID:       cell.Target,
			TargetResolved:    tgt.found,
			TargetExists:      tgt.exists,
			ColumnLevel:       src.columnID != "" || tgt.columnID != "",
		})
	}
	return rels
}

// floatingTouchTolerance는 관계선의 끝점이 테이블에 «닿았다»고 볼 여유(px)다.
// draw.io의 기본 격자가 10이라, 테이블 위에 놓으려다 붙지 않은 끝점은 이 안에
// 들어온다. 이 값을 키우면 멀리 놓인 장식용 화살표까지 진단으로 올라온다.
const floatingTouchTolerance = 10.0

// touchesBox는 점 (x,y)가 사각형 g의 안이나 그 언저리에 있는지 본다.
func touchesBox(x, y float64, g *RawGeometry) bool {
	if g == nil {
		return false
	}
	return x >= g.X-floatingTouchTolerance && x <= g.X+g.Width+floatingTouchTolerance &&
		y >= g.Y-floatingTouchTolerance && y <= g.Y+g.Height+floatingTouchTolerance
}

// looseEnds는 관계선의 두 끝 중 어느 도형에도 붙지 않아 좌표로만 남은 것을
// 돌려준다.
func looseEnds(g *RawGeometry) []RawPoint {
	if g == nil {
		return nil
	}
	var out []RawPoint
	for _, p := range g.Points {
		if p.As == "sourcePoint" || p.As == "targetPoint" {
			out = append(out, p)
		}
	}
	return out
}

// ExtractFloatingRelations는 source/target이 둘 다 없는데 **그림에서는
// 테이블에 닿아 있는** ER 관계선을 찾는다.
//
// ExtractRelationships는 그런 선을 건너뛴다. 아무 것에도 연결을 시도하지
// 않았으므로 관계가 아니기 때문이다. 그런데 draw.io에서는 선 끝을 테이블
// 위에 놓고도 실제 연결이 안 잡히는 일이 흔하다 — 그때 파일에 남는 것은
// source/target이 아니라 sourcePoint/targetPoint 좌표뿐이다. 그림으로는
// 두 테이블이 이어져 보이는데 파일이 말하는 연결은 없는 상태이고, 그대로
// 두면 관계 하나가 정의서에서 **아무 말 없이** 사라진다.
//
// 장식용 화살표와 가르는 것은 좌표다. draw.io의 ER 도형 라이브러리를 붙여
// 둔 참고용 화살표는 테이블에서 멀리 떨어져 있다(실측: 픽스처
// table_level_relations.drawio의 테이블은 y=40~190, 화살표 열여섯은
// y=690~980). 어느 테이블에도 닿지 않는 선은 여기서도 건너뛴다.
//
// tables는 이 페이지의 테이블 도형이고, 좌표 기준계가 같은 것끼리만 견준다
// (부모가 다르면 x·y가 서로 다른 원점을 가리킨다).
func ExtractFloatingRelations(cells []RawCell, tables []RawCell) []model.FloatingRelation {
	var out []model.FloatingRelation
	for _, cell := range cells {
		style := ParseStyle(cell.Style)
		if style["edgeStyle"] != "entityRelationEdgeStyle" {
			continue
		}
		if cell.Source != "" || cell.Target != "" {
			continue // 한쪽이라도 이었다면 ExtractRelationships가 맡는다
		}

		var near []string
		for _, p := range looseEnds(cell.Geometry) {
			for _, t := range tables {
				if t.Parent != cell.Parent {
					continue
				}
				if touchesBox(p.X, p.Y, t.Geometry) && !containsString(near, t.ID) {
					near = append(near, t.ID)
				}
			}
		}
		if len(near) == 0 {
			continue // 어느 테이블에도 안 닿는다 — 참고용 화살표다
		}

		out = append(out, model.FloatingRelation{
			ID:           cell.ID,
			Label:        cell.Value,
			NearTableIDs: near,
		})
	}
	return out
}
