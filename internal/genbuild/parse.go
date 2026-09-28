package genbuild

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

var tableHeader = []string{"테이블명", "순번", "컬럼명", "컬럼유형", "색인여부", "널허용", "단일값"}

func checkHeader(row []string, want []string, blockName string) error {
	if len(row) < len(want) {
		return fmt.Errorf("%s 헤더 열 개수가 부족하다: got %d, want %d", blockName, len(row), len(want))
	}
	for i, w := range want {
		if strings.TrimSpace(row[i]) != w {
			return fmt.Errorf("%s 헤더 %d번째 열: got %q, want %q", blockName, i+1, row[i], w)
		}
	}
	return nil
}

func parseBool(raw, field string) (bool, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s 값이 True/False가 아니다: %q", field, raw)
	}
}

// tableRow는 정렬 전 원본 순서를 함께 들고 있어야 순번이 같을 때 시트 등장
// 순서로 동점을 깬다(sort.SliceStable).
type tableRow struct {
	seq int
	col ColumnDef
}

// parseTableBlock은 테이블 목록 블록(헤더 포함)을 TableDef 슬라이스로
// 바꾼다. 테이블은 시트에 처음 등장한 순서를 유지하고, 각 테이블 안의
// 컬럼은 순번 오름차순으로 정렬한다(원장: "순번 오름차순으로 컬럼 순서를
// 정한다 — 표에 등장하는 순서에 기대지 않는다").
func parseTableBlock(rows [][]string) ([]TableDef, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("테이블 목록 블록이 비어 있다")
	}
	if err := checkHeader(rows[0], tableHeader, "테이블 목록"); err != nil {
		return nil, err
	}

	order := []string{}
	byName := map[string][]tableRow{}
	seen := map[string]bool{} // "테이블명\x00컬럼명" 중복 검출

	for i, row := range rows[1:] {
		if allBlank(row) {
			continue
		}
		if len(row) < 7 {
			return nil, fmt.Errorf("테이블 목록 %d행: 열 개수가 부족하다(got %d, want 7)", i+2, len(row))
		}
		name := strings.TrimSpace(row[0])
		seq, err := strconv.Atoi(strings.TrimSpace(row[1]))
		if err != nil {
			return nil, fmt.Errorf("테이블 목록 %d행: 순번이 정수가 아니다: %q", i+2, row[1])
		}
		colName := strings.TrimSpace(row[2])
		colType := strings.TrimSpace(row[3])
		if err := rejectEmbeddedSpace(colName, fmt.Sprintf("테이블 목록 %d행(테이블 %q) 컬럼명", i+2, name)); err != nil {
			return nil, err
		}
		// 컬럼유형의 공백 금지는 걷었다(Phase 2b 작업 2). drawio.parseColumnValue가
		// 오른쪽 기준으로 읽으므로 `timestamp with time zone` 같은 실제 DB 타입이
		// 왕복에서 살아남는다. 컬럼명은 여전히 첫 낱말로 잘리므로 금지가 남는다.
		dupKey := name + "\x00" + colName
		if seen[dupKey] {
			return nil, fmt.Errorf("테이블 %q에 컬럼 %q가 중복된다", name, colName)
		}
		seen[dupKey] = true

		nullable, err := parseBool(row[5], fmt.Sprintf("테이블 목록 %d행 널허용", i+2))
		if err != nil {
			return nil, err
		}
		unique, err := parseBool(row[6], fmt.Sprintf("테이블 목록 %d행 단일값", i+2))
		if err != nil {
			return nil, err
		}

		if _, ok := byName[name]; !ok {
			order = append(order, name)
		}
		byName[name] = append(byName[name], tableRow{
			seq: seq,
			col: ColumnDef{
				Name:     colName,
				Type:     colType,
				Key:      strings.TrimSpace(row[4]),
				Nullable: nullable,
				Unique:   unique,
			},
		})
	}

	tables := make([]TableDef, 0, len(order))
	for _, name := range order {
		trs := byName[name]
		sort.SliceStable(trs, func(a, b int) bool { return trs[a].seq < trs[b].seq })
		cols := make([]ColumnDef, len(trs))
		for i, tr := range trs {
			cols[i] = tr.col
		}
		tables = append(tables, TableDef{Name: name, Columns: cols})
	}
	return tables, nil
}

// rejectEmbeddedSpace는 값 안에 공백(스페이스/탭 등)이 있으면 에러를
// 낸다. 이제 컬럼명에만 쓴다 — internal/genbuild/emit.go columnValue가 셀
// 값을 공백으로 이어 붙이고 internal/drawio/tables.go parseColumnValue가
// 그중 «첫 낱말»을 이름으로 읽으므로, 이름에 공백이 있으면 경계가 깨진다
// (예: 컬럼명="고객 번호" -> 되읽으면 이름="고객", 타입="번호"). 형식을
// 조용히 바꾸는 대신 여기서 시트 입력 단계에 크게 거부한다. 컬럼유형은
// 가운데 전부를 모아 읽으므로 공백이 있어도 깨지지 않는다.
func rejectEmbeddedSpace(value, field string) error {
	if len(strings.Fields(value)) > 1 {
		return fmt.Errorf("%s에 공백을 넣을 수 없다(셀 값의 첫 낱말이 곧 컬럼명이므로 되읽을 때 이름·타입이 잘못 갈린다): %q", field, value)
	}
	return nil
}

