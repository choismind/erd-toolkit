// internal/ir/assemble_test.go
package ir

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAssemble_RealFixture(t *testing.T) {
	doc, dups, err := Assemble("../drawio/testdata/relationship_logical.drawio")
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	if doc.IRVersion == "" {
		t.Fatalf("expected non-empty IRVersion")
	}
	totalTables := 0
	for _, d := range doc.Diagrams {
		totalTables += len(d.Tables)
	}
	if totalTables != 6 { // relationshipLogicalTableCount
		t.Fatalf("expected 6 tables total, got %d", totalTables)
	}
	// relationship_logical.drawio는 페이지마다 "고객"/"주문"/"선적"이 두 번씩
	// 등장하지 않고(고객 vs 고객2), 중복 이름 사례가 아니다.
	if len(dups) != 0 {
		t.Fatalf("expected no duplicate-name warnings, got %v", dups)
	}
}

func TestAssemble_DuplicateNameAcrossPages(t *testing.T) {
	doc, dups, err := Assemble("../drawio/testdata/entity_table_basic.drawio")
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	_ = doc
	// entity_table_basic.drawio는 실제로는 3페이지다(테이블 2개는
	// 페이지-1에만 있고, 페이지-2/3엔 관계선/앵커 도형뿐). 테이블 이름이
	// 겹치는 페이지가 없어 중복이 없어야 정상이다.
	// (다중 페이지에 동일 이름 테이블이 있는 픽스처가 생기면 이 테스트를
	// 그 파일로 바꿔 dups가 1건 이상임을 확인한다.)
	if len(dups) != 0 {
		t.Fatalf("expected no duplicates in single-page fixture, got %v", dups)
	}
}

// TestAssemble_StructuralCellsNotShapeViolations는 I4 회귀 테스트다:
// draw.io의 필수 구조 셀(id="0" 루트, id="1" 기본 부모)은 style 속성이
// 아예 없어 예전엔 ClassifyShape이 ShapeClassViolation으로 오판했다 —
// 매 페이지마다 가짜 shape_violation 2건이 검증 리포트 맨 위에 뜨는
// 원인이었다.
func TestAssemble_StructuralCellsNotShapeViolations(t *testing.T) {
	doc, _, err := Assemble("../drawio/testdata/entity_table_basic.drawio")
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	for _, d := range doc.Diagrams {
		for _, v := range d.Violations {
			if v.ID == "0" || v.ID == "1" {
				t.Fatalf("expected draw.io's mandatory structural cell (id=%s) to never be a shape_violation, got %+v on page %q", v.ID, v, d.Name)
			}
		}
	}
}

func TestWrite_ProducesValidJSON(t *testing.T) {
	doc, _, err := Assemble("../drawio/testdata/rowspan_example.xml")
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	out := filepath.Join(t.TempDir(), "ir.json")
	if err := Write(doc, out); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	var round map[string]any
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("written file is not valid JSON: %v", err)
	}
	if _, ok := round["ir_version"]; !ok {
		t.Fatalf("expected ir_version key in written JSON")
	}
}

// fixtureDuplicateNames는 이름이 겹치는 테이블이 «한 페이지 안»에도
// «페이지 사이»에도 있는 파일이다. 고객은 5곳(A에 3, B에 2), 주문은
// 2곳(A·B에 하나씩)에 있고, 페이지는 둘뿐이다.
const fixtureDuplicateNames = `<mxfile>
<diagram name="A" id="pgA"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <mxCell id="a1" value="고객" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"><mxGeometry x="0" y="0" width="200" height="30" as="geometry"/></mxCell>
  <mxCell id="a2" value="고객" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"><mxGeometry x="220" y="0" width="200" height="30" as="geometry"/></mxCell>
  <mxCell id="a3" value="고객" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"><mxGeometry x="440" y="0" width="200" height="30" as="geometry"/></mxCell>
  <mxCell id="a4" value="주문" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"><mxGeometry x="0" y="60" width="200" height="30" as="geometry"/></mxCell>
</root></mxGraphModel></diagram>
<diagram name="B" id="pgB"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <mxCell id="b1" value="고객" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"><mxGeometry x="0" y="0" width="200" height="30" as="geometry"/></mxCell>
  <mxCell id="b2" value="고객" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"><mxGeometry x="220" y="0" width="200" height="30" as="geometry"/></mxCell>
  <mxCell id="b3" value="주문" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"><mxGeometry x="0" y="60" width="200" height="30" as="geometry"/></mxCell>
</root></mxGraphModel></diagram>
</mxfile>`

func writeFixture(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "dup.drawio")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

// PageNames는 «페이지»여야 한다 — 테이블 하나마다 페이지 이름을 밀어
// 넣으면 같은 페이지 이름이 여러 번 들어가고, 그것을 세는 쪽
// (validate.Required)이 «N개 페이지»라고 말하면 파일에 없는 숫자가
// 사용자에게 보인다. 몇 «곳»에 있는지는 Occurrences가 따로 든다.
func TestAssemble_DuplicateNameCountsPagesNotOccurrences(t *testing.T) {
	_, dups, err := Assemble(writeFixture(t, fixtureDuplicateNames))
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	byName := map[string]DuplicateNameWarning{}
	for _, d := range dups {
		byName[d.TableName] = d
	}
	if len(dups) != 2 {
		t.Fatalf("중복 이름이 %d개다; 고객·주문 2개여야 한다: %+v", len(dups), dups)
	}

	got := byName["고객"]
	if want := []string{"A", "B"}; !reflect.DeepEqual(got.PageNames, want) {
		t.Errorf("고객 PageNames=%v; %v여야 한다 — 페이지는 둘뿐이다", got.PageNames, want)
	}
	if got.Occurrences != 5 {
		t.Errorf("고객 Occurrences=%d; 5여야 한다(A에 3, B에 2)", got.Occurrences)
	}

	got = byName["주문"]
	if want := []string{"A", "B"}; !reflect.DeepEqual(got.PageNames, want) {
		t.Errorf("주문 PageNames=%v; %v여야 한다", got.PageNames, want)
	}
	if got.Occurrences != 2 {
		t.Errorf("주문 Occurrences=%d; 2여야 한다", got.Occurrences)
	}
}

// dups의 순서는 실행마다 같아야 한다. 예전에는 map을 그대로 순회해
// 만들었다 — 중복 이름이 둘 이상이면 실행마다 순서가 갈리고, 그 순서가
// validate.Required의 finding 순서 → 검증 리포트 줄 순서 → annotate가
// 요약 박스에 쓰는 줄 순서로 그대로 내려간다. annotate를 두 번 돌려도
// 같은 바이트가 나와야 한다는 Phase 3의 계약이 거기서 깨진다.
func TestAssemble_DuplicateWarningsAreDeterministic(t *testing.T) {
	path := writeFixture(t, fixtureDuplicateNames)

	var first []string
	for i := 0; i < 50; i++ {
		_, dups, err := Assemble(path)
		if err != nil {
			t.Fatalf("Assemble: %v", err)
		}
		names := make([]string, 0, len(dups))
		for _, d := range dups {
			names = append(names, d.TableName)
		}
		if first == nil {
			first = names
			continue
		}
		if !reflect.DeepEqual(names, first) {
			t.Fatalf("%d회차 순서가 다르다: %v vs %v", i, names, first)
		}
	}
}
