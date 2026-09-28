package model

// ID는 이 컬럼에 해당하는 행(tableRow) 셀의 draw.io id다. 관계선은 항상
// 행에 연결되므로 Relationship.SourceColumnID/TargetColumnID와 같은 id
// 공간이며, 그 값들로 이 컬럼을 조회할 수 있다(IR 1.0 계약).
type Column struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	// Unique는 정의 셀 값 끝에 UNIQUE가 붙어 있었는지다. 복합 UNIQUE는
	// 여기가 아니라 키 셀과 rowspan으로 그린다(복합 PK를 그리는 그 방식).
	Unique   bool   `json:"unique,omitempty"`
	Key      string `json:"key"`
	RawValue string `json:"raw_value"`

	// 아래 셋은 그림에 적을 자리가 없어서 draw.io의 «데이터 편집»(Ctrl+M)
	// 커스텀 속성으로 받는 값이다 — erd_check / erd_default / erd_comment
	// (2026-09-03 결정, 작업 기록). 안 붙였으면 빈 문자열이다.
	//
	// 값이 충돌해서 «어느 쪽도 고르지 않은» 경우도 빈 문자열이 된다. 그 둘을
	// 가르는 것은 Diagram.AttrConflicts다 — 리포터는 충돌한 자리에 조용히
	// 빈 칸을 두지 않고 주석을 남겨야 하기 때문이다.
	Check   string `json:"check,omitempty"`
	Default string `json:"default,omitempty"`
	Comment string `json:"comment,omitempty"`
}