func allBlank(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

const relationMarker = "## 관계"

var relationHeader = []string{
	"순번", "원천테이블명", "원천컬럼명", "원천카디널리티", "원천색인",
	"연관명", "목표카디널리티", "목표색인", "목표테이블명", "목표컬럼명",
}

// splitBlocks는 relationMarker가 첫 열에 적힌 행을 기준으로 시트를
// 테이블 목록 블록과 관계 목록 블록으로 나눈다. 두 블록 모두 자신의
// 헤더 행을 포함해서 반환한다.
func splitBlocks(rows [][]string) (tableRows, relationRows [][]string, err error) {
	for i, row := range rows {
		if len(row) > 0 && strings.TrimSpace(row[0]) == relationMarker {
			return rows[:i], rows[i+1:], nil
		}
	}
	return nil, nil, fmt.Errorf("%q 마커 행이 없다", relationMarker)
}

// parseRelationBlock은 관계 목록 블록(헤더 포함)을 RelationDef 슬라이스로
// 바꾼다. 여기서는 카디널리티 코드 유효성과 컬럼 필수 여부만 검사한다 —
// 테이블/컬럼이 실제로 존재하는지는 validatePage(작업 4)가 다른 블록과
// 대조해서 검사한다.
func parseRelationBlock(rows [][]string) ([]RelationDef, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("관계 목록 블록이 비어 있다")
	}
	if err := checkHeader(rows[0], relationHeader, "관계 목록"); err != nil {
		return nil, err
	}

	var rels []RelationDef
	for i, row := range rows[1:] {
		if allBlank(row) {
			continue
		}
		if len(row) < 10 {
			return nil, fmt.Errorf("관계 목록 %d행: 열 개수가 부족하다(got %d, want 10)", i+2, len(row))
		}
		seq, err := strconv.Atoi(strings.TrimSpace(row[0]))
		if err != nil {
			return nil, fmt.Errorf("관계 목록 %d행: 순번이 정수가 아니다: %q", i+2, row[0])
		}
		r := RelationDef{
			Seq:               seq,
			SourceTable:       strings.TrimSpace(row[1]),
			SourceColumn:      strings.TrimSpace(row[2]),
			SourceCardinality: strings.TrimSpace(row[3]),
			SourceKey:         strings.TrimSpace(row[4]),
			RelationName:      strings.TrimSpace(row[5]),
			TargetCardinality: strings.TrimSpace(row[6]),
			TargetKey:         strings.TrimSpace(row[7]),
			TargetTable:       strings.TrimSpace(row[8]),
			TargetColumn:      strings.TrimSpace(row[9]),
		}
		if r.SourceColumn == "" || r.TargetColumn == "" {
			return nil, fmt.Errorf("관계 목록 %d행: 원천컬럼명/목표컬럼명은 비울 수 없다(컬럼 레벨 관계만 지원)", i+2)
		}
		if !ValidCardinality[r.SourceCardinality] {
			return nil, fmt.Errorf("관계 목록 %d행: 알 수 없는 원천카디널리티 %q", i+2, r.SourceCardinality)
		}
		if !ValidCardinality[r.TargetCardinality] {
			return nil, fmt.Errorf("관계 목록 %d행: 알 수 없는 목표카디널리티 %q", i+2, r.TargetCardinality)
		}
		rels = append(rels, r)
	}
	return rels, nil
}

// ParseSheet는 시트 한 장(엑셀 GetRows 결과 또는 csv.ReadAll 결과)을
// PageDef로 바꾼다. 테이블/관계 블록을 분리해 각각 파싱한 뒤, 관계가
// 가리키는 테이블/컬럼/색인 라벨이 테이블 블록과 실제로 맞는지
// 교차검증한다. 어느 단계든 실패하면 그 페이지 전체를 에러로 되돌린다.
func ParseSheet(pageName string, rows [][]string) (PageDef, error) {
	tableRows, relationRows, err := splitBlocks(rows)
	if err != nil {
		return PageDef{}, fmt.Errorf("시트 %q: %w", pageName, err)
	}
	tables, err := parseTableBlock(tableRows)
	if err != nil {
		return PageDef{}, fmt.Errorf("시트 %q: %w", pageName, err)
	}
	relations, err := parseRelationBlock(relationRows)
	if err != nil {
		return PageDef{}, fmt.Errorf("시트 %q: %w", pageName, err)
	}

	page := PageDef{Name: pageName, Tables: tables, Relations: relations}
	if err := validatePage(page); err != nil {
		return PageDef{}, fmt.Errorf("시트 %q: %w", pageName, err)
	}
	return page, nil
}

// validatePage는 관계 블록이 가리키는 테이블/컬럼/색인 라벨이 테이블
// 블록과 실제로 일치하는지 검사한다.
func validatePage(page PageDef) error {
	type colKey struct{ table, col string }
	colOf := map[colKey]ColumnDef{}
	for _, t := range page.Tables {
		for _, c := range t.Columns {
			colOf[colKey{t.Name, c.Name}] = c
		}
	}

	check := func(table, col, key, side string) error {
		found, ok := colOf[colKey{table, col}]
		if !ok {
			return fmt.Errorf("%s %q.%q가 테이블 목록에 없다", side, table, col)
		}
		if found.Key != key {
			return fmt.Errorf("%s %q.%q의 색인 라벨이 테이블 목록과 다르다: 관계=%q, 테이블=%q", side, table, col, key, found.Key)
		}
		return nil
	}

	for _, r := range page.Relations {
		if err := check(r.SourceTable, r.SourceColumn, r.SourceKey, "원천"); err != nil {
			return err
		}
		if err := check(r.TargetTable, r.TargetColumn, r.TargetKey, "목표"); err != nil {
			return err
		}
	}
	return nil
}
