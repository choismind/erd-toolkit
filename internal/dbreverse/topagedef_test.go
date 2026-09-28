package dbreverse

import "testing"

func sampleSchema() Schema {
	return Schema{
		Name: "public",
		Tables: []Table{
			{Name: "customer", Columns: []Column{
				{Name: "id", Type: "integer", Ordinal: 1, PK: true},
				{Name: "name", Type: "character varying(50)", Ordinal: 2, Nullable: true},
			}},
			{Name: "orders", Columns: []Column{
				{Name: "id", Type: "integer", Ordinal: 1, PK: true},
				{Name: "customer_id", Type: "integer", Ordinal: 2}, // NOT NULL
			}},
		},
		ForeignKeys: []ForeignKey{{
			Name: "orders_customer_id_fkey", Table: "orders",
			TargetSchema: "public", TargetTable: "customer",
			Columns: []ForeignKeyColumn{{Column: "customer_id", TargetColumn: "id"}},
		}},
	}
}

func TestToPageDefs_MapsTablesAndColumns(t *testing.T) {
	res := ToPageDefs([]Schema{sampleSchema()})
	if len(res.Pages) != 1 || res.Pages[0].Name != "public" {
		t.Fatalf("페이지가 스키마와 1:1이어야 한다: %+v", res.Pages)
	}
	p := res.Pages[0]
	if len(p.Tables) != 2 {
		t.Fatalf("테이블 개수 = %d; want 2", len(p.Tables))
	}
	cust := p.Tables[0]
	if cust.Name != "customer" || cust.Columns[0].Key != "PK" {
		t.Errorf("PK 표시가 없다: %+v", cust.Columns[0])
	}
	if got := cust.Columns[1].Type; got != "character varying(50)" {
		t.Errorf("타입이 가공됐다: %q", got)
	}
}

// FK 컬럼은 FK1으로, PK이면서 FK면 "PK,FK1"으로 표시한다.
// 이 표기는 커밋된 픽스처에 실제로 있는 값이다.
func TestToPageDefs_KeyMarkers(t *testing.T) {
	s := sampleSchema()
	s.Tables[1].Columns[1].PK = true // orders.customer_id를 PK이자 FK로
	res := ToPageDefs([]Schema{s})
	orders := res.Pages[0].Tables[1]
	if got := orders.Columns[1].Key; got != "PK,FK1" {
		t.Errorf("Key = %q; want %q", got, "PK,FK1")
	}
}

// 관계는 부모 -> 자식 방향이고, 카디널리티는 DecideCardinality가 정한다.
func TestToPageDefs_RelationDirectionAndCardinality(t *testing.T) {
	res := ToPageDefs([]Schema{sampleSchema()})
	rels := res.Pages[0].Relations
	if len(rels) != 1 {
		t.Fatalf("관계 개수 = %d; want 1", len(rels))
	}
	r := rels[0]
	if r.SourceTable != "customer" || r.SourceColumn != "id" {
		t.Errorf("Source가 부모여야 한다: %+v", r)
	}
	if r.TargetTable != "orders" || r.TargetColumn != "customer_id" {
		t.Errorf("Target이 자식이어야 한다: %+v", r)
	}
	if r.SourceCardinality != "ERmandOne" || r.TargetCardinality != "ERzeroToMany" {
		t.Errorf("카디널리티 = {%q,%q}; want {ERmandOne, ERzeroToMany}", r.SourceCardinality, r.TargetCardinality)
	}
	if r.SourceKey != "PK" || r.TargetKey != "FK1" {
		t.Errorf("키 표시 = {%q,%q}; want {PK, FK1}", r.SourceKey, r.TargetKey)
	}
}

// 복합 FK는 컬럼 쌍마다 관계 하나씩 낸다.
func TestToPageDefs_CompositeForeignKeyBecomesOneRelationPerColumnPair(t *testing.T) {
	s := sampleSchema()
	s.Tables[0].Columns = append(s.Tables[0].Columns, Column{Name: "region", Type: "text", Ordinal: 3, PK: true})
	s.Tables[1].Columns = append(s.Tables[1].Columns, Column{Name: "customer_region", Type: "text", Ordinal: 3})
	s.ForeignKeys[0].Columns = append(s.ForeignKeys[0].Columns,
		ForeignKeyColumn{Column: "customer_region", TargetColumn: "region"})

	res := ToPageDefs([]Schema{s})
	if got := len(res.Pages[0].Relations); got != 2 {
		t.Fatalf("관계 개수 = %d; 복합 FK는 컬럼 쌍마다 하나여야 한다", got)
	}
}

