package drawio

import "testing"

func TestIsTable_ExcludesTableRow(t *testing.T) {
	table := RawCell{ID: "t1", Value: "Orders",
		Style: "shape=table;startSize=30;container=1;childLayout=tableLayout;"}
	row := RawCell{ID: "r1", Value: "",
		Style: "shape=tableRow;horizontal=0;startSize=0;"}

	if !IsTable(table) {
		t.Fatalf("expected table cell to be recognized as table")
	}
	// 회귀 테스트: 4개 기존 구현체를 전부 깨뜨린 정확한 재현.
	if IsTable(row) {
		t.Fatalf("tableRow must NOT be misidentified as table")
	}
}

func TestFindTables_RealFixture(t *testing.T) {
	diagrams, err := LoadDiagrams("testdata/entity_table_basic.drawio")
	if err != nil {
		t.Fatalf("LoadDiagrams failed: %v", err)
	}
	total := 0
	for _, d := range diagrams {
		total += len(FindTables(d.Cells))
	}
	if total != entityTableBasicTableCount {
		t.Fatalf("expected %d tables, got %d", entityTableBasicTableCount, total)
	}
}

func TestExtractColumns_Basic(t *testing.T) {
	diagrams, err := LoadDiagrams("testdata/rowspan_example.xml")
	if err != nil {
		t.Fatalf("LoadDiagrams failed: %v", err)
	}
	d := diagrams[0]
	idx := BuildIndex(d.Cells)
	tables := FindTables(d.Cells)
	if len(tables) != rowspanExampleTableCount {
		t.Fatalf("expected %d table, got %d", rowspanExampleTableCount, len(tables))
	}
	cols := ExtractColumns(tables[0], idx)
	if len(cols) != rowspanExampleColumnCount {
		t.Fatalf("expected %d columns, got %d", rowspanExampleColumnCount, len(cols))
	}
}

func TestExtractColumns_RowspanMergedKey(t *testing.T) {
	// example of rowspan.xml은 이름 그대로 세로 병합된 복합키 케이스를
	// 담고 있다(부록 참고). 병합된 행은 키 셀 value가 비어 있어도 직전
	// 행의 키를 이어받아야 한다.
	diagrams, _ := LoadDiagrams("testdata/rowspan_example.xml")
	idx := BuildIndex(diagrams[0].Cells)
	tables := FindTables(diagrams[0].Cells)
	cols := ExtractColumns(tables[0], idx)

	emptyKeyAfterCompositeStart := false
	for i := 1; i < len(cols); i++ {
		if cols[i].Key != "" && cols[i].Key == cols[i-1].Key {
			emptyKeyAfterCompositeStart = true
		}
	}
	if !emptyKeyAfterCompositeStart {
		t.Fatalf("expected at least one rowspan-inherited key value across consecutive rows")
	}
}

func TestExtractColumns_NonStandardValuePreservesRaw(t *testing.T) {
	// "UniqueID"처럼 타입 없이 이름만 있는 값 -> 플레이스홀더로 대체하지
	// 않고 RawValue를 보존해야 한다 (부록 B, gen_tablespec.py의 결함).
	//
	// 픽스처는 irregular_rows.drawio다. 2026-09-21까지 이 테스트는
	// entity_table_basic.drawio를 읽고 있었는데 그 파일에는 "UniqueID"가
	// 없어 t.Skip으로 빠졌다 — 처음부터 한 번도 안 돈 테스트였다.
	// 대상을 못 찾으면 건너뛰지 않고 실패한다. 건너뛰면 픽스처가 바뀌어
	// 대상이 사라져도 초록이 나온다.
	diagrams, err := LoadDiagrams("testdata/irregular_rows.drawio")
	if err != nil {
		t.Fatalf("픽스처 읽기: %v", err)
	}
	found := false
	for _, d := range diagrams {
		idx := BuildIndex(d.Cells)
		for _, table := range FindTables(d.Cells) {
			for _, col := range ExtractColumns(table, idx) {
				if col.RawValue == "UniqueID" {
					found = true
					if col.Name != "UniqueID" {
						t.Fatalf("expected raw value preserved as Name, got %q", col.Name)
					}
				}
			}
		}
	}
	if !found {
		t.Fatalf("픽스처에 타입 없는 컬럼 \"UniqueID\"가 없다 — 픽스처를 확인해라")
	}
}

