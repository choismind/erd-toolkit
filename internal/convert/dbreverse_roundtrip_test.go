package convert

import (
	"testing"

	"erdtool/internal/dbreverse"
	"erdtool/internal/genbuild"
	"erdtool/internal/glossary"
)

// TestFile_AcceptsDBReverseOutput는 역공학 산출물이 convert의 «페이지 사이
// 셀 id 충돌» 가드를 통과함을 고정한다. Phase 2a에서 태스크별 리뷰 11번을
// 전부 통과한 결함이 바로 이 자리였다 — emitPage가 페이지마다 셀 id를 0부터
// 다시 매기던 시절, 두 페이지짜리 산출물을 convert.File에 넣으면 하드
// 에러로 거부됐다.
//
// SQLite로는 재현되지 않는다(페이지가 main 하나뿐이다). 그래서 스키마 둘을
// 손으로 만들어 ToPageDefs부터의 진짜 경로에 태운다.
func TestFile_AcceptsDBReverseOutput(t *testing.T) {
	schemas := []dbreverse.Schema{
		{
			Name: "주문정보",
			Tables: []dbreverse.Table{
				{Name: "고객", Columns: []dbreverse.Column{
					{Name: "고객번호", Type: "integer", Ordinal: 1, PK: true},
				}},
				{Name: "주문", Columns: []dbreverse.Column{
					{Name: "주문번호", Type: "integer", Ordinal: 1, PK: true},
					{Name: "주문고객번호", Type: "integer", Ordinal: 2},
				}},
			},
			ForeignKeys: []dbreverse.ForeignKey{{
				Name: "주문_fk1", Table: "주문",
				TargetSchema: "주문정보", TargetTable: "고객",
				Columns: []dbreverse.ForeignKeyColumn{
					{Column: "주문고객번호", TargetColumn: "고객번호"},
				},
			}},
		},
		{
			Name: "선적정보",
			Tables: []dbreverse.Table{
				{Name: "배송지", Columns: []dbreverse.Column{
					{Name: "배송지번호", Type: "integer", Ordinal: 1, PK: true},
				}},
			},
		},
	}

	src, err := genbuild.Build(dbreverse.ToPageDefs(schemas).Pages)
	if err != nil {
		t.Fatalf("genbuild.Build: %v", err)
	}

	d := glossary.MustLoadForTest(
		map[string]string{
			"고객번호": "CUST_NO", "주문번호": "ORDR_NO",
			"주문고객번호": "ORDR_CUST_NO", "배송지번호": "SHIP_ADDR_NO",
		},
		map[string]string{
			"고객": "CUST", "번호": "NO", "주문": "ORDR", "배송지": "SHIP_ADDR",
		},
	)

	out, stats, err := File(src, d)
	if err != nil {
		t.Fatalf("convert가 역공학 산출물을 거부했다: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("변환 결과가 비었다")
	}
	if stats.Converted == 0 {
		t.Errorf("변환된 셀이 0건이다: %+v", stats)
	}
}
