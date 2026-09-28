package report

import (
	"strings"
	"testing"

	"erdtool/internal/dialect"
	"erdtool/internal/drawio"
	"erdtool/internal/model"
)

// constraintDoc은 제약이 붙은 컬럼 하나, 충돌한 컬럼 하나, 테이블 제약을
// 가진 문서다. 네 리포터가 모두 이 하나를 쓴다.
func constraintDoc() model.Document {
	return model.Document{Diagrams: []model.Diagram{{
		ID: "p1", Name: "물리",
		Tables: []model.Table{{
			ID: "t1", Name: "고객", Comment: "고객 마스터", Check: "CUST_NO > 0",
			Columns: []model.Column{
				{ID: "r1", Name: "CUST_NO", Type: "int", Key: "PK"},
				{ID: "r2", Name: "CUST_NM", Type: "char(50)", Nullable: true,
					Default: "'미상'", Comment: "고객명"},
				{ID: "r3", Name: "EMAIL", Type: "char(100)", Nullable: true},
			},
		}},
		AttrConflicts: []model.AttrIssue{{
			CellID: "r3", TableName: "고객", ColumnName: "EMAIL",
			Name: drawio.AttrCheck, Values: []string{"a", "b"},
		}},
	}}}
}

func TestTableDoc_네포맷이같은열을낸다(t *testing.T) {
	// I6의 재발 방지: page_as_domain이 Markdown에만 걸려 있어 같은 실행의
	// 산출물들이 서로 다른 제목을 달고 나갔다. 열도 같은 실패가 가능하다.
	md := TableDocMarkdown(constraintDoc(), Options{})
	html := TableDocHTML(constraintDoc(), Options{})
	for _, header := range tableDocHeaders {
		if !strings.Contains(md, header) {
			t.Errorf("Markdown에 열 %q가 없다", header)
		}
		if !strings.Contains(html, header) {
			t.Errorf("HTML에 열 %q가 없다", header)
		}
	}
}

func TestTableDoc_제약값이실린다(t *testing.T) {
	md := TableDocMarkdown(constraintDoc(), Options{})
	for _, want := range []string{"'미상'", "고객명", "설명: 고객 마스터", "테이블 CHECK: CUST_NO > 0"} {
		if !strings.Contains(md, want) {
			t.Errorf("정의서에 %q가 없다:\n%s", want, md)
		}
	}
}

func TestTableDoc_충돌은빈칸이아니라문구로남는다(t *testing.T) {
	// 「값을 안 적었다」와 「적었는데 도구가 못 골랐다」는 사용자가 할 일이
	// 다르다. 빈 칸은 전자로만 읽힌다.
	for name, out := range map[string]string{
		"Markdown": TableDocMarkdown(constraintDoc(), Options{}),
		"HTML":     TableDocHTML(constraintDoc(), Options{}),
	} {
		if !strings.Contains(out, conflictNote) {
			t.Errorf("%s가 충돌을 조용히 삼켰다:\n%s", name, out)
		}
	}
}

func TestSQLDDL_제약과충돌(t *testing.T) {
	ddl := SQLDDL(constraintDoc(), Options{})
	if !strings.Contains(ddl, `"CUST_NM" char(50) DEFAULT '미상'`) {
		t.Errorf("DEFAULT 절이 없다:\n%s", ddl)
	}
	if !strings.Contains(ddl, "CHECK (CUST_NO > 0)") {
		t.Errorf("테이블 CHECK 절이 없다:\n%s", ddl)
	}
	// 충돌한 자리는 실행하면 걸리도록 주석을 남긴다(타입 없음 처리와 같은 방식).
	if !strings.Contains(ddl, "erd_check 충돌") {
		t.Errorf("충돌 주석이 없다:\n%s", ddl)
	}
}

func TestSQLDDL_충돌한값을DDL에쓰지않는다(t *testing.T) {
	doc := constraintDoc()
	// 충돌인데도 값이 남아 있는 경우를 일부러 만든다. 리포터가 Column의
	// 값만 보고 찍으면 «어느 쪽도 고르지 않는다»는 계약이 여기서 깨진다.
	doc.Diagrams[0].Tables[0].Columns[2].Check = "a"
	ddl := SQLDDL(doc, Options{})
	if strings.Contains(ddl, "CHECK (a)") {
		t.Errorf("충돌한 값이 DDL에 실렸다:\n%s", ddl)
	}
}

