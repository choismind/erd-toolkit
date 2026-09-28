// internal/annotate/file_test.go
package annotate

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"erdtool/internal/config"
	"erdtool/internal/convert"
	"erdtool/internal/drawio"
	"erdtool/internal/ir"
	"erdtool/internal/model"
	"erdtool/internal/validate"
)

// fixtureWithMissingPK는 PK 없는 테이블 하나짜리 페이지다.
const fixtureWithMissingPK = `<mxfile><diagram name="주문" id="pg1"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <mxCell id="t1" value="주문" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1">
    <mxGeometry x="80" y="40" width="240" height="60" as="geometry"/>
  </mxCell>
  <mxCell id="r1" style="shape=tableRow;" vertex="1" parent="t1">
    <mxGeometry y="30" width="240" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1k" value="" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry width="30" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1d" value="주문일자 DATE" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry x="30" width="210" height="30" as="geometry"/>
  </mxCell>
</root></mxGraphModel></diagram></mxfile>`

// fixtureLonelyTable은 행이 하나도 없는 테이블 하나짜리 페이지다(작업 16
// 리뷰 라운드 1). fixtureWithMissingPK로는 페이지 레벨 뒤집힘을 검증할 수
// 없다 — 그 픽스처는 Parsed 셀이 4개(t1·r1·r1k·r1d)인데 annotate가 한 번의
// 실행에서 실제로 손대는 셀은 진단 대상 t1과 새로 넣는 요약 박스 1개뿐이라,
// 최악의 경우에도 conceptual은 최대 2까지만 늘고 남은 Parsed 3개
// (annotate가 손대지 않는 r1·r1k·r1d)가 classify.PageLevel의
// "relational >= conceptual" 판정을 항상 지켜준다 — 요약 박스 도형이나 마크
// 스타일이 아무리 망가져도 이 픽스처의 페이지 레벨은 수학적으로 절대 안
// 뒤집힌다.
//
// 이 픽스처는 Parsed 셀이 t1 하나뿐이다(행이 없어 r1·r1k·r1d에 해당하는
// 것이 아예 없다). annotate가 손대는 셀(t1 자신 + 요약 박스 1개)이 곧
// 이 페이지의 Parsed 셀 전부이므로, 요약 박스 도형이 shape=note가 아니게
// 되거나 마크 스타일이 shape=table을 잃으면 conceptual이 relational을
// 실제로 앞질러 페이지 레벨이 뒤집힌다 — TestAnnotatedFileKeepsPageLevel이
// 방어하는 결함이 이 픽스처에서는 진짜로 감지된다.
const fixtureLonelyTable = `<mxfile><diagram name="주문" id="pg1"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <mxCell id="t1" value="주문" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1">
    <mxGeometry x="80" y="40" width="240" height="30" as="geometry"/>
  </mxCell>
</root></mxGraphModel></diagram></mxfile>`

// fixtureClean은 진단이 하나도 안 나오는 페이지다.
const fixtureClean = `<mxfile><diagram name="고객" id="pg1"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <mxCell id="t1" value="CUSTOMER" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1">
    <mxGeometry x="80" y="40" width="240" height="60" as="geometry"/>
  </mxCell>
  <mxCell id="r1" style="shape=tableRow;" vertex="1" parent="t1">
    <mxGeometry y="30" width="240" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1k" value="PK" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry width="30" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1d" value="CUST_ID INTEGER" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry x="30" width="210" height="30" as="geometry"/>
  </mxCell>
</root></mxGraphModel></diagram></mxfile>`

// assembleFixture는 바이트를 임시 파일에 써서 기존 파이프라인으로 읽는다.
// ir.Assemble이 경로를 받으므로 이 왕복이 필요하다.
func assembleFixture(t *testing.T, src []byte) (model.Document, []validate.Finding) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.drawio")
	if err := os.WriteFile(p, src, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg := config.Config{}
	doc, dups, err := ir.Assemble(p)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	return doc, append(validate.Required(doc, dups), validate.Optional(doc, cfg)...)
}

// 계약 1: 두 번 돌리면 바이트가 완전히 같아야 한다.
func TestAnnotateIsIdempotent(t *testing.T) {
	src := []byte(fixtureWithMissingPK)
	doc, findings := assembleFixture(t, src)

	once, r1, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("1회차: %v", err)
	}
	if !r1.Changed || r1.Marked == 0 {
		t.Fatalf("1회차에 아무것도 안 바뀌었다: %+v", r1)
	}

	// 2회차는 «이미 표시된 파일»을 입력으로 받는다. 검증도 다시 돌린다.
	doc2, findings2 := assembleFixture(t, once)
	twice, _, err := Annotate(once, doc2, findings2)
	if err != nil {
		t.Fatalf("2회차: %v", err)
	}
	if !bytes.Equal(once, twice) {
		t.Errorf("두 번 돌리면 바이트가 달라진다\n1회차:\n%s\n2회차:\n%s", once, twice)
	}
}

// 계약 3: 진단이 0건이면 입력을 그대로 돌려주고 Changed=false다.
func TestAnnotateWritesNothingWhenNoFindings(t *testing.T) {
	src := []byte(fixtureClean)
	doc, findings := assembleFixture(t, src)
	if len(findings) != 0 {
		t.Fatalf("픽스처에 진단이 %d건 있다; 0건이어야 한다: %v", len(findings), findings)
	}
	out, r, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	if r.Changed {
		t.Error("Changed=true; 진단이 없으면 false여야 한다")
	}
	if !bytes.Equal(src, out) {
		t.Errorf("진단이 없는데 바이트가 달라졌다:\n%s", out)
	}
}

// 사람이 진단을 고치면 옛 요약 박스와 마크가 사라져야 한다. 남아
// 있으면 툴이 «이미 없는 문제»를 계속 보고하는 것이다.
//
// 계약 2(코드리뷰 뒤 수정된 버전): 진단이 사라지면 "입력 원본 바이트"가
// 아니라 "그 원본을 convert.RewriteMxFilePlan으로 아무 편집 없이 한 번
// 돌린 결과"와 같아져야 한다. encoding/xml의 Encoder는 self-closing 빈
// 요소(<mxCell id="0"/>)와 여닫는 쌍(<mxCell id="0"></mxCell>)을 토큰
// 스트림에서 구별하지 못하므로, "원본 바이트 자체로 돌아온다"는 이
// 인코더로는 애초에 닿을 수 없는 목표였다(그것을 정규식/스캐너로 억지로
// 되돌리려던 첫 시도가 결함을 냈다). annotate가 실제로
// 지켜야 하는 것은 "자기 표식을 하나도 안 남기고, 그 밖의 무엇도 안
// 바꾼다"이고, 그 기준선은 convert 자신이 이미 쓰고 있는 정규화 형태다.
func TestAnnotateRemovesStaleMarksWhenFixed(t *testing.T) {
	src := []byte(fixtureWithMissingPK)
	doc, findings := assembleFixture(t, src)
	marked, _, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	markedPerPage, _ := ScanExisting(marked)
	if e := markedPerPage[0]; len(e.Summaries) == 0 || len(e.Marked) == 0 {
		t.Fatalf("표시가 안 됐다: %+v", e)
	}

	// 진단이 사라진 상황을 흉내낸다: 같은 파일에 findings를 0건으로 준다.
	doc2, _ := assembleFixture(t, marked)
	cleared, r, err := Annotate(marked, doc2, nil)
	if err != nil {
		t.Fatalf("2회차: %v", err)
	}
	if !r.Changed {
		t.Error("Changed=false; 표식을 지웠으므로 true여야 한다")
	}
	ePerPage, err := ScanExisting(cleared)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	e := ePerPage[0]
	if len(e.Summaries) != 0 {
		t.Errorf("옛 요약 박스가 남았다: %v", e.Summaries)
	}
	if len(e.Marked) != 0 {
		t.Errorf("옛 마크가 남았다: %v", e.Marked)
	}
	if len(e.WrappedByUs) != 0 {
		t.Errorf("erdtoolWrapped 표가 남았다: %v", e.WrappedByUs)
	}

	want, err := convert.RewriteMxFilePlan(src, convert.RewritePlan{})
	if err != nil {
		t.Fatalf("RewriteMxFilePlan(순수 왕복): %v", err)
	}
	if !bytes.Equal(want, cleared) {
		t.Errorf("정규화된 원본으로 안 돌아왔다\n기대(순수 왕복):\n%s\n결과:\n%s", want, cleared)
	}
}

