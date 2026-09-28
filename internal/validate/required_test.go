// internal/validate/required_test.go
package validate

import (
	"strings"
	"testing"

	"erdtool/internal/ir"
	"erdtool/internal/model"
)

func TestRequired_ShapeViolation(t *testing.T) {
	doc := model.Document{Diagrams: []model.Diagram{{
		Name:       "페이지-1",
		Violations: []model.ShapeViolation{{ID: "x1", Shape: "shape=rhombus", Reason: "허용 도형 세트 밖"}},
	}}}
	findings := Required(doc, nil)
	if !hasRule(findings, "shape_violation") {
		t.Fatalf("expected shape_violation finding, got %+v", findings)
	}
}

func TestRequired_MissingPrimaryKey(t *testing.T) {
	doc := model.Document{Diagrams: []model.Diagram{{
		Name: "페이지-1",
		Tables: []model.Table{{
			Name:    "NoKeyTable",
			Columns: []model.Column{{Name: "just_a_column", Type: "int"}},
		}},
	}}}
	findings := Required(doc, nil)
	if !hasRule(findings, "missing_primary_key") {
		t.Fatalf("expected missing_primary_key finding, got %+v", findings)
	}
}

func TestRequired_EmptyTable(t *testing.T) {
	doc := model.Document{Diagrams: []model.Diagram{{
		Name:   "페이지-1",
		Tables: []model.Table{{Name: "EmptyTable"}},
	}}}
	findings := Required(doc, nil)
	if !hasRule(findings, "empty_table") {
		t.Fatalf("expected empty_table finding, got %+v", findings)
	}
}

func TestRequired_BrokenReference(t *testing.T) {
	// 이 테스트는 model.Document를 손으로 조립한다. resolveEnd/
	// ExtractRelationships를 거치지 않으므로 SourceResolved/TargetResolved를
	// 직접 명시해야 한다 — 실제 파서가 만들어낼 수 있는 상태(양쪽이
	// SourceTableID/TargetTableID 텍스트값만으로 판정되는 상태)를 흉내내는
	// 게 아니라, 검증기가 어떤 신호로 판정하는지를 정확히 반영해야 한다.
	// 파서가 실제로 만드는 끊어진 관계 상태는 relationships_test.go +
	// broken_reference_*.drawio 픽스처로 별도 검증한다.
	doc := model.Document{Diagrams: []model.Diagram{{
		Name: "페이지-1",
		Tables: []model.Table{
			{ID: "t1", Name: "A", Columns: []model.Column{{ID: "c1", Name: "id", Key: "PK"}}},
		},
		Relationships: []model.Relationship{
			{ID: "r1", SourceTableID: "t1", SourceColumnID: "c1", SourceRawID: "t1", SourceResolved: true,
				TargetTableID: "", TargetColumnID: "", TargetRawID: "does-not-exist", TargetResolved: false,
				ColumnLevel: true},
		},
	}}}
	findings := Required(doc, nil)
	if !hasRule(findings, "broken_reference") {
		t.Fatalf("expected broken_reference finding, got %+v", findings)
	}
}

func TestRequired_BrokenReference_ExistsButUnresolvedIsNotFlagged(t *testing.T) {
	// 회귀 방지: raw id가 idx.ByID에 실존하지만(SourceExists=true) 이
	// 파서가 인식하지 못하는 도형 변형이라 해석에 실패한 경우(SourceResolved
	// =false)는 broken_reference로 플래그되면 안 된다 — 진짜 dangling
	// reference는 raw id가 idx.ByID에 아예 없는 경우(Exists=false)뿐이다.
	// out_of_order_cells.drawio(구버전 2단 테이블, partialRectangle 직속
	// 행)가 실제로 이 상태를 만들어낸다.
	doc := model.Document{Diagrams: []model.Diagram{{
		Name: "페이지-1",
		Tables: []model.Table{
			{ID: "t1", Name: "A", Columns: []model.Column{{ID: "c1", Name: "id", Key: "PK"}}},
		},
		Relationships: []model.Relationship{
			{ID: "r1", SourceTableID: "t1", SourceColumnID: "c1", SourceRawID: "t1", SourceResolved: true, SourceExists: true,
				TargetTableID: "", TargetColumnID: "", TargetRawID: "row-shape-unrecognized", TargetResolved: false, TargetExists: true,
				ColumnLevel: true},
		},
	}}}
	findings := Required(doc, nil)
	if hasRule(findings, "broken_reference") {
		t.Fatalf("expected no broken_reference finding for an existing-but-unresolved endpoint, got %+v", findings)
	}
}

