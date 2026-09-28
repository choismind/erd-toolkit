package annotate

import (
	"testing"

	"erdtool/internal/drawio"
)

// 브리프가 준 두 테스트: 마크·BaseStyle·요약 박스를 찾아내는지, 깨끗한
// 파일에서는 아무것도 안 잡히는지.
func TestScanExistingFindsMarksAndBaseStyles(t *testing.T) {
	src := []byte(`<mxfile><diagram name="p" id="pg1"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <object label="주문" erdtoolIssue="PK가 없음" erdtoolBaseStyle="shape=table;" id="t1">
    <mxCell style="shape=table;strokeColor=#FF3333;" vertex="1" parent="1">
      <mxGeometry x="80" y="40" width="200" height="100" as="geometry"/>
    </mxCell>
  </object>
  <object label="요약" erdtoolAnnotation="page-summary" id="erdtool-summary-pg1">
    <mxCell style="shape=note;" vertex="1" parent="1">
      <mxGeometry x="320" y="40" width="260" height="60" as="geometry"/>
    </mxCell>
  </object>
</root></mxGraphModel></diagram></mxfile>`)

	perPage, err := ScanExisting(src)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	got := perPage[0]
	if !got.Marked["t1"] {
		t.Error("t1이 마크된 셀로 안 잡혔다")
	}
	if got.BaseStyle["t1"] != "shape=table;" {
		t.Errorf("t1의 BaseStyle=%q; \"shape=table;\"이어야 한다", got.BaseStyle["t1"])
	}
	if !got.Summaries["erdtool-summary-pg1"] {
		t.Error("요약 박스가 안 잡혔다")
	}
}

func TestScanExistingOnCleanFile(t *testing.T) {
	src := []byte(`<mxfile><diagram name="p" id="pg1"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
</root></mxGraphModel></diagram></mxfile>`)
	perPage, err := ScanExisting(src)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	got := perPage[0]
	if len(got.Marked) != 0 || len(got.BaseStyle) != 0 || len(got.Summaries) != 0 {
		t.Errorf("깨끗한 파일인데 표식이 잡혔다: %+v", got)
	}
}

// 실제 .drawio 페이지는 대개 평문이 아니라 deflate+base64로 압축돼 있다.
// ForEachGraphModel이 그것을 풀지 않으면 ScanExisting은 실사용 파일에서
// 항상 빈 결과를 내고, --clean이 "성공"만 보고하는 아무 일도 안 하는
// 명령이 된다. 여기서 drawio.Compress로 실제 압축 바이트를 만들어 그
// 경로를 태운다.
func TestScanExistingHandlesCompressedPages(t *testing.T) {
	plain := `<mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <object label="주문" erdtoolIssue="PK가 없음" erdtoolBaseStyle="shape=table;" id="t1">
    <mxCell style="shape=table;strokeColor=#FF3333;" vertex="1" parent="1">
      <mxGeometry x="80" y="40" width="200" height="100" as="geometry"/>
    </mxCell>
  </object>
</root></mxGraphModel>`

	packed, err := drawio.Compress(plain)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	src := []byte(`<mxfile compressed="true"><diagram name="p" id="pg1">` + packed + `</diagram></mxfile>`)

	perPage, err := ScanExisting(src)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	got := perPage[0]
	if !got.Marked["t1"] {
		t.Error("압축된 페이지에서 t1이 마크된 셀로 안 잡혔다 — 압축을 안 풀고 읽은 것으로 보인다")
	}
	if got.BaseStyle["t1"] != "shape=table;" {
		t.Errorf("압축된 페이지의 t1 BaseStyle=%q; \"shape=table;\"이어야 한다", got.BaseStyle["t1"])
	}
}

// 파서(drawio.LoadDiagrams)는 <object>와 <UserObject>를 똑같이 «id를 가진
// 셀»로 다룬다(internal/drawio/testdata/object_wrapped_table.drawio가 두
// 모양을 다 가진 픽스처다). ScanExisting이 <object>만 보면 UserObject에
// 붙은 마크는 안 보이는 셀이 되어, annotate가 이미 마크된 셀을 다시
// 마크하려 들고 --clean은 그 흔적을 못 지운다.
func TestScanExistingFindsUserObjectMarks(t *testing.T) {
	src := []byte(`<mxfile><diagram name="p" id="pg1"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <UserObject label="UserObject테이블" erdtoolIssue="PK가 없음" erdtoolBaseStyle="shape=table;" id="userobjwrap-table-1">
    <mxCell style="shape=table;strokeColor=#FF3333;" vertex="1" parent="1">
      <mxGeometry x="280" y="40" width="180" height="90" as="geometry"/>
    </mxCell>
  </UserObject>
</root></mxGraphModel></diagram></mxfile>`)

	perPage, err := ScanExisting(src)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	got := perPage[0]
	if !got.Marked["userobjwrap-table-1"] {
		t.Error("UserObject로 감싼 셀의 마크가 안 잡혔다")
	}
	if got.BaseStyle["userobjwrap-table-1"] != "shape=table;" {
		t.Errorf("UserObject의 BaseStyle=%q; \"shape=table;\"이어야 한다", got.BaseStyle["userobjwrap-table-1"])
	}
}