// 계약 2, value="" 케이스(코드리뷰 라운드 2). 마크되는 셀 자신이 원래
// value=""를 갖고 있는 흔한 draw.io 모양 — 이름 없는 테이블 —에서도
// 계약 2가 성립하는지 본다. fixtureWithMissingPK와 거의 같지만 t1의
// value를 "주문"에서 ""로 바꿨을 뿐이다(이름이 없어도 PK 없음 진단은
// 그대로 나온다). 라운드 1의 첫 고침("hasLabel && label != \"\""로
// «값 있음»을 판정)은 이 케이스에서 value="" 속성 자체를 지워버려
// 계약 2를 깼다 — "값이 비었다"와 "속성이 없다"는 다른 사실이라는 것을
// 이 테스트가 실제 Annotate 왕복으로 못박는다.
func TestAnnotateContract2SurvivesMarkedCellWithExplicitEmptyValue(t *testing.T) {
	src := []byte(`<mxfile><diagram name="주문" id="pg1"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <mxCell id="t1" value="" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1">
    <mxGeometry x="80" y="40" width="240" height="60" as="geometry"/>
  </mxCell>
  <mxCell id="r1" style="shape=tableRow;" vertex="1" parent="t1">
    <mxGeometry y="30" width="240" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1k" value="" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry width="30" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1d" value="주문일자 DATE" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry x="30" width="210" height="30" as="geometry"/>
  </mxCell>
</root></mxGraphModel></diagram></mxfile>`)
	doc, findings := assembleFixture(t, src)
	if len(findings) == 0 {
		t.Fatal("픽스처에 진단이 없다 — 이름 없는 테이블도 PK가 없으면 진단이 나야 한다")
	}

	marked, r, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("1회차: %v", err)
	}
	if r.Marked == 0 {
		t.Fatal("t1이 마크되지 않았다")
	}

	doc2, _ := assembleFixture(t, marked)
	cleared, _, err := Annotate(marked, doc2, nil)
	if err != nil {
		t.Fatalf("2회차: %v", err)
	}

	want, err := convert.RewriteMxFilePlan(src, convert.RewritePlan{})
	if err != nil {
		t.Fatalf("RewriteMxFilePlan(순수 왕복): %v", err)
	}
	if !bytes.Equal(want, cleared) {
		t.Errorf("value=\"\"였던 t1이 순수 왕복 결과와 달라졌다\n기대(순수 왕복):\n%s\n결과:\n%s", want, cleared)
	}
}

// 마크된 셀의 원래 스타일이 보존되고, 두 번째 실행이 그것을 덮어쓰지
// 않는지 본다. 덮어쓰면 --clean이 원본을 복원하지 못한다.
func TestAnnotatePreservesBaseStyleAcrossRuns(t *testing.T) {
	src := []byte(fixtureWithMissingPK)
	doc, findings := assembleFixture(t, src)
	once, _, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("1회차: %v", err)
	}
	e1PerPage, err := ScanExisting(once)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	e1 := e1PerPage[0]

	doc2, findings2 := assembleFixture(t, once)
	twice, _, err := Annotate(once, doc2, findings2)
	if err != nil {
		t.Fatalf("2회차: %v", err)
	}
	e2PerPage, err := ScanExisting(twice)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	e2 := e2PerPage[0]
	for id, base := range e1.BaseStyle {
		if e2.BaseStyle[id] != base {
			t.Errorf("%s의 BaseStyle이 2회차에 바뀌었다: %q -> %q", id, base, e2.BaseStyle[id])
		}
		if drawio.ParseStyle(base)["strokeColor"] == markStrokeColor {
			t.Errorf("%s의 BaseStyle에 마크 색이 굳었다: %q", id, base)
		}
	}
}

// 계약 4: erdtool 표식이 없는 남의 것은 절대 안 건드린다. 기존 테스트는
// fixtureClean(진단 0건)으로 이걸 확인했는데, 그 경로는 계약 3(진단
// 0건이면 무변화)과 검증 대상이 완전히 같다 — findings가 처음부터
// 0건이면 마킹도, 되돌리기도, 벗기기도 전혀 일어나지 않으므로(남의
// <object> 래퍼를 구조만 보고 벗겨버리는 결함)를 이 테스트는 절대 못
// 잡는다. 진짜로 필요한 것은 "마크되는 셀"이 "이미 사용자의 다른 이유로
// <object>에 감싸여 있을 때" 마크→해제 한 바퀴를 도는 시나리오다.
//
// t1은 사용자가 이미 logicalName·tooltip을 넣어 둔 <object>로 감싸여
// 있고, PK가 없어 실제로 마크 대상이 된다. 마크한 뒤에도, 진단이
// 사라져 마크를 지운 뒤에도 그 두 속성과 래퍼 자체가 그대로 있어야
// 한다 — erdtoolWrapped 표가 없는 래퍼이므로 annotate는 절대 벗기지
// 않는다(existing.WrappedByUs로 판정).
func TestAnnotateMarkThenClearPreservesForeignWrapperAttrs(t *testing.T) {
	src := []byte(`<mxfile><diagram name="주문" id="pg1"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <object label="주문" logicalName="주문번호" tooltip="사용자가 남긴 설명" id="t1">
    <mxCell style="shape=table;childLayout=tableLayout;" vertex="1" parent="1">
      <mxGeometry x="80" y="40" width="240" height="60" as="geometry"/>
    </mxCell>
  </object>
  <mxCell id="r1" style="shape=tableRow;" vertex="1" parent="t1">
    <mxGeometry y="30" width="240" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1k" value="" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry width="30" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1d" value="주문일자 DATE" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry x="30" width="210" height="30" as="geometry"/>
  </mxCell>
</root></mxGraphModel></diagram></mxfile>`)
	doc, findings := assembleFixture(t, src)
	if len(findings) == 0 {
		t.Fatal("픽스처에 진단이 없다 — t1은 PK가 없어야 한다")
	}

	marked, r, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("1회차: %v", err)
	}
	if r.Marked == 0 {
		t.Fatal("t1이 마크되지 않았다")
	}
	eMarkedPerPage, err := ScanExisting(marked)
	if err != nil {
		t.Fatalf("ScanExisting(marked): %v", err)
	}
	eMarked := eMarkedPerPage[0]
	if !eMarked.Marked["t1"] {
		t.Fatal("t1이 마크된 것으로 안 잡힌다")
	}
	if eMarked.WrappedByUs["t1"] {
		t.Error("t1의 래퍼는 사용자 것인데 우리 것으로 표시됐다")
	}
	for _, needle := range []string{`logicalName="주문번호"`, `tooltip="사용자가 남긴 설명"`} {
		if !strings.Contains(string(marked), needle) {
			t.Errorf("마크 직후 사용자 속성이 사라졌다(%s 없음):\n%s", needle, marked)
		}
	}

	doc2, _ := assembleFixture(t, marked)
	cleared, r2, err := Annotate(marked, doc2, nil)
	if err != nil {
		t.Fatalf("2회차: %v", err)
	}
	if !r2.Changed {
		t.Error("Changed=false; erdtoolIssue/BaseStyle을 지웠으므로 true여야 한다")
	}
	eClearedPerPage, err := ScanExisting(cleared)
	if err != nil {
		t.Fatalf("ScanExisting(cleared): %v", err)
	}
	eCleared := eClearedPerPage[0]
	if eCleared.Marked["t1"] {
		t.Error("진단이 사라졌는데 t1이 여전히 마크돼 있다")
	}
	if eCleared.WrappedByUs["t1"] {
		t.Error("우리가 벗기지도 않은 래퍼에 erdtoolWrapped가 생겼다")
	}
	if !eCleared.Wrapped["t1"] {
		t.Error("사용자의 <object> 래퍼 자체가 사라졌다 — 우리 것이 아닌 래퍼는 절대 벗기면 안 된다")
	}
	for _, needle := range []string{`logicalName="주문번호"`, `tooltip="사용자가 남긴 설명"`, "<object"} {
		if !strings.Contains(string(cleared), needle) {
			t.Errorf("마크를 지운 뒤 사용자 래퍼/속성이 사라졌다(%s 없음):\n%s", needle, cleared)
		}
	}
}