// 스키마를 가로지르는 FK는 그리지 못하되 조용히 버리지 않는다.
func TestToPageDefs_CrossSchemaForeignKeyIsReportedNotDrawn(t *testing.T) {
	s := sampleSchema()
	s.ForeignKeys[0].TargetSchema = "other"
	res := ToPageDefs([]Schema{s})
	if got := len(res.Pages[0].Relations); got != 0 {
		t.Errorf("관계 개수 = %d; 페이지를 가로지르면 그릴 수 없다", got)
	}
	if len(res.CrossSchemaFKs) != 1 {
		t.Fatalf("보고 항목 = %v; 조용히 버리면 안 된다", res.CrossSchemaFKs)
	}
	want := "public.orders.customer_id -> other.customer.id"
	if res.CrossSchemaFKs[0] != want {
		t.Errorf("보고 문구 = %q; want %q", res.CrossSchemaFKs[0], want)
	}
}

// 존재하지 않는 컬럼을 가리키는 FK는 관계를 만들지 않는다. genbuild.emitPage가
// 「행을 찾을 수 없다」는 내부 불변식 위반으로 하드 에러를 내기 때문이다.
func TestToPageDefs_SkipsForeignKeyWithUnknownColumn(t *testing.T) {
	s := sampleSchema()
	s.ForeignKeys[0].Columns[0].Column = "없는컬럼"
	res := ToPageDefs([]Schema{s})
	if got := len(res.Pages[0].Relations); got != 0 {
		t.Errorf("관계 개수 = %d; 찾을 수 없는 컬럼은 건너뛰어야 한다", got)
	}
}

// 복합 PK의 «한 컬럼»에 걸린 FK는 유일하지 않다. 그 컬럼만으로는 자식 행이
// 하나로 정해지지 않기 때문이다 — 유일하다고 보면 「부모에게 자식이 최대
// 하나」라는, DB가 보장하지 않는 사실을 관계선이 주장하게 된다.
func TestToPageDefs_ColumnOfCompositePKIsNotUnique(t *testing.T) {
	s := sampleSchema()
	// orders의 PK를 (id, customer_id) 복합으로 만든다. FK는 customer_id 하나뿐.
	s.Tables[1].Columns[1].PK = true
	res := ToPageDefs([]Schema{s})
	r := res.Pages[0].Relations[0]
	if r.TargetCardinality != "ERzeroToMany" {
		t.Errorf("TargetCardinality = %q; 복합 PK의 한 컬럼은 유일하지 않으므로 ERzeroToMany여야 한다", r.TargetCardinality)
	}
}

// 단일 컬럼 PK에 걸린 FK는 유일하다(1:1 관계).
func TestToPageDefs_SingleColumnPKForeignKeyIsUnique(t *testing.T) {
	s := sampleSchema()
	// orders.id(단일 PK)가 곧 FK인 모양 — 흔한 1:1 확장 테이블이다.
	s.ForeignKeys[0].Columns[0].Column = "id"
	res := ToPageDefs([]Schema{s})
	r := res.Pages[0].Relations[0]
	if r.TargetCardinality != "ERzeroToOne" {
		t.Errorf("TargetCardinality = %q; want ERzeroToOne", r.TargetCardinality)
	}
}

// 복합 FK가 곧 복합 PK 전체면 유일하다.
func TestToPageDefs_CompositeForeignKeyCoveringWholePKIsUnique(t *testing.T) {
	s := sampleSchema()
	s.Tables[0].Columns = append(s.Tables[0].Columns, Column{Name: "region", Type: "text", Ordinal: 3, PK: true})
	s.Tables[1].Columns = append(s.Tables[1].Columns, Column{Name: "customer_region", Type: "text", Ordinal: 3, PK: true})
	s.Tables[1].Columns[1].PK = true  // customer_id도 PK
	s.Tables[1].Columns[0].PK = false // orders.id는 PK에서 뺀다
	s.ForeignKeys[0].Columns = append(s.ForeignKeys[0].Columns,
		ForeignKeyColumn{Column: "customer_region", TargetColumn: "region"})

	res := ToPageDefs([]Schema{s})
	for _, r := range res.Pages[0].Relations {
		if r.TargetCardinality != "ERzeroToOne" {
			t.Errorf("TargetCardinality = %q; FK가 PK 전체를 덮으므로 ERzeroToOne이어야 한다", r.TargetCardinality)
		}
	}
}