// relationDoc은 부모(고객)와 자식(주문)에 관계선 하나를 둔 문서다.
func refDoc() model.Document {
	return model.Document{Diagrams: []model.Diagram{{
		ID: "p1", Name: "물리",
		Tables: []model.Table{
			{ID: "t1", Name: "고객", Columns: []model.Column{
				{ID: "r1", Name: "고객번호", Type: "int", Key: "PK"}}},
			{ID: "t2", Name: "주문", Columns: []model.Column{
				{ID: "r2", Name: "주문번호", Type: "int", Key: "PK"},
				{ID: "r3", Name: "주문고객번호", Type: "int", Key: "FK1"}}},
		},
		Relationships: []model.Relationship{{
			ID:            "e1",
			SourceTableID: "t1", SourceColumnID: "r1", SourceResolved: true,
			SourceCardinality: "ERmandOne",
			TargetTableID:     "t2", TargetColumnID: "r3", TargetResolved: true,
			TargetCardinality: "ERzeroToMany",
		}},
	}}}
}

func TestTableDoc_참조칸에대상이실린다(t *testing.T) {
	// FK 칸의 `FK1`은 «외래키다»만 말한다. 무엇을 가리키는지는 관계선에
	// 있고, 정의서만 보는 사람에게는 이 칸이 없으면 닿지 않는다.
	md := TableDocMarkdown(refDoc(), Options{})
	if !strings.Contains(md, "고객.고객번호") {
		t.Errorf("참조 대상이 없다:\n%s", md)
	}
}

func TestSQLDDL_FOREIGN_KEY가나온다(t *testing.T) {
	ddl := SQLDDL(refDoc(), Options{})
	want := `FOREIGN KEY ("주문고객번호") REFERENCES "고객" ("고객번호")`
	if !strings.Contains(ddl, want) {
		t.Errorf("%s가 없다:\n%s", want, ddl)
	}
}

func TestSQLDDL_참조컬럼을모르면컬럼목록을비운다(t *testing.T) {
	// 관계선이 테이블에 직접 연결된 경우다. ANSI에서 컬럼 목록 없는
	// REFERENCES는 «그 테이블의 기본키»를 뜻한다 — 이름을 지어내지 않는다.
	doc := refDoc()
	doc.Diagrams[0].Relationships[0].SourceColumnID = ""
	ddl := SQLDDL(doc, Options{})
	if !strings.Contains(ddl, `REFERENCES "고객"`) {
		t.Fatalf("REFERENCES가 없다:\n%s", ddl)
	}
	if strings.Contains(ddl, `REFERENCES "고객" (`) {
		t.Errorf("없는 컬럼 이름을 지어냈다:\n%s", ddl)
	}
}

func TestRelationDoc_카디널리티를사람말로쓴다(t *testing.T) {
	// 예전에는 draw.io 스타일 값(ERmandOne)을 그대로 찍었다. 정의서를 받는
	// 쪽은 그 낱말이 무슨 뜻인지 알 길이 없다.
	out := RelationDocMarkdown(refDoc())
	if !strings.Contains(out, "0..N (없거나 여럿)") {
		t.Errorf("사람 말이 없다:\n%s", out)
	}
	// 원본 코드는 남겨 둔다 — 그림과 대조할 단서다.
	if !strings.Contains(out, "[ERzeroToMany]") {
		t.Errorf("원본 코드가 사라졌다:\n%s", out)
	}
}

