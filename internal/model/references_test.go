package model

import "testing"

// twoTables는 부모(고객) 하나와 자식(주문) 하나에 관계선 하나를 둔 페이지다.
// srcKey/tgtKey로 양 끝의 키 셀 표기를 바꿔 가며 판정을 확인한다.
func twoTables(srcKey, tgtKey string) Diagram {
	return Diagram{
		ID: "p1", Name: "물리",
		Tables: []Table{
			{ID: "t1", Name: "고객", Columns: []Column{{ID: "r1", Name: "고객번호", Key: srcKey}}},
			{ID: "t2", Name: "주문", Columns: []Column{{ID: "r2", Name: "주문고객번호", Key: tgtKey}}},
		},
		Relationships: []Relationship{{
			ID:             "e1",
			SourceTableID:  "t1",
			SourceColumnID: "r1",
			SourceResolved: true,
			TargetTableID:  "t2",
			TargetColumnID: "r2",
			TargetResolved: true,
		}},
	}
}

func TestColumnReferences_FK를단쪽이자식이다(t *testing.T) {
	refs := twoTables("PK", "FK1").ColumnReferences()
	ref, ok := refs["r2"]
	if !ok {
		t.Fatalf("자식 컬럼에 참조가 붙어야 한다: %+v", refs)
	}
	if ref.TableName != "고객" || ref.ColumnName != "고객번호" {
		t.Errorf("참조 대상: %+v", ref)
	}
	if _, ok := refs["r1"]; ok {
		t.Errorf("부모 컬럼에는 참조가 붙지 않는다")
	}
}

func TestColumnReferences_방향이반대여도FK표기를따른다(t *testing.T) {
	// 관계선을 자식에서 부모로 그리는 사람도 있다. 판정은 선의 방향이 아니라
	// 키 표기다.
	refs := twoTables("FK1", "PK").ColumnReferences()
	ref, ok := refs["r1"]
	if !ok {
		t.Fatalf("FK를 단 쪽에 참조가 붙어야 한다: %+v", refs)
	}
	if ref.TableName != "주문" {
		t.Errorf("참조 대상: %+v", ref)
	}
}

func TestColumnReferences_판정못하면짓지않는다(t *testing.T) {
	// 둘 다 FK가 없거나 둘 다 FK면 어느 쪽이 자식인지 알 수 없다. 지어내면
	// 정의서가 «A가 B를 참조한다»고 단언하는데 실제로는 반대일 수 있고,
	// 문서를 읽는 사람은 의심할 근거가 없다.
	for _, keys := range [][2]string{{"PK", ""}, {"FK1", "FK1"}, {"", ""}} {
		d := twoTables(keys[0], keys[1])
		if refs := d.ColumnReferences(); len(refs) != 0 {
			t.Errorf("키가 %v일 때 참조를 지어냈다: %+v", keys, refs)
		}
		if un := d.UnmatchedRelationships(); len(un) != 1 {
			t.Errorf("키가 %v일 때 판정 못한 관계로 보고해야 한다: %+v", keys, un)
		}
	}
}

func TestColumnReferences_끊어진관계는건너뛴다(t *testing.T) {
	// 그쪽은 broken_reference가 따로 본다. 여기서 또 진단을 내면 한 실수가
	// 두 줄로 보고된다.
	d := twoTables("PK", "FK1")
	d.Relationships[0].SourceResolved = false
	if len(d.ColumnReferences()) != 0 || len(d.UnmatchedRelationships()) != 0 {
		t.Errorf("끊어진 관계는 이 층이 건드리지 않는다")
	}
}

func TestColumnReferences_테이블에직접연결되면컬럼이름이없다(t *testing.T) {
	// 원장: 관계선이 행이 아니라 테이블에 직접 연결된 경우.
	d := twoTables("PK", "FK1")
	d.Relationships[0].SourceColumnID = "" // 부모 쪽이 테이블에 붙었다
	ref := d.ColumnReferences()["r2"]
	if ref.TableName != "고객" {
		t.Fatalf("테이블은 알 수 있어야 한다: %+v", ref)
	}
	if ref.ColumnName != "" {
		t.Errorf("없는 컬럼 이름을 지어내면 안 된다: %q", ref.ColumnName)
	}
}
