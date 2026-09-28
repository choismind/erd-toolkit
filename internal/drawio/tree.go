package drawio

type CellIndex struct {
	ByID       map[string]RawCell
	ChildrenOf map[string][]RawCell
}

// BuildIndex는 mxCell의 id/parent 속성만으로 부모-자식 관계를 인덱싱한다.
// XML 문서 순서에 의존하지 않는다 — out_of_order_cells.drawio처럼 형제
// 순서가 어긋난 실제 파일에서도 정확해야 하기 때문이다.
func BuildIndex(cells []RawCell) CellIndex {
	idx := CellIndex{
		ByID:       make(map[string]RawCell, len(cells)),
		ChildrenOf: make(map[string][]RawCell),
	}
	for _, c := range cells {
		idx.ByID[c.ID] = c
	}
	for _, c := range cells {
		if c.Parent == "" {
			continue
		}
		idx.ChildrenOf[c.Parent] = append(idx.ChildrenOf[c.Parent], c)
	}
	return idx
}
