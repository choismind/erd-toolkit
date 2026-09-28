package dbreverse

import (
	"reflect"
	"testing"
)

func TestSchemaSort_IsDeterministic(t *testing.T) {
	s := Schema{
		Name: "public",
		Tables: []Table{
			{Name: "orders", Columns: []Column{
				{Name: "amount", Ordinal: 2}, {Name: "id", Ordinal: 1},
			}},
			{Name: "customer", Columns: []Column{{Name: "id", Ordinal: 1}}},
		},
		ForeignKeys: []ForeignKey{
			{Name: "fk_b", Table: "orders"},
			{Name: "fk_a", Table: "orders"},
		},
	}
	s.Sort()

	if s.Tables[0].Name != "customer" || s.Tables[1].Name != "orders" {
		t.Errorf("테이블이 이름 오름차순이 아니다: %v", []string{s.Tables[0].Name, s.Tables[1].Name})
	}
	if s.Tables[1].Columns[0].Name != "id" {
		t.Errorf("컬럼이 Ordinal 오름차순이 아니다: %+v", s.Tables[1].Columns)
	}
	if s.ForeignKeys[0].Name != "fk_a" {
		t.Errorf("FK가 이름 오름차순이 아니다: %+v", s.ForeignKeys)
	}

	// 두 번 불러도 같아야 한다. Sort가 제자리에서 돌므로 반드시 깊은 복사와
	// 비교한다 — 얕은 복사는 같은 저장소를 가리켜 언제나 통과한다.
	before := s.clone()
	s.Sort()
	if !reflect.DeepEqual(before, s) {
		t.Errorf("Sort를 두 번 부르면 결과가 달라진다:\n첫 번째: %+v\n두 번째: %+v", before, s)
	}
}

// TestSchemaClone_IsDeep은 위 테스트가 기대는 clone 자체를 검사한다.
// 이것이 얕으면 위 «두 번 불러도 같다»가 조용히 아무것도 검사하지 않게 된다.
func TestSchemaClone_IsDeep(t *testing.T) {
	s := Schema{
		Tables: []Table{{Name: "t", Columns: []Column{{Name: "a"}}}},
		ForeignKeys: []ForeignKey{
			{Name: "fk", Columns: []ForeignKeyColumn{{Column: "a", TargetColumn: "b"}}},
		},
	}
	c := s.clone()
	s.Tables[0].Columns[0].Name = "바뀜"
	s.ForeignKeys[0].Columns[0].Column = "바뀜"
	if c.Tables[0].Columns[0].Name != "a" {
		t.Error("clone이 컬럼 슬라이스를 공유한다")
	}
	if c.ForeignKeys[0].Columns[0].Column != "a" {
		t.Error("clone이 FK 컬럼 슬라이스를 공유한다")
	}
}
