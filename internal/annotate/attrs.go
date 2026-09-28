// Package annotate는 «erdtool annotate» 서브커맨드가 쓴다. 사용자의
// .drawio를 제자리에서 고쳐 진단이 붙은 셀의 테두리를 빨갛게 물들이고,
// --clean으로 그 표식을 전부 되돌린다.
//
// «원래 바이트»가 아니라 «표식을 되돌린다»인 것이 중요하다. Go의
// encoding/xml은 self-closing 빈 요소(<a/>)와 여닫는 쌍(<a></a>)을 토큰
// 수준에서 구별하지 못해, 재작성기를 한 번이라도 거치면 그 차이는
// 원리적으로 되돌릴 수 없다. --clean이 실제로 지키는 계약은
//
//	Clean(Annotate(x)) == convert.RewriteMxFilePlan(x, convert.RewritePlan{})
//
// 즉 «자기 표식을 하나도 안 남기고 그 밖의 무엇도 안 바꾼다»이며, 그
// 기준선은 원본 자체가 아니라 convert가 이미 쓰고 있는 정규화 왕복형이다
// (Clean의 문서 주석 참고).
package annotate

// erdtool이 .drawio에 남기는 커스텀 속성들이다.
//
// 제자리 수정에서 «사람 것을 안 건드린다»는 무엇이 내 것인지 알아야
// 성립한다. convert가 logicalName으로 쓴 수법과 같다. draw.io의 예약
// 속성(label, placeholders, link, tooltip)과 겹치지 않는다.
const (
	// AttrIssue는 이 셀에 붙은 진단 내용이다. 있으면 내가 마크한 셀이다.
	AttrIssue = "erdtoolIssue"
	// AttrBaseStyle은 마크하기 전의 원래 스타일이다. 복원의 진실의
	// 출처이며 **이미 값이 있으면 절대 덮어쓰지 않는다** — 덮어쓰면
	// 두 번째 실행에서 빨간 테두리가 «원래 스타일»로 굳어 --clean이
	// 원본을 복원하지 못한다.
	AttrBaseStyle = "erdtoolBaseStyle"
	// AttrAnnotation은 이 셀 자체가 erdtool이 만든 물건임을 말한다.
	AttrAnnotation = "erdtoolAnnotation"
	// AttrWrapped는 «이 <object>/<UserObject> 래퍼 자체를 erdtool이
	// 만들었다»는 표다(값은 "1"). 마크가 원래 맨 <mxCell>이던 셀을 새로
	// 감쌀 때만 붙인다 — 이미 사용자가 다른 이유(link·tooltip·자기
	// 데이터)로 감싸 둔 래퍼에 진단을 얹을 때는 안 붙인다.
	//
	// 이 표가 있어야 진단이 사라졌을 때 «이 래퍼를 벗겨도 되는가»를
	// 안전하게 판정할 수 있다. 예전에는 "편집 후 label·id 말고 남은
	// 속성이 없으면 벗긴다"는 구조적 추측을 썼는데, 그 추측은 사용자가
	// 원래부터 label·id만 갖고 있던 빈 래퍼(우리와 무관하게 이미 그
	// 모양이었던)까지 똑같이 벗겨 사용자 래퍼를 지워버렸다(코드리뷰)
	// . 우리가 만든 래퍼에만 이 표를 남기고, 벗기기는 이 표를
	// 실제로 갖고 있던 래퍼에만 허용하면 그 사고가 안 난다 — 다른 셋
	// (Issue·BaseStyle·Annotation)과 같은 "내 물건에만 내 표를 남긴다"
	// 원칙이다.
	AttrWrapped = "erdtoolWrapped"
)

// AnnotationSummary는 AttrAnnotation의 값 중 «페이지 요약 박스»다.
const AnnotationSummary = "page-summary"

// SummaryCellID는 페이지 하나의 요약 박스 셀 id를 만든다. 페이지마다
// 하나뿐이고 실행마다 같아야 한다 — 그래야 두 번째 실행이 «옛 박스»를
// 찾아 지울 수 있다.
func SummaryCellID(diagramID string) string {
	return "erdtool-summary-" + diagramID
}
