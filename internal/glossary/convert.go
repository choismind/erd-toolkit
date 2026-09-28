// internal/glossary/convert.go
package glossary

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Part는 변환 결과의 조각 하나다. Matched가 false면 사전에 없어서 한글
// 원문을 그대로 남긴 조각이며, 이때 Abbr == Source다.
type Part struct {
	Source  string
	Abbr    string
	Matched bool
}

// Route는 어느 경로로 변환됐는지다. 호출자가 --dry-run 표에 찍는다.
type Route int

const (
	RouteTermWhole          Route = iota // 용어사전에 통째로 있었다
	RouteTermJoined                      // 구분자를 뗀 문자열이 용어사전에 있었다
	RouteSegmented                       // 단어사전으로 분해했다
	RouteSeparatorSplit                  // 구분자로 쪼개 조각별로 조회했고, 적어도 하나는 찾았다
	RouteSeparatorSegmented              // 구분자로 쪼갠 뒤, 못 찾은 조각을 분해했다
	RouteUnmatched                       // 아무것도 못 맞혀 이름이 통째로 그대로다
	RouteWordWhole                       // 단어사전에 통째로 있었다
)

// 새 Route는 «끝에만» 붙인다. 사이에 끼워 넣으면 뒤쪽 값이 통째로 밀리는데,
// 지금은 직렬화하는 곳이 없어 무해해도 다음 사람이 그 관례를 안 지킨다.
// (실제로 RouteWordWhole을 사이에 끼웠다가 RouteUnmatched가 5에서 6으로
// 밀린 적이 있고, 그것을 되돌린 것이 지금 이 순서다.)

func (r Route) String() string {
	switch r {
	case RouteTermWhole:
		return "용어사전 통째"
	case RouteTermJoined:
		return "용어사전(구분자제거)"
	case RouteSegmented:
		return "분해"
	case RouteSeparatorSplit:
		return "구분자분리"
	case RouteSeparatorSegmented:
		return "구분자분리+분해"
	case RouteWordWhole:
		return "단어사전 통째"
	case RouteUnmatched:
		return "미변환"
	}
	return "알 수 없음"
}

// Result는 변환 하나의 결과다. Parts를 통째로 들고 있어야 --dry-run 표와
// 미매칭 요약을 «엔진이 아니라 호출자가» 만든다 — 엔진은 출력 포맷을 모른다.
type Result struct {
	Physical string
	Parts    []Part
	Route    Route
}

// HasUnmatched는 사전에 없어 한글이 남은 조각이 하나라도 있는지다.
func (r Result) HasUnmatched() bool {
	for _, p := range r.Parts {
		if !p.Matched {
			return true
		}
	}
	return false
}

