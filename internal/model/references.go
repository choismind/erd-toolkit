// internal/model/references.go
//
// 관계선과 컬럼을 맞대는 자리다.
//
// 이 층이 없어서 «연관관계 정보»가 세 곳에서 빠져 있었다(2026-09-04 소유자
// 진단): 정의서의 FK 칸은 `FK1`만 있고 무엇을 가리키는지 없었고, schema.sql
// 에는 FOREIGN KEY가 한 줄도 없었으며, 검증은 「관계선은 그렸는데 FK 표기가
// 없다」를 못 잡았다. 셋 다 재료는 IR에 다 있었고 — 관계 22건이 그대로
// 들어 있다 — 맞대 보는 코드만 없었다.
//
// 리포터도 검증기도 여기 하나를 쓴다. 각자 관계를 읽으면 «어느 쪽이 부모인가»
// 판정이 두 벌이 되고, 정의서와 DDL이 서로 다른 방향을 말하게 된다.
package model

// ColumnRef는 어떤 컬럼이 «가리키는» 대상이다.
type ColumnRef struct {
	TableName string
	// ColumnName은 관계선이 행이 아니라 테이블에 직접 연결된 경우 빈
	// 문자열이다. 그때 DDL은 컬럼 목록 없이 REFERENCES만 쓴다(ANSI에서
	// 그것은 «그 테이블의 기본키»를 뜻한다) — 없는 컬럼 이름을 지어내지
	// 않는다.
	ColumnName string
	// RelationshipID는 이 참조를 만든 관계선의 셀 id다. 진단을 그 선에
	// 붙일 때 쓴다.
	RelationshipID string
}

// ColumnReferences는 «컬럼 id -> 그 컬럼이 가리키는 대상»을 만든다. 컬럼
// id는 행(tableRow) 셀 id다.
//
// 어느 쪽이 자식(외래키를 든 쪽)인지는 **키 셀의 FK 표기로 정한다.** 한쪽만
// FK로 표시돼 있으면 그쪽이 자식이다. 둘 다이거나 둘 다 아니면 판정하지
// 않고 건너뛴다 — 지어내면 정의서가 «A가 B를 참조한다»고 단언하는데 실제로는
// 반대일 수 있고, 문서를 읽는 사람은 그것을 의심할 근거가 없다.
//
// 판정하지 못한 관계는 UnmatchedRelationships가 따로 돌려준다. 검증이 그것을
// 진단으로 올린다 — 조용히 빠지면 그림에는 선이 있는데 문서에는 아무 흔적도
// 없게 된다.
func (d Diagram) ColumnReferences() map[string]ColumnRef {
	refs, _ := d.resolveReferences()
	return refs
}

// UnmatchedRelationships는 양쪽 끝이 다 풀렸는데도 «어느 쪽이 자식인지»를
// 정하지 못한 관계다. 대부분은 FK 표기를 빠뜨린 경우다.
func (d Diagram) UnmatchedRelationships() []Relationship {
	_, unmatched := d.resolveReferences()
	return unmatched
}

func (d Diagram) resolveReferences() (map[string]ColumnRef, []Relationship) {
	tables := map[string]Table{}
	columns := map[string]Column{}
	for _, t := range d.Tables {
		tables[t.ID] = t
		for _, c := range t.Columns {
			columns[c.ID] = c
		}
	}

	refs := map[string]ColumnRef{}
	var unmatched []Relationship

	for _, r := range d.Relationships {
		if !r.SourceResolved || !r.TargetResolved {
			continue // 끊어진 관계는 broken_reference가 따로 본다
		}

		srcIsFK := r.SourceColumnID != "" && columns[r.SourceColumnID].IsForeignKey()
		tgtIsFK := r.TargetColumnID != "" && columns[r.TargetColumnID].IsForeignKey()

		var childCol, parentTable, parentCol string
		switch {
		case tgtIsFK && !srcIsFK:
			childCol, parentTable, parentCol = r.TargetColumnID, r.SourceTableID, r.SourceColumnID
		case srcIsFK && !tgtIsFK:
			childCol, parentTable, parentCol = r.SourceColumnID, r.TargetTableID, r.TargetColumnID
		default:
			unmatched = append(unmatched, r)
			continue
		}

		refs[childCol] = ColumnRef{
			TableName:      tables[parentTable].Name,
			ColumnName:     columns[parentCol].Name,
			RelationshipID: r.ID,
		}
	}
	return refs, unmatched
}
