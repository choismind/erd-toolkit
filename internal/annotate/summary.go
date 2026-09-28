package annotate

import (
	"math"
	"strings"

	"erdtool/internal/convert"
	"erdtool/internal/drawio"
)

const (
	// summaryStyle은 정확히 shape=note여야 한다. drawio.ClassifyShape이
	// note만 ShapeClassIgnored로 두고 나머지를 전부 Violation으로
	// 떨어뜨리므로, 다른 도형을 쓰면 우리가 만든 박스가 매 실행마다
	// 새 shape_violation을 낳는다 — 툴이 자기 물건을 결함이라고
	// 보고하는 꼴이다.
	summaryStyle = "shape=note;whiteSpace=wrap;html=1;"

	summaryWidth      = 260.0
	summaryLineHeight = 20.0
	summaryMinHeight  = 60.0
	summaryGapX       = 40.0
)

// SummaryCell은 페이지 하나의 요약 박스를 만든다.
//
// 이미 요약 박스가 있으면 그 좌표와 부모를 그대로 쓴다 — 사람이 보기
// 좋은 데로 옮겼으면 존중하고 내용만 갈아 끼운다. 단 부모가 파일에 없는
// 셀이면 존중할 뜻 자체가 없으므로 물러난다(아래 재사용 갈래 주석).
// 처음 만드는 박스의 부모는 summaryParent가 페이지에서 찾아낸다(박아
// 두지 않는다).
func SummaryCell(p PagePlan, cells []drawio.RawCell) convert.NewCell {
	id := SummaryCellID(p.DiagramID)

	// 기존 박스를 먼저 찾는다. drawio.TopLevelBounds는 이 박스 자신도
	// 최상위 셀로 세므로, bounding box를 우선으로 쓰고 기존 좌표는
	// fallback으로만 돌리면 두 번째 실행부터 TopLevelBounds가 항상
	// 뭔가를 찾아내(그 요약 박스 자신을 포함해서) reuse 분기가 죽은
	// 코드가 되고, 매 실행 박스가 자기 자신의 오른쪽에 다시 놓이며
	// summaryWidth+summaryGapX만큼 우측으로 밀린다 — 파일이 재실행해도
	// 안정되지 않아 Phase 3의 재실행 안정성 계약이 깨진다. 그래서 기존
	// 좌표 탐색을 무조건 먼저 하고, 못 찾았을 때만 bounding box
	// 계산으로 넘어간다.
	idx := drawio.BuildIndex(cells)
	if c, ok := idx.ByID[id]; ok && c.Geometry != nil {
		// 부모도 좌표와 같이 존중한다 — 사람이 박스를 다른 레이어로
		// 옮겼으면 그 뜻을 지우지 않는다. 매 실행 원래 레이어로 되돌리면
		// 파일이 재실행해도 안정되지 않아 멱등성 계약이 깨진다.
		//
		// 다만 «존중»의 조건은 그 부모가 파일에 실제로 있는 것이다.
		// 비어 있거나 매달린 참조(레이어를 지웠다 다시 만든 파일에서
		// 생긴다)면 summaryParent로 물러난다 — 아래 summaryParent가
		// 처음 만드는 박스에서 일부러 거르는 바로 그 상태를 재사용
		// 경로가 통과시키면, 존재하지 않는 셀을 부모로 가리키는 박스가
		// 나가면서 erdtool은 「페이지 N개 요약」이라고 성공을 보고한다.
		// 사람이 draw.io에서 열면 그 박스가 없다(코드리뷰).
		parent := c.Parent
		if _, alive := idx.ByID[parent]; parent == "" || !alive {
			parent = summaryParent(cells)
		}
		return newSummaryCell(id, p, parent, c.Geometry.X, c.Geometry.Y)
	}

	x, y := 0.0, 0.0
	if b, ok := drawio.TopLevelBounds(cells); ok {
		x = b.X + b.Width + summaryGapX
		y = b.Y
	}
	return newSummaryCell(id, p, summaryParent(cells), x, y)
}

