// Package genbuild는 표 형태의 데이터 설계서(xlsx/csv)를 읽어 .drawio ERD를
// 새로 쓴다. Phase 1c의 internal/glossary와 같은 자리 — .drawio를 읽는 게
// 아니라 만드는 쪽이다.
package genbuild

// ColumnDef는 테이블 목록 블록의 행 하나(컬럼 하나)를 그대로 옮긴 것이다.
type ColumnDef struct {
	Name     string
	Type     string
	Key      string // "", "PK", "FK1", "FK2", ... 시트 값 그대로, 가공하지 않는다.
	Nullable bool
	Unique   bool
}

// TableDef는 같은 테이블명을 가진 ColumnDef들을 모은 것이다.
type TableDef struct {
	Name    string
	Columns []ColumnDef
}

// RelationDef는 관계 목록 블록의 행 하나다. SourceCardinality/TargetCardinality는
// draw.io entityRelationEdgeStyle의 startArrow/endArrow 코드를 그대로 받는다
// (internal/drawio/relationships.go의 SourceCardinality=startArrow,
// TargetCardinality=endArrow 매핑과 대칭이다).
type RelationDef struct {
	Seq               int
	SourceTable       string
	SourceColumn      string
	SourceCardinality string
	SourceKey         string
	RelationName      string
	TargetCardinality string
	TargetKey         string
	TargetTable       string
	TargetColumn      string
}

// PageDef는 탭(엑셀 시트) 하나 = 생성될 drawio 페이지 하나에 대응한다.
type PageDef struct {
	Name      string
	Tables    []TableDef
	Relations []RelationDef
}

// ValidCardinality는 docs/drawio-er-palette.md가 실측한 entityRelationEdgeStyle의
// endArrow/startArrow 6종 코드다. 이 밖의 문자열은 시트 입력 단계에서 거부한다.
var ValidCardinality = map[string]bool{
	"ERone":        true,
	"ERmandOne":    true,
	"ERzeroToOne":  true,
	"ERzeroToMany": true,
	"ERoneToMany":  true,
	"ERmany":       true,
}