// Convert는 논리명(한글)을 물리명(영문 약어)으로 바꾼다.
//
// 레퍼런스 구현(trans_logic2physic.py의 logi_to_physi)의 알고리즘을 따르되
// 두 가지가 다르다:
//   - 형태소 분석기를 쓰지 않는다. 단어사전 최장일치 분해가 그 자리를 대신한다
//     (공통표준용어 1693건 중 1692건 재현, 실제 ERD 정답 13/13).
//   - 조각에 .title()을 걸지 않는다. 레퍼런스의 그 처리는 GIS를 Gis로
//     망가뜨려 사전 조회를 실패시킨다.
//
// 사전에 없는 낱말은 한글 원문을 그대로 남긴다. 영문으로 추측해 채우지
// 않는다 — 설계자가 산출물을 읽기만 해도 미매칭 지점을 찾을 수 있어야 한다.
func (d *Dict) Convert(logical string) Result {
	// 경계 공백은 잘라낸 다음 변환한다.
	//
	// 자르지 않으면 "주문고객번호 "가 «구분자가 있는 이름»으로 떨어져 분해
	// 경로를 통째로 건너뛰고, 낱말 하나를 통째로 조회했다가 실패해서
	// 한글이 그대로 남았다("주문고객번호"). 붙여 쓴 같은 이름은
	// ORDR_CUST_NO가 되는데 뒤에 공백 하나가 붙었다는 이유로 변환이
	// 통째로 안 되는 셈이었고, 그것을 «미매칭»이라고 보고했다.
	//
	// 소유자 규칙: 한글 테이블명·속성명의 맨 앞뒤에는 공백이 없어야 한다.
	// 그럼에도 공백이 있으면 «제거한 다음» 변환한다.
	trimmed := strings.TrimFunc(logical, unicode.IsSpace)
	if trimmed == "" {
		// 통째로 공백이거나 빈 이름. 잘라낸 결과로 바꾸면 이름이 사라진다.
		// 분해를 타지 않았으므로 [분해]가 아니다 — 아무것도 안 일어났다.
		return Result{Physical: logical, Route: RouteUnmatched}
	}

	pieces, seps := splitSeparators(trimmed)
	if len(pieces) == 0 {
		// 구분자만 있는 이름("___"). 바꿀 낱말이 하나도 없으므로 그대로
		// 둔다 — 빈 문자열로 만들면 이름이 사라진다. 이 이름은 buildEdits가
		// 거르지 않으므로 --dry-run에 실제로 나온다("column ___ -> ___").
		// 분해를 타지 않았으니 [미변환]이다.
		return Result{Physical: trimmed, Route: RouteUnmatched}
	}

	// 앞뒤에 붙어 있던 구분자는 출력에도 그대로 돌려놓는다(소유자 규칙:
	// «명칭에 있는 언더바는 지우지 않는다»).
	head, tail := joiner(seps[0]), joiner(seps[len(seps)-1])

	if len(pieces) == 1 {
		// 실제로 쪼개진 것이 없다. 앞뒤 구분자를 되돌려 붙이는 것 말고는
		// 조각 하나와 똑같이 다룬다. 여기서 분해를 건너뛰면 "_주문고객번호"
		// 처럼 밑줄이나 공백이 하나 붙었다는 이유만으로 변환이 통째로
		// 실패한다.
		//
		// 조각 하나를 푸는 절차는 resolvePiece 한 곳뿐이다. 여기서 그
		// 절차를 다시 쓰면 두 분기가 또 어긋난다 — 실제로 두 번 어긋났다.
		// (1) 한글 가드가 조각 쪽에만 있어 ZIP이 Z_IP가 됐고,
		// (2) 그것을 고치면서 단어사전 조회가 이쪽에만 빠져 GIS·SMS 같은
		//     «사전에 있는» 영문 표제어가 미매칭으로 보고됐다.
		// 이제 순서(용어사전 -> 단어사전 -> 가드 -> 분해)는 저기 한 벌만
		// 살고, 이 분기는 그 «결과»를 자기 Route로 옮기기만 한다.
		parts, outcome := d.resolvePiece(pieces[0])
		return Result{
			Physical: head + joinParts(parts, nil) + tail,
			Parts:    parts,
			Route:    outcome.wholeNameRoute(),
		}
	}

	// 조각 사이의 구분자도 원문 모양대로 되돌린다.
	mid := make([]string, 0, len(pieces)-1)
	plain := true // 조각 사이가 모두 «밑줄 하나»인가
	for _, sep := range seps[1 : len(seps)-1] {
		j := joiner(sep)
		if j != "_" {
			plain = false
		}
		mid = append(mid, j)
	}

	// 용어사전 통째 조회는 구분자를 뗀 문자열로 하므로, 그 답을 쓰는 순간
	// 조각 사이가 몇 칸이었는지는 알 수 없게 된다(약어 자신의 밑줄만 남는다).
	// 그래서 조각 사이가 모두 밑줄 하나였을 때만 이 지름길을 쓴다 —
	// "고객__번호"의 겹친 밑줄은 사용자가 찍은 글자이므로 지우면 안 된다.
	if plain {
		if abbr, ok := d.Term(strings.Join(pieces, "")); ok {
			return Result{
				Physical: head + abbr + tail,
				Parts:    []Part{{Source: trimmed, Abbr: abbr, Matched: true}},
				Route:    RouteTermJoined,
			}
		}
	}

	// 조각마다 «용어사전 -> 단어사전 -> (둘 다 놓쳤을 때만) 분해»로 푼다.
	// 조각 하나가 여러 Part로 갈릴 수 있으므로 조각별로 묶어 둔다 — 묶지
	// 않고 평평한 Part 목록에 mid를 위치로 대면 개수가 어긋나 구분자가
	// 엉뚱한 자리에 박힌다.
	groups := make([][]Part, len(pieces))
	parts := make([]Part, 0, len(pieces))
	decomposed := false
	for i, p := range pieces {
		g, outcome := d.resolvePiece(p)
		if outcome == pieceSegmented {
			decomposed = true
		}
		groups[i] = g
		parts = append(parts, g...)
	}

	var b strings.Builder
	b.WriteString(head)
	for i, g := range groups {
		if i > 0 {
			b.WriteString(mid[i-1])
		}
		// 조각 «안»에서 분해가 만든 자리는 밑줄 하나로 잇는다(분해 경로와
		// 같은 규칙). 조각 «사이»는 원문의 구분자 런 모양대로다.
		b.WriteString(joinParts(g, nil))
	}
	b.WriteString(tail)

	// Route는 «무슨 일이 실제로 일어났는가»를 그대로 비춘다.
	//
	//   하나도 못 찾음      -> [미변환]        (구분자가 없는 이름과 같은 답)
	//   찾긴 했는데 분해 없음 -> [구분자분리]
	//   분해가 기여함        -> [구분자분리+분해]
	//
	// 하나도 못 찾았는데 [구분자분리]라고 찍으면 그 라벨의 정의(«조각별로
	// 조회했고 적어도 하나는 찾았다»)가 그 자리에서 거짓이 된다. 게다가
	// 밑줄 유무만으로 라벨이 갈렸다 — CUSTOMER는 [미변환]인데 CUST_NO는
	// [구분자분리]였고, 둘 다 한 글자도 변환되지 않은 이름이다. 구분자
	// 하나 때문에 답이 갈리는 것은 이 파일이 반복해서 없애 온 부류다.
	route := RouteSeparatorSplit
	switch {
	case decomposed:
		// 분해가 기여했다는 것은 조각을 하나 이상 맞혔다는 뜻이므로
		// 아래 «하나도 못 찾음»과 겹치지 않는다.
		route = RouteSeparatorSegmented
	case !anyMatched(parts):
		route = RouteUnmatched
	}
	return Result{Physical: b.String(), Parts: parts, Route: route}
}