func TestCardinalityLabel_모르는코드는원문을둔다(t *testing.T) {
	if got := cardinalityLabel("ERsomethingNew"); got != "ERsomethingNew" {
		t.Errorf("got %q", got)
	}
	if got := cardinalityLabel(""); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestTableDoc_관계가그테이블자리에붙는다(t *testing.T) {
	// 2026-09-04 소유자 결정: 관계정보는 그 테이블의 자리에 함께 적는다.
	// 별도 파일(`--relations`)로만 나가면 기본 산출물만 받은 사람은 ERD의
	// 절반을 못 본다.
	md := TableDocMarkdown(refDoc(), Options{})
	for _, want := range []string{"**관계**", "→ 참조함", "← 참조됨"} {
		if !strings.Contains(md, want) {
			t.Errorf("%q가 없다:\n%s", want, md)
		}
	}
}

func TestTableRelationRows_이테이블기준으로낸다(t *testing.T) {
	d := refDoc().Diagrams[0]
	refs := d.ColumnReferences()

	child := tableRelationRows(d, d.Tables[1], refs) // 주문
	if len(child) != 1 {
		t.Fatalf("주문에 관계 하나여야 한다: %+v", child)
	}
	if child[0][1] != dirRefersTo {
		t.Errorf("외래키를 든 쪽은 «참조함»이다: %q", child[0][1])
	}
	if child[0][2] != "고객" || child[0][3] != "주문고객번호" || child[0][4] != "고객번호" {
		t.Errorf("이 테이블 기준으로 상대/이쪽/저쪽이 배치돼야 한다: %v", child[0])
	}

	parent := tableRelationRows(d, d.Tables[0], refs) // 고객
	if len(parent) != 1 || parent[0][1] != dirReferredBy {
		t.Errorf("참조당하는 쪽은 «참조됨»이다: %+v", parent)
	}
	// 같은 관계가 양쪽 테이블에 한 줄씩 나온다 — 어느 쪽을 펴 봐도 보여야 한다.
	if parent[0][2] != "주문" {
		t.Errorf("상대 테이블: %q", parent[0][2])
	}
}

func TestTableRelationRows_판정못한관계도싣는다(t *testing.T) {
	// 조용히 빠지면 그림에는 선이 있는데 문서에는 흔적도 없다.
	doc := refDoc()
	doc.Diagrams[0].Tables[1].Columns[1].Key = "" // FK 표기를 지웠다
	d := doc.Diagrams[0]
	rows := tableRelationRows(d, d.Tables[1], d.ColumnReferences())
	if len(rows) != 1 {
		t.Fatalf("관계는 그대로 실려야 한다: %+v", rows)
	}
	if rows[0][1] != dirUnknown {
		t.Errorf("방향은 «미정»이어야 한다: %q", rows[0][1])
	}
}

func TestDocHeader_타깃DBMS를박는다(t *testing.T) {
	// 조용히 비우면 「ANSI로 정한 것」인지 「깜빡한 것」인지 산출물만 봐서는
	// 알 수 없다.
	if md := TableDocMarkdown(refDoc(), Options{}); !strings.Contains(md, "ANSI (지정 안 함)") {
		t.Errorf("지정 안 했다는 사실이 문서에 없다:\n%s", md)
	}
	md := TableDocMarkdown(refDoc(), Options{Dialect: dialect.MySQL})
	if !strings.Contains(md, "타깃 DBMS") || !strings.Contains(md, "MySQL") {
		t.Errorf("타깃이 문서에 없다:\n%s", md)
	}
}

func TestSQLDDL_타깃DBMS에맞게식별자를감싼다(t *testing.T) {
	// 예전에는 인용은 ANSI 큰따옴표 고정인데 타입은 원본 DB 카탈로그 원문을
	// 그대로 옮겨, 두 DBMS 문법이 한 파일에 섞여 나갔다.
	ansi := SQLDDL(refDoc(), Options{})
	if !strings.Contains(ansi, `CREATE TABLE "고객"`) {
		t.Errorf("ANSI는 큰따옴표다:\n%s", ansi)
	}
	my := SQLDDL(refDoc(), Options{Dialect: dialect.MySQL})
	if !strings.Contains(my, "CREATE TABLE `고객`") {
		t.Errorf("MySQL은 백틱이다:\n%s", my)
	}
	// SQL 파일만 남았을 때도 어느 DB용인지 알 수 있어야 한다.
	if !strings.Contains(my, "-- 타깃 DBMS: MySQL") {
		t.Errorf("머리 주석이 없다:\n%s", my)
	}
}
