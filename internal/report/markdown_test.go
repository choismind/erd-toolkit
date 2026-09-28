package report

import (
	"strings"
	"testing"

	"erdtool/internal/model"
	"erdtool/internal/validate"
)

func sampleDoc() model.Document {
	return model.Document{
		Diagrams: []model.Diagram{{
			Name: "페이지-1",
			Tables: []model.Table{{
				Name: "Orders",
				Columns: []model.Column{
					{Name: "order_id", Type: "int", Key: "PK", Nullable: false},
					{Name: "customer_id", Type: "int", Key: "FK1", Nullable: false},
				},
			}},
			Relationships: []model.Relationship{{
				ID: "r1", SourceTableID: "t1", TargetTableID: "t2",
				SourceCardinality: "ERone", TargetCardinality: "ERzeroToMany", ColumnLevel: true,
			}},
		}},
	}
}

func TestTableDocMarkdown(t *testing.T) {
	md := TableDocMarkdown(sampleDoc(), Options{})
	for _, want := range []string{"Orders", "order_id", "int", "PK", "customer_id", "FK1"} {
		if !strings.Contains(md, want) {
			t.Fatalf("expected markdown to contain %q, got:\n%s", want, md)
		}
	}
	if strings.Contains(md, "도메인") {
		t.Fatalf("expected no domain column when pageAsDomain=false, got:\n%s", md)
	}
}

func TestTableDocMarkdown_PageAsDomain(t *testing.T) {
	// 원장 "다중 페이지 처리": page_as_domain 옵션이 켜지면 테이블정의서에
	// 페이지명을 도메인 컬럼으로 노출한다.
	md := TableDocMarkdown(sampleDoc(), Options{PageAsDomain: true})
	if !strings.Contains(md, "도메인") || !strings.Contains(md, "페이지-1") {
		t.Fatalf("expected domain column with page name when pageAsDomain=true, got:\n%s", md)
	}
}

func TestValidationMarkdown(t *testing.T) {
	findings := []validate.Finding{{
		Rule: "missing_primary_key", Severity: validate.SeverityWarning,
		Message: "테이블 X에 PK가 없음", Brief: "기본키 표기가 없음",
		Action: "기본키 컬럼의 키 셀에 PK를 적는다", Scope: validate.ScopeTable,
		DiagramName: "물리", TableName: "고객",
	}}
	md := ValidationMarkdown(refDoc(), findings, Options{})
	for _, want := range []string{"missing_primary_key", "기본키 표기가 없음"} {
		if !strings.Contains(md, want) {
			t.Fatalf("%q가 없다:\n%s", want, md)
		}
	}
}

func TestValidationMarkdown_페이지와테이블로묶는다(t *testing.T) {
	// 진단을 찾은 순서대로 늘어놓으면 한 테이블을 고치려고 파일을 위아래로
	// 읽어야 한다(2026-09-04 소유자 지적).
	findings := []validate.Finding{
		{Rule: "missing_primary_key", Severity: validate.SeverityWarning,
			Brief: "기본키 표기가 없음", Scope: validate.ScopeTable,
			DiagramName: "물리", TableName: "주문"},
		{Rule: "empty_table", Severity: validate.SeverityWarning,
			Brief: "컬럼이 하나도 없음", Scope: validate.ScopeTable,
			DiagramName: "물리", TableName: "고객"},
		{Rule: "relationship_without_fk", Severity: validate.SeverityWarning,
			Brief: "어느 쪽이 외래키인지 알 수 없음", Scope: validate.ScopeRelation,
			DiagramName: "물리"},
	}
	md := ValidationMarkdown(refDoc(), findings, Options{})

	for _, want := range []string{"## 페이지: 물리", "### 테이블 고객", "### 테이블 주문", "### 관계선"} {
		if !strings.Contains(md, want) {
			t.Errorf("%q가 없다:\n%s", want, md)
		}
	}
	// 테이블 순서는 그림 순서를 따른다. refDoc의 테이블은 고객, 주문 순이다.
	if strings.Index(md, "### 테이블 고객") > strings.Index(md, "### 테이블 주문") {
		t.Errorf("테이블이 그림 순서가 아니다:\n%s", md)
	}
}

