package convert

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"erdtool/internal/drawio"
)

// parseCells는 mxGraphModel XML 조각을 기존 파서로 읽는다. LoadDiagrams가
// 경로를 받으므로 mxfile로 감싸 임시 파일에 쓴다.
func parseCells(t *testing.T, graphModel []byte) []drawio.RawCell {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.drawio")
	content := `<mxfile><diagram name="p" id="p">` + string(graphModel) + `</diagram></mxfile>`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	ds, err := drawio.LoadDiagrams(p)
	if err != nil {
		t.Fatalf("LoadDiagrams: %v", err)
	}
	if len(ds) != 1 {
		t.Fatalf("다이어그램 %d개; 1개여야 한다", len(ds))
	}
	return ds[0].Cells
}

const sampleGraphModel = `<mxGraphModel dx="800" dy="600" grid="1" gridSize="10" 알수없는속성="보존되어야함">
  <root>
    <mxCell id="0"/>
    <mxCell id="1" parent="0"/>
    <mxCell id="t1" value="고객" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1">
      <mxGeometry x="80" y="80" width="240" height="90" as="geometry"/>
    </mxCell>
    <mxCell id="r1" style="shape=tableRow;" vertex="1" parent="t1">
      <mxGeometry y="30" width="240" height="30" as="geometry"/>
    </mxCell>
    <mxCell id="r1k" value="PK" style="shape=partialRectangle;" vertex="1" parent="r1">
      <mxGeometry width="30" height="30" as="geometry">
        <mxRectangle width="30" height="30" as="alternateBounds"/>
      </mxGeometry>
    </mxCell>
    <mxCell id="r1d" value="고객번호 int" style="shape=partialRectangle;" vertex="1" parent="r1">
      <mxGeometry x="30" width="210" height="30" as="geometry"/>
    </mxCell>
    <mxCell id="e1" style="edgeStyle=entityRelationEdgeStyle;endArrow=ERone;" edge="1" parent="1" source="r1" target="r1">
      <mxGeometry relative="1" as="geometry">
        <Array as="points">
          <mxPoint x="10" y="20"/>
        </Array>
      </mxGeometry>
    </mxCell>
  </root>
</mxGraphModel>`

func TestRewriteCells_NoEditsPreservesEverything(t *testing.T) {
	out, err := rewriteCells([]byte(sampleGraphModel), nil)
	if err != nil {
		t.Fatalf("rewriteCells: %v", err)
	}

	want := parseCells(t, []byte(sampleGraphModel))
	got := parseCells(t, out)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("셀이 달라졌다\ngot  %+v\nwant %+v", got, want)
	}
}

