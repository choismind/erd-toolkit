// internal/annotate/consumers_test.go
package annotate

import (
	"strings"
	"testing"

	"erdtool/internal/convert"
)

// annotate가 만든 파일을 generate에 태워도 진단이 늘지 않아야 한다.
// 요약 박스가 shape_violation으로 잡히면 돌릴 때마다 진단이 하나씩
// 늘어나고, 그 진단이 다음 요약 박스에 실려 무한히 자란다.
func TestAnnotatedFileDoesNotGrowFindings(t *testing.T) {
	src := []byte(fixtureWithMissingPK)
	doc, before := assembleFixture(t, src)

	out, _, err := Annotate(src, doc, before)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	_, after := assembleFixture(t, out)

	if len(after) != len(before) {
		t.Errorf("진단이 %d건 -> %d건으로 바뀌었다\n이전: %v\n이후: %v",
			len(before), len(after), before, after)
	}
	for _, f := range after {
		if f.Rule == "shape_violation" {
			t.Errorf("새 shape_violation이 생겼다(cellID=%q): %s", f.CellID, f.Message)
		}
	}
}

// 페이지 레벨도 안 뒤집혀야 한다. Ignored는 Parsed/Violation 집계에
// 안 들어가므로 note를 넣어도 relational이 유지된다.
//
// fixtureWithMissingPK가 아니라 fixtureLonelyTable을 쓴다(작업 16 리뷰
// 라운드 1) — fixtureWithMissingPK는 Parsed 셀이 4개라 annotate가 손대는
// annotate는 자기 물건(요약 박스, <object> 래퍼)을 그림에 더한다. 그것 때문에
// 다시 읽었을 때 결과가 달라지면 안 된다. fixtureLonelyTable은 읽히는 도형이
// t1 하나뿐이라 annotate가 더한 것의 비중이 가장 큰 픽스처다.
func TestAnnotatedFileKeepsWhatIsParsed(t *testing.T) {
	src := []byte(fixtureLonelyTable)
	doc, findings := assembleFixture(t, src)
	out, _, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	doc2, findings2 := assembleFixture(t, out)
	if len(doc2.Diagrams[0].Tables) != len(doc.Diagrams[0].Tables) {
		t.Errorf("annotate 뒤 테이블이 %d개 -> %d개로 바뀌었다",
			len(doc.Diagrams[0].Tables), len(doc2.Diagrams[0].Tables))
	}
	if len(findings2) != len(findings) {
		t.Errorf("annotate 뒤 진단이 %d건 -> %d건으로 바뀌었다\n이전: %v\n이후: %v",
			len(findings), len(findings2), findings, findings2)
	}
}

// convert의 재작성 경로도 태운다. annotate가 남긴 <object> 래퍼와
// 커스텀 속성이 convert의 logicalName 처리와 부딪히지 않아야 한다.
//
// convert.File이 아니라 재작성기를 직접 쓰는 이유: File은
// *glossary.Dict를 요구하는데 그 구조체는 필드가 비공개이고 xlsx를
// 읽는 Load 말고 생성자가 없다(2026-08-28 확인). 테스트가 확인하려는
// 것은 사전 변환이 아니라 «두 패키지의 래퍼 처리가 부딪히는가»이므로
// 재작성기를 직접 부르는 편이 좁고 정확하다.
//
// 예전에는 convert.RewriteMxFile을 썼다. 그 함수는 「파일 전체에 한 벌의
// 편집」이라는 페이지 단위화 이전의 뜻을 들고 있었고 생산 코드에는
// 호출부가 없어 2026-09-01에 공개 API에서 걷어냈다. 여기서는 대신
// convert.File이 실제로 만드는 것과 같은 모양 — 페이지별 PagePlan —
// 을 직접 조립한다. 픽스처가 한 페이지짜리라 PagePlan도 하나다.
func TestAnnotatedFileSurvivesConvertRewrite(t *testing.T) {
	src := []byte(fixtureWithMissingPK)
	doc, findings := assembleFixture(t, src)
	marked, _, err := Annotate(src, doc, findings)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}

	// convert가 하는 일과 같은 모양의 편집을 건다: 같은 셀의 label을
	// 바꾸고 logicalName을 남긴다. 속성 이름 "logicalName"은 convert의
	// 비공개 상수와 같은 값이며, 아래에서 산출물을 그 문자열로 다시
	// 확인하므로 둘이 갈리면 이 테스트가 잡는다.
	label := "ORDER"
	converted, err := convert.RewriteMxFilePlan(marked, convert.RewritePlan{
		Pages: []convert.PagePlan{{
			DiagramID: "pg1",
			Edits: map[string]convert.CellEdit{"t1": {
				Label: &label,
				Attrs: map[string]string{"logicalName": "주문"},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("convert.RewriteMxFilePlan: %v", err)
	}

	// 마크 표식이 살아남아야 한다 — convert는 이름만 바꾼다.
	ePerPage, err := ScanExisting(converted)
	if err != nil {
		t.Fatalf("ScanExisting: %v", err)
	}
	e := ePerPage[0]
	if !e.Marked["t1"] {
		t.Error("convert를 지나며 t1의 마크가 사라졌다")
	}
	if e.BaseStyle["t1"] == "" {
		t.Error("convert를 지나며 erdtoolBaseStyle이 사라졌다 — --clean이 죽는다")
	}
	// 그리고 convert의 것도 살아남아야 한다.
	if !strings.Contains(string(converted), `logicalName="주문"`) {
		t.Errorf("logicalName이 안 붙었다:\n%s", converted)
	}
	// 이중 래핑이 없어야 한다. 있으면 파서가 테이블을 통째로 놓친다.
	if strings.Count(string(converted), `id="t1"`) != 1 {
		t.Errorf("t1이 %d번 나온다; 1번이어야 한다 (이중 래핑)",
			strings.Count(string(converted), `id="t1"`))
	}

	// 위의 속성 확인만으로는 부족하다(작업 16 리뷰 라운드 1) —
	// erdtoolIssue/erdtoolBaseStyle은 속성이고, 실제 마크(빨간 테두리)와
	// shape=table은 같은 style 문자열 안에 있다. convert가 감싸인 mxCell의
	// style을 지우거나(예: style="") shape=table을 잃게 만들어도 위 두
	// 속성 확인은 여전히 통과한다 — Phase 2a가 놓친 바로 그 실패 모양(한
	// 층에서만 통과했다)이 한 겹 안쪽에서 재발할 수 있다는 뜻이다. 그래서
	// convert를 지난 바이트를 실제로 다시 파싱해 테이블이 여전히
	// 테이블로, 진단 수가 그대로인지까지 본다.
	doc3, f3 := assembleFixture(t, converted)
	if len(doc3.Diagrams[0].Tables) != 1 {
		t.Errorf("convert 이후 테이블이 %d개 파싱됐다; 1개여야 한다(t1)", len(doc3.Diagrams[0].Tables))
	}
	if len(f3) != len(findings) {
		t.Errorf("convert 이후 진단이 %d건 -> %d건으로 바뀌었다\n이전: %v\n이후: %v",
			len(findings), len(f3), findings, f3)
	}
}