// resolvePiece는 이름 조각 하나를 푼다. «조각»은 구분자로 갈린 조각일 수도,
// 구분자가 아예 없는 이름 하나일 수도 있다 — 두 경우가 같은 절차를 쓴다.
//
// 순서는 «용어사전 -> 단어사전 -> (둘 다 놓쳤을 때만) 분해»다. 앞의 둘로
// 풀리는 조각은 분해를 아예 태우지 않는다 — 분해는 조회의 «대체»가 아니라
// «폴백»이다. 그래서 이 규칙은 순수하게 더하기다: 오늘 풀리는 조각은
// 내일도 똑같은 값으로 풀리고, 오늘 한글로 남던 조각만 새로 풀릴 수 있다.
//
// 분해를 폴백으로 두는 이유: 사전에 통째로 등록된 답이 언제나 권위 있는
// 답이다. 분해는 그 답이 «없을 때» 낱말을 이어 붙여 «추측»하는 것이므로,
// 사전이 말해준 답을 추측으로 덮어쓰면 되던 변환이 조용히 달라진다.
//
// 분해에 태울 때는 segmentGuarded를 통한다 — 한글이 없는 조각을 걸러내는
// 가드가 거기 한 벌만 산다.
//
// 두 번째 반환값은 «어떻게 풀렸는가»다. 두 분기(구분자 없는 한 낱말,
// 구분자로 갈린 조각)가 이 값을 각자의 Route로 옮기기만 하므로, 조각을
// 푸는 «순서»는 이 함수 하나에만 산다.
func (d *Dict) resolvePiece(piece string) ([]Part, pieceOutcome) {
	if abbr, ok := d.Term(piece); ok {
		return []Part{{Source: piece, Abbr: abbr, Matched: true}}, pieceTerm
	}
	if abbr, ok := d.Word(piece); ok {
		return []Part{{Source: piece, Abbr: abbr, Matched: true}}, pieceWord
	}
	parts, matched := d.segmentGuarded(piece)
	if matched {
		return parts, pieceSegmented
	}
	return parts, pieceUnresolved
}

// pieceOutcome은 조각 하나가 «어떻게» 풀렸는지다.
type pieceOutcome int

const (
	pieceTerm       pieceOutcome = iota // 용어사전에 통째로 있었다
	pieceWord                           // 단어사전에 통째로 있었다
	pieceSegmented                      // 분해가 답에 기여했다
	pieceUnresolved                     // 아무것도 못 맞혔다
)