func TestRewriteCells_NoEditsKeepsUnmodeledContent(t *testing.T) {
	out, err := rewriteCells([]byte(sampleGraphModel), nil)
	if err != nil {
		t.Fatalf("rewriteCells: %v", err)
	}
	s := string(out)
	for _, needle := range []string{
		`알수없는속성="보존되어야함"`,
		`<Array as="points">`,
		`<mxPoint x="10" y="20">`,
		`<mxRectangle width="30" height="30" as="alternateBounds">`,
		`gridSize="10"`,
	} {
		if !contains(s, needle) {
			t.Errorf("사라진 내용: %s\n출력:\n%s", needle, s)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}

// TestRewriteCells_AllFixturesPreserved는 저장소의 실제 픽스처를 전부
// 무변환 재작성해 셀이 하나도 달라지지 않는지 본다. 손으로 만든 샘플만으로는
// 실제 draw.io가 쓰는 구조를 다 덮지 못한다.
func TestRewriteCells_AllFixturesPreserved(t *testing.T) {
	paths, err := filepath.Glob("../drawio/testdata/*.drawio")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("픽스처를 하나도 못 찾았다")
	}

	for _, p := range paths {
		t.Run(filepath.Base(p), func(t *testing.T) {
			want, err := drawio.LoadDiagrams(p)
			if err != nil {
				t.Skipf("원본이 파싱되지 않는 픽스처다: %v", err)
			}

			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			out, err := rewriteMxFile(data, nil)
			if err != nil {
				t.Fatalf("RewriteMxFile: %v", err)
			}

			dst := filepath.Join(t.TempDir(), filepath.Base(p))
			if err := os.WriteFile(dst, out, 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			got, err := drawio.LoadDiagrams(dst)
			if err != nil {
				t.Fatalf("재작성 결과가 파싱되지 않는다: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("셀이 달라졌다\ngot  %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestRewriteCells_WrapsPlainCell(t *testing.T) {
	out, err := rewriteCells([]byte(sampleGraphModel), map[string]Edit{
		"t1": {Label: "CUST", LogicalName: "고객"},
	})
	if err != nil {
		t.Fatalf("rewriteCells: %v", err)
	}
	s := string(out)

	if !contains(s, `<object label="CUST"`) {
		t.Fatalf("<object> 래핑이 없다:\n%s", s)
	}
	if !contains(s, `logicalName="고객"`) {
		t.Fatalf("logicalName이 없다:\n%s", s)
	}

	cells := parseCells(t, out)
	var found bool
	for _, c := range cells {
		if c.ID != "t1" {
			continue
		}
		found = true
		if c.Value != "CUST" {
			t.Fatalf("t1.Value = %q; want CUST", c.Value)
		}
		if c.Style != "shape=table;childLayout=tableLayout;" {
			t.Fatalf("t1.Style이 바뀌었다: %q", c.Style)
		}
		if c.Parent != "1" {
			t.Fatalf("t1.Parent = %q; want 1", c.Parent)
		}
	}
	if !found {
		t.Fatal("래핑 후 t1을 파서가 못 찾는다")
	}
}

func TestRewriteCells_UpdatesExistingObject(t *testing.T) {
	src := `<mxGraphModel><root>
  <mxCell id="0"/>
  <mxCell id="1" parent="0"/>
  <object label="고객" placeholders="1" id="t1">
    <mxCell style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"/>
  </object>
</root></mxGraphModel>`

	out, err := rewriteCells([]byte(src), map[string]Edit{
		"t1": {Label: "CUST", LogicalName: "고객"},
	})
	if err != nil {
		t.Fatalf("rewriteCells: %v", err)
	}
	s := string(out)

	if !contains(s, `placeholders="1"`) {
		t.Fatalf("기존 <object>의 다른 속성이 사라졌다:\n%s", s)
	}
	if !contains(s, `label="CUST"`) {
		t.Fatalf("label이 안 바뀌었다:\n%s", s)
	}
	if !contains(s, `logicalName="고객"`) {
		t.Fatalf("logicalName이 안 붙었다:\n%s", s)
	}
	if contains(s, `<object`) && contains(s, `<object label="고객"`) {
		t.Fatalf("옛 label이 남아 있다:\n%s", s)
	}
}

func TestRewriteCells_EditsOnlyTargets(t *testing.T) {
	out, err := rewriteCells([]byte(sampleGraphModel), map[string]Edit{
		"r1d": {Label: "CUST_NO int", LogicalName: "고객번호"},
	})
	if err != nil {
		t.Fatalf("rewriteCells: %v", err)
	}
	cells := parseCells(t, out)
	for _, c := range cells {
		switch c.ID {
		case "r1d":
			if c.Value != "CUST_NO int" {
				t.Fatalf("r1d.Value = %q; want 'CUST_NO int'", c.Value)
			}
		case "t1":
			if c.Value != "고객" {
				t.Fatalf("대상이 아닌 t1이 바뀌었다: %q", c.Value)
			}
		case "r1k":
			if c.Value != "PK" {
				t.Fatalf("키 셀이 바뀌었다: %q", c.Value)
			}
		}
	}
}

// TestRewriteCells_MultipleEditsStayNested는 한 파일에 편집 대상이 둘 이상일
// 때 새로 연 <object>가 «감싼 mxCell의 끝»에서 닫히는지 본다. 형제 요소로
// 새면 XML 자체가 깨지므로 파서가 먼저 터진다.
//
// t1은 자식(mxGeometry)을 가진 셀이고 r1d는 t1의 손자다 — 깊이가 다른 둘을
// 한꺼번에 태워야 closeObjectAt의 깊이 키가 실제로 검증된다.
func TestRewriteCells_MultipleEditsStayNested(t *testing.T) {
	out, err := rewriteCells([]byte(sampleGraphModel), map[string]Edit{
		"t1":  {Label: "CUST", LogicalName: "고객"},
		"r1d": {Label: "CUST_NO int", LogicalName: "고객번호"},
	})
	if err != nil {
		t.Fatalf("rewriteCells: %v", err)
	}

	cells := parseCells(t, out)
	got := map[string]drawio.RawCell{}
	for _, c := range cells {
		got[c.ID] = c
	}

	for id, want := range map[string]string{
		"t1":  "CUST",
		"r1d": "CUST_NO int",
		"r1k": "PK",
	} {
		c, ok := got[id]
		if !ok {
			t.Fatalf("%s를 못 찾는다:\n%s", id, out)
		}
		if c.Value != want {
			t.Errorf("%s.Value = %q; want %q", id, c.Value, want)
		}
	}

	// 부모 관계가 그대로여야 <object> 래핑이 트리를 건드리지 않은 것이다.
	if got["r1"].Parent != "t1" {
		t.Errorf("r1.Parent = %q; want t1", got["r1"].Parent)
	}
	if got["r1d"].Parent != "r1" {
		t.Errorf("r1d.Parent = %q; want r1", got["r1d"].Parent)
	}
}

// TestRewritePlanKeepsLabelUntouchedWhenNil은 CellEdit.Label이 nil이면
// value/label을 건드리지 않고 커스텀 속성만 더하는지 본다. annotate가
// 결함 표식을 붙일 때 label을 건드리면 안 되므로 이 구별이 핵심이다.
func TestRewritePlanKeepsLabelUntouchedWhenNil(t *testing.T) {
	src := []byte(sampleGraphModel)
	out, err := RewriteCellsPlan(src, PagePlan{
		Edits: map[string]CellEdit{
			"t1": {Attrs: map[string]string{"erdtoolIssue": "PK가 없음"}},
		},
	})
	if err != nil {
		t.Fatalf("RewriteCellsPlan: %v", err)
	}
	cells := parseCells(t, out)
	idx := drawio.BuildIndex(cells)
	if got := idx.ByID["t1"].Value; got != "고객" {
		t.Errorf("value=%q; \"고객\"이 그대로여야 한다", got)
	}
	if !strings.Contains(string(out), `erdtoolIssue="PK가 없음"`) {
		t.Errorf("erdtoolIssue 속성이 없다:\n%s", out)
	}
}

// TestRewritePlanReplacesStyleOnPlainCell은 감싸이지 않은 맨 mxCell의
// style이 CellEdit.Style로 바뀌는지 본다.
func TestRewritePlanReplacesStyleOnPlainCell(t *testing.T) {
	newStyle := "shape=table;childLayout=tableLayout;strokeColor=#FF0000;"
	out, err := RewriteCellsPlan([]byte(sampleGraphModel), PagePlan{
		Edits: map[string]CellEdit{"t1": {Style: &newStyle}},
	})
	if err != nil {
		t.Fatalf("RewriteCellsPlan: %v", err)
	}
	idx := drawio.BuildIndex(parseCells(t, out))
	if got := drawio.ParseStyle(idx.ByID["t1"].Style)["strokeColor"]; got != "#FF0000" {
		t.Errorf("strokeColor=%q; \"#FF0000\"이어야 한다 (style=%q)", got, idx.ByID["t1"].Style)
	}
	// Style만 있고 Label도 Attrs도 없으면 <object>로 감싸지 않아야 한다
	// (작업 13의 --clean이 원본 바이트를 복원해야 하므로, 불필요한 래퍼가
	// 생기면 안 된다). 파서는 감싼 것과 안 감싼 것을 같은 인덱스 셀로
	// 평탄화하므로, 위 strokeColor 검사만으로는 이 불변식이 지켜지는지
	// 알 수 없다 — 출력 바이트를 직접 봐야 한다.
	if strings.Contains(string(out), "<object") {
		t.Errorf("style만 바뀐 셀이 <object>로 감싸였다:\n%s", out)
	}
	if !strings.Contains(string(out), `id="t1"`) {
		t.Errorf("t1의 id가 mxCell에 남아 있지 않다(감싸였다는 뜻):\n%s", out)
	}
}

// TestRewritePlanReplacesStyleAndWrapsWithLabel은 Style과 Label을 함께
// 편집하는 경우 — annotate가 결함 표식(빨간 테두리)과 함께 label도 바꾸는
// 시나리오 — 를 본다. 이 경로는 새로 <object>로 감싸면서 동시에 style도
// 갈아 끼워야 한다. 감싸는 코드와 style 교체 코드가 둘 다 맞아야 통과한다.
func TestRewritePlanReplacesStyleAndWrapsWithLabel(t *testing.T) {
	newStyle := "shape=table;childLayout=tableLayout;strokeColor=#FF0000;"
	newLabel := "CUST"
	out, err := RewriteCellsPlan([]byte(sampleGraphModel), PagePlan{
		Edits: map[string]CellEdit{"t1": {Style: &newStyle, Label: &newLabel}},
	})
	if err != nil {
		t.Fatalf("RewriteCellsPlan: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `<object label="CUST"`) {
		t.Fatalf("<object> 래핑이 없다:\n%s", s)
	}
	idx := drawio.BuildIndex(parseCells(t, out))
	if got := drawio.ParseStyle(idx.ByID["t1"].Style)["strokeColor"]; got != "#FF0000" {
		t.Errorf("감싸며 바뀐 셀의 strokeColor=%q; \"#FF0000\"이어야 한다 (style=%q)", got, idx.ByID["t1"].Style)
	}
	if idx.ByID["t1"].Value != "CUST" {
		t.Errorf("label이 안 바뀌었다: %q", idx.ByID["t1"].Value)
	}
}

// <object>로 이미 감싸인 셀은 style이 안쪽 mxCell에 있다. 래퍼만 보고
// 지나가면 스타일이 조용히 안 바뀐다.
func TestRewritePlanReplacesStyleInsideWrapper(t *testing.T) {
	src := []byte(`<mxGraphModel><root>
  <mxCell id="0"/>
  <mxCell id="1" parent="0"/>
  <object label="고객" logicalName="고객" id="t1">
    <mxCell style="shape=table;childLayout=tableLayout;" vertex="1" parent="1">
      <mxGeometry x="80" y="80" width="240" height="90" as="geometry"/>
    </mxCell>
  </object>
</root></mxGraphModel>`)
	newStyle := "shape=table;childLayout=tableLayout;strokeColor=#FF0000;"
	out, err := RewriteCellsPlan(src, PagePlan{
		Edits: map[string]CellEdit{"t1": {Style: &newStyle}},
	})
	if err != nil {
		t.Fatalf("RewriteCellsPlan: %v", err)
	}
	idx := drawio.BuildIndex(parseCells(t, out))
	if got := drawio.ParseStyle(idx.ByID["t1"].Style)["strokeColor"]; got != "#FF0000" {
		t.Errorf("감싸인 셀의 strokeColor=%q; \"#FF0000\"이어야 한다", got)
	}
	// 래퍼의 기존 커스텀 속성은 살아 있어야 한다.
	if !strings.Contains(string(out), `logicalName="고객"`) {
		t.Errorf("logicalName이 사라졌다:\n%s", out)
	}
}

// TestRewritePlanInsertsCellIntoNamedDiagram은 Inserts가 diagram id로 갈려
// «그 페이지»에만 들어가는지 본다. 여러 페이지짜리 파일에서 pg1에 건 삽입이
// pg2로 새면 안 된다 — Inserts가 diagram id로 걸리는 이유 자체가 이것이다.
func TestRewritePlanInsertsCellIntoNamedDiagram(t *testing.T) {
	src := []byte(`<mxfile>
  <diagram name="주문" id="pg1"><mxGraphModel><root>
    <mxCell id="0"/><mxCell id="1" parent="0"/>
  </root></mxGraphModel></diagram>
  <diagram name="고객" id="pg2"><mxGraphModel><root>
    <mxCell id="0"/><mxCell id="1" parent="0"/>
  </root></mxGraphModel></diagram>
</mxfile>`)
	out, err := RewriteMxFilePlan(src, RewritePlan{Pages: []PagePlan{
		{DiagramID: "pg1", Inserts: []NewCell{{
			ID: "erdtool-summary-pg1", Value: "PK가 없음",
			Style:  "shape=note;whiteSpace=wrap;html=1;",
			Parent: "1", X: 600, Y: 40, Width: 240, Height: 120,
		}}},
		{DiagramID: "pg2"},
	}})
	if err != nil {
		t.Fatalf("RewriteMxFilePlan: %v", err)
	}
	ds, err := drawio.LoadDiagramsBytes(out)
	if err != nil {
		t.Fatalf("LoadDiagramsBytes: %v", err)
	}
	if _, ok := drawio.BuildIndex(ds[0].Cells).ByID["erdtool-summary-pg1"]; !ok {
		t.Error("pg1에 요약 셀이 없다")
	}
	if _, ok := drawio.BuildIndex(ds[1].Cells).ByID["erdtool-summary-pg1"]; ok {
		t.Error("pg2에 요약 셀이 들어갔다 — 페이지를 가려 넣어야 한다")
	}
}

// TestRewritePlanInsertsReachDiagramWithoutID는 id 없는 <diagram>도 자기
// 몫의 계획을 받는다는 것이다.
//
// 예전에는 반대였다: Inserts가 diagram id로 키를 잡는 맵이었고 빈 문자열이
// «이 페이지에 넣을 것»이라는 예약 키를 겸했기 때문에, id 없는 diagram이
// 아무도 지목하지 않은 삽입을 받는 사고를 막으려고 일부러 걸렀다. 이제
// 페이지를 첨자로 주소하므로 그 겹침 자체가 없고, 거를 이유도 없다.
func TestRewritePlanInsertsReachDiagramWithoutID(t *testing.T) {
	const src = `<mxfile host="test"><diagram name="id없음"><mxGraphModel><root>` +
		`<mxCell id="0"/><mxCell id="1" parent="0"/>` +
		`</root></mxGraphModel></diagram></mxfile>`
	out, err := RewriteMxFilePlan([]byte(src), RewritePlan{Pages: []PagePlan{{
		Inserts: []NewCell{{ID: "n1", Value: "새 셀", Style: "shape=note;", Parent: "1", Width: 10, Height: 10}},
	}}})
	if err != nil {
		t.Fatalf("RewriteMxFilePlan: %v", err)
	}
	if !strings.Contains(string(out), `id="n1"`) {
		t.Errorf("id 없는 diagram이 자기 몫의 삽입을 못 받았다:\n%s", out)
	}
}

// TestRewriteCellsPlanInsertsOnlyIntoRealRoot은 </root>가 닫힐 때마다
// 삽입을 내보내는 게 아니라 «부모가 mxGraphModel인 진짜 root»에서만
// 내보내는지 본다. 이 재작성기는 «모르는 요소는 그대로 흘려보낸다»고
// 약속하므로, 어떤 셀의 자식으로 우연히 이름이 같은 <root>가 들어와도
// 통과시켜야 한다 — 그런데 깊이만 보지 않고 이름까지 보지 않으면 그
// 안쪽 <root/>가 닫힐 때도 진짜 root로 오인해 셀을 한 번 더 내보낸다.
func TestRewriteCellsPlanInsertsOnlyIntoRealRoot(t *testing.T) {
	src := []byte(`<mxGraphModel><root><mxCell id="0"><root/></mxCell></root></mxGraphModel>`)
	out, err := RewriteCellsPlan(src, PagePlan{
		Inserts: []NewCell{{
			ID: "erdtool-summary-x", Value: "요약",
			Style:  "shape=note;whiteSpace=wrap;html=1;",
			Parent: "1", X: 600, Y: 40, Width: 240, Height: 120,
		}},
	})
	if err != nil {
		t.Fatalf("RewriteCellsPlan: %v", err)
	}
	s := string(out)
	n := strings.Count(s, `id="erdtool-summary-x"`)
	if n != 1 {
		t.Fatalf("삽입 셀이 %d번 나왔다; 1번이어야 한다\n%s", n, s)
	}
}

// TestEncodeNewCell_EmitsShapeAndEscapesValue는 삽입 셀이 «id만 있으면
// 그만»이 아니라 실제로 draw.io가 요구하는 모양(vertex="1",
// mxGeometry as="geometry")을 갖추고, Value 안의 <, &, "가 깨진 XML을
// 만들지 않고 이스케이프되는지 본다. drawio.BuildIndex(...).ByID로만
// 확인하면 id가 있다는 사실만 보고 이 모든 걸 놓친다 — 파서가 속성을
// 채워 주는 게 아니라 우리가 낸 바이트에 실제로 있어야 파서가 채울 수
// 있다.
func TestEncodeNewCell_EmitsShapeAndEscapesValue(t *testing.T) {
	src := []byte(`<mxGraphModel><root>
    <mxCell id="0"/><mxCell id="1" parent="0"/>
  </root></mxGraphModel>`)
	out, err := RewriteCellsPlan(src, PagePlan{
		Inserts: []NewCell{{
			ID: "erdtool-summary-y", Value: `A<B & "C"`,
			Style:  "shape=note;whiteSpace=wrap;html=1;",
			Parent: "1", X: 320, Y: 40.5, Width: 240, Height: 120,
		}},
	})
	if err != nil {
		t.Fatalf("RewriteCellsPlan: %v", err)
	}
	s := string(out)

	if !strings.Contains(s, `vertex="1"`) {
		t.Errorf("vertex=\"1\"이 없다:\n%s", s)
	}
	if !strings.Contains(s, `as="geometry"`) {
		t.Errorf("mxGeometry as=\"geometry\"가 없다:\n%s", s)
	}
	// 정수 좌표(320)는 소수점 없이 나와야 멱등성 비교가 맞는다.
	if !strings.Contains(s, `x="320"`) {
		t.Errorf("x=\"320\"이 아니다(소수점이 붙었을 수 있다):\n%s", s)
	}
	if strings.Contains(s, `x="320.0"`) {
		t.Errorf("x가 \"320.0\"으로 나왔다 — formatCoord가 소수점을 안 붙여야 한다:\n%s", s)
	}
	// 소수 좌표는 그대로 보존돼야 한다(반올림해서 정수로 뭉개면 안 된다).
	if !strings.Contains(s, `y="40.5"`) {
		t.Errorf("y=\"40.5\"가 아니다:\n%s", s)
	}
	// value의 <, &, "는 문자열을 이어 붙이면 XML을 깨뜨린다. 인코더가
	// 이스케이프한 결과(&lt; &amp; &quot;)가 바이트에 그대로 있어야
	// «인코더가 이스케이프를 책임진다»는 설계가 실제로 지켜진 것이다.
	for _, esc := range []string{"&lt;", "&amp;", "&#34;"} {
		if !strings.Contains(s, esc) {
			t.Errorf("value 이스케이프 %s가 없다:\n%s", esc, s)
		}
	}
	// 이스케이프 안 된 원본 조각이 그대로 남아 있으면 안 된다 — 이스케이프를
	// 빼먹고 문자열만 심은 것과 결과가 같아 보일 수 있어 위 검사만으로는
	// 부족하다.
	if strings.Contains(s, `value="A<B`) {
		t.Errorf("value가 이스케이프 없이 그대로 새어 나왔다:\n%s", s)
	}

	// 파서로도 왕복해 실제로 읽히는지 확인한다.
	idx := drawio.BuildIndex(parseCells(t, out))
	got, ok := idx.ByID["erdtool-summary-y"]
	if !ok {
		t.Fatal("erdtool-summary-y를 못 찾는다")
	}
	if got.Value != `A<B & "C"` {
		t.Errorf("Value = %q; want `A<B & \"C\"`", got.Value)
	}
}

// TestRemoveAttr_DoesNotMutateInput은 removeAttr가 입력 슬라이스의 백킹
// 배열을 재사용하지 않는지 본다. 재사용하면 반환값은 맞아 보여도 원본을
// 계속 들고 있는 호출자가 밀려난 원소를 보게 된다.
func TestRemoveAttr_DoesNotMutateInput(t *testing.T) {
	attrs := []xml.Attr{
		{Name: xml.Name{Local: "id"}, Value: "t1"},
		{Name: xml.Name{Local: "value"}, Value: "고객"},
		{Name: xml.Name{Local: "style"}, Value: "shape=table;"},
		{Name: xml.Name{Local: "parent"}, Value: "1"},
	}
	before := make([]xml.Attr, len(attrs))
	copy(before, attrs)

	removeAttr(attrs, "value")

	if !reflect.DeepEqual(attrs, before) {
		t.Fatalf("입력 슬라이스가 망가졌다\ngot  %+v\nwant %+v", attrs, before)
	}
}

// mxfileDiagramText는 <diagram>의 텍스트 내용만 본다. 압축 다이어그램은
// 자식 요소 없이 base64 텍스트뿐이므로 이것으로 충분하다.
type mxfileDiagramText struct {
	Compressed string   `xml:"compressed,attr"`
	Diagrams   []string `xml:"diagram"`
}

// TestRewriteMxFile_KeepsCompression은 «압축 여부는 입력을 그대로 따른다»는
// 계약을 묶어 둔다. 픽스처 보존 테스트만으로는 부족하다 — LoadDiagrams가
// 압축과 평문을 둘 다 읽어주므로, 모든 다이어그램을 평문으로 펴 버려도
// 그대로 통과한다.
//
// 압축 픽스처는 두 모양이다: mxfile에 compressed="true"가 붙은 것과, 속성은
// 없는데 내용만 압축된 것. 판정은 속성이 아니라 diagram 안에 요소가 있는지로
// 하므로(captureInner), 둘 다 태운다.
func TestRewriteMxFile_KeepsCompression(t *testing.T) {
	for _, src := range []string{
		"../drawio/testdata/relationship_logical.drawio", // compressed="true"
		"../drawio/testdata/out_of_order_cells.drawio",   // 속성은 없고 내용만 압축
	} {
		t.Run(filepath.Base(src), func(t *testing.T) {
			data, err := os.ReadFile(src)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			out, err := rewriteMxFile(data, nil)
			if err != nil {
				t.Fatalf("RewriteMxFile: %v", err)
			}

			var want, got mxfileDiagramText
			if err := xml.Unmarshal(data, &want); err != nil {
				t.Fatalf("원본 unmarshal: %v", err)
			}
			if err := xml.Unmarshal(out, &got); err != nil {
				t.Fatalf("출력 unmarshal: %v", err)
			}

			if got.Compressed != want.Compressed {
				t.Fatalf("compressed = %q; want %q", got.Compressed, want.Compressed)
			}
			if contains(string(out), "<mxGraphModel") {
				t.Fatalf("평문 mxGraphModel이 출력에 새어 나왔다:\n%s", out)
			}
			if len(got.Diagrams) != len(want.Diagrams) {
				t.Fatalf("다이어그램 %d개; %d개여야 한다", len(got.Diagrams), len(want.Diagrams))
			}

			for i := range want.Diagrams {
				wantPlain, err := drawio.Decompress(strings.TrimSpace(want.Diagrams[i]))
				if err != nil {
					t.Fatalf("원본 diagram %d 압축 해제: %v", i, err)
				}
				gotPlain, err := drawio.Decompress(strings.TrimSpace(got.Diagrams[i]))
				if err != nil {
					t.Fatalf("출력 diagram %d 압축 해제: %v", i, err)
				}
				if !reflect.DeepEqual(parseCells(t, []byte(gotPlain)), parseCells(t, []byte(wantPlain))) {
					t.Fatalf("diagram %d의 셀이 달라졌다\ngot  %s\nwant %s", i, gotPlain, wantPlain)
				}
			}
		})
	}
}

// TestRewritePlanDeletesXMLNestedChildrenNotGraphChildren은 Deletes가 실제로
// 지우는 «자식»의 범위를 못박는다: XML 중첩(mxGeometry 등)은 지워지지만,
// draw.io 그래프에서 parent/source/target으로만 이어진 다른 셀(행, 간선)은
// 지워지지 않고 매달린 참조로 남는다 — 삭제는 XML 트리 구조만 보고 그래프를
// 모르기 때문이다(Deletes 필드 주석 참고). t1을 지워도 t1의 행(r1, r1k, r1d)과
// 그 행을 source/target으로 문 간선(e1)은 그대로다. 그래프 상 자식까지 지우고
// 싶은 호출자는 그 id들도 직접 Deletes에 넣어야 한다 — 이 저장소는 그
// 자동 전파를 하지 않는다.
func TestRewritePlanDeletesXMLNestedChildrenNotGraphChildren(t *testing.T) {
	out, err := RewriteCellsPlan([]byte(sampleGraphModel), PagePlan{
		Deletes: map[string]bool{"t1": true},
	})
	if err != nil {
		t.Fatalf("RewriteCellsPlan: %v", err)
	}
	s := string(out)
	if strings.Contains(s, `id="t1"`) {
		t.Errorf("t1이 안 지워졌다:\n%s", s)
	}
	// t1의 mxGeometry(XML 중첩 자식)는 함께 사라져야 한다.
	if strings.Contains(s, `width="240" height="90"`) {
		t.Errorf("t1의 기하가 남았다:\n%s", s)
	}
	// 손대지 않은 셀은 그대로다.
	if !strings.Contains(s, `id="1"`) {
		t.Errorf("id=1이 사라졌다:\n%s", s)
	}
	// 그래프 상 자식(행)은 XML 중첩이 아니라 parent="t1" 참조로만 이어지므로
	// 지워지지 않는다 — «서브트리 삭제»가 XML 범위로만 정의된다는 사실을
	// 정직하게 드러내는 확인이다(의도된 동작이지 버그가 아니다).
	for _, id := range []string{"r1", "r1k", "r1d"} {
		if !strings.Contains(s, `id="`+id+`"`) {
			t.Errorf("%s가 지워졌다 — Deletes는 그래프를 따라가지 않아야 한다(의도된 동작):\n%s", id, s)
		}
	}
	if !strings.Contains(s, `id="e1"`) {
		t.Errorf("e1(간선)이 지워졌다 — Deletes는 그래프를 따라가지 않아야 한다:\n%s", s)
	}
}

// TestRewritePlanDeletesWrappedCell은 <object id="s1">가 감싼 셀을 지울 때
// 래퍼 태그 자체와 안쪽 mxCell·mxGeometry까지 통째로 사라지는지 본다.
// id는 래퍼에 있고 안쪽 mxCell에는 없으므로, 삭제 판정이 래퍼 요소에서
// 걸려야만(안쪽 mxCell을 우연히 건너뛴 결과가 아니라) 이 테스트가 맞는
// 이유로 통과한다 — 그래서 래퍼 태그 자체와 기하 값을 모두 직접 확인한다.
func TestRewritePlanDeletesWrappedCell(t *testing.T) {
	src := []byte(`<mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <object label="요약" erdtoolAnnotation="page-summary" id="s1">
    <mxCell style="shape=note;" vertex="1" parent="1">
      <mxGeometry x="600" y="40" width="240" height="120" as="geometry"/>
    </mxCell>
  </object>
</root></mxGraphModel>`)
	out, err := RewriteCellsPlan(src, PagePlan{Deletes: map[string]bool{"s1": true}})
	if err != nil {
		t.Fatalf("RewriteCellsPlan: %v", err)
	}
	s := string(out)
	if strings.Contains(s, "page-summary") {
		t.Errorf("감싸인 셀이 안 지워졌다:\n%s", s)
	}
	if strings.Contains(s, "<object") {
		t.Errorf("래퍼 태그 자체가 남았다:\n%s", s)
	}
	if strings.Contains(s, `width="240" height="120"`) {
		t.Errorf("감싸인 셀의 기하가 남았다:\n%s", s)
	}
}

// TestRewritePlanDeletesWrappedUserObjectCell은 위 테스트를 <UserObject>로
// 반복한다. object 갈래만 손보고 UserObject 갈래를 빠뜨리면, 이미 그렇게
// 감싸여 들어온(예: draw.io가 갱신한) 요약 박스가 --clean에서 안 지워지는
// 조용한 실패가 난다.
func TestRewritePlanDeletesWrappedUserObjectCell(t *testing.T) {
	src := []byte(`<mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <UserObject label="요약" erdtoolAnnotation="page-summary" id="s1">
    <mxCell style="shape=note;" vertex="1" parent="1">
      <mxGeometry x="600" y="40" width="240" height="120" as="geometry"/>
    </mxCell>
  </UserObject>
</root></mxGraphModel>`)
	out, err := RewriteCellsPlan(src, PagePlan{Deletes: map[string]bool{"s1": true}})
	if err != nil {
		t.Fatalf("RewriteCellsPlan: %v", err)
	}
	s := string(out)
	if strings.Contains(s, "page-summary") {
		t.Errorf("UserObject로 감싸인 셀이 안 지워졌다:\n%s", s)
	}
	if strings.Contains(s, "<UserObject") {
		t.Errorf("래퍼 태그 자체가 남았다:\n%s", s)
	}
}

// TestRewritePlanDeleteBeatsEdit는 같은 id가 Deletes와 Edits에 동시에
// 걸렸을 때 삭제가 이기는지 본다. annotate가 «이 셀은 지우기로 했는데
// 스타일도 같이 바꾸라는 요청이 남아 있는» 상태로 플랜을 짤 가능성이 있고,
// 그때 편집이 새어 나가 지워졌어야 할 셀이 살아남으면 안 된다.
func TestRewritePlanDeleteBeatsEdit(t *testing.T) {
	newStyle := "shape=table;childLayout=tableLayout;strokeColor=#FF0000;"
	out, err := RewriteCellsPlan([]byte(sampleGraphModel), PagePlan{
		Deletes: map[string]bool{"t1": true},
		Edits:   map[string]CellEdit{"t1": {Style: &newStyle}},
	})
	if err != nil {
		t.Fatalf("RewriteCellsPlan: %v", err)
	}
	s := string(out)
	if strings.Contains(s, `id="t1"`) {
		t.Errorf("삭제와 편집이 겹쳤는데 t1이 남았다(편집이 이겼다):\n%s", s)
	}
	if strings.Contains(s, "#FF0000") {
		t.Errorf("지워졌어야 할 셀의 편집 흔적(스타일)이 남았다:\n%s", s)
	}
}

// TestRewritePlanDeleteDoesNotLeakWrapperIDToUnrelatedSibling은 RewriteCellsPlan의
// StartElement 분기에서 삭제 판정(skipUntil)이 wrapperIDAt 대입보다 먼저
// 걸려야 한다는 순서를 못박는다.
//
// nameAt·wrapperAt와 달리 wrapperIDAt은 «무조건» 다시 쓰이지 않는다 —
// 래퍼(<object>/<UserObject>)가 자기 id를 가졌을 때만 쓰인다. 그러므로
// 지워지는 <object id="s1">가 skip 판정보다 먼저 wrapperIDAt[그 깊이]="s1"을
// 남기고, EndElement의 skip 갈래는 이 값을 정리하지 않은 채 지나가며, 뒤이어
// 같은 깊이에 오는 «id 없는» 형제 <object>는 자기 id가 없어 이 값을 덮어쓰지
// 못한다. 그 결과 형제의 안쪽 mxCell이 insideWrapper 판정에서 enclosingID로
// 남의 것(leaked "s1")을 받아, s1에 걸린 편집을 자기 것인 양 잘못 물려받는다
// — 이 시나리오로 실제 관찰 가능한 오염을 재현해 순서를 고정한다.
func TestRewritePlanDeleteDoesNotLeakWrapperIDToUnrelatedSibling(t *testing.T) {
	src := []byte(`<mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <object label="옛" id="s1">
    <mxCell style="OLD" vertex="1" parent="1"/>
  </object>
  <object label="무명">
    <mxCell id="inner" style="KEEPME" vertex="1" parent="1"/>
  </object>
</root></mxGraphModel>`)
	poison := "POISON"
	out, err := RewriteCellsPlan(src, PagePlan{
		Deletes: map[string]bool{"s1": true},
		Edits:   map[string]CellEdit{"s1": {Style: &poison}},
	})
	if err != nil {
		t.Fatalf("RewriteCellsPlan: %v", err)
	}
	s := string(out)
	if strings.Contains(s, "POISON") {
		t.Errorf("지워진 s1의 편집(스타일)이 무관한 형제 래퍼의 안쪽 셀로 새어 나갔다:\n%s", s)
	}
	if !strings.Contains(s, `id="inner" style="KEEPME"`) {
		t.Errorf("id 없는 형제 래퍼의 안쪽 mxCell 스타일이 손대지 말아야 할 자리에서 바뀌었다:\n%s", s)
	}
}

// TestRewritePlanDeleteThenReinsertSameID는 같은 회차 안에서 어떤 id를
// 지우면서 동시에 같은 id로 새 셀을 넣는 시나리오를 본다. 작업 12가
// «요약 박스를 지우고 다시 넣는다»를 이 방식으로 구현할 예정이라, 삭제는
// 옛 요소를 스트림에서 지나칠 때 일어나고 삽입은 </root>에서 일어나는
// 두 메커니즘이 부딪히지 않고 정확히 한 개의 셀만 남기는지가 멱등성
// 계약(재실행해도 바이트가 같다)의 전제다.
func TestRewritePlanDeleteThenReinsertSameID(t *testing.T) {
	src := []byte(`<mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <mxCell id="s1" value="옛 요약" style="shape=note;" vertex="1" parent="1">
    <mxGeometry x="600" y="40" width="240" height="120" as="geometry"/>
  </mxCell>
</root></mxGraphModel>`)
	out, err := RewriteCellsPlan(src, PagePlan{
		Deletes: map[string]bool{"s1": true},
		Inserts: []NewCell{{
			ID: "s1", Value: "새 요약",
			Style:  "shape=note;whiteSpace=wrap;html=1;",
			Parent: "1", X: 600, Y: 40, Width: 240, Height: 120,
		}},
	})
	if err != nil {
		t.Fatalf("RewriteCellsPlan: %v", err)
	}
	s := string(out)
	if n := strings.Count(s, `id="s1"`); n != 1 {
		t.Fatalf(`id="s1"이 %d번 나왔다; 지우고 다시 넣었으니 1번이어야 한다:%s`, n, s)
	}
	if strings.Contains(s, "옛 요약") {
		t.Errorf("옛 셀이 지워지지 않고 남았다:\n%s", s)
	}
	if !strings.Contains(s, "새 요약") {
		t.Errorf("새 셀이 안 들어갔다:\n%s", s)
	}
}

// TestRewriteMxFile_EditsUserObject는 <UserObject>로 감싸여 들어온 셀에도
// 편집이 실제로 닿는지 본다.
//
// 파서(internal/drawio/xmlraw.go)는 <object>와 <UserObject>를 똑같이
// «id를 가진 셀»로 평탄화한다. 재작성 쪽이 <object>만 보면, 사용자가 그 id에
// 편집을 걸어도 «에러 없이 원본 그대로» 나온다 — 이 저장소가 가장 경계하는
// 조용한 실패다. 그래서 실제 픽스처로 고정한다.
func TestRewriteMxFile_EditsUserObject(t *testing.T) {
	const src = "../drawio/testdata/object_wrapped_table.drawio"

	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// 픽스처에는 <object>로 감싼 테이블과 <UserObject>로 감싼 테이블이 하나씩
	// 있다. 둘을 한꺼번에 걸어 두 갈래가 같은 파일에서 함께 도는 것까지 본다.
	out, err := rewriteMxFile(data, map[string]Edit{
		"objwrap-table-1":     {Label: "OBJ_TBL", LogicalName: "객체테이블"},
		"userobjwrap-table-1": {Label: "USEROBJ_TBL", LogicalName: "UserObject테이블"},
	})
	if err != nil {
		t.Fatalf("RewriteMxFile: %v", err)
	}
	s := string(out)

	// 감싸는 요소 이름은 그대로여야 한다. UserObject를 object로 갈아 끼우거나
	// 바깥에 <object>를 덧씌우면 draw.io가 읽는 모양이 달라진다.
	if !contains(s, `<UserObject `) {
		t.Fatalf("<UserObject>가 사라졌다:\n%s", s)
	}
	if contains(s, `<object label="USEROBJ_TBL"`) {
		t.Fatalf("<UserObject>를 <object>로 덧씌웠다:\n%s", s)
	}
	if !contains(s, `logicalName="UserObject테이블"`) {
		t.Fatalf("UserObject에 logicalName이 안 붙었다:\n%s", s)
	}
	if !contains(s, `logicalName="객체테이블"`) {
		t.Fatalf("object에 logicalName이 안 붙었다:\n%s", s)
	}

	dst := filepath.Join(t.TempDir(), "out.drawio")
	if err := os.WriteFile(dst, out, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	ds, err := drawio.LoadDiagrams(dst)
	if err != nil {
		t.Fatalf("재작성 결과가 파싱되지 않는다: %v", err)
	}
	if len(ds) != 1 {
		t.Fatalf("다이어그램 %d개; 1개여야 한다", len(ds))
	}

	got := map[string]string{}
	for _, c := range ds[0].Cells {
		got[c.ID] = c.Value
	}
	for id, want := range map[string]string{
		"objwrap-table-1":     "OBJ_TBL",
		"userobjwrap-table-1": "USEROBJ_TBL",
		// 대상이 아닌 셀은 그대로여야 한다.
		"userobjwrap-row-1-def": "code varchar",
		"objwrap-row-1-def":     "id int",
	} {
		if got[id] != want {
			t.Errorf("%s.Value = %q; want %q", id, got[id], want)
		}
	}
}

// --- 작업 12 코드리뷰 라운드 1: 벗기기(unwrap) 전용 테스트 -------------
//
// 벗기기는 annotate 전용이 아니라 convert의 재작성 엔진(rewriteStart)
// 자체가 갖는 기능이므로, 계획상 작업 13 몫이던 것을 작업 12가 자기
// 계약(진단이 사라지면 마크·래퍼가 남지 않는다) 때문에 앞당겨 만들었다.
// annotate의 테스트는 findings/BuildPlans/ScanExisting을 거쳐야 이
// 기능에 닿으므로 여기서 벌어지는 구조적 결함(id/value 중복,
// 없던 value 속성이 생김, 남의 래퍼를 구조만 보고 벗김)을
// 못 본다. 그래서 CellEdit.Unwrap을 직접 조립해 이 파일에서 부순다.

// 안쪽 mxCell이 이미 자기 id/value를 갖고 있는 드문 모양
// (<object id="t1"><mxCell id="t1" value="주문" …>)을 벗기면, 먼저
// 지우지 않고 앞에 새로 붙이면 id/value가 두 번 나온다 — XML 1.0
// Unique Attribute Spec 위반이라 draw.io의 DOMParser가 거부하는데, Go의
// encoding/xml 디코더는 조용히 받아준다(그래서 저장소의 어떤 테스트도
// 예전에는 못 잡았다). encodeUnwrappedCellStart가 removeAttr로 먼저
// 지우는지를 문자열에 나타난 "id=" 등장 횟수로 직접 센다 —
// drawio.LoadDiagramsBytes로 왕복하면 중복 속성이 있어도 조용히 읽혀
// 이 결함을 다시 숨긴다.
func TestRewriteCellsPlanUnwrapDoesNotDuplicateIDAndValue(t *testing.T) {
	src := []byte(`<mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <object label="주문" erdtoolIssue="PK가 없음" id="t1">
    <mxCell id="t1" value="주문" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"/>
  </object>
</root></mxGraphModel>`)
	style := "shape=table;childLayout=tableLayout;"
	out, err := RewriteCellsPlan(src, PagePlan{
		Edits: map[string]CellEdit{
			"t1": {Style: &style, Attrs: map[string]string{"erdtoolIssue": ""}, Unwrap: true},
		},
	})
	if err != nil {
		t.Fatalf("RewriteCellsPlan: %v", err)
	}
	s := string(out)
	if contains(s, "<object") || contains(s, "</object>") {
		t.Fatalf("래퍼가 안 벗겨졌다:\n%s", s)
	}
	if n := strings.Count(s, `id="t1"`); n != 1 {
		t.Errorf(`id="t1"이 %d번 나왔다; 1번이어야 한다:%s`, n, s)
	}
	if n := strings.Count(s, `value="주문"`); n != 1 {
		t.Errorf(`value="주문"이 %d번 나왔다; 1번이어야 한다:%s`, n, s)
	}
	// 실제로 다시 파싱되는지도 본다 — 중복 속성은 Go 디코더가 조용히
	// 받아주므로 파싱 성공 자체는 결함의 증거가 못 되지만, 최소한 이
	// 재작성기가 낸 결과가 우리 자신의 파서로도 못 읽는 것은 아니어야
	// 한다.
	idx := drawio.BuildIndex(parseCells(t, out))
	if idx.ByID["t1"].Value != "주문" {
		t.Errorf("t1.Value = %q; want 주문", idx.ByID["t1"].Value)
	}
}

// value 속성이 원래 없던 셀(행·간선처럼)을 감쌌다가 벗기면, value=""를
// 새로 만들어 붙이면 안 된다 — 그 셀은 원래 value 속성 자체가 없었다.
// «없었다»는 래퍼에 label 속성 자체가 없는 것으로 표현한다(아래
// TestRewriteCellsPlanUnwrapKeepsExplicitEmptyValue의 label="" 과
// 구별되는 지점 — 코드리뷰 라운드 2가 이 둘을 갈랐다).
func TestRewriteCellsPlanUnwrapOmitsValueWhenLabelAbsent(t *testing.T) {
	src := []byte(`<mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <object erdtoolIssue="끊어진 참조" id="r1">
    <mxCell style="shape=tableRow;" vertex="1" parent="t1"/>
  </object>
</root></mxGraphModel>`)
	style := "shape=tableRow;"
	out, err := RewriteCellsPlan(src, PagePlan{
		Edits: map[string]CellEdit{
			"r1": {Style: &style, Attrs: map[string]string{"erdtoolIssue": ""}, Unwrap: true},
		},
	})
	if err != nil {
		t.Fatalf("RewriteCellsPlan: %v", err)
	}
	s := string(out)
	if contains(s, "<object") {
		t.Fatalf("래퍼가 안 벗겨졌다:\n%s", s)
	}
	if strings.Contains(s, "value=") {
		t.Errorf("value 속성이 없어야 하는데 생겼다:\n%s", s)
	}
	if !contains(s, `id="r1"`) || !contains(s, `style="shape=tableRow;"`) {
		t.Errorf("id/style이 안 살아있다:\n%s", s)
	}
}

// [코드리뷰 라운드 2에서 다시 나온 결함] value="" 를 원래부터 갖고 있던 셀(값이
// 빈 것과 속성이 없는 것은 다른 사실이다 — 이 저장소의
// fixtureWithMissingPK의 r1k가 실제로 이 모양이다)을 감쌌다가 벗기면,
// value="" 속성이 그대로 살아 있어야 한다. "값이 비었으니 안 낸다"로
// 판정하면(라운드 1의 첫 고침이 정확히 이렇게 했다) 이 흔한 draw.io
// 모양에서 매번 순수 왕복 결과와 달라진다.
func TestRewriteCellsPlanUnwrapKeepsExplicitEmptyValue(t *testing.T) {
	src := []byte(`<mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <object label="" erdtoolIssue="PK가 없음" id="t1">
    <mxCell style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"/>
  </object>
</root></mxGraphModel>`)
	style := "shape=table;childLayout=tableLayout;"
	out, err := RewriteCellsPlan(src, PagePlan{
		Edits: map[string]CellEdit{
			"t1": {Style: &style, Attrs: map[string]string{"erdtoolIssue": ""}, Unwrap: true},
		},
	})
	if err != nil {
		t.Fatalf("RewriteCellsPlan: %v", err)
	}
	s := string(out)
	if contains(s, "<object") {
		t.Fatalf("래퍼가 안 벗겨졌다:\n%s", s)
	}
	if !strings.Contains(s, `value=""`) {
		t.Errorf(`value=""가 사라졌다 — 원래 있던 빈 value 속성은 살아있어야 한다:%s`, s)
	}
	if !contains(s, `id="t1"`) || !contains(s, `style="shape=table;childLayout=tableLayout;"`) {
		t.Errorf("id/style이 안 살아있다:\n%s", s)
	}
}

// [게이트] 구조만(label·id만 남음) 보고 벗기던 옛 방식은 사용자가
// 원래부터 label·id만 가진 빈 래퍼를 쓰고 있었을 때도 똑같이 벗겼다.
// CellEdit.Unwrap을 세우지 않으면 — 즉 호출자가 "이건 내 래퍼다"라고
// 확언하지 않으면 — 구조가 아무리 비어 보여도 절대 안 벗긴다는 것을
// 직접 본다.
func TestRewriteCellsPlanUnwrapRequiresOptIn(t *testing.T) {
	src := []byte(`<mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <object label="주문" id="t1">
    <mxCell style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"/>
  </object>
</root></mxGraphModel>`)
	style := "shape=table;childLayout=tableLayout;strokeColor=#FF0000;"
	out, err := RewriteCellsPlan(src, PagePlan{
		// Unwrap을 세우지 않는다 — 편집 후에도 label·id만 남아 구조상
		// "벗길 수 있어" 보이지만, opt-in이 없으므로 벗기지 않아야 한다.
		Edits: map[string]CellEdit{"t1": {Style: &style}},
	})
	if err != nil {
		t.Fatalf("RewriteCellsPlan: %v", err)
	}
	s := string(out)
	if !contains(s, `<object label="주문"`) {
		t.Errorf("Unwrap:false인데 래퍼가 벗겨졌다:\n%s", s)
	}
}

// Unwrap을 세워도, 편집을 다 적용한 뒤 label·id 말고 다른 속성
// (logicalName처럼 이 편집이 모르는 속성)이 남아 있으면 절대 벗기지
// 않는다 — 호출자의 "이건 내 래퍼다" 판단이 틀렸을 때의 마지막 방어선이다.
func TestRewriteCellsPlanUnwrapRefusesWhenOtherAttrsRemain(t *testing.T) {
	src := []byte(`<mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <object label="주문" logicalName="주문번호" erdtoolIssue="PK가 없음" id="t1">
    <mxCell style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"/>
  </object>
</root></mxGraphModel>`)
	style := "shape=table;childLayout=tableLayout;"
	out, err := RewriteCellsPlan(src, PagePlan{
		Edits: map[string]CellEdit{
			"t1": {Style: &style, Attrs: map[string]string{"erdtoolIssue": ""}, Unwrap: true},
		},
	})
	if err != nil {
		t.Fatalf("RewriteCellsPlan: %v", err)
	}
	s := string(out)
	if !contains(s, "<object") {
		t.Fatalf("logicalName이 남아 있는데 래퍼가 벗겨졌다:\n%s", s)
	}
	if !contains(s, `logicalName="주문번호"`) {
		t.Errorf("logicalName이 사라졌다:\n%s", s)
	}
	if contains(s, "erdtoolIssue") {
		t.Errorf("erdtoolIssue는 지워졌어야 한다:\n%s", s)
	}
}

// [다운그레이드] 벗기기는 원래 순서를 저장해 두지 않으므로 draw.io의
// 관례(id, value, style, ...)를 벗어난 입력에서는 속성 "순서"가 원본과
// 달라질 수 있다 — 이건 의도적으로 안 고친 한계다(encodeUnwrappedCellStart
// 주석 참고). 이 테스트는 순서가 아니라 "집합과 값"만 못박는다: style이
// parent보다 앞에, value 관련 정보가 없는 자리에 있는 비정형 순서를
// 넣고, 벗긴 결과에 원래 속성이 전부(정확한 값으로) 남아 있는지만 본다.
func TestRewriteCellsPlanUnwrapPreservesAttributeSetRegardlessOfOrder(t *testing.T) {
	src := []byte(`<mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <object label="주문" erdtoolIssue="PK가 없음" id="t1">
    <mxCell style="shape=table;childLayout=tableLayout;" parent="1" vertex="1"/>
  </object>
</root></mxGraphModel>`)
	style := "shape=table;childLayout=tableLayout;"
	out, err := RewriteCellsPlan(src, PagePlan{
		Edits: map[string]CellEdit{
			"t1": {Style: &style, Attrs: map[string]string{"erdtoolIssue": ""}, Unwrap: true},
		},
	})
	if err != nil {
		t.Fatalf("RewriteCellsPlan: %v", err)
	}
	s := string(out)
	if contains(s, "<object") {
		t.Fatalf("래퍼가 안 벗겨졌다:\n%s", s)
	}
	for _, want := range []string{`id="t1"`, `value="주문"`, `style="shape=table;childLayout=tableLayout;"`, `parent="1"`, `vertex="1"`} {
		if !contains(s, want) {
			t.Errorf("속성이 사라졌다(%s 없음):\n%s", want, s)
		}
	}
	idx := drawio.BuildIndex(parseCells(t, out))
	got := idx.ByID["t1"]
	if got.Value != "주문" || got.Style != "shape=table;childLayout=tableLayout;" || got.Parent != "1" {
		t.Errorf("벗긴 뒤 값이 달라졌다: %+v", got)
	}
}

// applyAttrs의 sort.Strings(names)는 장식이 아니라 계약이다. annotate는
// 셀 하나에 커스텀 속성을 셋(erdtoolIssue·erdtoolBaseStyle·erdtoolWrapped)
// 한꺼번에 얹는데, 그 셋은 map으로 온다 — Go의 map 순회 순서는 실행마다
// 무작위이므로 정렬을 빼면 같은 입력에 서로 다른 바이트가 나온다(전체
// 리뷰: 정렬을 지우고 한 프로세스에서 Annotate를 50번 돌렸더니 서로 다른
// 산출물이 3종 나왔다). «두 번 돌리면 바이트가 같다»가 annotate의 계약
// 이므로 이건 결함이다.
//
// 한 번만 확인하면 정렬을 지워도 우연히 통과할 확률이 1/6이라 테스트가
// 오히려 거짓 초록을 낸다. 그래서 같은 편집을 여러 번 방출하고 매번
// 순서를 본다 — 정렬이 없으면 (1/6)^N로 사실상 반드시 빨개진다.
func TestApplyAttrsEmitsCustomAttrsInSortedOrder(t *testing.T) {
	src := []byte(`<mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <mxCell id="t1" value="주문" style="shape=table;" vertex="1" parent="1"/>
</root></mxGraphModel>`)
	style := "shape=table;strokeColor=#FF0000;"
	plan := PagePlan{
		Edits: map[string]CellEdit{
			"t1": {Style: &style, Attrs: map[string]string{
				"erdtoolIssue":     "PK가 없음",
				"erdtoolBaseStyle": "shape=table;",
				"erdtoolWrapped":   "1",
			}},
		},
	}
	const runs = 50
	var first string
	for i := 0; i < runs; i++ {
		out, err := RewriteCellsPlan(src, plan)
		if err != nil {
			t.Fatalf("RewriteCellsPlan(%d회차): %v", i, err)
		}
		s := string(out)
		base := strings.Index(s, "erdtoolBaseStyle")
		issue := strings.Index(s, "erdtoolIssue")
		wrapped := strings.Index(s, "erdtoolWrapped")
		if base < 0 || issue < 0 || wrapped < 0 {
			t.Fatalf("%d회차: 커스텀 속성이 안 나왔다:\n%s", i, s)
		}
		if !(base < issue && issue < wrapped) {
			t.Fatalf("%d회차: 커스텀 속성이 사전순으로 안 나왔다(erdtoolBaseStyle < erdtoolIssue < erdtoolWrapped여야 한다):\n%s", i, s)
		}
		if i == 0 {
			first = s
			continue
		}
		if s != first {
			t.Fatalf("%d회차 바이트가 1회차와 다르다 — 같은 입력에 같은 출력이 나와야 한다\n1회차:\n%s\n%d회차:\n%s", i, first, i, s)
		}
	}
}

// twoPageMxfile은 페이지 둘이 같은 셀 id("dup")를 쓰는 파일이다. draw.io가
// 직접 만든 파일에서는 잘 안 생기지만, 손편집·파일 병합·다른 도구 산출물에서
// 실제로 생긴다 — 이 재작성기가 신뢰하지 않기로 한 입력이 정확히 그것이다.
const twoPageMxfile = `<mxfile host="test">` +
	`<diagram id="pgA" name="A"><mxGraphModel><root>` +
	`<mxCell id="0"/><mxCell id="1" parent="0"/>` +
	`<mxCell id="dup" value="A쪽" style="rounded=0;" vertex="1" parent="1"/>` +
	`</root></mxGraphModel></diagram>` +
	`<diagram id="pgB" name="B"><mxGraphModel><root>` +
	`<mxCell id="0"/><mxCell id="1" parent="0"/>` +
	`<mxCell id="dup" value="B쪽" style="rounded=1;" vertex="1" parent="1"/>` +
	`</root></mxGraphModel></diagram></mxfile>`

// 페이지 계획은 «그 페이지에만» 적용된다. 이것이 이 변경 전체의 요점이다.
func TestRewriteMxFilePlanEditsOnlyItsOwnPage(t *testing.T) {
	styleA := "rounded=0;strokeColor=#FF0000;"
	out, err := RewriteMxFilePlan([]byte(twoPageMxfile), RewritePlan{Pages: []PagePlan{
		{DiagramID: "pgA", Edits: map[string]CellEdit{"dup": {Style: &styleA}}},
		{DiagramID: "pgB"},
	}})
	if err != nil {
		t.Fatalf("RewriteMxFilePlan: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `style="rounded=0;strokeColor=#FF0000;"`) {
		t.Errorf("A페이지의 편집이 적용되지 않았다:\n%s", s)
	}
	if !strings.Contains(s, `style="rounded=1;"`) {
		t.Errorf("B페이지의 동명 셀이 함께 바뀌었다 — 페이지 격리가 깨졌다:\n%s", s)
	}
}

// 계획 수가 파일의 <diagram> 수와 다르면 짧은 쪽에 맞춰 잘라 쓰지 않는다.
// 자르는 순간 나머지 페이지가 침묵 속에서 계획을 잃는다.
func TestRewriteMxFilePlanRejectsPageCountMismatch(t *testing.T) {
	_, err := RewriteMxFilePlan([]byte(twoPageMxfile), RewritePlan{Pages: []PagePlan{
		{DiagramID: "pgA"},
	}})
	if err == nil {
		t.Fatal("계획이 1개인데 파일의 <diagram>은 2개다 — 에러여야 한다")
	}
}

// 순서가 한 칸 밀리면 1페이지의 편집이 2페이지에 찍힌다. 그것이 이 설계의
// 최악의 실패이므로 DiagramID를 단언으로 들고 다닌다.
func TestRewriteMxFilePlanRejectsDiagramIDMismatch(t *testing.T) {
	_, err := RewriteMxFilePlan([]byte(twoPageMxfile), RewritePlan{Pages: []PagePlan{
		{DiagramID: "pgB"},
		{DiagramID: "pgA"},
	}})
	if err == nil {
		t.Fatal("계획의 DiagramID가 그 자리 <diagram>과 다르다 — 에러여야 한다")
	}
}

// 빈 계획은 «페이지가 없다»가 아니라 «할 일이 없다»다. 순수 왕복이어야 한다.
func TestRewriteMxFilePlanEmptyPlanIsPureRoundTrip(t *testing.T) {
	once, err := RewriteMxFilePlan([]byte(twoPageMxfile), RewritePlan{})
	if err != nil {
		t.Fatalf("RewriteMxFilePlan: %v", err)
	}
	twice, err := RewriteMxFilePlan(once, RewritePlan{})
	if err != nil {
		t.Fatalf("두 번째: %v", err)
	}
	if string(once) != string(twice) {
		t.Fatalf("순수 왕복이 멱등이 아니다\n1회:\n%s\n2회:\n%s", once, twice)
	}
}

func TestCountDiagrams(t *testing.T) {
	n, err := CountDiagrams([]byte(twoPageMxfile))
	if err != nil {
		t.Fatalf("CountDiagrams: %v", err)
	}
	if n != 2 {
		t.Fatalf("CountDiagrams = %d; want 2", n)
	}
}

// <root>가 없는 mxGraphModel에 Inserts를 주면 넣을 자리가 없다. 예전에는
// 그것을 조용히 버리고 nil 에러를 냈다 — 호출자(annotate)는 「페이지 N개
// 요약」이라고 성공을 보고하는데 파일에는 박스가 없다. 이 저장소가 존재하는
// 이유인 «툴은 성공을 보고하는데 산출물이 기대와 다르다»를 재작성기 자신이
// 저지르는 자리라 시끄럽게 멈춘다.
func TestRewriteCellsPlanFailsWhenNoRootToInsertInto(t *testing.T) {
	src := []byte(`<mxGraphModel dx="1" dy="1"></mxGraphModel>`)
	p := PagePlan{Inserts: []NewCell{{ID: "n1", Style: "shape=note;", Parent: "1"}}}

	if _, err := RewriteCellsPlan(src, p); err == nil {
		t.Fatal("에러가 없다 — 넣을 <root>가 없으면 조용히 버리지 말고 멈춰야 한다")
	}
}

// 반대쪽: Inserts가 없으면 <root>가 없어도 멀쩡한 왕복이어야 한다.
// 위 단언이 «<root> 없는 입력을 전부 거부한다»로 번지면 모르는 내용을
// 그대로 흘려보낸다는 이 재작성기의 계약이 깨진다.
func TestRewriteCellsPlanPassesRootlessInputWithoutInserts(t *testing.T) {
	src := []byte(`<mxGraphModel dx="1" dy="1"></mxGraphModel>`)

	out, err := RewriteCellsPlan(src, PagePlan{})
	if err != nil {
		t.Fatalf("삽입이 없으면 <root>가 없어도 통과해야 한다: %v", err)
	}
	if !strings.Contains(string(out), "mxGraphModel") {
		t.Errorf("내용이 사라졌다: %s", out)
	}
}

// NewCell.Attrs는 «커스텀» 속성만을 위한 것이다. 래퍼의 구조적 속성인
// id와 label을 거기 담으면 encodeNewWrappedCell이 applyAttrs를 id를 붙이기
// «전»에 돌리므로, id가 두 번 실린 <object>가 나가거나(XML은 같은 속성을
// 두 번 못 갖는다) label이 Value 대신 조용히 갈린다. 오늘은 annotate만
// Attrs를 채우고 erdtool* 키만 쓰므로 도달하지 못하지만, 도달하면 조용히
// 깨진 XML이 나가는 자리라 구조적으로 막는다.
func TestNewCellAttrsRejectsStructuralKeys(t *testing.T) {
	src := []byte(`<mxGraphModel><root><mxCell id="0"/><mxCell id="1" parent="0"/></root></mxGraphModel>`)

	for _, key := range []string{"id", "label"} {
		t.Run(key, func(t *testing.T) {
			p := PagePlan{Inserts: []NewCell{{
				ID:     "n1",
				Value:  "본래 값",
				Style:  "shape=note;",
				Parent: "1",
				Attrs:  map[string]string{"erdtoolAnnotation": "summary", key: "가로챈 값"},
			}}}
			out, err := RewriteCellsPlan(src, p)
			if err == nil {
				t.Fatalf("%q를 Attrs에 넣었는데 에러가 없다 — 산출물: %s", key, out)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("에러 문구에 문제의 키 %q가 없다: %v", key, err)
			}
		})
	}
}

// 반대쪽: 평범한 커스텀 속성은 그대로 실려야 한다.
func TestNewCellAttrsCarriesCustomKeys(t *testing.T) {
	src := []byte(`<mxGraphModel><root><mxCell id="0"/><mxCell id="1" parent="0"/></root></mxGraphModel>`)
	p := PagePlan{Inserts: []NewCell{{
		ID: "n1", Value: "본래 값", Style: "shape=note;", Parent: "1",
		Attrs: map[string]string{"erdtoolAnnotation": "summary"},
	}}}

	out, err := RewriteCellsPlan(src, p)
	if err != nil {
		t.Fatalf("평범한 커스텀 속성이 거부됐다: %v", err)
	}
	for _, want := range []string{`erdtoolAnnotation="summary"`, `id="n1"`, `label="본래 값"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("%s가 없다: %s", want, out)
		}
	}
}

// 완전히 빈 CellEdit{}는 «이 셀을 손대지 마라»와 같다. 에러도 아니고
// 부수 효과도 없어야 한다 — 호출자가 셀별로 편집을 조립하다 결과가 비는
// 것은 정상이고(annotate.revertMarkEdit가 저장된 BaseStyle이 없을 때
// Style을 nil로 두는 것이 그 예다), 그때마다 호출자가 맵에서 키를 도로
// 빼게 만들면 «넣을지 말지»의 판단이 두 군데로 갈린다.
//
// 특히 맨 mxCell이 빈 편집 때문에 <object>로 감싸이면 안 된다. 감싸기는
// 실을 커스텀 속성이 있을 때만 뜻이 있고, 빈 래퍼는 draw.io의 «데이터
// 편집» 창에 사용자가 지우지도 못하는 빈 줄을 만든다.
func TestEmptyCellEditIsANoOp(t *testing.T) {
	src := []byte(sampleGraphModel)

	untouched, err := RewriteCellsPlan(src, PagePlan{})
	if err != nil {
		t.Fatalf("빈 계획: %v", err)
	}
	got, err := RewriteCellsPlan(src, PagePlan{Edits: map[string]CellEdit{"t1": {}}})
	if err != nil {
		t.Fatalf("빈 CellEdit: %v", err)
	}

	// 기준선은 원본 바이트가 아니라 «편집이 하나도 없는 왕복»이다 —
	// 재작성기는 편집이 없어도 self-closing 태그를 펼치기 때문이다.
	if !bytes.Equal(untouched, got) {
		t.Errorf("빈 CellEdit가 산출물을 바꿨다:\n편집 없음:\n%s\n빈 편집:\n%s",
			untouched, got)
	}
	if bytes.Contains(got, []byte("<object")) {
		t.Errorf("빈 편집 때문에 맨 mxCell이 <object>로 감싸였다:\n%s", got)
	}
}

// 한 파일 안에 <diagram id>가 겹쳐도 삽입은 첨자가 가리키는 페이지에만
// 들어간다. 옛 구현은 p.Inserts[diagramID]로 «맵 조회»를 해서 겹친 두
// 페이지가 **둘 다** 같은 삽입을 받았다 — 둘 다 틀렸지만 둘 다 뭔가는
// 받았고, 그것이 이월 minor 목록의 «id가 같은 diagram 둘이면 양쪽 다
// inserts를 받는다»였다. 페이지 단위화가 Pages[i] 첨자 조회로
// 바꾸면서 그 항목은 무효가 됐고, 여기서 그 사실을 못박는다.
//
// (한 페이지가 아무것도 못 받는 것 자체는 이 층위에서 고칠 문제가 아니다.
// 뿌리는 annotate.BuildPlans가 findings를 DiagramID로 묶는 단계에서 이미
// 두 페이지를 하나로 합치는 것이고, id가 겹치면 애초에 어느 페이지가
// 어느 페이지인지 구별할 방법이 없다 — 원장의 "다이어그램 id 중복" 절.)
func TestRewriteMxFilePlanInsertsByIndexNotByDuplicateID(t *testing.T) {
	src := []byte(`<mxfile>
  <diagram name="A" id="same"><mxGraphModel><root>
    <mxCell id="0"/><mxCell id="1" parent="0"/>
  </root></mxGraphModel></diagram>
  <diagram name="B" id="same"><mxGraphModel><root>
    <mxCell id="0"/><mxCell id="1" parent="0"/>
  </root></mxGraphModel></diagram>
</mxfile>`)

	out, err := RewriteMxFilePlan(src, RewritePlan{Pages: []PagePlan{
		{DiagramID: "same"},
		{DiagramID: "same", Inserts: []NewCell{{
			ID: "only-on-b", Style: "shape=note;", Parent: "1",
		}}},
	}})
	if err != nil {
		t.Fatalf("RewriteMxFilePlan: %v", err)
	}

	ds, err := drawio.LoadDiagramsBytes(out)
	if err != nil {
		t.Fatalf("LoadDiagramsBytes: %v", err)
	}
	if len(ds) != 2 {
		t.Fatalf("페이지가 %d개다; 2개여야 한다", len(ds))
	}
	if _, ok := drawio.BuildIndex(ds[0].Cells).ByID["only-on-b"]; ok {
		t.Errorf("첫 페이지가 둘째 페이지의 삽입을 받았다 — id가 아니라 첨자로 주소해야 한다:\n%s", out)
	}
	if _, ok := drawio.BuildIndex(ds[1].Cells).ByID["only-on-b"]; !ok {
		t.Errorf("둘째 페이지에 삽입이 안 들어갔다:\n%s", out)
	}
}

// rewriteCells / rewriteMxFile은 페이지 단위화 이전의 옛 API를
// **테스트 안에서만** 되살린 것이다. 예전에는 같은 이름의 공개 함수가
// 있었는데, 생산 코드의 마지막 호출부가 그때 사라졌는데도 남아 있었다 —
// 「파일 전체에 한 벌의 편집을 적용한다」는 뜻은 페이지 단위화가 일부러
// 없앤 것이므로, 그것을 공개 API로 두면 언젠가 누가 다시 그 길로 들어선다.
//
// 그렇다고 지우면 이 파일의 가장 오래된 회귀 테스트들(«맨 mxCell 감싸기»,
// «모르는 내용 그대로 흘려보내기», «빈 편집이면 순수 왕복»)이 함께
// 사라진다. 그래서 함수는 공개 API에서 걷어내고 그 뜻만 여기로 옮겼다.
func rewriteCells(src []byte, edits map[string]Edit) ([]byte, error) {
	return RewriteCellsPlan(src, PagePlan{Edits: cellEditsFrom(edits)})
}

func rewriteMxFile(src []byte, edits map[string]Edit) ([]byte, error) {
	n, err := CountDiagrams(src)
	if err != nil {
		return nil, err
	}
	pages := make([]PagePlan, n)
	for i := range pages {
		// 읽기 전용으로만 쓰이므로 맵을 공유해도 안전하다.
		pages[i] = PagePlan{Edits: cellEditsFrom(edits)}
	}
	return RewriteMxFilePlan(src, RewritePlan{Pages: pages})
}
