package validate

import (
	"strings"
	"testing"

	"erdtool/internal/model"
)

func colDoc(cols ...model.Column) model.Document {
	return model.Document{Diagrams: []model.Diagram{{
		ID: "p1", Name: "물리",
		Tables: []model.Table{{ID: "t1", Name: "고객", Columns: cols}},
	}}}
}

func rules(findings []Finding) []string {
	var out []string
	for _, f := range findings {
		out = append(out, f.Rule)
	}
	return out
}

func TestColumn_타입이없으면진단을낸다(t *testing.T) {
	doc := colDoc(model.Column{ID: "r1", Name: "CUST_NO", Key: "PK", RawValue: "CUST_NO"})
	f, ok := findRule(Required(doc, nil), "missing_column_type")
	if !ok {
		t.Fatalf("missing_column_type이 없다: %v", rules(Required(doc, nil)))
	}
	// 원본을 보여줘야 사용자가 어느 셀인지 안다.
	if !strings.Contains(f.Message, `"CUST_NO"`) {
		t.Errorf("정의 셀 원문이 문구에 있어야 한다: %s", f.Message)
	}
	if f.CellID != "r1" {
		t.Errorf("행 셀을 가리켜야 한다: %q", f.CellID)
	}
}

func TestColumn_이름이없으면타입은묻지않는다(t *testing.T) {
	// 이름이 없으면 그 행은 아직 컬럼이 아니다. 진단 둘을 겹쳐 내면
	// 사용자는 고칠 곳이 둘인 줄 안다.
	doc := colDoc(model.Column{ID: "r1", RawValue: ""})
	got := rules(Required(doc, nil))
	for _, r := range got {
		if r == "missing_column_type" {
			t.Fatalf("이름이 없는 행에 타입까지 묻고 있다: %v", got)
		}
	}
	if _, ok := findRule(Required(doc, nil), "missing_column_name"); !ok {
		t.Fatalf("missing_column_name이 없다: %v", got)
	}
}

func TestColumn_이름이겹치면두번째만진단을낸다(t *testing.T) {
	doc := colDoc(
		model.Column{ID: "r1", Name: "A", Type: "int", Key: "PK"},
		model.Column{ID: "r2", Name: "A", Type: "int"},
	)
	var dups []Finding
	for _, f := range Required(doc, nil) {
		if f.Rule == "duplicate_column_name" {
			dups = append(dups, f)
		}
	}
	if len(dups) != 1 {
		t.Fatalf("진단은 하나여야 한다: %+v", dups)
	}
	// 첫 행까지 진단을 내면 사용자가 멀쩡한 쪽을 건드린다.
	if dups[0].CellID != "r2" {
		t.Errorf("두 번째 행을 가리켜야 한다: %q", dups[0].CellID)
	}
}

func TestTypeBaseName_괄호뒤에남은낱말을살린다(t *testing.T) {
	// 예전에는 괄호 «앞»까지만 잘라서 char(20) NOTNULL이 CHAR로 줄었고,
	// 표준 타입 목록을 그대로 통과했다.
	cases := map[string]string{
		"char(20) NOTNULL":         "CHAR NOTNULL",
		"numeric(10,2)":            "NUMERIC",
		"character  varying(255)":  "CHARACTER VARYING",
		"smallint unsigned":        "SMALLINT UNSIGNED",
		"enum('G','PG')":           "ENUM",
		"timestamp with time zone": "TIMESTAMP WITH TIME ZONE",
	}
	for in, want := range cases {
		if got := typeBaseName(in); got != want {
			t.Errorf("typeBaseName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTypeBaseName_괄호가안닫히면원문을돌려준다(t *testing.T) {
	// 걷어낼 범위를 모르므로 손대지 않는다. 목록에 있을 리 없으니 진단으로
	// 올라가고, 정확한 원인은 DDL 시험 실행(dry run)이 말한다.
	if got := typeBaseName("varchar(50"); got != "VARCHAR(50" {
		t.Errorf("got %q", got)
	}
}

func TestRelationship_FK표기가없으면진단을낸다(t *testing.T) {
	// 그림에는 선이 멀쩡히 있으니 사람은 문서가 맞는 줄 안다. 조용히
	// 빠지면 정의서의 참조 칸도 DDL의 FOREIGN KEY도 그 관계를 잃는다.
	doc := model.Document{Diagrams: []model.Diagram{{
		ID: "p1", Name: "물리",
		Tables: []model.Table{
			{ID: "t1", Name: "고객", Columns: []model.Column{
				{ID: "r1", Name: "고객번호", Type: "int", Key: "PK"}}},
			{ID: "t2", Name: "주문", Columns: []model.Column{
				{ID: "r2", Name: "주문고객번호", Type: "int"}}}, // FK를 안 적었다
		},
		Relationships: []model.Relationship{{
			ID:            "e1",
			SourceTableID: "t1", SourceColumnID: "r1", SourceResolved: true,
			TargetTableID: "t2", TargetColumnID: "r2", TargetResolved: true,
		}},
	}}}

	f, ok := findRule(Required(doc, nil), "relationship_without_fk")
	if !ok {
		t.Fatalf("진단이 없다: %v", rules(Required(doc, nil)))
	}
	if f.CellID != "e1" {
		t.Errorf("관계선을 가리켜야 한다: %q", f.CellID)
	}
	// 셀 id만 주면 사용자가 그림에서 그것을 찾을 방법이 마땅치 않다.
	if !strings.Contains(f.Message, "고객.고객번호") || !strings.Contains(f.Message, "주문.주문고객번호") {
		t.Errorf("양 끝이 문구에 있어야 한다: %s", f.Message)
	}
}

func TestRelationship_FK표기가있으면조용하다(t *testing.T) {
	doc := model.Document{Diagrams: []model.Diagram{{
		ID: "p1", Name: "물리",
		Tables: []model.Table{
			{ID: "t1", Name: "고객", Columns: []model.Column{
				{ID: "r1", Name: "고객번호", Type: "int", Key: "PK"}}},
			{ID: "t2", Name: "주문", Columns: []model.Column{
				{ID: "r2", Name: "주문고객번호", Type: "int", Key: "FK1"}}},
		},
		Relationships: []model.Relationship{{
			ID:            "e1",
			SourceTableID: "t1", SourceColumnID: "r1", SourceResolved: true,
			TargetTableID: "t2", TargetColumnID: "r2", TargetResolved: true,
		}},
	}}}
	if _, ok := findRule(Required(doc, nil), "relationship_without_fk"); ok {
		t.Errorf("멀쩡한 관계에 진단을 냈다")
	}
}
