// internal/convert/genbuild_roundtrip_test.go
package convert

import (
	"testing"

	"erdtool/internal/genbuild"
	"erdtool/internal/glossary"
)

// TestFile_AcceptsMultiPageGenbuildOutput은 build->convert 흐름 전체를
// 재현한다(원장의 headline 플로우: "이름 변환이 필요하면 convert를 이어서
// 돌린다"). genbuild.Build가 만든 여러 페이지짜리 산출물을 실제로
// convert.File에 먹여서, 페이지 사이 셀 id 겹침으로 거부되지 않는지
// 확인한다.
//
// 리뷰에서 발견한 치명적 결함: emitPage가 페이지마다 t0/t0_r0/t0_r0d/e0를 처음부터
// 다시 매기던 시절에는, 두 페이지가 있는 build 산출물을 convert.File에
// 넣으면 "셀 id가 페이지 사이에서 겹친다"는 하드 에러로 거부됐다 — build의
// 산출물을 convert가 받아들이지 못하는 것은 이 툴체인의 존재 이유(설계서 ->
// drawio -> (선택) 이름 변환 -> generate)를 정면으로 어긴다.
//
// 2026-08-31 갱신: convert.RewritePlan이 페이지 단위로 주소되면서
// 그 하드 에러 자체가 없어졌다 — 이 테스트는 이제 그 이유로는
// 실패할 수 없다. 그렇다고 회귀 방어가 사라진 것은 아니다. genbuild가
// 다시 페이지마다 id를 0부터 매기는 버그로 돌아가더라도
// TestBuild_CellIDsAreUniqueAcrossPages(internal/genbuild/emit_test.go)가
// 그 성질을 발생원(genbuild.Build)에서 직접 강제한다 — 이 테스트는 이제
// "convert가 그런 산출물도 받아들인다"만 지킨다(어차피 참이지만, 페이지
// 단위 주소화 전에도 이 자체 경로는 항상 성립했을 성질이다).
func TestFile_AcceptsMultiPageGenbuildOutput(t *testing.T) {
	// 두 페이지는 테이블/컬럼 이름을 서로 다르게 둔다 — 완전히 같은 편집은
	// 겹쳐도 손상이 아니라서(File의 "완전히 같은 편집이 겹치는 것은 손상이
	// 아니다" 처리) id만 같고 내용까지 같으면 이 회귀를 못 잡는다. 서로
	// 다른 물리명으로 변환되는 두 페이지라야 접두 없는 옛 emitPage가 만든
	// 겹치는 id 위에서 실제 충돌(다른 값)이 발생해 File이 거부한다.
	pages := []genbuild.PageDef{
		{
			Name: "주문정보",
			Tables: []genbuild.TableDef{
				{Name: "고객", Columns: []genbuild.ColumnDef{
					{Name: "고객번호", Type: "int", Key: "PK", Nullable: false},
				}},
			},
		},
		{
			Name: "선적정보",
			Tables: []genbuild.TableDef{
				{Name: "배송지", Columns: []genbuild.ColumnDef{
					{Name: "배송지번호", Type: "int", Key: "PK", Nullable: false},
				}},
			},
		},
	}

	src, err := genbuild.Build(pages)
	if err != nil {
		t.Fatalf("genbuild.Build: %v", err)
	}

	d := glossary.MustLoadForTest(
		map[string]string{"고객번호": "CUST_NO", "배송지번호": "SHIP_ADDR_NO"},
		map[string]string{"고객": "CUST", "번호": "NO", "배송지": "SHIP_ADDR"},
	)

	out, stats, err := File(src, d)
	if err != nil {
		t.Fatalf("convert.File이 다중 페이지 build 산출물을 거부했다: %v", err)
	}
	if stats.Converted == 0 {
		t.Fatalf("변환된 항목이 0개다: %+v", stats)
	}
	if len(out) == 0 {
		t.Fatal("출력이 비어 있다")
	}
}