// applyAttrs(Task 8)는 값이 빈 속성을 아예 제거한다 — 그래서 --clean이 끝난
// 파일은 erdtoolIssue가 «없는» 상태지, ""로 남지 않는다. 하지만 손으로
// 편집한 파일에는 erdtoolIssue=""가 남을 수 있다. 그것을 "마크됨"으로
// 잘못 읽으면 annotate가 저장된 적 없는 스타일을 복원하려 든다. 존재하되
// 빈 값은 없는 것과 같이 다뤄야 한다.
func TestScanExistingTreatsEmptyAttrAsAbsent(t *testing.T) {
	src := []byte(`<mxfile><diagram name="p" id="pg1"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <object label="주문" erdtoolIssue="" erdtoolBaseStyle="" erdtoolAnnotation="" id="t1">
    <mxCell style="shape=table;" vertex="1" parent="1">
      <mxGeometry x="80" y="40" width="200" height="100" as="geometry"/>
    </mxCell>
  </object>
</root></mxGraphModel></diagram></mxfile>`)

	perPage, err := ScanExisting(src)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	got := perPage[0]
	if got.Marked["t1"] {
		t.Error("빈 erdtoolIssue가 마크로 잡혔다 — 빈 값은 없는 것과 같이 다뤄야 한다")
	}
	if _, ok := got.BaseStyle["t1"]; ok {
		t.Error("빈 erdtoolBaseStyle이 BaseStyle로 잡혔다")
	}
	if got.Summaries["t1"] {
		t.Error("빈 erdtoolAnnotation이 요약 박스로 잡혔다")
	}
}

// 같은 셀 id가 두 페이지에 있으면 저장된 원래 스타일도 페이지마다 다르다.
// 파일 전체를 맵 하나로 평탄화하면 뒤 페이지 값이 앞 페이지 값을 덮고,
// --clean이 A페이지 셀을 B페이지의 원래 스타일로 «복원»한다. 이 도구는
// 백업을 일부러 안 남기므로 그건 복구 불가능한 손실이다.
func TestScanExistingSplitsByPage(t *testing.T) {
	src := `<mxfile host="test">` +
		`<diagram id="pgA" name="A"><mxGraphModel><root>` +
		`<object id="dup" label="A" ` + AttrIssue + `="진단A" ` + AttrBaseStyle + `="rounded=0;">` +
		`<mxCell vertex="1" parent="1"/></object>` +
		`</root></mxGraphModel></diagram>` +
		`<diagram id="pgB" name="B"><mxGraphModel><root>` +
		`<object id="dup" label="B" ` + AttrIssue + `="진단B" ` + AttrBaseStyle + `="rounded=1;">` +
		`<mxCell vertex="1" parent="1"/></object>` +
		`</root></mxGraphModel></diagram></mxfile>`
	got, err := ScanExisting([]byte(src))
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("페이지 %d개; 2개여야 한다", len(got))
	}
	if got[0].BaseStyle["dup"] != "rounded=0;" {
		t.Errorf("A페이지 BaseStyle = %q; rounded=0;이어야 한다", got[0].BaseStyle["dup"])
	}
	if got[1].BaseStyle["dup"] != "rounded=1;" {
		t.Errorf("B페이지 BaseStyle = %q; rounded=1;이어야 한다 — 페이지가 평탄화됐다", got[1].BaseStyle["dup"])
	}
}

// 압축된 <diagram> 내용이 깨져 있으면 ScanExisting은 에러를 내야 한다.
// 코드로는 옳았지만(convert.ForEachGraphModel이 Decompress 에러를 그대로
// 올린다) 이것을 붙잡는 단언이 하나도 없었다 — 언젠가 누가 «못 읽는
// 페이지는 건너뛰자»로 바꿔도 아무 테스트가 안 걸리고, 그러면 --clean이
// 그 페이지의 표식을 못 본 채 「전부 되돌렸다」고 보고한다.
func TestScanExistingPropagatesCorruptPageError(t *testing.T) {
	src := []byte(`<mxfile><diagram name="p" id="pg1">이건 base64가 아니다</diagram></mxfile>`)

	if _, err := ScanExisting(src); err == nil {
		t.Fatal("에러가 없다 — 못 읽는 페이지를 조용히 건너뛰면 안 된다")
	}
}

// 손상된 페이지가 «둘째»여도 마찬가지다. 첫 페이지를 성공적으로 읽었다고
// 해서 그 뒤의 실패가 묻히면 안 된다.
func TestScanExistingPropagatesCorruptSecondPageError(t *testing.T) {
	src := []byte(`<mxfile>` +
		`<diagram name="a" id="pg1"><mxGraphModel><root>` +
		`<mxCell id="0"/><mxCell id="1" parent="0"/>` +
		`</root></mxGraphModel></diagram>` +
		`<diagram name="b" id="pg2">이건 base64가 아니다</diagram>` +
		`</mxfile>`)

	if _, err := ScanExisting(src); err == nil {
		t.Fatal("에러가 없다 — 둘째 페이지의 실패도 올라와야 한다")
	}
}