func TestRequired_BrokenReference_RealFixture_OutOfOrderCells_NoFalsePositive(t *testing.T) {
	// C2 재검토에서 발견된 회귀: out_of_order_cells.drawio는 구버전 2단
	// 테이블 형식(행이 shape=tableRow가 아니라 partialRectangle로 테이블에
	// 직접 매달림)을 쓴다. resolveEnd가 이 행 변형을 인식하지 못해 모든
	// 관계 끝이 Resolved=false가 되지만, raw id 자체는 실존하는 셀을
	// 가리키므로(Exists=true) 진짜 dangling reference가 아니다 —
	// broken_reference 오탐이 0건이어야 한다. (empty_table/
	// duplicate_table_name은 이 픽스처의 별개의, 이미 알려진 미지원 케이스라
	// 그대로 남는다.)
	doc, dups, err := ir.Assemble("../drawio/testdata/out_of_order_cells.drawio")
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	findings := Required(doc, dups)
	count := 0
	for _, f := range findings {
		if f.Rule == "broken_reference" {
			count++
		}
	}
	if count != 0 {
		t.Fatalf("expected 0 broken_reference findings (unsupported row shape, not dangling), got %d: %+v", count, findings)
	}
}

// TestRequired_BrokenReference_RealFixture_BothEndsDangling과
// TestRequired_BrokenReference_RealFixture_OneEndDangling은 C2 회귀
// 테스트다: model.Document를 손으로 만들지 않고 실제 파서
// (ir.Assemble)가 만들어내는 상태를 그대로 검증기에 넣는다. 이전 구현은
// resolveEnd가 실패한 관계를 아예 드롭하거나(양쪽 다 끊긴 경우) 끊어진
// 쪽 필드를 빈 문자열로 채운 채 통과시켰고(한쪽만 끊긴 경우), 검증기의
// `known[id]` 룩업은 둘 다 놓쳤다 — 이 두 테스트가 그 시나리오를 각각
// 재현한다.
func TestRequired_BrokenReference_RealFixture_BothEndsDangling(t *testing.T) {
	doc, dups, err := ir.Assemble("../drawio/testdata/broken_reference_both_dangling.drawio")
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	findings := Required(doc, dups)
	count := 0
	for _, f := range findings {
		if f.Rule == "broken_reference" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected 2 broken_reference findings (source+target both dangling), got %d: %+v", count, findings)
	}
}

func TestRequired_BrokenReference_RealFixture_OneEndDangling(t *testing.T) {
	doc, dups, err := ir.Assemble("../drawio/testdata/broken_reference_one_dangling.drawio")
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	findings := Required(doc, dups)
	count := 0
	for _, f := range findings {
		if f.Rule == "broken_reference" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 broken_reference finding (target dangling only), got %d: %+v", count, findings)
	}
}

func TestRequired_DuplicateTableName(t *testing.T) {
	doc := model.Document{Diagrams: []model.Diagram{
		{Name: "페이지-1", Tables: []model.Table{{Name: "Customer", Columns: []model.Column{{Name: "id", Key: "PK"}}}}},
		{Name: "페이지-2", Tables: []model.Table{{Name: "Customer", Columns: []model.Column{{Name: "id", Key: "PK"}}}}},
	}}
	dups := []ir.DuplicateNameWarning{{TableName: "Customer", PageNames: []string{"페이지-1", "페이지-2"}}}
	findings := Required(doc, dups)
	if !hasRule(findings, "duplicate_table_name") {
		t.Fatalf("expected duplicate_table_name finding, got %+v", findings)
	}
}

