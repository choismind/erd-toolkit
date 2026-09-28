package validate

import (
	"strings"
	"testing"

	"erdtool/internal/drawio"
	"erdtool/internal/model"
)

func attrDoc(conflicts, unknown []model.AttrIssue) model.Document {
	return model.Document{Diagrams: []model.Diagram{{
		ID: "p1", Name: "물리",
		Tables: []model.Table{{
			ID: "t1", Name: "고객",
			Columns: []model.Column{{ID: "r1", Name: "CUST_NO", Type: "int", Key: "PK"}},
		}},
		AttrConflicts: conflicts,
		UnknownAttrs:  unknown,
	}}}
}

func findRule(findings []Finding, rule string) (Finding, bool) {
	for _, f := range findings {
		if f.Rule == rule {
			return f, true
		}
	}
	return Finding{}, false
}

func TestRequired_충돌은진단으로올라간다(t *testing.T) {
	doc := attrDoc([]model.AttrIssue{{
		CellID: "r9", TableName: "고객", ColumnName: "EMAIL",
		Name: drawio.AttrCheck, Values: []string{"LENGTH(EMAIL) >= 5", "EMAIL LIKE '%@%'"},
	}}, nil)

	f, ok := findRule(Required(doc, nil), "attr_conflict")
	if !ok {
		t.Fatal("attr_conflict 진단이 없다")
	}
	// annotate가 이 id로 그 행에 빨간 테두리를 친다. 없으면 그림에 못 남는다.
	if f.CellID != "r9" {
		t.Errorf("진단이 셀을 가리켜야 한다: got %q", f.CellID)
	}
	// 값의 경계가 보여야 한다 — SQL 식에는 공백이 흔하다.
	for _, want := range []string{`"LENGTH(EMAIL) >= 5"`, `"EMAIL LIKE '%@%'"`, "고객.EMAIL"} {
		if !strings.Contains(f.Message, want) {
			t.Errorf("문구에 %s가 없다: %s", want, f.Message)
		}
	}
}

func TestRequired_모르는이름은읽는이름을함께알려준다(t *testing.T) {
	doc := attrDoc(nil, []model.AttrIssue{
		{CellID: "r1", TableName: "고객", ColumnName: "CUST_NO", Name: "erd_chek"},
		{CellID: "t1", TableName: "고객", Name: drawio.AttrDefault},
	})
	findings := Required(doc, nil)

	var messages []string
	for _, f := range findings {
		if f.Rule == "unknown_attr" {
			messages = append(messages, f.Message)
		}
	}
	if len(messages) != 2 {
		t.Fatalf("진단 둘이어야 한다: %v", messages)
	}
	// 컬럼 쪽은 erd_default를 쓸 수 있다고 알려주고,
	if !strings.Contains(messages[0], drawio.AttrDefault) {
		t.Errorf("컬럼 문구에 erd_default가 있어야 한다: %s", messages[0])
	}
	// 테이블 쪽은 알려주면 안 된다 — 기본값은 컬럼의 성질이다.
	if strings.Contains(messages[1], drawio.AttrDefault+" /") ||
		strings.Contains(messages[1], "/ "+drawio.AttrDefault) {
		t.Errorf("테이블 문구가 erd_default를 쓸 수 있다고 말한다: %s", messages[1])
	}
}
