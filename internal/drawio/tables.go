package drawio

import (
	"strconv"
	"strings"

	"erdtool/internal/model"
)

// IsTable은 셀이 실제 테이블 컨테이너인지 판정한다. shape==table 뿐 아니라
// childLayout==tableLayout도 함께 확인해서 shape=tableRow와의 오탐(부분
// 문자열 매칭 버그의 근본 원인이었던 케이스)을 막는다.
func IsTable(cell RawCell) bool {
	style := ParseStyle(cell.Style)
	return style["shape"] == "table" && style["childLayout"] == "tableLayout"
}

func FindTables(cells []RawCell) []RawCell {
	var tables []RawCell
	for _, c := range cells {
		if IsTable(c) {
			tables = append(tables, c)
		}
	}
	return tables
}

// RowBoxes는 행 셀에서 키 셀과 정의 셀을 골라낸다.
//
// 행의 자식을 위치로만 집으면(children[0]/children[1]) 행 안에 장식용 도형이
// 하나라도 끼어 있는 순간 엉뚱한 셀을 키/정의 셀로 읽는다.
// shape=partialRectangle인 자식만 골라낸 뒤 앞의 둘을 쓴다.
//
// ExtractColumns(읽기)와 internal/convert(쓰기)가 이 함수를 공유한다.
// 정의 셀을 고르는 규칙이 두 벌 생기면 두 경로의 결과가 조용히 갈린다 —
// Column.ID는 정의 셀이 아니라 행 셀의 id이므로,
// 쓰기 쪽은 ExtractColumns의 반환값만으로는 정의 셀을 찾을 수 없다.
func RowBoxes(row RawCell, idx CellIndex) (key, def RawCell, ok bool) {
	var boxes []RawCell
	for _, child := range idx.ChildrenOf[row.ID] {
		if ParseStyle(child.Style)["shape"] == "partialRectangle" {
			boxes = append(boxes, child)
		}
	}
	if len(boxes) < 2 {
		return RawCell{}, RawCell{}, false
	}
	return boxes[0], boxes[1], true
}

// TableExtract는 테이블 셀 하나에서 읽어낸 것 전부다. 진단 둘을 따로 드는
// 이유는 검증 규칙이 다르기 때문이다 — 충돌은 «값을 못 골랐다»이고 모르는
// 이름은 «값이 조용히 사라졌다»라, 사용자가 할 일이 서로 다르다.
type TableExtract struct {
	Table     model.Table
	Conflicts []model.AttrIssue
	Unknown   []model.AttrIssue
}

// ExtractTable은 테이블 셀 하나를 model.Table로 읽는다. 컬럼 순회는
// table -> tableRow(직계 자식) -> partialRectangle×2(키 셀, 정의 셀)
// 3단계를 ID/parent 인덱스로 돈다. XML 문서 순서에는 의존하지 않는다.
func ExtractTable(tableCell RawCell, idx CellIndex) TableExtract {
	out := TableExtract{Table: model.Table{ID: tableCell.ID, Name: tableCell.Value}}

	tableAttrs := TableErdAttrs(tableCell)
	out.Table.Check = tableAttrs.Get(AttrCheck)
	out.Table.Comment = tableAttrs.Get(AttrComment)
	for _, c := range tableAttrs.Conflicts {
		out.Conflicts = append(out.Conflicts, model.AttrIssue{
			CellID: tableCell.ID, TableName: tableCell.Value, Name: c.Name, Values: c.Values})
	}
	for _, name := range tableAttrs.Unknown {
		out.Unknown = append(out.Unknown, model.AttrIssue{
			CellID: tableCell.ID, TableName: tableCell.Value, Name: name})
	}

	rows := idx.ChildrenOf[tableCell.ID]

	var columns []model.Column
	rowspanRemaining := 0
	var inheritedKey string

	for _, row := range rows {
		rowStyle := ParseStyle(row.Style)
		if rowStyle["shape"] != "tableRow" {
			continue
		}
		keyCell, defCell, ok := RowBoxes(row, idx)
		if !ok {
			continue
		}

		var key string
		if rowspanRemaining > 1 {
			key = inheritedKey
			rowspanRemaining--
		} else {
			key = strings.TrimSpace(keyCell.Value)
			if (model.Column{Key: key}).IsKey() {
				keyStyle := ParseStyle(keyCell.Style)
				if span, ok := keyStyle["rowspan"]; ok {
					// ParseStyle이 이미 ";" 구분자를 잘라내고 map에 넣으므로
					// span에는 세미콜론이 남아있지 않다 — TrimSuffix는 항상
					// no-op이었다(죽은 코드).
					if n, err := strconv.Atoi(span); err == nil && n > 1 {
						rowspanRemaining = n
						inheritedKey = key
					}
				}
			}
		}

		col := parseColumnValue(defCell.Value, key)
		// Column.ID는 정의 셀이 아니라 행(tableRow) 셀의 id다. 관계선은
		// 언제나 행에 연결되므로 resolveEnd가 돌려주는
		// Relationship.SourceColumnID/TargetColumnID도 행 id다 — 둘을 같은
		// id 공간에 두어야 IR 1.0에서 관계 -> 컬럼 조인이 성립한다.
		col.ID = row.ID

		// 행·키 셀·정의 셀을 모두 이 컬럼의 제약으로 받는다. 셋 다 같은
		// 컬럼을 가리키기 때문이다(작업 기록).
		attrs := ColumnErdAttrs(row, keyCell, defCell)
		col.Check = attrs.Get(AttrCheck)
		col.Default = attrs.Get(AttrDefault)
		col.Comment = attrs.Get(AttrComment)
		for _, c := range attrs.Conflicts {
			out.Conflicts = append(out.Conflicts, model.AttrIssue{
				CellID: row.ID, TableName: tableCell.Value, ColumnName: col.Name,
				Name: c.Name, Values: c.Values})
		}
		for _, name := range attrs.Unknown {
			out.Unknown = append(out.Unknown, model.AttrIssue{
				CellID: row.ID, TableName: tableCell.Value, ColumnName: col.Name, Name: name})
		}

		columns = append(columns, col)
	}
	out.Table.Columns = columns
	return out
}