// 검증이 가리킨 셀이 파일에 없으면 지어내지 않고 건너뛴다 — 진단 자체는
// 요약 박스에 실린다.
func TestAnnotateSkipsFindingWithCellIDNotInFile(t *testing.T) {
	src := []byte(fixtureWithMissingPK)
	doc, findings := assembleFixture(t, src)
	if len(findings) == 0 {
		t.Fatal("픽스처에 진단이 없다")
	}
	// finding 하나를 파일에 없는 cellID로 바꿔치기한다.
	findings[0].CellID = "no-such-cell"

	out, r, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	if r.Marked != 0 {
		t.Errorf("Marked=%d; 파일에 없는 셀은 마크되면 안 된다", r.Marked)
	}
	ePerPage, err := ScanExisting(out)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	e := ePerPage[0]
	if len(e.Marked) != 0 {
		t.Errorf("존재하지 않는 셀 id가 마크로 지어내졌다: %v", e.Marked)
	}
	// 요약 박스에는 여전히 실려야 한다.
	if len(e.Summaries) == 0 {
		t.Error("요약 박스가 안 만들어졌다 — 진단 자체는 요약에 남아야 한다")
	}
}

// 작업 13 — Clean.
//
// 계약: Clean(Annotate(x)) == convert.RewriteMxFilePlan(x, RewritePlan{})
// («원본 그대로»가 아니라 「그 파일 자신의 정규화 왕복형」이다 — 원장
// "멱등성·안전 계약" 절, encoding/xml이 self-closing 태그를 못 구별하는
// 것이 이유다). 기대값은 항상 그 자리에서 convert.RewriteMxFilePlan을 직접
// 돌려 계산한다 — 손으로 적은 문자열과 비교하면 그 문자열 자체가 언젠가
// 실제 인코더 출력과 갈라져 거짓 성공을 낸다.
func TestCleanRestoresNormalizedRoundtrip(t *testing.T) {
	src := []byte(fixtureWithMissingPK)
	doc, findings := assembleFixture(t, src)

	marked, r, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	if !r.Changed || r.Marked == 0 {
		t.Fatalf("표시가 안 됐다: %+v", r)
	}

	back, rc, err := Clean(marked)
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if !rc.Changed || rc.Marked == 0 {
		t.Errorf("Clean 결과: %+v; 지울 표식이 있었으므로 Changed/Marked가 나야 한다", rc)
	}
	// 코드리뷰: Pages는 지운 요약 박스 수다. fixtureWithMissingPK는
	// 페이지 하나짜리이고 그 페이지에 진단이 있었으므로 요약 박스가
	// 하나였다 — Clean이 그 하나를 지웠으니 Pages==1이어야 한다.
	if rc.Pages != 1 {
		t.Errorf("Pages=%d; 요약 박스 1개를 지웠으므로 1이어야 한다: %+v", rc.Pages, rc)
	}

	want, err := convert.RewriteMxFilePlan(src, convert.RewritePlan{})
	if err != nil {
		t.Fatalf("RewriteMxFilePlan(순수 왕복): %v", err)
	}
	if !bytes.Equal(want, back) {
		t.Errorf("정규화 왕복형으로 안 돌아왔다\n기대:\n%s\n결과:\n%s", want, back)
	}

	ePerPage, err := ScanExisting(back)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	e := ePerPage[0]
	if len(e.Marked) != 0 || len(e.Summaries) != 0 || len(e.WrappedByUs) != 0 || len(e.BaseStyle) != 0 {
		t.Errorf("Clean 뒤에도 표식이 남았다: %+v", e)
	}
}

// 한 번도 마크된 적 없는 파일에 Clean을 돌리면 손대지 않는다 — 이 경로는
// 재작성기를 아예 안 탄다(Annotate가 진단 0건일 때 그러는 것과 같은
// 이유). 그래서 여기서는 「정규화 왕복형」이 아니라 「원본 바이트 그대로」와
// 비교한다.
func TestCleanOnNeverAnnotatedFileChangesNothing(t *testing.T) {
	src := []byte(fixtureClean)
	out, r, err := Clean(src)
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if r.Changed {
		t.Errorf("Changed=true; 지울 것이 없으면 false여야 한다: %+v", r)
	}
	if !bytes.Equal(src, out) {
		t.Errorf("표식이 없는 파일인데 바이트가 달라졌다\n원본:\n%s\n결과:\n%s", src, out)
	}
}

// Clean은 멱등이어야 한다 — 두 번째 실행은 이미 지울 표식이 없으므로 위
// TestCleanOnNeverAnnotatedFileChangesNothing과 같은 조기 반환 경로를 타야
// 정상이다. 이 테스트는 그 경로가 "마크된 파일을 한 번 Clean한 뒤"에도
// 실제로 재확인되는지 본다 — Clean 자신이 자기 표식을 하나라도 흘리면
// 두 번째 실행이 뭔가를 또 지우려 들어 이 비교가 깨진다.
func TestCleanIsIdempotent(t *testing.T) {
	src := []byte(fixtureWithMissingPK)
	doc, findings := assembleFixture(t, src)
	marked, _, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}

	once, r1, err := Clean(marked)
	if err != nil {
		t.Fatalf("1회차 Clean: %v", err)
	}
	if !r1.Changed {
		t.Fatal("1회차: 지울 표식이 있었는데 Changed=false")
	}

	twice, r2, err := Clean(once)
	if err != nil {
		t.Fatalf("2회차 Clean: %v", err)
	}
	if r2.Changed {
		t.Errorf("2회차: Changed=true; 이미 깨끗하므로 false여야 한다: %+v", r2)
	}
	if !bytes.Equal(once, twice) {
		t.Errorf("두 번째 Clean이 바이트를 또 바꿨다\n1회차:\n%s\n2회차:\n%s", once, twice)
	}
}