// summaryParent는 요약 박스를 붙일 부모 셀 id를 페이지에서 «찾는다».
//
// 예전에는 "1"을 박아 뒀다. draw.io가 기본 레이어에 id="1"을 주는 것이
// 보통이라 대개 맞지만, 레이어를 지웠다 다시 만든 파일이나 다른 도구가
// 만든 파일에는 그 id가 아예 없을 수 있다. 그러면 존재하지 않는 셀을
// 부모로 가리키는 박스가 나가는데 erdtool은 「페이지 N개 요약」이라고
// 성공을 보고한다 — 사람이 draw.io에서 열면 그 박스가 없다. 도구가 한
// 일과 다른 말을 하는 것이라 이 저장소가 가장 경계하는 실패 유형이다.
//
// 판정 기준은 drawio.TopLevelBounds와 같다: 기하가 있고 그 부모가
// «기하 없는 셀»(= 레이어)인 셀이 최상위다. 그 셀의 부모가 곧 사람이
// 그림을 그려 둔 레이어이므로, 요약 박스도 같은 레이어에 붙인다.
// 부모가 파일에 실제로 없는(매달린 참조) 셀은 건너뛴다 — 그걸 그대로
// 쓰면 고치려던 결함을 그대로 재현한다.
//
// 기하를 가진 최상위 셀이 하나도 없으면(앵커 없는 진단만 나온 페이지가
// 이럴 수 있다) 부모가 "0"인 첫 셀, 즉 첫 레이어로 물러난다. 그것마저
// 없는 파일은 draw.io 페이지의 구조 자체가 없는 것이므로 마지막 수단으로
// 옛 값 "1"을 쓴다.
func summaryParent(cells []drawio.RawCell) string {
	idx := drawio.BuildIndex(cells)
	for _, c := range cells {
		if c.Geometry == nil {
			continue
		}
		parent, ok := idx.ByID[c.Parent]
		if !ok || parent.Geometry != nil {
			continue
		}
		return c.Parent
	}
	for _, c := range cells {
		if c.Parent == "0" {
			return c.ID
		}
	}
	return "1"
}

func newSummaryCell(id string, p PagePlan, parent string, x, y float64) convert.NewCell {
	height := summaryMinHeight
	if h := float64(wrappedLines(p.Summary)) * summaryLineHeight; h > height {
		height = h
	}

	return convert.NewCell{
		ID:     id,
		Value:  summaryValue(p.Summary),
		Style:  summaryStyle,
		Parent: parent,
		X:      x, Y: y, Width: summaryWidth, Height: height,
		// Attrs가 있어야 convert.encodeNewCell이 <object>로 감싸 이
		// 속성을 XML에 싣는다. 이게 없으면 이 박스는 «내가 만든 것»이라는
		// 표가 하나도 없는 그냥 note가 되어, ScanExisting이 다음 실행에서
		// 이 박스를 옛 요약 박스로 못 찾는다 — 지우고 다시 넣는 대신
		// 매번 새 박스가 옆에 쌓인다(작업 12의 멱등성 계약이 이걸 잡는다).
		Attrs: map[string]string{AttrAnnotation: AnnotationSummary},
	}
}

// summaryUnitsPerLine은 폭 260에 한 줄로 들어가는 «글자 너비»의 합이다.
// 한글은 1, 나머지는 0.5로 센다 — 상자가 `whiteSpace=wrap`이라 긴 진단
// 문구는 두세 줄로 접히는데, 접힌 줄을 안 세면 상자가 짧아 **마지막 줄이
// 상자 밖으로 잘린다**(2026-09-22에 블로그용 그림을 뽑다 드러났다).
//
// 글꼴 너비를 정확히 재는 것은 여기서 할 수 있는 일이 아니므로 어림한다.
// 어림이 빗나가는 방향은 **상자가 커지는 쪽**이어야 한다 — 큰 상자는
// 보기 나쁠 뿐이고, 작은 상자는 글자를 잃는다.
const summaryUnitsPerLine = 16.0

// wrappedLines는 요약 상자가 실제로 차지할 줄 수를 어림한다. 제목 줄
// 하나에 진단마다 «접힌 줄 수»를 더한다.
func wrappedLines(lines []string) int {
	n := 1 // <b>erdtool 검증</b>
	for _, l := range lines {
		units := 2.0 // 줄머리의 "· "
		for _, r := range l {
			if r > 0x2000 {
				units++ // 한글·전각 문장부호
			} else {
				units += 0.5
			}
		}
		rows := int(math.Ceil(units / summaryUnitsPerLine))
		if rows < 1 {
			rows = 1
		}
		n += rows
	}
	return n
}

// summaryValue는 박스에 들어갈 HTML을 만든다. draw.io는 html=1인 셀의
// value를 HTML로 읽으므로 줄바꿈이 <br>다.
//
// 값 자체의 이스케이프는 XML 인코더가 한다(convert.encodeNewCell). 여기서
// 또 이스케이프하면 이중으로 걸려 &amp;lt;가 사용자 눈에 보인다.
func summaryValue(lines []string) string {
	var b strings.Builder
	b.WriteString("<b>erdtool 검증</b>")
	for _, l := range lines {
		b.WriteString("<br>· ")
		b.WriteString(l)
	}
	return b.String()
}