// wholeNameRoute는 «구분자가 없는 이름 하나»일 때의 Route다.
//
// 라벨의 쓸모는 경고다 — «이 답에는 사전에서 찾은 것이 아니라 낱말을 이어
// 붙인 추측이 섞여 있다». 그래서 사전에서 통째로 찾은 것(용어/단어)과
// 분해로 지어낸 것을 가르고, 아무것도 못 맞혀 이름이 그대로인 것은
// 추측이 없으므로 [미변환]이다.
func (o pieceOutcome) wholeNameRoute() Route {
	switch o {
	case pieceTerm:
		return RouteTermWhole
	case pieceWord:
		return RouteWordWhole
	case pieceSegmented:
		return RouteSegmented
	}
	return RouteUnmatched
}

// segmentGuarded는 «분해해도 되는 문자열만 분해한다». 호출자는 resolvePiece
// 하나뿐이다 — 조각을 푸는 «순서»는 거기가 갖고, 이 함수는 «분해를 태워도
// 되는가»라는 별개 규칙만 갖는다. 둘을 한 함수에 합치지 않는 이유는 순서를
// 읽으러 온 사람이 가드의 근거까지 읽을 필요는 없기 때문이다.
//
// 가드: 한글이 한 글자도 없는 문자열은 분해하지 않는다. 분해는 «한글을
// 단어사전 표제어로 쪼개는» 일인데, 단어사전에는 SMS·IP·GIS처럼 영문
// 표제어도 몇 개 섞여 있어서 이미 영문인 이름이 그 표제어에 걸려 터진다.
// 실측: 가드 없이는 ZIP -> Z_IP, SLIP -> SL_IP, OZIP -> OZ_IP,
// DSMSL -> D_SMS_L. 전부 «이미 완전한 영문 약어»였던 이름이고, 원장의 승인
// 결정 6(«이미 영문인 이름은 전부 미매칭으로 떨어져 그대로 남으므로 무해하며
// 멱등적»)을 정면으로 깬다. 그 결정이 참이 되려면 이 가드가 있어야 한다.
//
// 피해 시나리오는 가정이 아니다: `우편번호`를 변환하면 `ZIP`이 나오고,
// 그 `ZIP`을 손으로 컬럼에 써 둔 파일을 다시 convert하면 `Z_IP`가 됐다.
//
// 두 번째 반환값은 «분해가 답에 기여했는가»다. 가드에 걸렸거나, 태웠는데
// 아무 조각도 못 맞혔으면 false다 — 두 경우 모두 문자열이 통째로 미매칭
// 조각 하나로 남아 결과가 원문과 같으므로, 호출자는 이 값으로 «추측이
// 섞였는지»를 판정한다.
func (d *Dict) segmentGuarded(s string) (parts []Part, matched bool) {
	if !hasHangul(s) {
		return []Part{{Source: s, Abbr: s, Matched: false}}, false
	}
	parts = d.lookupParts(d.segment(s))
	for _, p := range parts {
		if p.Matched {
			return parts, true
		}
	}
	return parts, false
}