// Clean은 자기가 만들지 않은 래퍼와 그 안의 남의 속성(예: convert의
// logicalName)을 절대 벗기거나 지우지 않는다. t1은 사용자가 이미
// logicalName·tooltip을 붙여 둔 <object>이고, PK가 없어 실제로 마크
// 대상이 된다 — WrappedByUs는 false로 남으므로(existing.go), Clean이
// 이 래퍼를 벗기면 안 된다. Annotate(findings=nil)로 되돌리는 경로는
// TestAnnotateMarkThenClearPreservesForeignWrapperAttrs가 이미 검사하지만,
// 이 테스트는 Clean 함수 자체를 직접 호출해 같은 계약을 확인한다 — 갈라
// 두지 않았다는 것 자체가 작업 13의 요구사항이다.
func TestCleanPreservesForeignWrapperAttrs(t *testing.T) {
	src := []byte(`<mxfile><diagram name="주문" id="pg1"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <object label="주문" logicalName="주문번호" tooltip="사용자가 남긴 설명" id="t1">
    <mxCell style="shape=table;childLayout=tableLayout;" vertex="1" parent="1">
      <mxGeometry x="80" y="40" width="240" height="60" as="geometry"/>
    </mxCell>
  </object>
  <mxCell id="r1" style="shape=tableRow;" vertex="1" parent="t1">
    <mxGeometry y="30" width="240" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1k" value="" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry width="30" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1d" value="주문일자 DATE" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry x="30" width="210" height="30" as="geometry"/>
  </mxCell>
</root></mxGraphModel></diagram></mxfile>`)
	doc, findings := assembleFixture(t, src)
	if len(findings) == 0 {
		t.Fatal("픽스처에 진단이 없다 — t1은 PK가 없어야 한다")
	}

	marked, r, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	if r.Marked == 0 {
		t.Fatal("t1이 마크되지 않았다")
	}

	cleaned, rc, err := Clean(marked)
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if !rc.Changed {
		t.Error("Changed=false; erdtoolIssue/BaseStyle을 지웠으므로 true여야 한다")
	}

	ePerPage, err := ScanExisting(cleaned)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	e := ePerPage[0]
	if e.Marked["t1"] {
		t.Error("Clean 뒤에도 t1이 여전히 마크돼 있다")
	}
	if e.WrappedByUs["t1"] {
		t.Error("우리가 만들지도 않은 래퍼에 erdtoolWrapped가 생겼다")
	}
	if !e.Wrapped["t1"] {
		t.Error("사용자의 <object> 래퍼 자체가 사라졌다 — 우리 것이 아닌 래퍼는 절대 벗기면 안 된다")
	}
	for _, needle := range []string{`logicalName="주문번호"`, `tooltip="사용자가 남긴 설명"`, "<object"} {
		if !strings.Contains(string(cleaned), needle) {
			t.Errorf("Clean 뒤 사용자 래퍼/속성이 사라졌다(%s 없음):\n%s", needle, cleaned)
		}
	}
}

// 재발 방지. 위 테스트는 사용자 래퍼가 logicalName·tooltip처럼 남는
// 속성을 갖고 있어서, convert.onlyLabelAndID(구조만 보는 최후 방어선)
// 하나만으로도 벗기기가 막힌다 — annotate 쪽의 WrappedByUs 판정이 실제로
// 하는 일을 이 테스트는 증명하지 못한다.
//
// 여기서는 사용자의 래퍼가 원래부터 label과 id **말고는 아무 속성도** 없는
// 경우를 쓴다. 마크하면 erdtoolIssue·erdtoolBaseStyle이 붙어 label·id
// 말고 다른 속성이 생기지만, Clean이 그 둘을 지우고 나면 다시 label·id만
// 남아 «구조만 보면 벗겨도 되는» 모양으로 돌아간다 — 바로 이 순간
// convert의 구조적 방어선(onlyLabelAndID)은 더 이상 막아주지 못하고,
// annotate가 Unwrap을 세우지 않는 것(WrappedByUs가 false라서)만이 이
// 래퍼를 지킨다. Unwrap을 여기서 잘못 세우면(예: WrappedByUs 확인을
// 건너뛰면) 이 래퍼는 사용자가 원래 감싸 둔 것인데도 벗겨져 맨 mxCell로
// 바뀐다 — 코드리뷰가 잡은 바로 그 사고다.
func TestCleanNeverUnwrapsUserBareWrapperEvenWhenStructurallyEligible(t *testing.T) {
	src := []byte(`<mxfile><diagram name="주문" id="pg1"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <object label="주문" id="t1">
    <mxCell style="shape=table;childLayout=tableLayout;" vertex="1" parent="1">
      <mxGeometry x="80" y="40" width="240" height="60" as="geometry"/>
    </mxCell>
  </object>
  <mxCell id="r1" style="shape=tableRow;" vertex="1" parent="t1">
    <mxGeometry y="30" width="240" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1k" value="" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry width="30" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1d" value="주문일자 DATE" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry x="30" width="210" height="30" as="geometry"/>
  </mxCell>
</root></mxGraphModel></diagram></mxfile>`)
	doc, findings := assembleFixture(t, src)
	if len(findings) == 0 {
		t.Fatal("픽스처에 진단이 없다 — t1은 PK가 없어야 한다")
	}

	marked, r, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	if r.Marked == 0 {
		t.Fatal("t1이 마크되지 않았다")
	}
	markedPerPage2, _ := ScanExisting(marked)
	if eMarked := markedPerPage2[0]; eMarked.WrappedByUs["t1"] {
		t.Fatal("전제 조건 붕괴: t1의 래퍼는 사용자 것인데 우리 것으로 표시됐다")
	}

	cleaned, _, err := Clean(marked)
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}

	ePerPage, err := ScanExisting(cleaned)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	e := ePerPage[0]
	if !e.Wrapped["t1"] {
		t.Error("사용자가 원래 label·id만으로 감싸 둔 래퍼가 벗겨졌다 — 구조가 우연히 벗기기 조건과 같아졌다고 벗기면 안 된다")
	}
	if !strings.Contains(string(cleaned), `<object label="주문" id="t1">`) {
		t.Errorf("t1의 래퍼 모양이 원래(label, id)와 달라졌다:\n%s", cleaned)
	}
}

// 실제 .drawio 페이지는 대개 평문이 아니라 deflate+base64로 압축돼 있다.
// Clean이 그것을 못 풀면 실사용 파일에서는 항상 "지울 표식 없음"만 보고,
// --clean이 아무 일도 안 하면서 성공만 외치는 명령이 된다. 여기서
// drawio.Compress로 실제 압축 바이트를 만들어 그 경로를 태운다.
func TestCleanWorksOnCompressedPage(t *testing.T) {
	plain := strings.TrimPrefix(fixtureWithMissingPK, `<mxfile><diagram name="주문" id="pg1">`)
	plain = strings.TrimSuffix(plain, `</diagram></mxfile>`)

	packed, err := drawio.Compress(plain)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	src := []byte(`<mxfile compressed="true"><diagram name="주문" id="pg1">` + packed + `</diagram></mxfile>`)

	doc, findings := assembleFixture(t, src)
	if len(findings) == 0 {
		t.Fatal("픽스처에 진단이 없다")
	}

	marked, r, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	if !r.Changed || r.Marked == 0 {
		t.Fatalf("압축된 페이지가 마크되지 않았다: %+v", r)
	}
	// 마킹 자체가 압축을 유지하는지도 함께 확인한다 — 평문으로 풀려
	// 나오면 이 테스트가 애초에 압축 경로를 태우지 못한 것이다.
	if !strings.Contains(string(marked), `compressed="true"`) {
		t.Fatalf("마크된 결과가 압축을 잃었다:\n%s", marked)
	}

	cleaned, rc, err := Clean(marked)
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if !rc.Changed || rc.Marked == 0 {
		t.Fatalf("Clean이 압축된 페이지의 표식을 못 찾았다: %+v", rc)
	}

	want, err := convert.RewriteMxFilePlan(src, convert.RewritePlan{})
	if err != nil {
		t.Fatalf("RewriteMxFilePlan(순수 왕복): %v", err)
	}
	if !bytes.Equal(want, cleaned) {
		t.Errorf("압축된 페이지가 정규화 왕복형으로 안 돌아왔다\n기대:\n%s\n결과:\n%s", want, cleaned)
	}

	ePerPage, err := ScanExisting(cleaned)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	e := ePerPage[0]
	if len(e.Marked) != 0 || len(e.Summaries) != 0 {
		t.Errorf("압축된 페이지에 표식이 남았다: %+v", e)
	}
}