func TestExtractColumns_ObjectWrappedTable(t *testing.T) {
	// C1 회귀: <object>/<UserObject>로 감싼 테이블도 일반 테이블과 동일하게
	// 행(tableRow)->컬럼 추출까지 끝까지 동작해야 한다. FindTables만 통과하고
	// 자식 인덱싱(부모 id 불일치 등)이 깨지면 컬럼 0개로 조용히 비게 된다.
	diagrams, err := LoadDiagrams("testdata/object_wrapped_table.drawio")
	if err != nil {
		t.Fatalf("LoadDiagrams failed: %v", err)
	}
	d := diagrams[0]
	idx := BuildIndex(d.Cells)
	tables := FindTables(d.Cells)
	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}
	for _, tbl := range tables {
		cols := ExtractColumns(tbl, idx)
		if len(cols) != 1 {
			t.Fatalf("expected 1 column for table %q (id=%s), got %d", tbl.Value, tbl.ID, len(cols))
		}
	}
}

func TestExtractColumns_IrregularRowsDoNotPanic(t *testing.T) {
	// irregular_rows.drawio ("ERD_변형도형")는 원본 gen_tablespec.py를
	// IndexError로 크래시시켰던 파일이다(부록 B). ExtractColumns는 이런
	// 파일에서도 패닉 없이 실행되어야 한다.
	diagrams, err := LoadDiagrams("testdata/irregular_rows.drawio")
	if err != nil {
		t.Fatalf("LoadDiagrams failed: %v", err)
	}
	for _, d := range diagrams {
		idx := BuildIndex(d.Cells)
		for _, table := range FindTables(d.Cells) {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("ExtractColumns panicked on irregular structure: %v", r)
					}
				}()
				_ = ExtractColumns(table, idx)
			}()
		}
	}
	if len(FindTables(diagrams[0].Cells)) != irregularRowsTableCount {
		t.Fatalf("expected %d tables in irregular_rows fixture", irregularRowsTableCount)
	}
}

func TestExtractColumns_PopulatesColumnID(t *testing.T) {
	// I7: model.Column.ID가 한 번도 채워지지 않아 JSON IR에 항상 "" 로
	// 나갔다. Relationship.SourceColumnID/TargetColumnID는 resolveEnd가
	// 반환하는 tableRow 셀의 id이므로, Column.ID도 같은 tableRow id여야
	// 두 값이 서로 조인 가능한 참조가 된다(IR 1.0 계약).
	diagrams, err := LoadDiagrams("testdata/rowspan_example.xml")
	if err != nil {
		t.Fatalf("LoadDiagrams failed: %v", err)
	}
	idx := BuildIndex(diagrams[0].Cells)
	tables := FindTables(diagrams[0].Cells)
	cols := ExtractColumns(tables[0], idx)

	rowIDs := map[string]bool{}
	for _, row := range idx.ChildrenOf[tables[0].ID] {
		if ParseStyle(row.Style)["shape"] == "tableRow" {
			rowIDs[row.ID] = true
		}
	}
	for i, c := range cols {
		if c.ID == "" {
			t.Fatalf("column %d (%q) has empty ID", i, c.Name)
		}
		if !rowIDs[c.ID] {
			t.Fatalf("column %d ID %q is not a tableRow id of this table", i, c.ID)
		}
	}
}