func hasRule(findings []Finding, rule string) bool {
	for _, f := range findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

func TestRequired_LowercasePKIsRecognized(t *testing.T) {
	// M1: 키 셀에 "pk"라고 소문자로 적힌 테이블은 PK가 있는 것이다. 판정이
	// 대소문자를 구분하면 멀쩡한 테이블에 missing_primary_key 경고가 뜬다.
	doc := model.Document{Diagrams: []model.Diagram{{
		Name: "p1",
		Tables: []model.Table{{
			Name:    "Orders",
			Columns: []model.Column{{Name: "order_id", Type: "int", Key: "pk"}},
		}},
	}}}
	for _, f := range Required(doc, nil) {
		if f.Rule == "missing_primary_key" {
			t.Fatalf("lowercase key %q must count as a primary key, got finding: %s", "pk", f.Message)
		}
	}
}

func TestFindingCarriesCellID(t *testing.T) {
	doc := model.Document{Diagrams: []model.Diagram{{
		ID:   "pg1",
		Name: "주문",
		Tables: []model.Table{
			{ID: "t-empty", Name: "빈테이블"},
			{ID: "t-nopk", Name: "PK없음", Columns: []model.Column{
				{ID: "c1", Name: "이름", Type: "VARCHAR"},
			}},
		},
	}}}

	got := map[string]string{} // rule -> cellID
	for _, f := range Required(doc, nil) {
		got[f.Rule] = f.CellID
		if f.DiagramID != "pg1" {
			t.Errorf("규칙 %s의 DiagramID=%q; \"pg1\"이어야 한다", f.Rule, f.DiagramID)
		}
	}
	if got["empty_table"] != "t-empty" {
		t.Errorf("empty_table의 CellID=%q; \"t-empty\"여야 한다", got["empty_table"])
	}
	if got["missing_primary_key"] != "t-nopk" {
		t.Errorf("missing_primary_key의 CellID=%q; \"t-nopk\"여야 한다", got["missing_primary_key"])
	}
}

// duplicate_table_name은 ir.DuplicateNameWarning에서 오고 그 구조체에
// 셀 id가 없다. 지어내지 않고 빈 문자열로 두는 것이 계약이다.
func TestDuplicateTableNameHasNoCellID(t *testing.T) {
	dups := []ir.DuplicateNameWarning{{TableName: "고객", PageNames: []string{"a", "b"}}}
	fs := Required(model.Document{}, dups)
	if len(fs) != 1 {
		t.Fatalf("finding %d개; 1개여야 한다", len(fs))
	}
	if fs[0].CellID != "" {
		t.Errorf("CellID=%q; 빈 문자열이어야 한다", fs[0].CellID)
	}
}

func TestRequired_ShapeViolationMessageUsesShapeName(t *testing.T) {
	// M11: 위반 메시지에 style 문자열 전체가 아니라 도형 이름이 들어가야
	// 한다. 이름을 특정할 수 없는 스타일일 때만 원본 style로 폴백한다 —
	// 그마저 없으면 사용자는 어떤 도형인지 알 방법이 없다.
	doc := model.Document{Diagrams: []model.Diagram{{
		Name: "p1",
		Violations: []model.ShapeViolation{
			{ID: "e1", Shape: "ellipse", Style: "ellipse;whiteSpace=wrap;html=1;", Reason: "허용 도형 세트 밖"},
			{ID: "x1", Shape: "", Style: "rounded=0;whiteSpace=wrap;", Reason: "허용 도형 세트 밖"},
		},
	}}}
	findings := Required(doc, nil)
	var msgs []string
	for _, f := range findings {
		if f.Rule == "shape_violation" {
			msgs = append(msgs, f.Message)
		}
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 shape_violation findings, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0], "ellipse") {
		t.Errorf("expected shape name in message, got: %s", msgs[0])
	}
	if strings.Contains(msgs[0], "whiteSpace") {
		t.Errorf("expected raw style NOT in message when a shape name is known, got: %s", msgs[0])
	}
	// 이름을 못 뽑을 때 원본 style을 대신 싣던 자리다. annotate가 그 셀의
	// style에 테두리 색을 더하므로, 문구가 실행마다 달라져 같은 파일이 두
	// 번 다르게 나왔다(2026-09-06). 문구에는 변하지 않는 것만 담는다.
	if strings.Contains(msgs[1], "rounded=0") {
		t.Errorf("문구에 원본 style이 들어갔다 — annotate가 style을 바꾸면 문구가 흔들린다: %s", msgs[1])
	}
	if !strings.Contains(msgs[1], "x1") {
		t.Errorf("이름을 못 뽑았으면 id라도 있어야 찾을 수 있다: %s", msgs[1])
	}
}

