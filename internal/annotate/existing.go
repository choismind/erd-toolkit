// internal/annotate/existing.go
package annotate

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"

	"erdtool/internal/convert"
)

// Existing은 파일에 이미 있는 erdtool 표식이다.
type Existing struct {
	BaseStyle map[string]string // 셀 id -> 저장된 원래 스타일
	Marked    map[string]bool   // 셀 id -> erdtoolIssue가 붙어 있다
	Summaries map[string]bool   // 셀 id -> 요약 박스다
	// Wrapped는 «이 셀 id가 지금 <object>/<UserObject>로 감싸여 있다»다.
	// 누가 감쌌는지는 안 가린다 — 사용자의 원래 래퍼도 포함한다.
	Wrapped map[string]bool
	// WrappedByUs는 Wrapped 중에서도 «그 래퍼에 erdtoolWrapped="1"이
	// 붙어 있다»(즉 erdtool이 직접 만들었다고 표시해 둔 것)다. 이 맵에
	// 있는 셀만 annotate가 나중에 안전하게 벗길 수 있다 — Wrapped에는
	// 있지만 WrappedByUs에는 없는 셀은 사용자의 래퍼이므로 절대 안 벗긴다.
	WrappedByUs map[string]bool
}

// ResidueIDs는 erdtool 잔재가 하나라도 남은 셀 id 전부다.
//
// Marked(erdtoolIssue) 하나만으로 "지울 것이 있는가"를 판정하면 안
// 된다(코드리뷰 [중요 1]). draw.io의 «데이터 편집» 창은 속성을 하나씩
// 지울 수 있는 일반 UI라서, 사람이 erdtoolIssue만 지우고
// erdtoolBaseStyle·erdtoolWrapped는 그대로 두는 것이 실제로 가능하다.
// 그러면 Marked는 비었는데 셀은 여전히 원래 스타일을 잃은 채(erdtoolBaseStyle이
// 저장하고 있는 값과 다른 빨간 테두리 style을 두르고) 남고, 래퍼도
// erdtoolWrapped="1"을 단 채로 남는다. Marked만 보는 호출자는 이 셀을
// «표식 없음»으로 오판해 --clean이 아무것도 안 하면서 성공을 보고하는
// 사고(이 저장소가 가장 경계하는 실패 유형)로 이어진다.
//
// Wrapped(그냥 <object>/<UserObject>로 감싸여 있다는 사실)는 넣지 않는다
// — 그것만으로는 우리 표가 전혀 없을 수 있다(사용자의 순수한 자기 래퍼).
// WrappedByUs(erdtoolWrapped="1")만 넣는다.
func (e Existing) ResidueIDs() map[string]bool {
	out := map[string]bool{}
	for id := range e.Marked {
		out[id] = true
	}
	for id := range e.BaseStyle {
		out[id] = true
	}
	for id := range e.WrappedByUs {
		out[id] = true
	}
	return out
}

// ScanExisting은 파일의 erdtool 커스텀 속성을 페이지별로 모은다.
// 첨자가 <diagram>의 등장 순서다.
//
// drawio.LoadDiagrams는 <object>의 id와 label만 읽고 커스텀 속성은
// 버리므로 원문을 따로 읽는다. convert.collectLogicalNames가 같은 이유로
// 같은 일을 한다 — 이 함수는 그 형제다.
//
// 페이지별로 가르는 이유: 셀 id가 페이지 사이에서 겹치면 파일 전체 맵
// 하나로는 BaseStyle이 마지막 페이지 값으로 덮인다. 그 상태로 Clean이
// 복원하면 A페이지 셀이 B페이지의 원래 스타일을 뒤집어쓴다.
func ScanExisting(src []byte) ([]Existing, error) {
	n, err := convert.CountDiagrams(src)
	if err != nil {
		return nil, err
	}
	out := make([]Existing, n)
	for i := range out {
		out[i] = newExisting()
	}
	err = convert.ForEachGraphModel(src, func(pageIdx int, _ string, graphModel []byte) error {
		return scanGraphModel(graphModel, out[pageIdx])
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// newExisting은 빈 Existing을 돌려준다. 필드마다 nil이 아니라 빈 맵을
// 채워 두므로, 호출자는 nil 맵에 쓰는 것을 막는 방어 없이 바로 대입할
// 수 있다.
func newExisting() Existing {
	return Existing{
		BaseStyle:   map[string]string{},
		Marked:      map[string]bool{},
		Summaries:   map[string]bool{},
		Wrapped:     map[string]bool{},
		WrappedByUs: map[string]bool{},
	}
}

func scanGraphModel(graphModel []byte, out Existing) error {
	dec := xml.NewDecoder(bytes.NewReader(graphModel))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("scan erdtool attrs: %w", err)
		}
		st, ok := tok.(xml.StartElement)
		// 파서(drawio.LoadDiagrams)가 <object>와 <UserObject>를 똑같이
		// «id를 가진 셀»로 평탄화하므로 여기서도 둘 다 본다. 한쪽만 보면
		// 그 셀의 마크가 안 보이는 것이 되어 annotate가 이미 마크된 셀을
		// 다시 마크하고, --clean은 그 흔적을 못 지운다.
		if !ok || (st.Name.Local != "object" && st.Name.Local != "UserObject") {
			continue
		}
		id := attrOf(st, "id")
		if id == "" {
			continue
		}
		// <object>/<UserObject>로 감싸여 있다는 사실 자체는 우리 표와
		// 무관하게 참이다 — 이 요소를 만난 것 자체가 증거다. 누가
		// 감쌌는지(Wrapped 대 WrappedByUs)는 아래에서 따로 가린다.
		out.Wrapped[id] = true
		// 값이 빈 속성은 없는 것과 같이 다룬다. applyAttrs(Task 8)가 빈
		// 값을 아예 제거하므로 --clean 뒤에는 속성 자체가 없다 — 하지만
		// 손편집 파일에는 erdtoolIssue=""가 남을 수 있고, 그것을
		// "마크됨"으로 읽으면 저장된 적 없는 스타일을 복원하려 든다.
		if v := attrOf(st, AttrIssue); v != "" {
			out.Marked[id] = true
		}
		if v := attrOf(st, AttrBaseStyle); v != "" {
			out.BaseStyle[id] = v
		}
		if attrOf(st, AttrAnnotation) == AnnotationSummary {
			out.Summaries[id] = true
		}
		// "1"과 정확히 같을 때만 우리 것으로 인정한다(다른 값·빈 문자열은
		// 손편집이나 다른 도구가 우연히 같은 이름을 쓴 것일 수 있으므로
		// 소유권 증거로 안 쓴다).
		if attrOf(st, AttrWrapped) == "1" {
			out.WrappedByUs[id] = true
		}
	}
}

func attrOf(st xml.StartElement, name string) string {
	for _, a := range st.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}