// 코드리뷰 수정: existing.Marked(erdtoolIssue)만 보고 "지울 것이
// 있는가"를 판정하면 안 된다. draw.io의 «데이터 편집» 창은 속성을 하나씩
// 지울 수 있는 일반 UI라서, 사람이 erdtoolIssue만 지우고
// erdtoolBaseStyle·erdtoolWrapped는 그대로 두는 것이 실제로 가능하다.
// 옛 코드는 이 상태를 "표식 없음"으로 오판해 Clean이 Changed=false·바이트
// 무변화로 «성공」을 보고하면서 실제로는 빨간 테두리와 잔여 속성을 하나도
// 안 지웠다 — 이 저장소가 가장 경계하는 실패 유형(조용히 틀린 성공)을
// --clean 자신이 저지르는 사고였다.
func TestCleanRemovesResidueEvenWithoutErdtoolIssue(t *testing.T) {
	src := []byte(`<mxfile><diagram name="주문" id="pg1"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <object label="주문" erdtoolBaseStyle="shape=table;childLayout=tableLayout;" erdtoolWrapped="1" id="t1">
    <mxCell style="shape=table;childLayout=tableLayout;strokeColor=#FF3333;strokeWidth=3;" vertex="1" parent="1">
      <mxGeometry x="80" y="40" width="240" height="60" as="geometry"/>
    </mxCell>
  </object>
</root></mxGraphModel></diagram></mxfile>`)

	// 전제조건: 이 픽스처는 erdtoolIssue가 없으므로 옛 판정 기준(existing.Marked)
	// 으로는 "표식 없음"이다.
	prePerPage, err := ScanExisting(src)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	pre := prePerPage[0]
	if len(pre.Marked) != 0 {
		t.Fatal("전제 조건 붕괴: 이 픽스처는 erdtoolIssue가 없어야 한다")
	}
	if len(pre.BaseStyle) == 0 || len(pre.WrappedByUs) == 0 {
		t.Fatal("전제 조건 붕괴: erdtoolBaseStyle·erdtoolWrapped 잔재가 있어야 한다")
	}

	out, r, err := Clean(src)
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if !r.Changed {
		t.Error("Changed=false; erdtoolIssue 없이도 지울 잔재(erdtoolBaseStyle·erdtoolWrapped)가 있었으므로 true여야 한다")
	}
	if r.Marked != 1 {
		t.Errorf("Marked=%d; 잔재 셀 1개를 되돌렸으므로 1이어야 한다", r.Marked)
	}

	ePerPage, err := ScanExisting(out)
	if err != nil {
		t.Fatalf("ScanExisting(out): %v", err)
	}
	e := ePerPage[0]
	if len(e.BaseStyle) != 0 || len(e.WrappedByUs) != 0 || e.Wrapped["t1"] {
		t.Errorf("잔재가 남았다: %+v", e)
	}
	if !strings.Contains(string(out), `style="shape=table;childLayout=tableLayout;"`) {
		t.Errorf("스타일이 원래대로(빨간 테두리 없이) 안 돌아왔다:\n%s", out)
	}
}

// 위 시나리오를 압축된 페이지(실제 .drawio 모양)에서도 확인한다 — 코드
// 리뷰가 지적했듯, 이전 라운드의 압축 테스트는 "메커니즘상 당연히
// 된다"는 추론에만 기댔지 실제로 빨간불을 본 적이 없었다. 이 테스트는
// 뮤테이션(existing.Marked만 보도록 되돌리기)으로 실제 실패를 확인한 뒤
// 고쳤다 — 구현 코드의 커밋 로그와 함께 검증 과정을 report에 남긴다.
func TestCleanRemovesResidueEvenWithoutErdtoolIssueOnCompressedPage(t *testing.T) {
	plain := `<mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <object label="주문" erdtoolBaseStyle="shape=table;childLayout=tableLayout;" erdtoolWrapped="1" id="t1">
    <mxCell style="shape=table;childLayout=tableLayout;strokeColor=#FF3333;strokeWidth=3;" vertex="1" parent="1">
      <mxGeometry x="80" y="40" width="240" height="60" as="geometry"/>
    </mxCell>
  </object>
</root></mxGraphModel>`
	packed, err := drawio.Compress(plain)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	src := []byte(`<mxfile compressed="true"><diagram name="주문" id="pg1">` + packed + `</diagram></mxfile>`)

	out, r, err := Clean(src)
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if !r.Changed {
		t.Error("Changed=false; 압축된 페이지에도 지울 잔재가 있었으므로 true여야 한다")
	}

	ePerPage, err := ScanExisting(out)
	if err != nil {
		t.Fatalf("ScanExisting(out): %v", err)
	}
	e := ePerPage[0]
	if len(e.BaseStyle) != 0 || len(e.WrappedByUs) != 0 || e.Wrapped["t1"] {
		t.Errorf("압축된 페이지에 잔재가 남았다: %+v", e)
	}
}

// 코드리뷰 수정: erdtoolBaseStyle이 없는 마크된 셀(사람이
// 「데이터 편집」 창에서 erdtoolBaseStyle만 지운 경우)에서 Clean이
// style=""를 써 넣어 실제 모양을 지워 버리면 안 된다. 저장된 원래
// 스타일이 없으면 지금 스타일(빨간 테두리 포함)을 그대로 둔다 — 지어낸
// 빈 스타일보다는 훨씬 낫다.
func TestCleanDoesNotBlankStyleWhenBaseStyleMissing(t *testing.T) {
	src := []byte(`<mxfile><diagram name="주문" id="pg1"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <object label="주문" erdtoolIssue="PK가 없음" id="t1">
    <mxCell style="shape=table;childLayout=tableLayout;strokeColor=#FF3333;strokeWidth=3;" vertex="1" parent="1">
      <mxGeometry x="80" y="40" width="240" height="60" as="geometry"/>
    </mxCell>
  </object>
</root></mxGraphModel></diagram></mxfile>`)

	prePerPage, err := ScanExisting(src)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	pre := prePerPage[0]
	if _, had := pre.BaseStyle["t1"]; had {
		t.Fatal("전제 조건 붕괴: 이 픽스처는 erdtoolBaseStyle이 없어야 한다")
	}

	out, r, err := Clean(src)
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if !r.Changed {
		t.Error("Changed=false; erdtoolIssue를 지웠으므로 true여야 한다")
	}
	if strings.Contains(string(out), `style=""`) {
		t.Fatalf("style이 빈 문자열로 지워졌다 — 실제 모양을 잃었다:\n%s", out)
	}
	if !strings.Contains(string(out), `style="shape=table;childLayout=tableLayout;strokeColor=#FF3333;strokeWidth=3;"`) {
		t.Errorf("style이 그대로 안 남았다:\n%s", out)
	}
}

