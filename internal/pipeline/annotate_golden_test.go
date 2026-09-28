// internal/pipeline/annotate_golden_test.go
package pipeline

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"erdtool/internal/annotate"
	"erdtool/internal/config"
	"erdtool/internal/convert"
	"erdtool/internal/ir"
	"erdtool/internal/validate"
)

// 이 파일은 annotate의 계약 네 가지를 «실물 픽스처»에 건다.
//
// annotate 스펙의 "파일 왕복 — 계약 4종"이 요구하는 것이 이것인데 지금까지 없었다. annotate의
// 모든 픽스처는 손으로 적은 인라인 문자열이었다 — 평문 XML, 한 페이지,
// 레이어 없음, 간선 waypoint 없음. 그 모양 하나만 보고 있으면 «소비자 하나,
// 모양 하나»의 함정에 그대로 빠진다. 지난 페이즈의 Critical이 리뷰 열한 번을
// 통과한 경로가 정확히 그것이었다.
//
// 골든(internal/drawio/testdata/*.drawio)은 이 저장소가 실제로 다루는 모양을
// 담고 있다: 여러 페이지(entity_table_basic 3장, out_of_order_cells 2장),
// <object> 래퍼로 감싼 앵커 도형, 매달린 참조, 뒤섞인 셀 순서,
// "text;strokeColor=none;" 같은 bare 도형 토큰, 그리고 **압축된 페이지**.
//
// 압축 골든은 둘이다. relationship_logical은 진단이 0건이라 아래 규칙대로
// 건너뛰지만, out_of_order_cells는 **진단이 나므로 실제로 계약 넷을 전부
// 탄다** — 이 파일에는 mxGraphModel 요소가 하나도 없고 <diagram> 둘의 몸통이
// base64 + raw-deflate다(압축을 풀면 mxGraphModel이 나온다. compressed="true"
// 속성은 draw.io가 늘 적지는 않으므로 그 속성으로 판별하면 이 파일을 놓친다).
// 페이지 2장에 진단 7건·마크 6개가 나고, 진단 «정체» 비교를 깨는 변이를
// 잡아낸 것도 이 골든이다. 즉 스펙이 요구한 «실물 픽스처»에 압축된 실제
// draw.io 파일이 들어 있다.
//
// 그리고 이 테스트는 순수 함수가 아니라 «파이프라인»에 건다 — pipeline.Annotate가
// 파일을 읽고, 표시하고, 제자리에 쓰고, --clean이 다시 제자리에 쓴다. 그
// 쓰기 경로(Task 14)는 지금까지 테스트가 하나도 없었다.

// findingKey는 진단 하나의 «정체»다. 문구는 안 본다 — 문구는 얼마든지
// 다듬을 수 있고, 여기서 지키려는 것은 "annotate를 돌려도 검증이 보는
// 사실 자체는 그대로다"이기 때문이다.
type findingKey struct {
	Rule   string
	CellID string
}

// findingMultiset은 진단을 «집합»으로 만든다. 개수만 세면 진단 하나가
// 사라지고 다른 하나가 새로 생긴 것을 못 잡는다 — 실제로 그 비교를
// 쓰고 있었고(Task 16), 그래서 여기서는 정체를 센다. 같은 (규칙, 셀)이
// 두 번 나오는 것도 사실이므로 개수까지 함께 본다.
func findingMultiset(fs []validate.Finding) map[findingKey]int {
	out := map[findingKey]int{}
	for _, f := range fs {
		out[findingKey{Rule: f.Rule, CellID: f.CellID}]++
	}
	return out
}

// findingsOf는 파일 하나를 pipeline.Annotate와 똑같은 방식으로 검증한다.
func findingsOf(t *testing.T, path string, cfg config.Config) []validate.Finding {
	t.Helper()
	doc, dups, err := ir.Assemble(path)
	if err != nil {
		t.Fatalf("Assemble(%s): %v", path, err)
	}
	return append(validate.Required(doc, dups), validate.Optional(doc, cfg)...)
}

