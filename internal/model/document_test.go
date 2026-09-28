package model

import (
	"encoding/json"
	"testing"
)

func TestDocumentJSONShape(t *testing.T) {
	doc := Document{
		IRVersion:  "1.0",
		SourceFile: "sample.drawio",
		Diagrams: []Diagram{{
			ID:   "d1",
			Name: "페이지-1",
			Tables: []Table{{
				ID:   "t1",
				Name: "Orders",
				Columns: []Column{{
					ID: "c1", Name: "order_id", Type: "int",
					Nullable: false, Key: "PK", RawValue: "order_id int NOT NULL",
				}},
			}},
			Relationships: []Relationship{{
				ID: "r1", Label: "", SourceTableID: "t1", SourceColumnID: "c1",
				SourceCardinality: "ERone", TargetTableID: "t2", TargetColumnID: "",
				TargetCardinality: "ERzeroToMany", ColumnLevel: false,
			}},
		}},
	}

	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var round Document
	if err := json.Unmarshal(b, &round); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if round.Diagrams[0].Tables[0].Columns[0].Key != "PK" {
		t.Fatalf("expected Key=PK, got %q", round.Diagrams[0].Tables[0].Columns[0].Key)
	}
	if round.Diagrams[0].Relationships[0].ColumnLevel != false {
		t.Fatalf("expected ColumnLevel=false to round-trip")
	}
}

func TestColumnIsPrimaryKey(t *testing.T) {
	// M1: PK 판정이 tables.go/required.go/sql.go 3곳에 흩어져 있었고 대소문자
	// 처리가 서로 달랐다(isKeyLike만 대소문자 무시). 단일 진입점으로 통합한다.
	cases := []struct {
		key  string
		want bool
	}{
		{"PK", true},
		{"pk", true},  // draw.io 사용자는 소문자로도 쓴다
		{"PK1", true}, // 복합키 번호
		{"PK, FK1", true},
		{" pk ", true}, // 셀 값에 공백이 섞임
		{"PK/FK", true},
		{"FK", false},
		{"FK1", false},
		{"", false},
		{"PACKAGE", false}, // "PK" 접두 오탐 방지: P-K가 연속이어야 한다
	}
	for _, c := range cases {
		got := Column{Key: c.key}.IsPrimaryKey()
		if got != c.want {
			t.Errorf("Column{Key:%q}.IsPrimaryKey() = %v, want %v", c.key, got, c.want)
		}
	}
}

func TestColumnIsForeignKey(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"FK", true},
		{"fk1", true},
		{"PK, FK1", true}, // PK이면서 동시에 FK인 컬럼
		{"PK", false},
		{"", false},
	}
	for _, c := range cases {
		got := Column{Key: c.key}.IsForeignKey()
		if got != c.want {
			t.Errorf("Column{Key:%q}.IsForeignKey() = %v, want %v", c.key, got, c.want)
		}
	}
}