// 위와 같은 계약을 Annotate 쪽 호출자에도 건다 — revertMarkEdit를
// 공유하므로 Clean만 고치고 Annotate를 빠뜨리면 두 함수가 다시 갈라진다.
// 1회차로 정상 마크한 뒤, 사람이 erdtoolBaseStyle만 지운 것을 흉내내고,
// 진단이 사라진 2회차 Annotate(findings=nil)가 그 셀을 되돌릴 때
// style=""을 쓰지 않는지 본다.
func TestAnnotateRevertDoesNotBlankStyleWhenBaseStyleMissing(t *testing.T) {
	src := []byte(fixtureWithMissingPK)
	doc, findings := assembleFixture(t, src)
	marked, r, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("1회차: %v", err)
	}
	if r.Marked == 0 {
		t.Fatal("t1이 마크되지 않았다")
	}

	// 사람이 「데이터 편집」 창에서 erdtoolBaseStyle만 지운 것을 흉내낸다.
	needle := []byte(` erdtoolBaseStyle="shape=table;childLayout=tableLayout;"`)
	stripped := bytes.Replace(marked, needle, nil, 1)
	if bytes.Equal(stripped, marked) {
		t.Fatal("erdtoolBaseStyle을 못 지웠다 — 마크된 결과의 속성 모양이 바뀌었을 수 있다")
	}

	doc2, _ := assembleFixture(t, stripped)
	cleared, r2, err := Annotate(stripped, doc2, nil)
	if err != nil {
		t.Fatalf("2회차: %v", err)
	}
	if !r2.Changed {
		t.Error("Changed=false; erdtoolIssue가 남아 있었으므로 되돌리기가 일어나 true여야 한다")
	}
	if strings.Contains(string(cleared), `style=""`) {
		t.Errorf("style이 빈 문자열로 지워졌다:\n%s", cleared)
	}
}

// 코드리뷰 수정: 요약 박스만 남고 마크된 셀이 하나도 없는
// 파일에서 Result.Pages가 0으로 남으면 안 된다. Tasks 14/15가 이 값을
// 그대로 사용자에게 출력하므로, 파일이 실제로 바뀌었는데 "0개 되돌림"으로
// 보고하는 것은 거짓말이다.
func TestCleanReportsPagesForSummaryOnlyResidue(t *testing.T) {
	src := []byte(`<mxfile><diagram name="주문" id="pg1"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <object label="erdtool 검증" erdtoolAnnotation="page-summary" id="erdtool-summary-pg1">
    <mxCell style="shape=note;whiteSpace=wrap;html=1;" vertex="1" parent="1">
      <mxGeometry x="320" y="40" width="260" height="60" as="geometry"/>
    </mxCell>
  </object>
</root></mxGraphModel></diagram></mxfile>`)

	out, r, err := Clean(src)
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if !r.Changed {
		t.Error("Changed=false; 요약 박스를 지웠으므로 true여야 한다")
	}
	if r.Marked != 0 {
		t.Errorf("Marked=%d; 마크된 셀이 없으므로 0이어야 한다", r.Marked)
	}
	if r.Pages != 1 {
		t.Errorf("Pages=%d; 요약 박스가 있던 페이지 1개를 지웠으므로 1이어야 한다", r.Pages)
	}

	ePerPage, err := ScanExisting(out)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	e := ePerPage[0]
	if len(e.Summaries) != 0 {
		t.Errorf("요약 박스가 안 지워졌다: %v", e.Summaries)
	}
}

// <diagram>에 id가 없는 파일은 «성공했다고 말하면서 아무것도 안 하는»
// 자리였다.
//
// validate가 DiagramID를 d.ID(=빈 문자열)로 적으면 BuildPlans는 그 진단을
// 전부 «페이지를 가로지르는 사실»로 읽어 어느 셀도 마크하지 않고,
// convert.RewriteMxFilePlan은 id 없는 <diagram>에 Inserts를 아예 안
// 넘긴다(요약 박스도 안 들어간다). 그런데도 res.Pages는 올라가서
// erdtool은 「페이지 1개 요약」이라고 보고했다 — 화면에 찍힌 숫자와
// 파일 안의 사실이 다르다. draw.io는 언제나 id를 쓰므로 이 경로에
// 닿는 것은 손편집 파일이나 다른 도구의 산출물뿐이지만, 이 저장소가
// 신뢰하지 않기로 한 것이 정확히 그런 입력이다.
//
// 세지 않고 «건너뛴다고 말한다».
func TestAnnotateSkipsDiagramWithoutIDAndSaysSo(t *testing.T) {
	src := []byte(`<mxfile><diagram name="주문"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <mxCell id="t1" value="주문" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1">
    <mxGeometry x="80" y="40" width="240" height="60" as="geometry"/>
  </mxCell>
  <mxCell id="r1" style="shape=tableRow;" vertex="1" parent="t1">
    <mxGeometry y="30" width="240" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1k" value="" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry width="30" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1d" value="주문일자 DATE" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry x="30" width="210" height="30" as="geometry"/>
  </mxCell>
</root></mxGraphModel></diagram></mxfile>`)
	doc, findings := assembleFixture(t, src)
	if len(findings) == 0 {
		t.Fatalf("픽스처에 진단이 없다 — 이 테스트가 무의미해진다")
	}

	out, r, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	if r.Pages != 0 {
		t.Errorf("Pages=%d; id 없는 페이지에는 요약 박스를 못 넣으므로 0이어야 한다: %+v", r.Pages, r)
	}
	if r.Marked != 0 {
		t.Errorf("Marked=%d; id 없는 페이지의 셀은 마크되지 않으므로 0이어야 한다: %+v", r.Marked, r)
	}
	if r.Changed {
		t.Errorf("Changed=true; 실제로 넣은 것이 없으면 파일을 다시 쓰지 않아야 한다: %+v", r)
	}
	if !bytes.Equal(src, out) {
		t.Errorf("아무것도 못 넣었는데 바이트가 달라졌다:\n%s", out)
	}
	if r.Findings != len(findings) {
		t.Errorf("Findings=%d; 진단은 %d건 있었다 — 검증 결과를 0건이라 말하면 안 된다", r.Findings, len(findings))
	}
	if len(r.Warnings) == 0 {
		t.Fatalf("건너뛴 사실을 아무도 말하지 않는다: %+v", r)
	}
	if !strings.Contains(strings.Join(r.Warnings, "\n"), "주문") {
		t.Errorf("경고가 건너뛴 페이지 이름(주문)을 안 담고 있다: %v", r.Warnings)
	}
}

// cellInPage는 특정 페이지의 셀 하나를 파일 바이트에서 읽는다. 교차 페이지
// 문제는 «어느 페이지의 셀인가»가 곧 사실이므로, 파일 전체를 평탄화해
// 보는 BuildIndex로는 검증할 수 없다.
func cellInPage(t *testing.T, src []byte, diagramID, cellID string) drawio.RawCell {
	t.Helper()
	diagrams, err := drawio.LoadDiagramsBytes(src)
	if err != nil {
		t.Fatalf("LoadDiagramsBytes: %v", err)
	}
	for _, d := range diagrams {
		if d.ID != diagramID {
			continue
		}
		c, ok := drawio.BuildIndex(d.Cells).ByID[cellID]
		if !ok {
			t.Fatalf("페이지 %q에 셀 %q가 없다", diagramID, cellID)
		}
		return c
	}
	t.Fatalf("페이지 %q가 없다", diagramID)
	return drawio.RawCell{}
}

// assertSameStyle은 두 스타일 문자열을 «부분 문자열»이 아니라 파싱한 맵으로
// 비교한다(저장소 규칙: shape=table이 shape=tableRow에 걸리는 그 실수).
func assertSameStyle(t *testing.T, got, want string) {
	t.Helper()
	g, w := drawio.ParseStyle(got), drawio.ParseStyle(want)
	if len(g) != len(w) {
		t.Fatalf("스타일이 다르다\n결과: %q\n기대: %q", got, want)
	}
	for k, wv := range w {
		if gv, ok := g[k]; !ok || gv != wv {
			t.Fatalf("스타일 키 %q가 다르다(결과 %q, 기대 %q)\n결과 전체: %q\n기대 전체: %q",
				k, gv, wv, got, want)
		}
	}
}

