package drawio

// TopLevelBounds는 «최상위» 셀들을 감싸는 사각형을 돌려준다.
//
// 최상위만 보는 이유: 행(tableRow)과 키/정의 셀(partialRectangle)의
// mxGeometry는 부모 기준 «상대» 좌표다. 함께 최소/최대를 잡으면 y=30 같은
// 값이 절대 좌표로 섞여 들어와, 실제 그림과 상관없는 박스가 나온다.
//
// 둘째 값이 false면 기하를 가진 최상위 셀이 하나도 없다는 뜻이다.
// 그때는 호출자가 자기 기본값을 쓴다 — 0으로 채운 사각형을 돌려주면
// «원점에 크기 0인 그림이 있다»는 거짓말이 된다.
func TopLevelBounds(cells []RawCell) (RawGeometry, bool) {
	idx := BuildIndex(cells)
	var minX, minY, maxX, maxY float64
	found := false

	for _, c := range cells {
		if c.Geometry == nil {
			continue
		}
		// 부모가 다른 «셀»이면 그 좌표는 상대다. 부모가 루트 레이어
		// (id="1" 같은, 기하가 없는 셀)면 절대다.
		//
		// 자기 자신을 부모로 가리키는 셀은 «다른 셀»이 아니다 — 좌표가
		// 자기 자신에 대해 상대일 수는 없다. 이 갈래를 안 가르면 그런
		// 퇴화 입력(손편집·파일 병합으로만 생긴다)이 bounds에서 통째로
		// 빠지고, annotate가 그 자리에 요약 박스를 겹쳐 놓는다.
		if p, ok := idx.ByID[c.Parent]; ok && p.Geometry != nil && c.Parent != c.ID {
			continue
		}
		g := *c.Geometry
		if !found {
			minX, minY = g.X, g.Y
			maxX, maxY = g.X+g.Width, g.Y+g.Height
			found = true
			continue
		}
		if g.X < minX {
			minX = g.X
		}
		if g.Y < minY {
			minY = g.Y
		}
		if g.X+g.Width > maxX {
			maxX = g.X + g.Width
		}
		if g.Y+g.Height > maxY {
			maxY = g.Y + g.Height
		}
	}
	if !found {
		return RawGeometry{}, false
	}
	return RawGeometry{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}, true
}