func TestRelationshipColumnIDJoinsToColumnID(t *testing.T) {
	// I7 계약 검증: 컬럼 단위 관계선의 SourceColumnID/TargetColumnID는
	// 같은 테이블의 어떤 Column.ID와 반드시 일치해야 한다.
	diagrams, err := LoadDiagrams("testdata/relationship_physical.drawio")
	if err != nil {
		t.Fatalf("LoadDiagrams failed: %v", err)
	}
	checked := 0
	for _, d := range diagrams {
		idx := BuildIndex(d.Cells)
		tableCells := FindTables(d.Cells)
		colIDsByTable := map[string]map[string]bool{}
		for _, tc := range tableCells {
			ids := map[string]bool{}
			for _, c := range ExtractColumns(tc, idx) {
				ids[c.ID] = true
			}
			colIDsByTable[tc.ID] = ids
		}
		for _, r := range ExtractRelationships(d.Cells, idx, tableCells) {
			if r.SourceColumnID != "" {
				checked++
				if !colIDsByTable[r.SourceTableID][r.SourceColumnID] {
					t.Fatalf("relationship %s source column %q not found among columns of table %s",
						r.ID, r.SourceColumnID, r.SourceTableID)
				}
			}
			if r.TargetColumnID != "" {
				checked++
				if !colIDsByTable[r.TargetTableID][r.TargetColumnID] {
					t.Fatalf("relationship %s target column %q not found among columns of table %s",
						r.ID, r.TargetColumnID, r.TargetTableID)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatalf("fixture has no column-level relationships; cannot verify the join contract")
	}
}

func TestExtractColumns_IgnoresNonPartialRectangleRowChildren(t *testing.T) {
	// M5: 행의 자식 2개를 children[0]/children[1]로 위치만 보고 키 셀/정의
	// 셀이라 가정하면, 행 안에 다른 도형(주석 텍스트, 아이콘 등)이 하나라도
	// 끼어 있는 순간 엉뚱한 셀을 컬럼 값으로 읽는다. 자식은 반드시
	// shape=partialRectangle인 것만 골라야 한다.
	const pr = "shape=partialRectangle;connectable=0;"
	cells := []RawCell{
		{ID: "t1", Value: "Orders", Style: "shape=table;childLayout=tableLayout;", Parent: "1"},
		{ID: "r1", Style: "shape=tableRow;", Parent: "t1"},
		// 행의 첫 자식이 partialRectangle이 아닌 장식용 도형이다.
		{ID: "note1", Value: "메모", Style: "text;html=1;", Parent: "r1"},
		{ID: "k1", Value: "PK", Style: pr, Parent: "r1"},
		{ID: "d1", Value: "order_id int NOT NULL", Style: pr, Parent: "r1"},
	}
	idx := BuildIndex(cells)
	cols := ExtractColumns(idx.ByID["t1"], idx)

	if len(cols) != 1 {
		t.Fatalf("expected 1 column, got %d: %+v", len(cols), cols)
	}
	if cols[0].Key != "PK" {
		t.Errorf("expected Key=PK from the partialRectangle key cell, got %q", cols[0].Key)
	}
	if cols[0].Name != "order_id" {
		t.Errorf("expected Name=order_id from the partialRectangle definition cell, got %q (raw %q)",
			cols[0].Name, cols[0].RawValue)
	}
}

func TestExtractColumns_SkipsRowWithoutTwoPartialRectangles(t *testing.T) {
	// partialRectangle 자식이 2개 미만인 행은 컬럼으로 해석할 수 없다.
	const pr = "shape=partialRectangle;"
	cells := []RawCell{
		{ID: "t1", Value: "Orders", Style: "shape=table;childLayout=tableLayout;", Parent: "1"},
		{ID: "r1", Style: "shape=tableRow;", Parent: "t1"},
		{ID: "k1", Value: "PK", Style: pr, Parent: "r1"},
		{ID: "x1", Value: "무언가", Style: "text;html=1;", Parent: "r1"},
	}
	idx := BuildIndex(cells)
	if cols := ExtractColumns(idx.ByID["t1"], idx); len(cols) != 0 {
		t.Fatalf("expected 0 columns for a row with only one partialRectangle, got %d: %+v", len(cols), cols)
	}
}

func TestRowBoxes_SkipsDecorationShapes(t *testing.T) {
	row := RawCell{ID: "r1", Style: "shape=tableRow;"}
	idx := BuildIndex([]RawCell{
		row,
		{ID: "deco", Parent: "r1", Style: "ellipse;whiteSpace=wrap;"},
		{ID: "k", Parent: "r1", Style: "shape=partialRectangle;", Value: "PK"},
		{ID: "d", Parent: "r1", Style: "shape=partialRectangle;", Value: "고객번호 int"},
	})

	key, def, ok := RowBoxes(row, idx)
	if !ok {
		t.Fatal("장식 도형이 끼어 있어도 키/정의 셀을 찾아야 한다")
	}
	if key.ID != "k" {
		t.Fatalf("키 셀: got %q, want %q", key.ID, "k")
	}
	if def.ID != "d" {
		t.Fatalf("정의 셀: got %q, want %q", def.ID, "d")
	}
}

func TestRowBoxes_FewerThanTwoBoxes(t *testing.T) {
	row := RawCell{ID: "r1", Style: "shape=tableRow;"}
	idx := BuildIndex([]RawCell{
		row,
		{ID: "k", Parent: "r1", Style: "shape=partialRectangle;", Value: "PK"},
	})

	if _, _, ok := RowBoxes(row, idx); ok {
		t.Fatal("partialRectangle 자식이 둘 미만이면 ok=false여야 한다")
	}
}

// TestParseColumnValue_GoldenFromCommittedFixtures는 커밋된 픽스처에 실제로
// 등장하는 컬럼 셀 값 전수(2026-08-27 실측)를 고정한다. 오른쪽 기준 파싱으로
// 바꿔도 이 표가 한 줄도 달라지면 안 된다 — 손으로 그린 .drawio를 읽는
// 계약이 깨진다는 뜻이기 때문이다.
func TestParseColumnValue_GoldenFromCommittedFixtures(t *testing.T) {
	cases := []struct {
		raw      string
		wantName string
		wantType string
		wantNull bool
	}{
		{"일련번호", "일련번호", "", true},
		{"컬럼-1", "컬럼-1", "", true},
		{"컬럼-2", "컬럼-2", "", true},
		{"컬럼-3", "컬럼-3", "", true},
		{"UniqueID", "UniqueID", "", true},
		{"id int", "id", "int", true},
		{"code varchar", "code", "varchar", true},
		{"Row 1", "Row", "1", true},
		{"Row 2", "Row", "2", true},
		{"Row 3", "Row", "3", true},
		{"Row 4", "Row", "4", true},
		{"고객번호 int NOT NULL", "고객번호", "int", false},
		{"고객명 char(50) NOT NULL", "고객명", "char(50)", false},
		{"주문번호 int NOT NULL", "주문번호", "int", false},
		{"주문고객번호 int NOT NULL", "주문고객번호", "int", false},
		{"주문일자 date NOT NULL", "주문일자", "date", false},
		{"선적번호 int NOT NULL", "선적번호", "int", false},
		{"선적일자 date NOT NULL", "선적일자", "date", false},
		{"CUST_NO int NOT NULL", "CUST_NO", "int", false},
		{"CUST_NM char(50) NOT NULL", "CUST_NM", "char(50)", false},
		{"ORDR_NO int NOT NULL", "ORDR_NO", "int", false},
		{"ORDR_CUST_NO int NOT NULL", "ORDR_CUST_NO", "int", false},
		{"ORDR_YMD date NOT NULL", "ORDR_YMD", "date", false},
		{"SHPMNT_NO int NOT NULL", "SHPMNT_NO", "int", false},
		{"SHPMNT_YMD date NOT NULL", "SHPMNT_YMD", "date", false},
	}
	for _, c := range cases {
		got := parseColumnValue(c.raw, "")
		if got.Name != c.wantName || got.Type != c.wantType || got.Nullable != c.wantNull {
			t.Errorf("parseColumnValue(%q) = {Name:%q Type:%q Nullable:%v}; want {Name:%q Type:%q Nullable:%v}",
				c.raw, got.Name, got.Type, got.Nullable, c.wantName, c.wantType, c.wantNull)
		}
		if got.RawValue != c.raw {
			t.Errorf("parseColumnValue(%q).RawValue = %q; 원본을 그대로 보존해야 한다", c.raw, got.RawValue)
		}
	}
}

// TestParseColumnValue_TypeWithSpaces는 이 태스크가 존재하는 이유다.
// 실제 DB가 내는 공백 든 타입이 잘리지 않아야 한다.
func TestParseColumnValue_TypeWithSpaces(t *testing.T) {
	cases := []struct {
		raw      string
		wantName string
		wantType string
		wantNull bool
	}{
		{"addr character varying(255)", "addr", "character varying(255)", true},
		{"created_at timestamp with time zone NOT NULL", "created_at", "timestamp with time zone", false},
		{"ratio double precision", "ratio", "double precision", true},
		{"qty int unsigned NOT NULL", "qty", "int unsigned", false},
		{"payload STRUCT(a INTEGER, b VARCHAR)", "payload", "STRUCT(a INTEGER, b VARCHAR)", true},
		// UNIQUE도 플래그다 — 타입에 섞이면 안 된다(genbuild.columnValue가 낸다).
		{"email varchar(200) NOT NULL UNIQUE", "email", "varchar(200)", false},
		{"nickname varchar(50) UNIQUE", "nickname", "varchar(50)", true},
	}
	for _, c := range cases {
		got := parseColumnValue(c.raw, "")
		if got.Name != c.wantName || got.Type != c.wantType || got.Nullable != c.wantNull {
			t.Errorf("parseColumnValue(%q) = {Name:%q Type:%q Nullable:%v}; want {Name:%q Type:%q Nullable:%v}",
				c.raw, got.Name, got.Type, got.Nullable, c.wantName, c.wantType, c.wantNull)
		}
	}
}