// fixtureCrossPageMarkedCollision은 페이지 pg1의 t1이 «erdtool이 마크해 둔»
// 상태이고, 페이지 pg2에 우연히 같은 id(t1)를 가진 «전혀 관계없는» 도형이
// 있는 파일이다. 손편집·파일 병합·다른 도구의 산출물이 섞이면 실제로 이
// 모양이 나온다.
const fixtureCrossPageMarkedCollision = `<mxfile>
  <diagram name="주문" id="pg1"><mxGraphModel><root>
    <mxCell id="0"/><mxCell id="1" parent="0"/>
    <object label="주문" erdtoolBaseStyle="shape=table;childLayout=tableLayout;" erdtoolIssue="PK가 없음" erdtoolWrapped="1" id="t1">
      <mxCell style="shape=table;childLayout=tableLayout;strokeColor=#FF3333;strokeWidth=3;" vertex="1" parent="1">
        <mxGeometry x="80" y="40" width="240" height="60" as="geometry"/>
      </mxCell>
    </object>
  </root></mxGraphModel></diagram>
  <diagram name="메모" id="pg2"><mxGraphModel><root>
    <mxCell id="0"/><mxCell id="1" parent="0"/>
    <mxCell id="t1" value="남의 도형" style="shape=hexagon;fillColor=#00FF00;" vertex="1" parent="1">
      <mxGeometry x="10" y="10" width="100" height="50" as="geometry"/>
    </mxCell>
  </root></mxGraphModel></diagram>
</mxfile>`

// --clean은 사용자의 탈출구이고 절대 멈추지 않는다. 이 픽스처는 pg1의 t1이
// erdtool이 마크해 둔 것이고, pg2의 t1은 우연히 같은 id를 가진 «전혀
// 관계없는» 도형이다. 편집이 페이지별로만 적용되므로(convert.PagePlan)
// pg2는 아예 안 건드린다 — pg2의 ScanExisting 결과에는 이 셀의 erdtool
// 잔재가 처음부터 없다.
func TestCleanDoesNotDestroyOtherPageStyleOnIDCollision(t *testing.T) {
	src := []byte(fixtureCrossPageMarkedCollision)

	out, _, err := Clean(src)
	if err != nil {
		t.Fatalf("Clean: %v — --clean은 사용자의 탈출구라 거부하면 안 된다", err)
	}

	// pg2의 남의 도형은 그대로여야 한다.
	assertSameStyle(t, cellInPage(t, out, "pg2", "t1").Style, "shape=hexagon;fillColor=#00FF00;")

	// pg1은 «마크됨» 상태에서 풀려나야 한다 — erdtoolIssue도
	// erdtoolWrapped도 안 남는다.
	ePerPage, err := ScanExisting(out)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	e := ePerPage[0]
	if len(e.ResidueIDs()) != 0 || len(e.Summaries) != 0 {
		t.Errorf("--clean 뒤에도 지워야 할 erdtool 표식이 남았다: %+v", e)
	}

	// pg1의 스타일은 마크되기 전 원래 모습(shape=table;...)으로 실제로
	// 복원돼야 한다 — 작업 6 이전에는 겹친 id에서 복원을 포기하고
	// erdtoolBaseStyle을 남긴 채 경고했지만, 편집이 페이지별로만
	// 적용되는 지금은 그럴 이유가 없다.
	assertSameStyle(t, cellInPage(t, out, "pg1", "t1").Style, "shape=table;childLayout=tableLayout;")
}

// 겹친 셀 id의 스타일을 페이지별로 «각각» 복원한다.
//
// 예전에는 복원을 아예 포기하고(다른 페이지의 동명 셀 모양을 덮어쓰므로)
// erdtoolBaseStyle을 남긴 뒤 «직접 되돌려라»고 경고했다. 이제 편집이 그
// 페이지에만 가므로 그럴 이유가 없다.
func TestCleanRestoresCollidingIDsPerPage(t *testing.T) {
	src := `<mxfile host="test">` +
		`<diagram id="pgA" name="A"><mxGraphModel><root>` +
		`<mxCell id="0"/><mxCell id="1" parent="0"/>` +
		`<object id="dup" label="A" ` + AttrIssue + `="진단A" ` + AttrBaseStyle + `="rounded=0;" ` + AttrWrapped + `="1">` +
		`<mxCell style="rounded=0;strokeColor=#FF0000;" vertex="1" parent="1"/></object>` +
		`</root></mxGraphModel></diagram>` +
		`<diagram id="pgB" name="B"><mxGraphModel><root>` +
		`<mxCell id="0"/><mxCell id="1" parent="0"/>` +
		`<object id="dup" label="B" ` + AttrIssue + `="진단B" ` + AttrBaseStyle + `="rounded=1;" ` + AttrWrapped + `="1">` +
		`<mxCell style="rounded=1;strokeColor=#FF0000;" vertex="1" parent="1"/></object>` +
		`</root></mxGraphModel></diagram></mxfile>`
	out, _, err := Clean([]byte(src))
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `style="rounded=0;"`) {
		t.Errorf("A페이지 스타일이 복원되지 않았다:\n%s", s)
	}
	if !strings.Contains(s, `style="rounded=1;"`) {
		t.Errorf("B페이지 스타일이 복원되지 않았다:\n%s", s)
	}
	if strings.Contains(s, AttrBaseStyle) {
		t.Errorf("%s 잔재가 남았다 — 잔재 0이어야 한다:\n%s", AttrBaseStyle, s)
	}
}

// id 없는 페이지의 진단이 «다른 페이지의 요약 박스»에 실려 사용자에게
// 보이면 안 된다. 그 페이지에는 아무것도 못 넣는다고 경고까지 해 놓고
// 진단만 엉뚱한 데 붙는 것은 거짓말이다.
func TestAnnotateDoesNotLeakIDLessPageFindingsIntoOtherPages(t *testing.T) {
	src := []byte(`<mxfile><diagram name="첫페이지"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <mxCell id="t1" value="주문" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1">
    <mxGeometry x="80" y="40" width="240" height="60" as="geometry"/>
  </mxCell>
  <mxCell id="r1" style="shape=tableRow;" vertex="1" parent="t1">
    <mxGeometry y="30" width="240" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1k" value="" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry width="30" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1d" value="주문일자 DATE" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry x="30" width="210" height="30" as="geometry"/>
  </mxCell>
</root></mxGraphModel></diagram><diagram id="pg2" name="둘째페이지"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <mxCell id="t2" value="고객" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1">
    <mxGeometry x="80" y="40" width="240" height="60" as="geometry"/>
  </mxCell>
  <mxCell id="r2" style="shape=tableRow;" vertex="1" parent="t2">
    <mxGeometry y="30" width="240" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r2k" value="" style="shape=partialRectangle;" vertex="1" parent="r2">
    <mxGeometry width="30" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r2d" value="가입일자 DATE" style="shape=partialRectangle;" vertex="1" parent="r2">
    <mxGeometry x="30" width="210" height="30" as="geometry"/>
  </mxCell>
</root></mxGraphModel></diagram></mxfile>`)
	doc, findings := assembleFixture(t, src)

	// 두 페이지 모두 PK가 없어 진단이 나야 이 테스트가 의미를 갖는다.
	var sawOrder, sawCustomer bool
	for _, f := range findings {
		if strings.Contains(f.Message, "주문") {
			sawOrder = true
		}
		if strings.Contains(f.Message, "고객") {
			sawCustomer = true
		}
	}
	if !sawOrder || !sawCustomer {
		t.Fatalf("픽스처가 두 페이지 진단을 다 내지 않는다 — 테스트가 무의미해진다: %+v", findings)
	}

	out, r, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	if len(r.Warnings) == 0 {
		t.Fatalf("id 없는 페이지를 건너뛴 사실을 아무도 말하지 않는다: %+v", r)
	}

	summary := cellInPage(t, out, "pg2", SummaryCellID("pg2"))
	if strings.Contains(summary.Value, "주문") {
		t.Errorf("둘째 페이지 요약 박스에 첫 페이지(id 없음)의 진단이 실렸다:\n%s", summary.Value)
	}
	if !strings.Contains(summary.Value, "고객") {
		t.Errorf("둘째 페이지 요약 박스에 자기 진단이 없다:\n%s", summary.Value)
	}
}

