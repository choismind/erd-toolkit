package ddlcheck

import (
	"strings"
	"testing"

	"erdtool/internal/model"
	"erdtool/internal/report"
	"erdtool/internal/validate"
)

func docWith(cols ...model.Column) model.Document {
	return model.Document{Diagrams: []model.Diagram{{
		ID: "p1", Name: "물리",
		Tables: []model.Table{{ID: "t1", Name: "고객", Columns: cols}},
	}}}
}

func TestDryRun_성립하는DDL은조용하다(t *testing.T) {
	doc := docWith(model.Column{ID: "r1", Name: "CUST_NO", Type: "int", Key: "PK"})
	if got := DryRun(doc, nil, report.Options{}); len(got) != 0 {
		t.Fatalf("진단이 없어야 한다: %+v", got)
	}
}

func TestDryRun_NOTNULL붙여쓰기를잡는다(t *testing.T) {
	// 파서는 이것을 타입의 일부로 삼키고, NULL 허용도 Y로 뒤집힌다.
	// 앞 단계 검사 어느 것도 못 잡는 자리다(2026-09-04 실측).
	doc := docWith(model.Column{ID: "r1", Name: "CUST_NM", Type: "char(20) NOTNULL", Nullable: true})
	got := DryRun(doc, nil, report.Options{})
	if len(got) != 1 || got[0].Rule != "ddl_syntax" {
		t.Fatalf("ddl_syntax 하나여야 한다: %+v", got)
	}
	if got[0].CellID != "t1" {
		t.Errorf("테이블 셀을 가리켜야 한다: %q", got[0].CellID)
	}
	// 테이블만 말하면 컬럼이 여럿일 때 어디를 고칠지 모른다. 엔진이 알려 준
	// 낱말이 한 줄에만 나오면 그 줄을 지목한다.
	if !strings.Contains(got[0].Message, "NOTNULL") {
		t.Errorf("문제가 된 줄을 지목해야 한다: %s", got[0].Message)
	}
	if got[0].Action == "" || got[0].Scope != validate.ScopeTable {
		t.Errorf("할 일과 묶을 자리가 있어야 한다: %+v", got[0])
	}
}

func TestDryRun_제약식의문법오류를잡는다(t *testing.T) {
	doc := docWith(model.Column{ID: "r1", Name: "EMAIL", Type: "varchar(100)",
		Nullable: true, Check: "LENGTH(EMAIL >= 5"})
	if got := DryRun(doc, nil, report.Options{}); len(got) != 1 {
		t.Fatalf("CHECK 식의 괄호 오류를 잡아야 한다: %+v", got)
	}
}

func TestDryRun_이미진단난테이블은건너뛴다(t *testing.T) {
	// 구조가 깨진 테이블을 또 걸면 같은 실수가 두 줄로 보고된다. 사용자는
	// 두 진단이 같은 것인지 다른 것인지 알 방법이 없다.
	doc := docWith(model.Column{ID: "r1", Name: "A", Type: "int"},
		model.Column{ID: "r2", Name: "A", Type: "int"})
	prior := []validate.Finding{{
		Rule: "duplicate_column_name", Severity: validate.SeverityWarning,
		DiagramID: "p1", TableName: "고객", CellID: "r2",
	}}
	if got := DryRun(doc, prior, report.Options{}); len(got) != 0 {
		t.Fatalf("앞 단계가 짚은 테이블은 건너뛰어야 한다: %+v", got)
	}
	// 앞 단계가 조용했다면 시험 실행(dry run)이 잡는다 — 안전망은 살아 있어야 한다.
	if got := DryRun(doc, nil, report.Options{}); len(got) != 1 {
		t.Fatalf("안전망이 죽었다: %+v", got)
	}
}
