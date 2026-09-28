package genbuild

import "math"

type dagEdge struct{ from, to string }

// layerTables는 관계 그래프(원천->목표)를 방향 그래프로 보고 위상적
// "층"을 배정한다. 사이클이 있으면 DFS 방문 순서로 찾은 back-edge를
// 층 계산에서만 무시한다(관계선 자체는 emit 단계에서 원래 방향대로
// 그린다 — layerTables는 좌표 계산 전용이다).
//
// tableOrder는 결정성을 위한 방문 순서다(map 순회는 순서가 없다) —
// 보통 테이블 목록 블록에 등장한 순서를 넘긴다.
func layerTables(tableOrder []string, relations []RelationDef) map[string]int {
	const (
		white = 0
		gray  = 1
		black = 2
	)

	adj := map[string][]string{}
	for _, r := range relations {
		adj[r.SourceTable] = append(adj[r.SourceTable], r.TargetTable)
	}

	color := map[string]int{}
	var dagEdges []dagEdge
	var visit func(u string)
	visit = func(u string) {
		color[u] = gray
		for _, v := range adj[u] {
			if color[v] == gray {
				continue // back-edge: 사이클, 층 계산에서 무시한다
			}
			dagEdges = append(dagEdges, dagEdge{u, v})
			if color[v] == white {
				visit(v)
			}
		}
		color[u] = black
	}
	for _, name := range tableOrder {
		if color[name] == white {
			visit(name)
		}
	}

	layer := map[string]int{}
	for _, name := range tableOrder {
		layer[name] = 0
	}
	// DAG의 최장 경로 길이만큼 relax하면 수렴한다. 노드 수만큼 반복하면
	// 항상 충분하다(어떤 경로도 노드 수-1개 간선을 넘지 않는다).
	for i := 0; i < len(tableOrder); i++ {
		changed := false
		for _, e := range dagEdges {
			if layer[e.to] < layer[e.from]+1 {
				layer[e.to] = layer[e.from] + 1
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return layer
}

// 아래 상수는 internal/drawio/testdata/entity_table_basic.drawio의 실제
// mxGeometry 값을 그대로 옮긴 것이다(테이블 폭 180 = 키 열 30 + 정의 열
// 150, 헤더 30, 행 높이 30) — 손으로 지어낸 숫자가 아니다. 폭 180은 이제
// 아래 한도이며, 정의 셀 글자가 그 안에 안 들어가면 tableWidthFor가 늘린다
// (2026-09-11 소유자 검증: 긴 컬럼이 두 줄로 접혀 읽기 나빴다).
const (
	minTableWidth = 180
	keyColWidth   = 30
	headerHeight  = 30
	rowHeight     = 30
	layerGapY     = 80
	tableGapX     = 40
	isolatedGapY  = 200
	isolatedCols  = 4

	// 층 묶음을 여러 열로 접는 기준이다(2026-09-21 소유자).
	//
	// 층마다 한 줄을 쓰므로, 층이 많으면 그림이 세로로만 길어진다. sakila를
	// 역공학하면 층 아홉 중 다섯이 테이블 하나짜리여서 가로 1254에 세로
	// 3850이 나왔다 — A4로 여덟 장에 걸친 띠였다.
	columnGapX = 200
	// 한 열로 쌓았을 때 이 높이를 넘으면 접기를 따져 본다. A4 한 장 남짓이며,
	// 그보다 짧으면 접을 이유가 없다 — 접으면 「부모가 위, 자식이 아래」라는
	// 읽는 순서만 끊긴다.
	foldMinHeight = 1400
	// 접을 때 겨누는 세로/가로 비다. 1이면 정사각형이고, 0.8이든 1.25든
	// 같은 만큼 벗어난 것으로 친다(아래 foldScore가 로그로 잰다).
	foldTargetRatio = 1.0
)

// layerBox는 층 하나가 차지하는 칸이다. w는 그 층 테이블들을 한 줄로
// 늘어놓았을 때의 가로, h는 그중 가장 높은 것이다.
type layerBox struct {
	names []string
	w, h  int
}

// foldScore는 세로/가로가 목표에서 얼마나 벗어났는지를 로그로 잰다.
// 로그로 재야 「두 배 납작」과 「두 배 길쭉」이 같은 벌점이 된다.
func foldScore(w, h int) float64 {
	if w <= 0 || h <= 0 {
		return math.Inf(1)
	}
	return math.Abs(math.Log(float64(h) / float64(w) / foldTargetRatio))
}

// cutsInto는 층 n개를 «차례를 지킨 채» k덩이로 가르는 모든 자름 자리를
// 낸다. 자름 자리는 층과 층 사이이므로 n-1자리 중 k-1을 고르는 것이다.
func cutsInto(n, k int) [][]int {
	if k <= 1 || n < k {
		return [][]int{{}}
	}
	var out [][]int
	var rec func(start int, chosen []int)
	rec = func(start int, chosen []int) {
		if len(chosen) == k-1 {
			c := make([]int, len(chosen))
			copy(c, chosen)
			out = append(out, c)
			return
		}
		// 남은 자리마다 최소 층 하나는 들어가야 한다.
		for i := start; i <= n-1-(k-1-len(chosen)); i++ {
			rec(i+1, append(chosen, i))
		}
	}
	rec(0, nil)
	return out
}

// splitAt은 자름 자리대로 층 묶음을 덩이로 가른다.
func splitAt(boxes []layerBox, cuts []int) [][]layerBox {
	var cols [][]layerBox
	prev := 0
	for _, c := range cuts {
		cols = append(cols, boxes[prev:c+1])
		prev = c + 1
	}
	cols = append(cols, boxes[prev:])
	return cols
}

// foldColumns는 층 묶음을 몇 열로, 어디서 접을지 정해 그 나눔을 돌려준다.
// 접을 이유가 없으면 한 열 그대로다.
//
// 자르는 자리를 높이만 보고 정하면 «건너가는 관계선»이 많아진다. sakila에서
// 높이로만 고른 자리가 스물둘 중 일곱을 건너가게 했고, 한 칸 위에서 자르면
// 모양이 비슷하면서 넷으로 줄었다. 그래서 셋을 함께 본다(2026-09-21 소유자:
// 「조금 어수선하다」).
func foldColumns(boxes []layerBox, crossing func(cols [][]layerBox) int, relCount, tableArea int) [][]layerBox {
	one := [][]layerBox{boxes}
	if len(boxes) < 4 {
		return one
	}

	measure := func(cols [][]layerBox) (w, h int) {
		for ci, c := range cols {
			colW, colH := 0, 0
			for i, b := range c {
				if b.w > colW {
					colW = b.w
				}
				colH += b.h
				if i > 0 {
					colH += layerGapY
				}
			}
			w += colW
			if ci > 0 {
				w += columnGapX
			}
			if colH > h {
				h = colH
			}
		}
		return w, h
	}

	// score는 셋을 더한다 — 모양이 목표에서 벗어난 정도, 열을 건너가는
	// 관계선의 비율, 그리고 테이블이 실제로 덮는 넓이에 견준 빈 공간이다.
	// 셋 다 낮을수록 좋고, 로그로 재서 배수 차이가 같은 벌점이 되게 한다.
	score := func(cols [][]layerBox) float64 {
		w, h := measure(cols)
		s := foldScore(w, h)
		if relCount > 0 {
			s += float64(crossing(cols)) / float64(relCount)
		}
		if tableArea > 0 && w > 0 && h > 0 {
			s += math.Log(float64(w) * float64(h) / float64(tableArea))
		}
		return s
	}

	if _, h1 := measure(one); h1 <= foldMinHeight {
		return one
	}

	best, bestScore := one, score(one)
	// 열이 넷을 넘으면 층 차례가 너무 잘게 끊긴다. 층이 아주 많으면 경우의
	// 수가 커지므로 거기서 멈춘다 — 결정성은 자름 자리를 사전순으로 내는
	// cutsInto가 지킨다.
	maxCols := 4
	if len(boxes) > 24 {
		maxCols = 2
	}
	for k := 2; k <= maxCols && k <= len(boxes); k++ {
		for _, cuts := range cutsInto(len(boxes), k) {
			cols := splitAt(boxes, cuts)
			if s := score(cols); s < bestScore {
				best, bestScore = cols, s
			}
		}
	}
	return best
}

type PlacedTable struct {
	TableDef
	X, Y, Width, Height int
}

type PageLayout struct {
	Name      string
	Tables    []PlacedTable
	Relations []RelationDef
}

// tableWidthFor는 정의 셀 글자가 한 줄에 들어가는 폭을 돌려준다. 아래
// 한도는 minTableWidth이고, 그보다 긴 글자가 있으면 거기에 맞춰 늘린다.
func tableWidthFor(t TableDef) int {
	w := minTableWidth
	for _, c := range t.Columns {
		// 키 열 + 글자 + 정의 셀의 spacingLeft(6)와 오른쪽 여백(6).
		if need := keyColWidth + textWidthPx(columnValue(c)) + 12; need > w {
			w = need
		}
	}
	// 제목 줄은 가운데 맞춤이라 양쪽에 여백이 붙는다.
	if need := textWidthPx(t.Name) + 20; need > w {
		w = need
	}
	return w
}

// textWidthPx는 draw.io 기본 글꼴(Helvetica 12px)로 그 글자가 차지하는
// 가로 픽셀을 어림한다. 한글·한자 같은 전각은 12, 나머지는 6.6으로 센다.
func textWidthPx(s string) int {
	px := 0.0
	for _, r := range s {
		if r > 0x2E80 {
			px += 12
		} else {
			px += 6.6
		}
	}
	return int(math.Ceil(px))
}

func tableHeight(t TableDef) int {
	return headerHeight + rowHeight*len(t.Columns)
}

// Layout은 PageDef의 테이블들을 관계 그래프 기반 계층 배치로 좌표를
// 붙인다(스펙 "배치 알고리즘" 절, 접근 A). 관계에 한 번이라도 등장하는
// 테이블은 층별로 나열하고, 전혀 등장하지 않는 고립 테이블은 그래프
// 아래쪽에 격자로 배치한다.
func Layout(p PageDef) PageLayout {
	tableOrder := make([]string, len(p.Tables))
	for i, t := range p.Tables {
		tableOrder[i] = t.Name
	}

	involved := map[string]bool{}
	firstAppearance := map[string]int{}
	for i, r := range p.Relations {
		for _, name := range [2]string{r.SourceTable, r.TargetTable} {
			involved[name] = true
			if _, ok := firstAppearance[name]; !ok {
				firstAppearance[name] = i
			}
		}
	}

	layer := layerTables(tableOrder, p.Relations)

	byName := map[string]TableDef{}
	for _, t := range p.Tables {
		byName[t.Name] = t
	}

	// 층별로 묶고, 같은 층 안에서는 관계 목록 최초 등장 순서로 정렬한다.
	layerMembers := map[int][]string{}
	maxLayer := -1
	for _, name := range tableOrder {
		if !involved[name] {
			continue
		}
		l := layer[name]
		layerMembers[l] = append(layerMembers[l], name)
		if l > maxLayer {
			maxLayer = l
		}
	}
	for l := range layerMembers {
		members := layerMembers[l]
		sortByFirstAppearance(members, firstAppearance)
		layerMembers[l] = members
	}

	// 층마다 차지할 칸을 먼저 재고, 세로로만 길어지면 여러 열로 접는다.
	var boxes []layerBox
	for l := 0; l <= maxLayer; l++ {
		members := layerMembers[l]
		b := layerBox{names: members}
		for i, name := range members {
			t := byName[name]
			b.w += tableWidthFor(t)
			if i > 0 {
				b.w += tableGapX
			}
			if h := tableHeight(t); h > b.h {
				b.h = h
			}
		}
		boxes = append(boxes, b)
	}

	// 열을 건너가는 관계선을 세는 함수와, 테이블이 실제로 덮는 넓이.
	// 둘 다 자를 자리를 고를 때 쓴다.
	tableArea := 0
	for _, t := range p.Tables {
		tableArea += tableWidthFor(t) * tableHeight(t)
	}
	crossing := func(cols [][]layerBox) int {
		colOf := map[string]int{}
		for ci, c := range cols {
			for _, b := range c {
				for _, name := range b.names {
					colOf[name] = ci
				}
			}
		}
		n := 0
		for _, r := range p.Relations {
			a, aok := colOf[r.SourceTable]
			b, bok := colOf[r.TargetTable]
			if aok && bok && a != b {
				n++
			}
		}
		return n
	}
	cols := foldColumns(boxes, crossing, len(p.Relations), tableArea)

	var placed []PlacedTable
	graphBottom := 0
	colX := 0
	for _, col := range cols {
		y := 0
		colWidth := 0
		for _, b := range col {
			x := colX
			for _, name := range b.names {
				t := byName[name]
				h := tableHeight(t)
				w := tableWidthFor(t)
				placed = append(placed, PlacedTable{TableDef: t, X: x, Y: y, Width: w, Height: h})
				x += w + tableGapX
			}
			if b.w > colWidth {
				colWidth = b.w
			}
			y += b.h + layerGapY
			if y-layerGapY > graphBottom {
				graphBottom = y - layerGapY
			}
		}
		colX += colWidth + columnGapX
	}
	if maxLayer < 0 {
		graphBottom = 0
	} else {
		graphBottom += isolatedGapY
	}

	// 고립 테이블: 테이블 블록 등장 순서대로 격자 배치.
	col := 0
	isolatedY := graphBottom
	rowMaxHeight := 0
	x := 0
	for _, name := range tableOrder {
		if involved[name] {
			continue
		}
		t := byName[name]
		h := tableHeight(t)
		w := tableWidthFor(t)
		placed = append(placed, PlacedTable{TableDef: t, X: x, Y: isolatedY, Width: w, Height: h})
		x += w + tableGapX
		if h > rowMaxHeight {
			rowMaxHeight = h
		}
		col++
		if col >= isolatedCols {
			col = 0
			x = 0
			isolatedY += rowMaxHeight + tableGapX
			rowMaxHeight = 0
		}
	}

	return PageLayout{Name: p.Name, Tables: placed, Relations: p.Relations}
}

func sortByFirstAppearance(names []string, firstAppearance map[string]int) {
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && firstAppearance[names[j-1]] > firstAppearance[names[j]]; j-- {
			names[j-1], names[j] = names[j], names[j-1]
		}
	}
}