// 겹친 셀 id를 가진 파일에서, 진단이 붙은 페이지의 셀만 마크된다. 예전에는
// 편집이 파일 전체에 적용돼 다른 페이지의 동명 셀까지 빨갛게 물들었고,
// 그것을 막으려고 Annotate가 아예 멈췄다.
func TestAnnotateMarksOnlyTheFindingsOwnPage(t *testing.T) {
	src := []byte(`<mxfile host="test">` +
		`<diagram id="pgA" name="A"><mxGraphModel><root>` +
		`<mxCell id="0"/><mxCell id="1" parent="0"/>` +
		`<mxCell id="dup" value="주문" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"><mxGeometry x="40" y="40" width="240" height="60" as="geometry"/></mxCell>` +
		`<mxCell id="dupr" style="shape=tableRow;" vertex="1" parent="dup"><mxGeometry y="30" width="240" height="30" as="geometry"/></mxCell>` +
		`<mxCell id="dupk" value="" style="shape=partialRectangle;" vertex="1" parent="dupr"><mxGeometry width="30" height="30" as="geometry"/></mxCell>` +
		`<mxCell id="dupd" value="주문일자 DATE" style="shape=partialRectangle;" vertex="1" parent="dupr"><mxGeometry x="30" width="210" height="30" as="geometry"/></mxCell>` +
		`</root></mxGraphModel></diagram>` +
		`<diagram id="pgB" name="B"><mxGraphModel><root>` +
		`<mxCell id="0"/><mxCell id="1" parent="0"/>` +
		`<mxCell id="dup" value="고객" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"><mxGeometry x="40" y="40" width="240" height="60" as="geometry"/></mxCell>` +
		`<mxCell id="dupr" style="shape=tableRow;" vertex="1" parent="dup"><mxGeometry y="30" width="240" height="30" as="geometry"/></mxCell>` +
		`<mxCell id="dupk" value="PK" style="shape=partialRectangle;" vertex="1" parent="dupr"><mxGeometry width="30" height="30" as="geometry"/></mxCell>` +
		`<mxCell id="dupd" value="고객번호 INTEGER" style="shape=partialRectangle;" vertex="1" parent="dupr"><mxGeometry x="30" width="210" height="30" as="geometry"/></mxCell>` +
		`</root></mxGraphModel></diagram></mxfile>`)
	doc, findings := assembleFixture(t, src)
	// 진단이 정말 A페이지 것만인지 먼저 확인한다 — 이 전제가 깨지면
	// 아래 단언이 무엇을 재는지 알 수 없다.
	for _, f := range findings {
		if f.CellID != "" && f.DiagramID != "pgA" {
			t.Fatalf("픽스처 전제가 깨졌다 — 앵커 있는 진단이 %q에도 있다: %+v", f.DiagramID, f)
		}
	}

	out, r, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("Annotate: %v — 겹친 id는 더는 멈출 사유가 아니다", err)
	}
	if r.Marked == 0 {
		t.Errorf("아무것도 마크되지 않았다: %+v", r)
	}
	scanned, err := ScanExisting(out)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	if !scanned[0].Marked["dup"] {
		t.Error("A페이지의 dup이 마크되지 않았다")
	}
	if scanned[1].Marked["dup"] {
		t.Error("B페이지의 동명 셀까지 마크됐다 — 페이지 격리가 깨졌다")
	}
}

// fixtureResidueOnly는 진단이 0건인데 erdtool 표식이 남아 있는 파일이다.
// erdtoolIssue는 사람이 draw.io의 «데이터 편집» 창에서 지웠고
// erdtoolBaseStyle과 빨간 테두리만 남았다 — Existing.Marked는 비어 있고
// ResidueIDs()만 이 셀을 잡는다. 요약 박스는 없다(있으면 Summaries가
// 조기 반환을 막아버려 ResidueIDs 분기만 따로 볼 수 없다).
const fixtureResidueOnly = `<mxfile><diagram name="고객" id="pg1"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <object label="CUSTOMER" erdtoolBaseStyle="shape=table;childLayout=tableLayout;" erdtoolWrapped="1" id="t1">
    <mxCell style="shape=table;childLayout=tableLayout;strokeColor=#FF3333;strokeWidth=3;" vertex="1" parent="1">
      <mxGeometry x="80" y="40" width="240" height="60" as="geometry"/>
    </mxCell>
  </object>
  <mxCell id="r1" style="shape=tableRow;" vertex="1" parent="t1">
    <mxGeometry y="30" width="240" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1k" value="PK" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry width="30" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1d" value="CUST_ID INTEGER" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry x="30" width="210" height="30" as="geometry"/>
  </mxCell>
</root></mxGraphModel></diagram></mxfile>`

// Annotate의 조기 반환은 «진단이 0건»만으로 판단하면 안 된다. 진단이
// 없어도 지난 실행의 잔재가 남아 있으면 되돌려야 한다 — 그러지 않으면
// 고쳐서 진단이 사라진 뒤에도 빨간 테두리가 영영 남고, erdtool은
// Changed=false로 «할 일 없음»을 보고한다.
//
// 이 분기는 Marked(erdtoolIssue)가 아니라 ResidueIDs()를 본다(코드리뷰)
// . 픽스처는 사람이 erdtoolIssue만 지운 상태라 Marked는 비어
// 있고 ResidueIDs()만 이 셀을 잡는다 — Marked로 되돌리면 이 테스트가
// 실패한다.
func TestAnnotateRevertsResidueWhenNoFindings(t *testing.T) {
	src := []byte(fixtureResidueOnly)
	doc, findings := assembleFixture(t, src)
	if len(findings) != 0 {
		t.Fatalf("픽스처에서 진단이 %d건 나왔다 — 0건이어야 이 분기를 격리한다: %v",
			len(findings), findings)
	}
	perPage, err := ScanExisting(src)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	if len(perPage[0].Marked) != 0 {
		t.Fatalf("픽스처에 erdtoolIssue가 남아 있다 — Marked가 아니라 ResidueIDs를 보는지 못 가린다")
	}

	out, res, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	if !res.Changed {
		t.Fatal("Changed=false — 잔재가 있으면 조기 반환하면 안 된다")
	}
	if bytes.Contains(out, []byte("erdtoolBaseStyle")) {
		t.Errorf("erdtoolBaseStyle이 남았다:\n%s", out)
	}
	if bytes.Contains(out, []byte("#FF3333")) {
		t.Errorf("빨간 테두리가 남았다:\n%s", out)
	}
	if !bytes.Contains(out, []byte("shape=table;childLayout=tableLayout;")) {
		t.Errorf("원래 스타일이 복원되지 않았다:\n%s", out)
	}
}

// 반대쪽: 진단도 잔재도 없으면 조기 반환이 그대로 살아 있어야 한다.
// 위 테스트를 통과시키려고 조기 반환 자체를 없애면 이쪽이 잡는다.
func TestAnnotateStillReturnsEarlyWhenNothingToDo(t *testing.T) {
	src := []byte(fixtureClean)
	doc, findings := assembleFixture(t, src)
	if len(findings) != 0 {
		t.Fatalf("픽스처에서 진단이 %d건 나왔다: %v", len(findings), findings)
	}

	out, res, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	if res.Changed {
		t.Error("Changed=true — 할 일이 없으면 재작성기를 아예 안 타야 한다")
	}
	if !bytes.Equal(src, out) {
		t.Errorf("바이트가 달라졌다:\n%s", out)
	}
}