// duplicate_table_name 문구는 «몇 개 페이지»와 «몇 곳»을 절대 뭉개면
// 안 된다. 예전에는 출현 수를 그대로 «N개 페이지»라고 불러, 페이지가
// 둘뿐인 파일이 "6개 페이지에 존재함"으로 보고됐다 — 화면의 숫자와 파일
// 안의 사실이 다른, 이 저장소가 존재하는 이유인 그 실패다.
func TestDuplicateNameMessageNeverMiscountsPages(t *testing.T) {
	cases := []struct {
		name string
		dup  ir.DuplicateNameWarning
		want string
	}{
		{
			name: "페이지마다 하나씩",
			dup:  ir.DuplicateNameWarning{TableName: "고객", PageNames: []string{"A", "B"}, Occurrences: 2},
			want: `동일 이름 테이블 "고객"이 2개 페이지에 존재함: [A B]`,
		},
		{
			name: "여러 페이지에 걸쳐 더 많이",
			dup:  ir.DuplicateNameWarning{TableName: "고객", PageNames: []string{"A", "B"}, Occurrences: 6},
			want: `동일 이름 테이블 "고객"이 2개 페이지에 걸쳐 6곳 존재함: [A B]`,
		},
		{
			name: "한 페이지 안에서만",
			dup:  ir.DuplicateNameWarning{TableName: "고객", PageNames: []string{"A"}, Occurrences: 3},
			want: `동일 이름 테이블 "고객"이 페이지 "A" 안에 3개 있음`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := duplicateNameMessage(c.dup); got != c.want {
				t.Errorf("문구가 다르다:\n얻음: %s\n기대: %s", got, c.want)
			}
		})
	}
}

// 한 페이지 안에서만 겹치는 경우도 반드시 보고된다 — 같은 이름 테이블
// 둘 중 어느 쪽이 진짜인지는 사람이 정해야 한다. 판정을 «페이지가 둘
// 이상»으로 좁히면 이 경우가 통째로 사라진다.
func TestRequired_ReportsSamePageDuplicate(t *testing.T) {
	dups := []ir.DuplicateNameWarning{
		{TableName: "고객", PageNames: []string{"A"}, Occurrences: 2},
	}
	findings := Required(model.Document{}, dups)
	if !hasRule(findings, "duplicate_table_name") {
		t.Fatalf("한 페이지 안의 중복이 보고되지 않았다: %+v", findings)
	}
}

// 그림에서는 두 테이블에 닿아 있는데 파일에는 연결이 없는 관계선. 이전
// 구현은 이것을 «아무 것에도 연결을 시도하지 않은 장식용 선»과 한 덩어리로
// 보고 조용히 버렸고, 그래서 관계 하나가 정의서에서 사라진 채 검증 리포트에
// 「발견된 문제 없음」이 찍혔다.
func TestRequired_FloatingRelationship_RealFixture(t *testing.T) {
	doc, dups, err := ir.Assemble("../drawio/testdata/floating_relation.drawio")
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	findings := Required(doc, dups)

	var got []Finding
	for _, f := range findings {
		if f.Rule == "floating_relationship" {
			got = append(got, f)
		}
	}
	if len(got) != 1 {
		t.Fatalf("floating_relationship=%d건; 1건이어야 한다: %+v", len(got), findings)
	}
	f := got[0]
	// annotate가 이 진단을 그림에 되반영하려면 붙일 셀 id가 있어야 한다.
	if f.CellID == "" {
		t.Error("CellID가 비었다 — annotate가 이 관계선을 표시할 수 없다")
	}
	// 문구가 어느 테이블 근처인지 말해 주지 않으면 사용자는 넓은 그림에서
	// 그 선을 못 찾는다.
	for _, want := range []string{"CUST", "ORDR"} {
		if !strings.Contains(f.Message, want) {
			t.Errorf("Message=%q; %q가 들어 있어야 한다", f.Message, want)
		}
	}
}