// goldenDrawioFiles는 internal/drawio/testdata의 .drawio 골든 전부다.
// rowspan_example.xml은 확장자가 달라 자연히 빠진다.
func goldenDrawioFiles(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "drawio", "testdata", "*.drawio"))
	if err != nil {
		t.Fatalf("골든 목록: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("골든을 하나도 못 찾았다 — 경로가 바뀌었으면 이 테스트는 아무것도 안 지키고 있다")
	}
	return paths
}

// TestAnnotateGoldenRoundTripContracts는 골든마다
// Annotate -> 재조립 -> Annotate -> Clean을 실제로 돌리고 계약 넷을 건다.
//
//  1. 멱등성: 두 번째 실행이 첫 번째와 «바이트가 같다».
//  2. 왕복: Clean(Annotate(x)) == convert.RewriteMxFilePlan(x, 빈 계획).
//  3. 잔재 0: --clean 뒤 erdtool 표식이 한 글자도 안 남는다.
//  4. 진단 집합 불변: annotate 뒤에도 (규칙, 셀id) 집합이 그대로다.
//
// 진단이 0건인 골든은 «건너뛴다». 그런 파일에 계약을 걸면 전부 자명하게
// 참이라(아무것도 안 바뀌므로) 초록은 나오는데 지키는 것이 없다 — 이
// 저장소가 반복해서 데인 «코드를 지워도 통과하는 단언»이 바로 그 모양이다.
func TestAnnotateGoldenRoundTripContracts(t *testing.T) {
	cfg := config.Config{}
	maxPages := 0
	multiPageMarked := false

	for _, golden := range goldenDrawioFiles(t) {
		t.Run(filepath.Base(golden), func(t *testing.T) {
			orig, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("골든 읽기: %v", err)
			}
			// annotate는 제자리 수정이다 — 골든 자체를 절대 안 건드린다.
			work := filepath.Join(t.TempDir(), filepath.Base(golden))
			if err := os.WriteFile(work, orig, 0o644); err != nil {
				t.Fatalf("작업본 쓰기: %v", err)
			}

			want := findingMultiset(findingsOf(t, work, cfg))
			if len(want) == 0 {
				t.Skip("이 골든은 진단이 0건이다 — 표시할 것이 없으므로 왕복 계약이 검증할 것도 없다")
			}

			// 1회차. 제자리에 쓴다.
			r1, err := Annotate(work, cfg, false, false)
			if err != nil {
				t.Fatalf("1회차 Annotate: %v", err)
			}
			if !r1.Changed {
				t.Fatalf("진단이 %d종 있는데 파일이 안 바뀌었다: %+v", len(want), r1)
			}
			if r1.Pages == 0 {
				t.Fatalf("진단이 있는데 요약 박스를 만든 페이지가 0이다: %+v", r1)
			}
			once, err := os.ReadFile(work)
			if err != nil {
				t.Fatalf("1회차 결과 읽기: %v", err)
			}
			if maxPages < r1.Pages {
				maxPages = r1.Pages
			}
			// Task 11의 «여러 페이지에 걸친 마크 누적»이 실제로 돈
			// 골든인지 본다.
			//
			// 예전에는 r1.Pages >= 2 && r1.Marked >= 2로 갈음했는데, 그
			// 둘은 파일 전체의 «요약 박스를 만든 페이지 수»와 «마크한 셀
			// 총합»이라 한 페이지에 마크가 둘 있고 다른 페이지는 요약
			// 박스만 받아도 참이다 — 재려던 것(마크가 두 페이지에
			// «걸쳤는가»)을 실제로는 안 쟀다. 산출물을 다시 읽어 페이지별
			// 마크를 세면 대용물이 아니라 그 사실 자체를 본다.
			perPage, err := annotate.ScanExisting(once)
			if err != nil {
				t.Fatalf("1회차 결과 스캔: %v", err)
			}
			pagesWithMarks := 0
			for _, e := range perPage {
				if len(e.Marked) > 0 {
					pagesWithMarks++
				}
			}
			if pagesWithMarks >= 2 {
				multiPageMarked = true
			}

			// 2회차. 이미 표시된 파일을 «다시 읽고 다시 검증»해서 돈다 —
			// 재조립이 여기 들어 있다.
			if _, err := Annotate(work, cfg, false, false); err != nil {
				t.Fatalf("2회차 Annotate: %v", err)
			}
			twice, err := os.ReadFile(work)
			if err != nil {
				t.Fatalf("2회차 결과 읽기: %v", err)
			}
			// 계약 1: 멱등성.
			if !bytes.Equal(once, twice) {
				t.Errorf("두 번 돌리면 바이트가 달라진다 (1회차 %d바이트, 2회차 %d바이트)", len(once), len(twice))
			}

			// 계약 4: 진단의 «집합»이 그대로다. 개수가 아니라 정체다 —
			// 하나가 사라지고 다른 하나가 생겨도 개수는 같다.
			if got := findingMultiset(findingsOf(t, work, cfg)); !reflect.DeepEqual(want, got) {
				t.Errorf("표시 뒤 진단의 집합이 달라졌다\n원본: %v\n표시 뒤: %v", want, got)
			}

			// --clean도 제자리 쓰기다(Task 14). 여기가 그 경로의 유일한 그물이다.
			rc, err := Annotate(work, cfg, true, false)
			if err != nil {
				t.Fatalf("--clean: %v", err)
			}
			if !rc.Changed {
				t.Fatalf("지울 표식이 있었는데 --clean이 아무것도 안 했다: %+v", rc)
			}
			if rc.Marked == 0 && rc.Pages == 0 {
				t.Errorf("--clean이 지운 것을 하나도 안 셌다: %+v", rc)
			}
			back, err := os.ReadFile(work)
			if err != nil {
				t.Fatalf("--clean 결과 읽기: %v", err)
			}

			// 계약 2: 「원본 바이트 그대로」가 아니라 「그 파일 자신의 정규화
			// 왕복형」이다. encoding/xml이 <a/>와 <a></a>를 토큰에서 구별하지
			// 못하므로 원본 자체로는 원리적으로 못 돌아간다(annotate.Clean 주석).
			normalized, err := convert.RewriteMxFilePlan(orig, convert.RewritePlan{})
			if err != nil {
				t.Fatalf("순수 왕복: %v", err)
			}
			if !bytes.Equal(normalized, back) {
				t.Errorf("--clean 뒤 정규화 왕복형으로 안 돌아왔다 (기대 %d바이트, 결과 %d바이트)",
					len(normalized), len(back))
			}

			// 계약 3: 잔재 0. 파싱해서 보고, 바이트로도 본다 — 파서가 못 보는
			// 자리에 남은 문자열도 사용자 눈에는 보인다.
			ePerPage, err := annotate.ScanExisting(back)
			if err != nil {
				t.Fatalf("ScanExisting: %v", err)
			}
			for i, e := range ePerPage {
				if len(e.ResidueIDs()) != 0 || len(e.Summaries) != 0 {
					t.Errorf("--clean 뒤에도 erdtool 잔재가 남았다(페이지 %d): %+v", i, e)
				}
			}
			if bytes.Contains(back, []byte("erdtool")) {
				t.Errorf("--clean 뒤에도 \"erdtool\" 문자열이 파일에 남아 있다")
			}
		})
	}

	// 골든 목록이 어느 날 한 페이지짜리만 남으면 이 테스트는 초록인 채로
	// 교차 페이지 누적(Task 11)을 안 덮게 된다. 그 사실을 조용히 넘기지
	// 않는다.
	if maxPages < 2 {
		t.Errorf("여러 페이지에 요약 박스를 넣은 골든이 하나도 없었다(최대 %d) — 교차 페이지 누적이 안 덮인다", maxPages)
	}
	if !multiPageMarked {
		t.Errorf("«두 페이지 이상에 각각» 셀을 마크한 골든이 하나도 없었다 — 교차 페이지 마크 누적이 안 덮인다")
	}
}
