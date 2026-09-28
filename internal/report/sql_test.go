package report

import (
	"strings"
	"testing"

	"erdtool/internal/model"
)

func TestSQLDDL(t *testing.T) {
	// 식별자는 항상 quote한다 — ERD 이름은 사람이 draw.io에서 자유
	// 입력한 값이라 quote 없이는 한글/공백/예약어에서 깨진다.
	ddl := SQLDDL(sampleDoc(), Options{})
	if !strings.Contains(ddl, `CREATE TABLE "Orders"`) {
		t.Fatalf("expected quoted CREATE TABLE, got:\n%s", ddl)
	}
	if !strings.Contains(ddl, `"order_id" int NOT NULL`) {
		t.Fatalf("expected NOT NULL column def, got:\n%s", ddl)
	}
	if !strings.Contains(ddl, `PRIMARY KEY ("order_id")`) {
		t.Fatalf("expected PRIMARY KEY clause, got:\n%s", ddl)
	}
}

func TestSQLDDL_LowercasePKEmitsPrimaryKey(t *testing.T) {
	// 키 셀이 소문자 "pk"여도 PRIMARY KEY 절이 나와야 한다. 검증
	// 리포트와 DDL이 서로 다른 PK 판정을 쓰면 두 산출물이 모순된다.
	doc := model.Document{Diagrams: []model.Diagram{{
		Name: "p1",
		Tables: []model.Table{{
			Name:    "Orders",
			Columns: []model.Column{{Name: "order_id", Type: "int", Key: "pk"}},
		}},
	}}}
	ddl := SQLDDL(doc, Options{})
	if !strings.Contains(ddl, "PRIMARY KEY") {
		t.Fatalf("expected PRIMARY KEY clause for lowercase pk, got:\n%s", ddl)
	}
}

func TestSQLDDL_QuotesIdentifiers(t *testing.T) {
	// 식별자를 quote하지 않아 한글/공백이 든 이름이면 깨진 DDL이 나왔다.
	doc := model.Document{Diagrams: []model.Diagram{{
		Name: "p1",
		Tables: []model.Table{{
			Name: "주문 정보",
			Columns: []model.Column{
				{Name: "주문 번호", Type: "int", Key: "PK", Nullable: false},
				{Name: "order date", Type: "date", Nullable: true},
			},
		}},
	}}}
	ddl := SQLDDL(doc, Options{})
	for _, want := range []string{
		`CREATE TABLE "주문 정보" (`,
		`"주문 번호" int NOT NULL`,
		`"order date" date`,
		`PRIMARY KEY ("주문 번호")`,
	} {
		if !strings.Contains(ddl, want) {
			t.Errorf("expected %q in DDL, got:\n%s", want, ddl)
		}
	}
}

func TestSQLDDL_EscapesEmbeddedQuote(t *testing.T) {
	doc := model.Document{Diagrams: []model.Diagram{{
		Name:   "p1",
		Tables: []model.Table{{Name: `we"ird`, Columns: []model.Column{{Name: "a", Type: "int"}}}},
	}}}
	if want := `CREATE TABLE "we""ird" (`; !strings.Contains(SQLDDL(doc, Options{}), want) {
		t.Fatalf("expected %q in DDL, got:\n%s", want, SQLDDL(doc, Options{}))
	}
}

func TestSQLDDL_MarksMissingTypeInsteadOfEmittingBlank(t *testing.T) {
	// Type이 빈 컬럼은 지금까지 `  name ` 처럼 타입 자리를 빈 칸으로
	// 남긴 채 나갔다 — 조용히 깨진 DDL이다. 타입을 추측해 채우지 않고
	// (이 프로젝트의 "플레이스홀더로 대체하지 않는다" 원칙) 눈에 띄는
	// 주석으로 표시해 실행 시 그 줄에서 바로 걸리게 한다.
	doc := model.Document{Diagrams: []model.Diagram{{
		Name: "p1",
		Tables: []model.Table{{
			Name:    "Orders",
			Columns: []model.Column{{Name: "UniqueID", Type: "", RawValue: "UniqueID"}},
		}},
	}}}
	ddl := SQLDDL(doc, Options{})
	if !strings.Contains(ddl, `"UniqueID"`) {
		t.Errorf("expected quoted column name, got:\n%s", ddl)
	}
	if !strings.Contains(ddl, "타입 없음") {
		t.Errorf("expected an explicit missing-type marker, got:\n%s", ddl)
	}
	if strings.Contains(ddl, `"UniqueID" ,`) || strings.Contains(ddl, `"UniqueID"\n`) {
		t.Errorf("missing type must not be emitted as a blank slot, got:\n%s", ddl)
	}
}
