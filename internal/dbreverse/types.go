package dbreverse

import "sort"

// Column은 DB 카탈로그에서 읽은 컬럼 하나다. 카탈로그 원문을 가공 없이
// 담는다 — 이름도 타입도 손대지 않는 것이 Phase 2a부터의 규칙이다.
type Column struct {
	Name     string // 카탈로그 원문
	Type     string // 카탈로그 원문. 공백을 포함할 수 있다.
	Nullable bool
	Ordinal  int  // 1부터. 정렬 기준이며 멱등성의 근거다.
	PK       bool // PK를 이루는 컬럼인가(복합 PK면 참여 컬럼 전부 true)
	Unique   bool // 단일 컬럼 유일 제약/인덱스가 있는가. 복합 UNIQUE는 false.
}

// ForeignKeyColumn은 FK 제약이 잇는 컬럼 쌍 하나다. 복합 FK면 여럿이다.
type ForeignKeyColumn struct {
	Column       string // 자식(FK를 가진 쪽)의 컬럼
	TargetColumn string // 부모(참조당하는 쪽)의 컬럼
}

// ForeignKey는 FK 제약 하나다.
type ForeignKey struct {
	Name         string // 제약 이름. SQLite처럼 없으면 "fk<id>"로 만든다.
	Table        string // 자식 테이블
	TargetSchema string // 부모 스키마. 스키마를 가로지르는지 판정하는 데 쓴다.
	TargetTable  string // 부모 테이블
	Columns      []ForeignKeyColumn
}

// Table은 스키마 하나 안의 테이블 하나다.
type Table struct {
	Name    string
	Columns []Column // Ordinal 오름차순
}

// Schema는 생성될 drawio 페이지 하나에 대응한다(스펙: 스키마 = 페이지).
type Schema struct {
	Name        string
	Tables      []Table      // 이름 오름차순
	ForeignKeys []ForeignKey // 이름 오름차순
}

// Sort는 DB가 보장하지 않는 순서를 명시적으로 고정한다. 같은 DB를 두 번
// 읽으면 바이트가 같아야 하므로, DBMS 구현은 반드시 이것을 마지막에 부른다.
func (s *Schema) Sort() {
	sort.Slice(s.Tables, func(i, j int) bool { return s.Tables[i].Name < s.Tables[j].Name })
	for ti := range s.Tables {
		cols := s.Tables[ti].Columns
		sort.Slice(cols, func(i, j int) bool { return cols[i].Ordinal < cols[j].Ordinal })
	}
	sort.Slice(s.ForeignKeys, func(i, j int) bool {
		if s.ForeignKeys[i].Name != s.ForeignKeys[j].Name {
			return s.ForeignKeys[i].Name < s.ForeignKeys[j].Name
		}
		return s.ForeignKeys[i].Table < s.ForeignKeys[j].Table
	})
}

// clone은 Sort가 제자리에서 도는 것을 테스트가 진짜로 비교할 수 있게
// 깊은 복사를 준다. 얕은 복사는 슬라이스 저장소를 공유해서 «두 번 불러도
// 같다»가 언제나 참이 되어 버린다 — 아무것도 검사하지 않는 검사가 된다.
func (s Schema) clone() Schema {
	out := Schema{Name: s.Name}
	out.Tables = make([]Table, len(s.Tables))
	for i, t := range s.Tables {
		out.Tables[i] = Table{Name: t.Name, Columns: append([]Column(nil), t.Columns...)}
	}
	out.ForeignKeys = make([]ForeignKey, len(s.ForeignKeys))
	for i, fk := range s.ForeignKeys {
		out.ForeignKeys[i] = fk
		out.ForeignKeys[i].Columns = append([]ForeignKeyColumn(nil), fk.Columns...)
	}
	return out
}