// hasHangul은 한글(음절 또는 자모)이 한 글자라도 있는지다.
func hasHangul(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

// anyMatched는 조각 중 하나라도 사전에서 답을 얻었는지다.
func anyMatched(parts []Part) bool {
	for _, p := range parts {
		if p.Matched {
			return true
		}
	}
	return false
}

// splitSeparators는 이름을 낱말과 «구분자 런»으로 나눈다. 구분자는 밑줄과
// 공백류(스페이스·탭·NBSP 등)다.
//
// seps는 언제나 len(pieces)+1개다: 맨 앞, 조각 사이마다 하나씩, 맨 뒤.
// 비어 있는 자리는 빈 문자열이다. 조각만 돌려주고 구분자를 버리면 원문에
// 있던 밑줄이 어디에 몇 개 있었는지 알 수 없어 출력에서 사라진다.
//
// 공백을 구분자로 보는 이유: 손으로 그린 논리 ERD는 테이블 이름을
// "고객 정보"처럼 띄어 쓰고, 테이블 셀은 값 «전체»를 변환하므로 그 공백이
// 그대로 들어온다(컬럼 셀은 splitFirstField가 첫 필드만 떼므로 무관하다).
// 공백을 구분자로 보지 않으면 분해 경로가 공백을 «사전에 없는 낱말»로
// 판정해서 두 가지가 함께 망가진다:
//   - 공백 양쪽에 밑줄을 둘러 "CUST_ _NO"를 만든다. 원문도 아니고
//     식별자도 아닌 값이 그대로 generate의 CREATE TABLE로 흘러간다
//     (CUST를 C_U_S_T로 터뜨리던 것과 같은 부류의 결함이다).
//   - 미매칭 목록에 " "가 실려 "미매칭 낱말 1개( )"로 보고된다. 사용자는
//     뭔가 실패했다는 말만 듣고 화면에서는 아무것도 볼 수 없는데, 한글을
//     남기는 이유가 바로 «눈으로 읽어 미매칭 지점을 찾게» 하는 것이다.
//
// 빈 조각을 만들지 않는 이유: "_고객"이나 "고객__번호"처럼 구분자가 앞뒤에
// 붙거나 겹칠 때 그 자리를 조각으로 세면 «보이지 않는 미매칭»이 하나
// 생긴다. 그 자리는 조각이 아니라 구분자 런의 길이로 기록된다.
func splitSeparators(s string) (pieces, seps []string) {
	isSep := func(r rune) bool { return r == '_' || unicode.IsSpace(r) }

	for i := 0; i < len(s); {
		j := i
		for j < len(s) {
			r, w := utf8.DecodeRuneInString(s[j:])
			if !isSep(r) {
				break
			}
			j += w
		}
		seps = append(seps, s[i:j])
		i = j
		for j < len(s) {
			r, w := utf8.DecodeRuneInString(s[j:])
			if isSep(r) {
				break
			}
			j += w
		}
		if j > i {
			pieces = append(pieces, s[i:j])
			i = j
		}
		// 한 바퀴마다 구분자 런이나 낱말 런 중 적어도 하나는 비어 있지
		// 않으므로 i는 반드시 앞으로 간다.
	}
	for len(seps) < len(pieces)+1 {
		seps = append(seps, "")
	}
	return pieces, seps
}

// joiner는 구분자 런 하나를 출력에 넣을 모양으로 바꾼다.
//
// 소유자 규칙: «명칭에 있는 언더바는 지우지 않는다». 밑줄은 사용자가 직접
// 찍은 글자이므로 몇 개가 어디에 있든 그 개수 그대로 살린다. 공백은 낱말을
// 가르는 «구분»일 뿐이므로 밑줄 하나로 합류시킨다 — 그래야 "고객 번호"가
// CUST_NO가 되고, 공백을 낱말로 취급하던 시절의 "CUST_ _NO"가 돌아오지
// 않는다. 경계 공백은 Convert가 이미 잘라내므로 여기까지 오지 않는다.
func joiner(sep string) string {
	if sep == "" {
		return ""
	}
	if n := strings.Count(sep, "_"); n > 0 {
		return strings.Repeat("_", n)
	}
	return "_"
}

// lookupParts는 조각마다 용어사전 -> 단어사전 순으로 약어를 찾고, 없으면
// 한글 원문을 그대로 둔다.
//
// segment가 이미 «사전에 없는 글자»로 표시한 조각(Matched=false)은 조회조차
// 하지 않는다 — 조회해봐야 없다.
func (d *Dict) lookupParts(segs []segmentPart) []Part {
	parts := make([]Part, 0, len(segs))
	for _, s := range segs {
		if !s.Matched {
			parts = append(parts, Part{Source: s.Text, Abbr: s.Text, Matched: false})
			continue
		}
		if abbr, ok := d.Term(s.Text); ok {
			parts = append(parts, Part{Source: s.Text, Abbr: abbr, Matched: true})
			continue
		}
		if abbr, ok := d.Word(s.Text); ok {
			parts = append(parts, Part{Source: s.Text, Abbr: abbr, Matched: true})
			continue
		}
		parts = append(parts, Part{Source: s.Text, Abbr: s.Text, Matched: false})
	}
	return parts
}

// joinParts는 조각의 약어를 이어 붙인다. seps[i]는 i번째와 i+1번째 조각
// 사이에 넣을 구분자이며, 모자라는 자리는 밑줄 하나로 잇는다(분해 경로가
// 만든 조각처럼 원문에 구분자가 없던 자리다).
func joinParts(parts []Part, seps []string) string {
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			if i-1 < len(seps) {
				b.WriteString(seps[i-1])
			} else {
				b.WriteString("_")
			}
		}
		b.WriteString(p.Abbr)
	}
	return b.String()
}