func TestValidationMarkdown_할일을함께적는다(t *testing.T) {
	findings := []validate.Finding{{
		Rule: "missing_primary_key", Severity: validate.SeverityWarning,
		Brief: "기본키 표기가 없음", Action: "기본키 컬럼의 키 셀에 PK를 적는다",
		Scope: validate.ScopeTable, DiagramName: "물리", TableName: "고객",
	}}
	md := ValidationMarkdown(refDoc(), findings, Options{})
	if !strings.Contains(md, "→ 기본키 컬럼의 키 셀에 PK를 적는다") {
		t.Errorf("무엇을 하면 되는지가 없다:\n%s", md)
	}
	// 셀 id로는 draw.io에서 도형을 못 찾는다. 찾는 방법을 알려 줘야 한다.
	if !strings.Contains(md, "annotate") {
		t.Errorf("그림에서 찾는 방법 안내가 없다:\n%s", md)
	}
}

func TestValidationMarkdown_NoFindings(t *testing.T) {
	md := ValidationMarkdown(refDoc(), nil, Options{})
	if !strings.Contains(md, "발견된") && !strings.Contains(md, "없음") {
		t.Fatalf("expected a clear 'no findings' message, got:\n%s", md)
	}
}

func relationDoc() model.Document {
	return model.Document{Diagrams: []model.Diagram{
		{
			Name: "물리",
			Tables: []model.Table{
				{ID: "t1", Name: "Orders", Columns: []model.Column{
					{ID: "c1", Name: "order_id", Key: "PK"},
					{ID: "c2", Name: "customer_id", Key: "FK1"},
				}},
				{ID: "t2", Name: "Customers", Columns: []model.Column{
					{ID: "c3", Name: "customer_id", Key: "PK"},
				}},
			},
			Relationships: []model.Relationship{
				{
					ID: "r1", ColumnLevel: true,
					SourceTableID: "t1", SourceColumnID: "c2", SourceCardinality: "ERmany",
					SourceRawID: "c2", SourceResolved: true, SourceExists: true,
					TargetTableID: "t2", TargetColumnID: "c3", TargetCardinality: "ERone",
					TargetRawID: "c3", TargetResolved: true, TargetExists: true,
				},
				{
					// 테이블 직결(컬럼 없음) + 한쪽 미해석
					ID:            "r2",
					SourceTableID: "t1", SourceCardinality: "ERone",
					SourceRawID: "t1", SourceResolved: true, SourceExists: true,
					TargetCardinality: "ERmany", TargetRawID: "ghost-9",
				},
			},
		},
		{
			// 테이블이 없는 페이지다. 뽑을 것이 없을 뿐, 건너뛰는 페이지는
			// 아니다 — 관계선은 그대로 표에 실린다.
			Name: "테이블없음",
			Relationships: []model.Relationship{
				{ID: "r3", SourceTableID: "cx", TargetTableID: "cy"},
			},
		},
	}}
}

func TestRelationDocMarkdown_UsesTableAndColumnNames(t *testing.T) {
	// 원시 셀 ID(t1/c2)가 아니라 테이블명/컬럼명이 나와야 한다.
	md := RelationDocMarkdown(relationDoc())
	for _, want := range []string{"Orders", "Customers", "customer_id"} {
		if !strings.Contains(md, want) {
			t.Errorf("expected %q in relation doc, got:\n%s", want, md)
		}
	}
	for _, notWant := range []string{"| t1 ", "| t2 ", "| c2 ", "| c3 "} {
		if strings.Contains(md, notWant) {
			t.Errorf("raw cell id %q leaked into relation doc:\n%s", notWant, md)
		}
	}
}

// 페이지 성격 판정을 없앤 뒤의 계약이다(2026-09-06). 예전에는 페이지를
// conceptual로 판정해 통째로 건너뛰었고, 그 페이지에 테이블이 있어도 함께
// 사라졌다. 이제는 페이지를 규정하지 않는다 — 읽을 수 있는 것은 읽는다.
func TestRelationDocMarkdown_DoesNotSkipPages(t *testing.T) {
	md := RelationDocMarkdown(relationDoc())
	if !strings.Contains(md, "테이블없음") {
		t.Fatalf("페이지를 건너뛰면 안 된다:\n%s", md)
	}
}

func TestRelationDocMarkdown_MarksUnresolvedEnd(t *testing.T) {
	// 해석되지 않은 끝점은 조용히 빈 칸으로 두지 않고 원본 id를 남긴다.
	md := RelationDocMarkdown(relationDoc())
	if !strings.Contains(md, "ghost-9") {
		t.Fatalf("expected unresolved raw id to be shown, got:\n%s", md)
	}
}