// ExtractColumns는 컬럼만 필요한 호출자를 위한 얇은 껍데기다. 순회 규칙이
// 두 벌 생기지 않도록 ExtractTable을 그대로 탄다.
func ExtractColumns(tableCell RawCell, idx CellIndex) []model.Column {
	return ExtractTable(tableCell, idx).Table.Columns
}

// trailingFlags는 컬럼 셀 값의 «뒤쪽»에 붙는, 타입이 아닌 낱말들이다.
// genbuild/emit.go의 columnValue가 내는 것과 짝을 이룬다.
// "NOT NULL"은 두 낱말이므로 뒤에서부터 읽을 때 NULL -> NOT 순으로 만난다.
var trailingFlagWords = map[string]bool{
	"NULL":   true,
	"NOT":    true,
	"UNIQUE": true,
}

// parseColumnValue는 "컬럼명 타입 [NOT NULL] [UNIQUE]" 형식을 파싱한다.
//
// «오른쪽 기준»으로 읽는다: 뒤에서부터 아는 플래그 낱말을 떼어내고, 남은
// 것 중 첫 낱말을 이름, 가운데 전부를 타입으로 본다. 실제 DB가 내는 타입에는
// 공백이 흔하기 때문이다 — PostgreSQL의 `character varying(255)`,
// `timestamp with time zone`, MySQL의 `int unsigned`. 왼쪽부터 위치로
// 쪼개면 이것들이 `character`/`timestamp`/`int`로 조용히 잘리고, 틀린 타입이
// SQL DDL까지 흘러가는데 검증 리포트는 "문제 없음"이라고 말한다.
//
// 형식을 벗어나도(예: "UniqueID"처럼 이름만 있는 경우) 플레이스홀더로
// 대체하지 않고 원본 값을 RawValue에 그대로 보존한다 (원장:
// 이전 구현은 이 경우 "컬럼유형 없음" 같은 문자열을 경고 없이
// 채워넣는 결함이 있었음).
func parseColumnValue(raw string, key string) model.Column {
	raw = strings.TrimSpace(raw)
	fields := strings.Fields(raw)

	col := model.Column{Key: key, RawValue: raw, Nullable: true}

	// 뒤에서부터 아는 플래그 낱말을 떼어낸다. 이름 하나만 있는 값
	// (예: "UNIQUE"라는 이름의 컬럼)까지 먹어치우지 않도록 최소 1개는 남긴다.
	end := len(fields)
	for end > 1 && trailingFlagWords[strings.ToUpper(fields[end-1])] {
		end--
	}
	flags := strings.ToUpper(strings.Join(fields[end:], " "))
	if strings.Contains(flags, "NOT NULL") {
		col.Nullable = false
	}
	// UNIQUE는 예전에 떼어내고 **아무 데도 넣지 않았다.** 그래서 정의 셀에
	// 적힌 UNIQUE가 정의서에도 DDL에도 안 나왔다 — 사용자가 쓴 것이 조용히
	// 사라지는, 이 저장소가 가장 경계하는 실패다.
	if strings.Contains(flags, "UNIQUE") {
		col.Unique = true
	}

	switch {
	case end >= 2:
		col.Name = fields[0]
		col.Type = strings.Join(fields[1:end], " ")
	case end == 1:
		col.Name = fields[0]
		col.Type = ""
	default:
		col.Name = ""
		col.Type = ""
	}
	return col
}