type Table struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`

	// 테이블 수준 제약. 컬럼과 달리 붙일 자리가 테이블 셀 하나뿐이다
	// (draw.io에서 «한 번 클릭»으로 잡히는 그것).
	Check   string `json:"check,omitempty"`
	Comment string `json:"comment,omitempty"`
}

// ColumnLevel이 false면 관계선이 행이 아니라 테이블에 직접 연결된 것이며
// SourceColumnID/TargetColumnID는 빈 문자열이다 (스펙: 관계선이 테이블에 직접
// 연결된 경우).
//
// SourceRawID/TargetRawID는 엣지의 원본 source/target 셀 id를 해석 성공
// 여부와 무관하게 항상 보존한다(id 속성 자체가 없었으면 빈 문자열). id는
// 있었지만 이 다이어그램의 테이블/행으로 해석되지 않았다면 SourceResolved/
// TargetResolved가 false가 되고, 그 경우 SourceTableID/SourceColumnID(또는
// Target 쪽)는 채워지지 않는다.
//
// SourceExists/TargetExists는 SourceResolved/TargetResolved와 별개의 신호다:
// raw id가 이 다이어그램의 셀 인덱스(idx.ByID)에 "어떤 셀로든" 존재하는지만
// 나타낸다. 이 값이 true인데 Resolved가 false인 경우는 진짜로 끊어진
// 참조가 아니라, 이 파서가 아직 행으로 인식하지 못하는 도형 변형(예: 구버전
// 2단 테이블의 partialRectangle 직속 행)을 가리킨 것이다 — 셀 자체는 실존
// 한다. 검증 단계(broken_reference)는 Resolved만으로는 이 둘을 구분할 수
// 없으므로 Exists도 함께 봐야 한다: id가 있는데 idx.ByID에도 전혀 없을
// 때만(!Resolved && !Exists) 진짜 dangling reference로 판정한다.
// SourceTableID/TargetTableID가 채워져 있는지로는 절대 판정할 수 없다(해석
// 실패 시 원래부터 빈 문자열이라 "알려진 테이블 없음" 룩업이 항상
// 통과해버리는 결함의 재발을 막기 위함).
type Relationship struct {
	ID                string `json:"id"`
	Label             string `json:"label"`
	SourceTableID     string `json:"source_table_id"`
	SourceColumnID    string `json:"source_column_id,omitempty"`
	SourceCardinality string `json:"source_cardinality"`
	SourceRawID       string `json:"source_raw_id,omitempty"`
	SourceResolved    bool   `json:"source_resolved"`
	SourceExists      bool   `json:"source_exists"`
	TargetTableID     string `json:"target_table_id"`
	TargetColumnID    string `json:"target_column_id,omitempty"`
	TargetCardinality string `json:"target_cardinality"`
	TargetRawID       string `json:"target_raw_id,omitempty"`
	TargetResolved    bool   `json:"target_resolved"`
	TargetExists      bool   `json:"target_exists"`
	ColumnLevel       bool   `json:"column_level"`
}

// Shape은 도형 이름만 담는다("note", "ellipse", ...). 예전에는 style 문자열
// 전체가 여기 들어가 IR과 검증 리포트 메시지에 그대로 노출됐다(M11). 이름을
// 특정할 수 없는 스타일도 있으므로(순수 key=value 조합) 원본은 Style에
// 남긴다.
type IgnoredShape struct {
	ID    string `json:"id"`
	Shape string `json:"shape"`
	Style string `json:"style,omitempty"`
}

// FloatingRelation은 ER 관계선인데 source/target 속성이 둘 다 없고, 그 대신
// 남은 끝점 좌표가 테이블에 닿아 있는 선이다. 그림으로는 이어져 보이지만
// 파일이 말하는 연결은 없다 — 관계로 세지 않고 검증에서 진단으로 올린다.
type FloatingRelation struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
	// NearTableIDs는 끝점이 닿은 테이블 도형의 id다. 진단 문구가 «어느
	// 테이블 근처인가»를 말해 주려면 이것이 있어야 한다.
	NearTableIDs []string `json:"near_table_ids,omitempty"`
}

type ShapeViolation struct {
	ID     string `json:"id"`
	Shape  string `json:"shape"`
	Style  string `json:"style,omitempty"`
	Reason string `json:"reason"`
}

// AttrIssue는 erd_* 커스텀 속성에서 생긴 진단 하나다. 충돌(같은 이름, 다른
// 값)과 모르는 이름(오타 등) 둘 다 이 모양으로 나른다 — 둘 다 «어느 셀의
// 어느 이름이 문제인가»가 전부이기 때문이다.
type AttrIssue struct {
	// CellID는 진단을 붙일 셀이다. 컬럼이면 행(tableRow) 셀 id(I7 계약),
	// 테이블이면 테이블 셀 id다. annotate가 이 값으로 빨간 테두리를 친다.
	CellID     string `json:"cell_id"`
	TableName  string `json:"table_name"`
	ColumnName string `json:"column_name,omitempty"` // 비어 있으면 테이블 제약이다
	Name       string `json:"name"`
	// Values는 충돌일 때 서로 다른 값 전부다(정렬됨). 모르는 이름이면 비어 있다.
	Values []string `json:"values,omitempty"`
}

type Diagram struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Tables        []Table          `json:"tables"`
	Relationships []Relationship   `json:"relationships"`
	Ignored       []IgnoredShape   `json:"ignored_shapes"`
	Violations    []ShapeViolation `json:"shape_violations"`
	// FloatingRelations는 Relationships에 들어가지 않는다 — 아무것도
	// 잇지 않으므로 관계가 아니다. 별도 목록으로 두어 관계정의서는
	// 그대로 두고 검증만 이것을 본다.
	FloatingRelations []FloatingRelation `json:"floating_relations,omitempty"`
	AttrConflicts     []AttrIssue        `json:"attr_conflicts,omitempty"`
	UnknownAttrs      []AttrIssue        `json:"unknown_attrs,omitempty"`
}

type Document struct {
	IRVersion  string `json:"ir_version"`
	SourceFile string `json:"source_file"`
	// SourceModified는 원본 .drawio의 수정 시각이다("2026-08-28 12:04").
	// 정의서 머리의 «기준 시각»이 이 값이며, «문서를 뽑은 시각»이 아니다.
	//
	// 뽑은 시각을 쓰면 같은 ERD로 두 번 돌린 산출물이 서로 다른 바이트가
	// 된다. watch는 저장할 때마다 다시 뽑으므로 그 차이가 계속 쌓이고,
	// 「문서가 바뀌었나」를 diff로 볼 수 없게 된다. 사람이 실제로 알고
	// 싶은 것도 «이 문서가 어느 시점의 ERD인가»이지 «언제 눌렀나»가 아니다.
	SourceModified string    `json:"source_modified,omitempty"`
	Diagrams       []Diagram `json:"diagrams"`
}
