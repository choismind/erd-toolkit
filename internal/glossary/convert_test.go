package glossary

import (
	"strings"
	"testing"
	"unicode"
)

func convTestDict(t *testing.T) *Dict {
	t.Helper()
	p := writeTestDict(t,
		[][2]string{
			{"고객번호", "CUST_NO"},
			{"주문일자", "ORDR_YMD"},
			{"주문고객", "ORDR_CUST"},
		},
		[][2]string{
			{"주문", "ORDR"}, {"고객", "CUST"}, {"번호", "NO"},
			{"일자", "YMD"}, {"명", "NM"}, {"GIS", "GIS"},
		},
	)
	d, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return d
}

func TestConvert_TermWhole(t *testing.T) {
	d := convTestDict(t)
	got := d.Convert("고객번호")
	if got.Physical != "CUST_NO" {
		t.Fatalf("Physical = %q; want CUST_NO", got.Physical)
	}
	if got.Route != RouteTermWhole {
		t.Fatalf("Route = %v; want RouteTermWhole", got.Route)
	}
	if got.HasUnmatched() {
		t.Fatal("미매칭이 없어야 한다")
	}
}

func TestConvert_Segmented(t *testing.T) {
	d := convTestDict(t)
	got := d.Convert("주문고객번호")
	if got.Physical != "ORDR_CUST_NO" {
		t.Fatalf("Physical = %q; want ORDR_CUST_NO", got.Physical)
	}
	if got.Route != RouteSegmented {
		t.Fatalf("Route = %v; want RouteSegmented", got.Route)
	}
}

func TestConvert_UnmatchedKeepsKorean(t *testing.T) {
	d := convTestDict(t)
	got := d.Convert("약어명")
	// 미매칭 "약어"는 한 덩어리로 남는다. 사전에 없는 글자 사이에 없던
	// 밑줄을 끼워 넣지 않는다.
	if got.Physical != "약어_NM" {
		t.Fatalf("Physical = %q; want 약어_NM", got.Physical)
	}
	if !got.HasUnmatched() {
		t.Fatal("미매칭이 있어야 한다")
	}
}

func TestConvert_UnderscoreJoinedHitsTerm(t *testing.T) {
	d := convTestDict(t)
	got := d.Convert("고객_번호")
	if got.Physical != "CUST_NO" {
		t.Fatalf("Physical = %q; want CUST_NO (밑줄 제거 후 용어사전 통째)", got.Physical)
	}
	if got.Route != RouteTermJoined {
		t.Fatalf("Route = %v; want RouteTermJoined", got.Route)
	}
}

func TestConvert_UnderscoreSplit(t *testing.T) {
	d := convTestDict(t)
	// 주문일자고객 은 용어사전에 없다 -> 밑줄로 쪼개 조각별로 본다.
	got := d.Convert("주문일자_고객")
	if got.Physical != "ORDR_YMD_CUST" {
		t.Fatalf("Physical = %q; want ORDR_YMD_CUST", got.Physical)
	}
	if got.Route != RouteSeparatorSplit {
		t.Fatalf("Route = %v; want RouteSeparatorSplit", got.Route)
	}
}

func TestConvert_DoesNotChangeAbbrCase(t *testing.T) {
	// 레퍼런스는 조각마다 .title()을 걸어 GIS를 Gis로 망가뜨렸다.
	d := convTestDict(t)
	if got := d.Convert("GIS명").Physical; got != "GIS_NM" {
		t.Fatalf("Physical = %q; want GIS_NM", got)
	}
}

func TestConvert_AlreadyPhysicalIsIdempotent(t *testing.T) {
	// 이미 영문인 이름은 한글 사전에 있을 리 없으므로 전부 미매칭 -> 그대로.
	d := convTestDict(t)
	if got := d.Convert("CUST_NO").Physical; got != "CUST_NO" {
		t.Fatalf("Physical = %q; want CUST_NO", got)
	}
}

func TestConvert_EmptyAndBlank(t *testing.T) {
	d := convTestDict(t)
	for _, in := range []string{"", "   "} {
		if got := d.Convert(in).Physical; got != in {
			t.Fatalf("Convert(%q).Physical = %q; want unchanged", in, got)
		}
	}
}

// TestConvert_AlreadyEnglishNameSurvives는 원장의 사용자 승인 결정 6
// («파일 안의 모든 테이블을 변환한다 — 이미 영문인 이름은 전부 미매칭으로
// 떨어져 그대로 남으므로 사실상 무해하며 멱등적»)의 전제를 붙잡는다.
//
// 그 전제는 밑줄이 있을 때만 참이었다. CUST_NO는 밑줄 경로로 가서 조각
// 단위로 조회되지만, 밑줄 없는 CUST는 분해 경로로 가서 한 글자씩 미매칭
// 조각이 되고 joinParts가 그것들을 밑줄로 이어 C_U_S_T를 만들었다.
// 없던 밑줄을 만들어 끼워 넣는 것은 «원문 그대로»가 아니다.
func TestConvert_AlreadyEnglishNameSurvives(t *testing.T) {
	d := convTestDict(t)
	for _, name := range []string{"CUST", "CUSTOMER", "ORDR_NO2"} {
		got := d.Convert(name)
		if got.Physical != name {
			t.Fatalf("Convert(%q).Physical = %q; 사전에 없는 이름은 원문 그대로여야 한다", name, got.Physical)
		}
	}
}

// TestConvert_SpaceIsASeparator는 공백이 «사전에 없는 낱말»로 떨어지지
// 않는지 붙잡는다.
//
// 손으로 그린 논리 ERD는 테이블 이름을 "고객 정보"처럼 띄어 쓰고, 테이블
// 셀은 값 «전체»를 변환한다. 공백을 구분자로 보지 않으면 분해 경로가 공백을
// 미매칭 조각으로 잡고 joinParts가 그 양쪽에 밑줄을 둘러 "CUST_ _NO"를
// 만든다 — 원문도 아니고 식별자도 아닌 값이 generate의 CREATE TABLE까지
// 그대로 흘러간다.
func TestConvert_SpaceIsASeparator(t *testing.T) {
	d := convTestDict(t)
	// 붙여 쓴 "고객번호"가 용어사전에 있으므로, 띄어 쓴 것도 같은 답이어야
	// 한다(밑줄로 쓴 "고객_번호"가 이미 그렇다).
	if got := d.Convert("고객 번호").Physical; got != "CUST_NO" {
		t.Fatalf("Convert(\"고객 번호\").Physical = %q; want CUST_NO", got)
	}
	// 용어사전에 통째로 없는 이름은 조각별로 찾아 밑줄로 잇는다.
	got := d.Convert("주문일자 고객")
	if got.Physical != "ORDR_YMD_CUST" {
		t.Fatalf("Physical = %q; want ORDR_YMD_CUST", got.Physical)
	}
	if got.Route != RouteSeparatorSplit {
		t.Fatalf("Route = %v; want RouteSeparatorSplit", got.Route)
	}
}

// TestConvert_NoBlankParts는 구분자가 «조각»으로 남지 않는지 본다.
//
// 조각으로 남으면 호출자의 미매칭 목록에 «보이지 않는 낱말»이 실려
// "미매칭 낱말 1개"라고만 보고된다. 사용자가 산출물을 눈으로 읽어 미매칭
// 지점을 찾을 수 있어야 한다는 것이 한글을 남기는 이유이므로, 보이지 않는
// 미매칭은 그 이유를 통째로 무너뜨린다.
//
// 예전에는 여기서 «출력에 밑줄이 앞뒤에 오거나 겹치면 안 된다»도 함께
// 봤다. 그 규칙은 소유자가 뒤집었다(«명칭에 있는 언더바는 지우지
// 않는다»). 밑줄이 살아남는 것과 빈 조각이 생기지 않는 것은 별개이며,
// 이 테스트가 지키는 것은 뒤쪽이다 — 출력 모양은
// TestConvert_UnderscoreSurvives가 글자 단위로 못 박는다.
func TestConvert_NoBlankParts(t *testing.T) {
	d := convTestDict(t)
	for _, in := range []string{
		"고객 번호", "주문일자 고객", "고객\t번호",
		"_고객", "고객_", "고객__번호", "  고객 번호  ", "___고객___번호___",
	} {
		res := d.Convert(in)
		for i, p := range res.Parts {
			if strings.TrimSpace(p.Source) == "" {
				t.Fatalf("Convert(%q).Parts[%d] = %+v; 공백/빈 조각은 나오면 안 된다 (Physical=%q)",
					in, i, p, res.Physical)
			}
		}
		// 공백은 구분자이므로 출력에 남으면 안 된다. 밑줄과 달리 그것은
		// 사용자가 «찍은 글자»가 아니라 낱말을 가른 자리다.
		if strings.ContainsFunc(res.Physical, unicode.IsSpace) {
			t.Fatalf("Convert(%q).Physical = %q; 공백이 남았다", in, res.Physical)
		}
	}
}

// TestConvert_SeparatorOnlyNameSurvives는 구분자만 있는 이름이 사라지지
// 않는지 본다. 빈 조각을 버리는 규칙을 그대로 밀면 "___"의 조각이 0개가
// 되어 Physical이 빈 문자열이 된다 — 이름을 지우는 것은 «그대로»가 아니다.
//
// 밑줄은 그대로 남고(소유자 규칙), 경계 공백은 잘린다(소유자 규칙) —
// " _ "의 답이 "_"인 것은 두 규칙이 겹친 자리다.
func TestConvert_SeparatorOnlyNameSurvives(t *testing.T) {
	d := convTestDict(t)
	for _, tc := range []struct{ in, want string }{
		{"___", "___"},
		{"_", "_"},
		{" _ ", "_"},
	} {
		if got := d.Convert(tc.in).Physical; got != tc.want {
			t.Fatalf("Convert(%q).Physical = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// TestConvert_TrimsBoundaryWhitespace는 소유자 결정 U2를 못 박는다.
//
//	«모든 한글테이블명, 한글속성명의 맨 앞단과 맨 뒷단에는 공백이 없어야
//	한다는 것을 전제조건으로 함. 만일 공백이 존재하는데 물리명으로
//	변환해야 할 경우 공백을 제거한 다음 변환.»
//
// 자르기 전에는 경계 공백 하나가 이름을 «구분자가 있는 이름»으로 만들어
// 분해 경로를 통째로 건너뛰게 했다: "주문고객번호 "는 낱말 하나로 조회돼
// 실패하고 한글이 그대로 남았다. 붙여 쓴 "주문고객번호"는 ORDR_CUST_NO가
// 되는데 뒤에 공백 하나가 붙었다는 이유로 변환이 통째로 안 됐고, 그것을
// «미매칭»이라고 보고했다 — 사용자가 눈으로 잡아내지 못하면 한글 이름이
// 그대로 물리 ERD에 실린다.
func TestConvert_TrimsBoundaryWhitespace(t *testing.T) {
	d := convTestDict(t)
	// 분해 경로(용어사전에 통째로 없다).
	for _, in := range []string{"주문고객번호 ", " 주문고객번호", " 주문고객번호 ", "주문고객번호\t", "\u00a0주문고객번호"} {
		res := d.Convert(in)
		if res.Physical != "ORDR_CUST_NO" {
			t.Fatalf("Convert(%q).Physical = %q; want ORDR_CUST_NO", in, res.Physical)
		}
		if res.Route != RouteSegmented {
			t.Fatalf("Convert(%q).Route = %v; want RouteSegmented", in, res.Route)
		}
		if res.HasUnmatched() {
			t.Fatalf("Convert(%q)에 미매칭이 있다: %+v", in, res.Parts)
		}
	}
	// 용어사전 통째 경로도 마찬가지다.
	if got := d.Convert(" 고객번호 "); got.Physical != "CUST_NO" || got.Route != RouteTermWhole {
		t.Fatalf("Convert(\" 고객번호 \") = %q/%v; want CUST_NO/RouteTermWhole", got.Physical, got.Route)
	}
	// 경계 공백만 자른다. 안쪽 공백은 여전히 구분자다.
	if got := d.Convert("  고객 번호  ").Physical; got != "CUST_NO" {
		t.Fatalf("Convert(\"  고객 번호  \").Physical = %q; want CUST_NO", got)
	}
}

// TestConvert_UnderscoreSurvives는 소유자 결정 U3을 못 박는다.
//
//	«명칭에 있는 언더바는 지우지 않는다.»
//
// 이것은 예전 결정(«구분자는 조각도 아니고 출력에도 남기지 않는다»)을
// 뒤집은 것이다. 밑줄은 사용자가 직접 찍은 글자이므로 몇 개가 어디에 있든
// 그대로 살아남는다. 공백은 다르다 — 낱말을 가르는 «구분»이므로 안쪽
// 공백은 밑줄 하나로 합류하고 경계 공백은 잘린다.
func TestConvert_UnderscoreSurvives(t *testing.T) {
	d := convTestDict(t)
	for _, tc := range []struct{ in, want string }{
		{"_고객", "_CUST"},
		{"고객_", "CUST_"},
		{"고객__번호", "CUST__NO"},
		{"고객_번호", "CUST_NO"},
		{"고객 번호", "CUST_NO"},
		{"주문고객번호 ", "ORDR_CUST_NO"},
		{"__고객__", "__CUST__"},
		{"_주문고객번호_", "_ORDR_CUST_NO_"},
		// 밑줄과 공백이 한 자리에 섞이면 밑줄만 남는다.
		{"고객 _ 번호", "CUST_NO"},
		// 사전에 없는 이름도 밑줄 모양 그대로다.
		{"_CUST__NO_", "_CUST__NO_"},
	} {
		if got := d.Convert(tc.in).Physical; got != tc.want {
			t.Fatalf("Convert(%q).Physical = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// TestConvert_ReconvertIsStable은 «변환 결과를 다시 변환해도 그대로»인지
// 본다. 파일 단위 멱등성은 logicalName이 지키지만(2회차의 변환 소스는
// 언제나 한글 원문이다), 엔진 자체도 자기 출력에 안정적이어야 손으로 물리명을
// 넣은 파일이 한 번 더 망가지지 않는다.
func TestConvert_ReconvertIsStable(t *testing.T) {
	d := convTestDict(t)
	for _, in := range []string{
		"_고객", "고객_", "고객__번호", "주문고객번호 ", "고객 번호", "___", "GIS명",
	} {
		once := d.Convert(in).Physical
		twice := d.Convert(once).Physical
		if once != twice {
			t.Fatalf("Convert(%q) = %q, 다시 변환하면 %q; 같아야 한다", in, once, twice)
		}
	}
}

// dpFallbackDict는 실물 표준용어사전의 «모양»을 그대로 축소한 사전이다.
// 이 규칙이 걸리는 자리가 전부 «용어사전의 통째 답과 단어 조합의 답이
// 다른» 지점이므로, 그 어긋남을 일부러 넣어 둔다:
//
//	계좌번호 -> ACTNO      그런데 계좌 -> BACNT, 번호 -> NO
//	건물층수 -> BLDG_FLR_CNT  그런데 층수 -> NOFL (층 -> FLR, 수 -> CNT)
//
// 실물 사전에서 그대로 가져온 값이다.
func dpFallbackDict(t *testing.T) *Dict {
	t.Helper()
	p := writeTestDict(t,
		[][2]string{
			{"고객번호", "CUST_NO"},
			{"계좌번호", "ACTNO"},
			{"건물층수", "BLDG_FLR_CNT"},
			{"이메일수신동의여부", "EML_RCPTN_AGRE_YN"},
		},
		[][2]string{
			{"주문", "ORDR"}, {"고객", "CUST"}, {"번호", "NO"},
			{"계좌", "BACNT"},
			{"건물", "BLDG"}, {"층수", "NOFL"}, {"층", "FLR"}, {"수", "CNT"},
			{"이메일", "EML"}, {"수신", "RCPTN"}, {"동의", "AGRE"}, {"여부", "YN"},
			// 실물 단어사전에는 영문 표제어도 섞여 있다(IP·SMS·GIS).
			{"IP", "IP"},
		},
	)
	d, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return d
}

// TestConvert_SeparatorPieceFallsBackToSegment는 구분자로 갈린 조각이
// 양쪽 사전을 다 놓쳤을 때 분해(DP)로 넘어가는지 본다.
//
// 이 규칙 전에는 조각을 «통째로»만 조회했다. 그래서 실물 사전에서
// `주문고객 번호`가 `주문고객_NO`가 됐다 — `주문고객`이 어느 사전에도
// 통째로 없어 한글로 남는데, 붙여 쓴 `주문고객번호`는 분해 경로를 타서
// `ORDR_CUST_NO`가 됐다. 공백 하나 때문에 답이 갈렸다.
func TestConvert_SeparatorPieceFallsBackToSegment(t *testing.T) {
	d := dpFallbackDict(t)
	for _, tc := range []struct{ in, want string }{
		{"주문고객 번호", "ORDR_CUST_NO"},
		{"주문고객_번호", "ORDR_CUST_NO"},
		{"주문고객__번호", "ORDR_CUST__NO"},
		{"_주문고객 번호_", "_ORDR_CUST_NO_"},
	} {
		got := d.Convert(tc.in)
		if got.Physical != tc.want {
			t.Fatalf("Convert(%q).Physical = %q; want %q", tc.in, got.Physical, tc.want)
		}
		if got.Route != RouteSeparatorSegmented {
			t.Fatalf("Convert(%q).Route = %v; want RouteSeparatorSegmented", tc.in, got.Route)
		}
		if got.HasUnmatched() {
			t.Fatalf("Convert(%q)에 미매칭이 있다: %+v", tc.in, got.Parts)
		}
	}
}

// TestConvert_SeparatorLookupBeatsSegment는 «분해는 대체가 아니라
// 폴백»이라는 설계 제약을 못 박는다.
//
// 조각이 용어사전이나 단어사전에 통째로 있으면 그 답이 이긴다. 분해가
// 그 자리를 대신하면 오늘 되던 변환이 조용히 다른 값으로 바뀐다 —
// 이 사전에서 `계좌번호`는 권위 있는 `ACTNO`인데 분해는 `BACNT_NO`를,
// `층수`는 `NOFL`인데 분해는 `FLR_CNT`를 내놓는다.
//
// 분해가 아예 안 돌았으므로 Route도 [구분자분리] 그대로여야 한다.
func TestConvert_SeparatorLookupBeatsSegment(t *testing.T) {
	d := dpFallbackDict(t)
	for _, tc := range []struct{ in, want string }{
		// 용어사전이 이긴다(분해면 ORDR__BACNT_NO).
		{"주문__계좌번호", "ORDR__ACTNO"},
		// 단어사전이 이긴다(분해면 BLDG__FLR_CNT).
		{"건물__층수", "BLDG__NOFL"},
	} {
		got := d.Convert(tc.in)
		if got.Physical != tc.want {
			t.Fatalf("Convert(%q).Physical = %q; want %q (조회가 분해를 이겨야 한다)", tc.in, got.Physical, tc.want)
		}
		if got.Route != RouteSeparatorSplit {
			t.Fatalf("Convert(%q).Route = %v; want RouteSeparatorSplit (분해가 돌면 안 된다)", tc.in, got.Route)
		}
	}
}

// TestConvert_BuildingFloorDivergenceRemains는 «이 규칙이 닫지 못하는
// 구멍»을 기록으로 남긴다. 고쳐진 척하지 않기 위한 테스트다.
//
// `건물층수`의 권위 있는 답은 용어사전의 통째 엔트리 `BLDG_FLR_CNT`다.
// 붙여 쓰거나 밑줄 하나·공백 하나로 쓰면 «구분자를 뗀 문자열»이 그
// 엔트리에 닿는다. 그러나 밑줄 둘로 쓰면 그 지름길을 쓸 수 없고
// (겹친 밑줄은 사용자가 찍은 글자라 지울 수 없다), 조각별 조회는
// `층수` -> `NOFL`에 먼저 걸린다. 조각별 분해는 `[건물, 층수]`밖에
// 못 보므로 통째 엔트리에 영영 닿지 않는다.
//
// 즉 이 어긋남의 원인은 «분해를 안 태워서»가 아니라 «지름길을 못 써서»다.
// 조각별 DP 폴백으로는 닫히지 않는다.
func TestConvert_BuildingFloorDivergenceRemains(t *testing.T) {
	d := dpFallbackDict(t)
	for _, tc := range []struct{ in, want string }{
		{"건물층수", "BLDG_FLR_CNT"},
		{"건물_층수", "BLDG_FLR_CNT"},
		{"건물 층수", "BLDG_FLR_CNT"},
		{"건물__층수", "BLDG__NOFL"}, // 아직 다르다
	} {
		if got := d.Convert(tc.in).Physical; got != tc.want {
			t.Fatalf("Convert(%q).Physical = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// TestConvert_SeparatorSegmentRecoversFullAbbr는 재리뷰가 «77건»으로
// 센 후퇴 부류를 하나 붙잡는다.
//
// 겹친 밑줄이 용어사전 지름길을 막으면 `이메일__수신동의여부`가
// `EML__수신동의여부`가 됐다 — 완전한 영문 약어였던 것이 부분 한글로
// 후퇴한 것이다. 조각 `수신동의여부`가 어느 사전에도 통째로 없기
// 때문인데, 분해하면 수신+동의+여부로 다 풀린다.
func TestConvert_SeparatorSegmentRecoversFullAbbr(t *testing.T) {
	d := dpFallbackDict(t)
	got := d.Convert("이메일__수신동의여부")
	if got.Physical != "EML__RCPTN_AGRE_YN" {
		t.Fatalf("Physical = %q; want EML__RCPTN_AGRE_YN", got.Physical)
	}
	if got.Route != RouteSeparatorSegmented {
		t.Fatalf("Route = %v; want RouteSeparatorSegmented", got.Route)
	}
	if got.HasUnmatched() {
		t.Fatalf("미매칭이 없어야 한다: %+v", got.Parts)
	}
	// 붙여 쓰거나 밑줄 하나로 쓰면 여전히 용어사전 통째 답이다.
	for _, in := range []string{"이메일수신동의여부", "이메일_수신동의여부", "이메일 수신동의여부"} {
		if p := d.Convert(in).Physical; p != "EML_RCPTN_AGRE_YN" {
			t.Fatalf("Convert(%q).Physical = %q; want EML_RCPTN_AGRE_YN", in, p)
		}
	}
}

// TestConvert_SeparatorSkipsSegmentForEnglishPiece는 한글이 없는 조각을
// 분해에 태우지 않는 가드를 못 박는다.
//
// 단어사전에는 IP·SMS·GIS 같은 영문 표제어가 섞여 있다. 가드가 없으면
// 이미 영문인 이름의 조각이 그 표제어에 걸려 터진다 — 실물 사전 코퍼스
// 측정에서 `HOME_ZIP` -> `HOME_Z_IP`, `SLIP_NO` -> `SL_IP_NO`,
// `DSMSL_YMD` -> `D_SMS_L_YMD` 부류로 17행이 망가졌다. 원장의 승인 결정
// 6(«이미 영문인 이름은 그대로 남으므로 무해하고 멱등적»)이 그 자리에서
// 거짓이 된다.
func TestConvert_SeparatorSkipsSegmentForEnglishPiece(t *testing.T) {
	d := dpFallbackDict(t)
	for _, name := range []string{"HOME_ZIP", "ZIP_NO", "SLIP_NO", "CUST_NO", "ORDR__ACTNO"} {
		got := d.Convert(name)
		if got.Physical != name {
			t.Fatalf("Convert(%q).Physical = %q; 이미 영문인 이름은 그대로여야 한다", name, got.Physical)
		}
		if got.Route == RouteSeparatorSegmented {
			t.Fatalf("Convert(%q).Route = %v; 한글 없는 조각에 분해가 돌면 안 된다", name, got.Route)
		}
	}
}

// TestConvert_SeparatorSegmentKeepsKorean은 분해가 «절반만» 풀었을 때
// 못 푼 낱말이 한글로 남아 눈에 보이는지 본다. 이 규칙은 시끄러운
// 실패를 조용한 성공으로 바꾸는 쪽이라, 아직 모르는 낱말까지 지어내지
// 않는다는 것을 함께 붙잡아야 한다.
func TestConvert_SeparatorSegmentKeepsKorean(t *testing.T) {
	d := dpFallbackDict(t)
	got := d.Convert("주문__약어여부")
	if got.Physical != "ORDR__약어_YN" {
		t.Fatalf("Physical = %q; want ORDR__약어_YN", got.Physical)
	}
	if !got.HasUnmatched() {
		t.Fatal("미매칭이 있어야 한다")
	}
	var missing []string
	for _, p := range got.Parts {
		if !p.Matched {
			missing = append(missing, p.Source)
		}
	}
	if len(missing) != 1 || missing[0] != "약어" {
		t.Fatalf("미매칭 = %v; want [약어]", missing)
	}
	// 분해가 답에 기여했으므로 Route에 그 사실이 남는다.
	if got.Route != RouteSeparatorSegmented {
		t.Fatalf("Route = %v; want RouteSeparatorSegmented", got.Route)
	}
}

// TestConvert_SeparatorSegmentIsIdempotent는 새 경로를 탄 결과를 다시
// 변환해도 그대로인지 본다. 파일 단위 멱등성은 logicalName이 지키지만,
// 사람이 물리명을 손으로 넣어 둔 파일은 엔진 자신이 자기 출력에
// 안정적이어야 한 번 더 망가지지 않는다.
func TestConvert_SeparatorSegmentIsIdempotent(t *testing.T) {
	d := dpFallbackDict(t)
	for _, in := range []string{
		"주문고객 번호", "주문고객__번호", "이메일__수신동의여부", "주문__약어여부",
		"주문__계좌번호", "건물__층수", "_주문고객 번호_", "HOME_ZIP",
	} {
		once := d.Convert(in).Physical
		twice := d.Convert(once).Physical
		if once != twice {
			t.Fatalf("Convert(%q) = %q, 다시 변환하면 %q; 같아야 한다", in, once, twice)
		}
	}
}

// TestConvert_SingleNameSkipsSegmentForEnglish는 소유자 결정 U4를 못 박는다.
//
//	«한글 가드의 비대칭을 없앤다.»
//
// 가드가 구분자로 갈린 조각 경로에만 있고 구분자 «없는» 한 낱말 경로에는
// 없었다. 그래서 밑줄 하나 없는 영문 이름이 단어사전의 영문 표제어(IP·SMS 등)에
// 걸려 터졌다 — 실물 사전에서 ZIP -> Z_IP, SLIP -> SL_IP, OZIP -> OZ_IP,
// DSMSL -> D_SMS_L.
//
// 이것은 원장의 승인 결정 6(«이미 영문인 이름은 전부 미매칭으로 떨어져 그대로
// 남으므로 사실상 무해하며 멱등적»)을 정면으로 깨는 자리였다.
//
// 처음에는 «두 경로가 같은 segmentGuarded를 통하므로 다시 어긋날 수 없다»고
// 적었는데, 그 주장은 곧 거짓으로 드러났다 — 어긋난 자리는 가드가 아니라 그
// «앞단»(단어사전 조회)이었고, 가드만 묶은 탓에 사전에 있는 GIS·SMS가
// 미매칭으로 보고됐다. 지금의 보장은 한 단계 위에서 온다: 두 경로가
// resolvePiece 하나를 통하므로 조회 순서와 가드가 함께 산다.
func TestConvert_SingleNameSkipsSegmentForEnglish(t *testing.T) {
	d := dpFallbackDict(t)
	// ZIP은 단어사전에 없지만 IP는 있다. 가드가 없으면 Z_IP로 터진다.
	for _, name := range []string{"ZIP", "OZIP", "SLIP", "IPZ", "CUSTIP"} {
		got := d.Convert(name)
		if got.Physical != name {
			t.Fatalf("Convert(%q).Physical = %q; 이미 영문인 이름은 그대로여야 한다", name, got.Physical)
		}
		if got.Route != RouteUnmatched {
			t.Fatalf("Convert(%q).Route = %v; want RouteUnmatched (분해를 안 탔다)", name, got.Route)
		}
		if !got.HasUnmatched() {
			t.Fatalf("Convert(%q)는 미매칭으로 떨어져야 한다: %+v", name, got.Parts)
		}
	}
	// 경계 구분자가 붙어도 마찬가지다. 예전에는 "_ZIP_"이 "_Z_IP_"가 됐다.
	for _, tc := range []struct{ in, want string }{
		{"_ZIP_", "_ZIP_"}, {"__ZIP", "__ZIP"}, {" ZIP ", "ZIP"},
	} {
		if got := d.Convert(tc.in).Physical; got != tc.want {
			t.Fatalf("Convert(%q).Physical = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// TestConvert_GuardDoesNotBlockMixedNames는 가드가 «한글이 한 글자도 없을
// 때»만 걸린다는 것을 못 박는다. 한글과 영문이 섞인 이름은 여전히 분해를
// 탄다 — 실물 사전의 용어에는 `GIS자료여부`·`SMS발송여부`처럼 섞인 것이
// 흔하고, 그것까지 막으면 이 규칙이 고치려던 것을 도로 부순다.
func TestConvert_GuardDoesNotBlockMixedNames(t *testing.T) {
	d := dpFallbackDict(t)
	for _, tc := range []struct {
		in, want string
		route    Route
	}{
		// 한글이 섞였으므로 분해를 탄다.
		{"고객번호이메일", "CUST_NO_EML", RouteSegmented},
		// 영문 조각이 섞여 있어도 한글이 있으면 분해한다.
		{"IP주문", "IP_ORDR", RouteSegmented},
		// 한글이 없으면 안 탄다.
		{"IPORDR", "IPORDR", RouteUnmatched},
	} {
		got := d.Convert(tc.in)
		if got.Physical != tc.want {
			t.Fatalf("Convert(%q).Physical = %q; want %q", tc.in, got.Physical, tc.want)
		}
		if got.Route != tc.route {
			t.Fatalf("Convert(%q).Route = %v; want %v", tc.in, got.Route, tc.route)
		}
	}
}

// TestConvert_RouteUnmatchedMeansNothingHappened는 [분해] 라벨의 뜻을
// 못 박는다.
//
// 라벨의 쓸모는 «이 이름에는 사전에서 찾은 답이 아니라 낱말을 이어 붙인
// 추측이 섞여 있다»는 경고다. 아무것도 못 맞혀 이름이 통째로 그대로인
// 줄에 [분해]를 찍으면 그 경고가 거짓이 된다 — 추측이 없는데 있다고
// 말하는 셈이다. 그런 줄은 [미변환]이다.
func TestConvert_RouteUnmatchedMeansNothingHappened(t *testing.T) {
	d := dpFallbackDict(t)
	// 분해가 답에 기여한 이름 -> [분해]
	for _, in := range []string{"고객번호이메일", "약어번호"} {
		if got := d.Convert(in); got.Route != RouteSegmented {
			t.Fatalf("Convert(%q).Route = %v; want RouteSegmented", in, got.Route)
		}
	}
	// 아무것도 못 맞힌 이름 -> [미변환], 그리고 Physical은 원문 그대로다.
	for _, in := range []string{"정보", "ZIP", "가나다"} {
		got := d.Convert(in)
		if got.Route != RouteUnmatched {
			t.Fatalf("Convert(%q).Route = %v; want RouteUnmatched", in, got.Route)
		}
		if got.Physical != in {
			t.Fatalf("Convert(%q).Physical = %q; RouteUnmatched면 원문 그대로여야 한다", in, got.Physical)
		}
	}
}

// TestConvert_ZipRoundTripIsStable은 이 결함의 «실제 피해 시나리오»를
// 끝에서 끝까지 돌린다.
//
//	우편번호 -> ZIP -> (다시 변환) -> Z_IP
//
// 사람이 물리명을 손으로 써 둔 파일을 다시 convert하면 멀쩡한 식별자가
// 조용히 망가졌다. 파일 단위 멱등성은 logicalName이 지키지만, 손으로 써
// 넣은 이름에는 logicalName이 없으므로 그 보호가 닿지 않는다.
func TestConvert_ZipRoundTripIsStable(t *testing.T) {
	d := writeTestDict(t,
		[][2]string{{"우편번호", "ZIP"}},
		[][2]string{{"우편", "POST"}, {"번호", "NO"}, {"IP", "IP"}},
	)
	dict, err := Load(d)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	once := dict.Convert("우편번호").Physical
	if once != "ZIP" {
		t.Fatalf("Convert(\"우편번호\") = %q; want ZIP", once)
	}
	twice := dict.Convert(once).Physical
	if twice != once {
		t.Fatalf("Convert(%q) = %q; 이미 물리명인 이름은 그대로여야 한다", once, twice)
	}
}

// TestConvert_WholeNameReachesWordDict는 FR1을 못 박는다.
//
// 조각을 푸는 순서는 «용어사전 -> 단어사전 -> 가드 -> 분해»다. 그런데 U4가
// 한글 가드를 넣으면서 «구분자 없는 한 낱말» 분기에는 단어사전 조회가 없어서,
// 가드가 조회보다 먼저 걸렸다. 그래서 단어사전에 «있는» 영문 표제어가
// 미매칭으로 보고됐다 — 실물 사전에서 GIS·HTML·IP·MAC·SMS·URL 여섯 개다.
//
//	column GIS -> GIS  [미변환]  미매칭: GIS      <- 사전 적중을 미스로 보고
//
// 실물 사전에서 그 여섯이 전부 항등 매핑(GIS -> GIS)이라 값 피해가 없었지만,
// 그것은 코드의 불변식이 아니라 그때 그 xlsx 파일의 성질일 뿐이다. 사전은
// 사용자가 주는 데이터다.
func TestConvert_WholeNameReachesWordDict(t *testing.T) {
	// 항등이 아닌 영문 표제어를 단어사전에 넣는다.
	p := writeTestDict(t,
		[][2]string{{"고객번호", "CUST_NO"}},
		[][2]string{{"EMAIL", "EML"}, {"고객", "CUST"}, {"번호", "NO"}, {"IP", "IP"}},
	)
	d, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// 혼자 있을 때와 구분자와 함께 있을 때가 «같은 답»이어야 한다. 이
	// 결함의 본질은 그 둘이 갈렸다는 것이다.
	got := d.Convert("EMAIL")
	if got.Physical != "EML" {
		t.Fatalf("Convert(\"EMAIL\").Physical = %q; want EML (단어사전에 있다)", got.Physical)
	}
	if got.HasUnmatched() {
		t.Fatalf("Convert(\"EMAIL\")를 미매칭으로 보고했다: %+v", got.Parts)
	}
	if got.Route != RouteWordWhole {
		t.Fatalf("Convert(\"EMAIL\").Route = %v; want RouteWordWhole", got.Route)
	}
	for _, tc := range []struct{ in, want string }{
		{"EMAIL_고객", "EML_CUST"},
		{"고객_EMAIL", "CUST_EML"},
		{"EMAIL 고객", "EML_CUST"},
		{"_EMAIL_", "_EML_"},
	} {
		if p := d.Convert(tc.in).Physical; p != tc.want {
			t.Fatalf("Convert(%q).Physical = %q; want %q", tc.in, p, tc.want)
		}
	}
	// 가드는 그대로 살아 있어야 한다 — 사전에 «없는» 영문은 안 쪼갠다.
	if got := d.Convert("ZIP"); got.Physical != "ZIP" || got.Route != RouteUnmatched {
		t.Fatalf("Convert(\"ZIP\") = %q/%v; want ZIP/RouteUnmatched", got.Physical, got.Route)
	}
}

// TestConvert_WholeNameRouteReflectsHowItWasResolved는 «구분자 없는 이름»의
// Route 네 갈래를 못 박는다. 라벨의 쓸모는 경고이므로 «사전에서 찾은 것»과
// «분해로 지어낸 것»이 갈려야 하고, 아무것도 못 맞힌 것은 추측이 없으므로
// 경고 대상이 아니다.
func TestConvert_WholeNameRouteReflectsHowItWasResolved(t *testing.T) {
	d := dpFallbackDict(t)
	for _, tc := range []struct {
		in, want string
		route    Route
	}{
		{"고객번호", "CUST_NO", RouteTermWhole},        // 용어사전 통째
		{"번호", "NO", RouteWordWhole},               // 단어사전 통째
		{"고객번호이메일", "CUST_NO_EML", RouteSegmented}, // 분해가 기여
		{"정보", "정보", RouteUnmatched},               // 아무것도 못 맞힘
		{"ZIP", "ZIP", RouteUnmatched},             // 가드에 걸려 분해도 안 탐
	} {
		got := d.Convert(tc.in)
		if got.Physical != tc.want {
			t.Fatalf("Convert(%q).Physical = %q; want %q", tc.in, got.Physical, tc.want)
		}
		if got.Route != tc.route {
			t.Fatalf("Convert(%q).Route = %v; want %v", tc.in, got.Route, tc.route)
		}
	}
}

// TestConvert_BlankAndSeparatorOnlyAreUnmatched는 FR2를 못 박는다.
//
// 빈 이름과 구분자뿐인 이름은 분해를 타지 않는다. 그런데도 [분해]로 찍혀서
// «추측이 섞였다»고 경고했다. `___`는 buildEdits가 빈 이름만 거르므로
// --dry-run에 실제로 나온다("column ___ -> ___").
func TestConvert_BlankAndSeparatorOnlyAreUnmatched(t *testing.T) {
	d := dpFallbackDict(t)
	for _, in := range []string{"", "   ", "___", "_", " _ "} {
		got := d.Convert(in)
		if got.Route != RouteUnmatched {
			t.Fatalf("Convert(%q).Route = %v; want RouteUnmatched (분해를 타지 않았다)", in, got.Route)
		}
		if got.HasUnmatched() {
			t.Fatalf("Convert(%q)에 미매칭 조각이 생겼다: %+v", in, got.Parts)
		}
	}
}

// TestConvert_SeparatorAllUnmatchedIsUnmatched는 소유자 결정 U5를 못 박는다.
//
// 구분자로 쪼갠 이름에서 조각을 «하나도» 못 찾았으면 [구분자분리]가 아니라
// [미변환]이다. RouteSeparatorSplit의 정의가 «조각별로 조회했고 적어도 하나는
// 찾았다»이므로, 하나도 못 찾았는데 그 라벨을 찍으면 정의가 거짓이 된다.
//
// 게다가 밑줄 유무만으로 라벨이 갈렸다 — 둘 다 한 글자도 변환되지 않았는데:
//
//	CUSTOMER  ->  CUSTOMER  [미변환]
//	CUST_NO   ->  CUST_NO   [구분자분리]   <- 같은 «아무것도 안 됨»인데 다른 라벨
//
// 구분자 하나 때문에 답이 갈리는 것은 이 파일이 반복해서 없애 온 부류다.
func TestConvert_SeparatorAllUnmatchedIsUnmatched(t *testing.T) {
	d := dpFallbackDict(t)
	for _, in := range []string{"CUST_NO", "HOME_ZIP", "ZIP_ZIP", "약어_XYZZY", "CUST NO", "_CUST_NO_"} {
		got := d.Convert(in)
		// 값은 한 글자도 안 바뀐다.
		want := in
		if in == "CUST NO" {
			want = "CUST_NO" // 안쪽 공백은 밑줄 하나로 합류(기존 규칙)
		}
		if got.Physical != want {
			t.Fatalf("Convert(%q).Physical = %q; want %q", in, got.Physical, want)
		}
		if got.Route != RouteUnmatched {
			t.Fatalf("Convert(%q).Route = %v; want RouteUnmatched (조각을 하나도 못 찾았다)", in, got.Route)
		}
		if !got.HasUnmatched() {
			t.Fatalf("Convert(%q)는 미매칭이어야 한다: %+v", in, got.Parts)
		}
	}
	// 구분자가 없는 같은 부류와 라벨이 같아야 한다 — 그게 이 결정의 요지다.
	if got := d.Convert("CUSTOMER"); got.Route != RouteUnmatched {
		t.Fatalf("Convert(\"CUSTOMER\").Route = %v; want RouteUnmatched", got.Route)
	}
}

// TestConvert_SeparatorPartialMatchKeepsSplitRoute는 U5의 «경계»를 못 박는다.
//
// 조각이 «일부만» 매칭된 이름은 여전히 [구분자분리]다. 라벨의 정의가
// «적어도 하나는 찾았다»이고, 실제로 찾았기 때문이다. 못 찾은 조각은
// 미매칭 열이 이름으로 지목하므로 정보가 사라지지 않는다.
//
// 여기서 [미변환]까지 쓰면 «절반은 변환됐다»는 사실이 라벨에서 사라진다.
func TestConvert_SeparatorPartialMatchKeepsSplitRoute(t *testing.T) {
	d := dpFallbackDict(t)
	for _, tc := range []struct {
		in, want string
		route    Route
	}{
		// 앞 조각만 매칭.
		{"고객_ZIP", "CUST_ZIP", RouteSeparatorSplit},
		// 뒤 조각만 매칭.
		{"ZIP_고객", "ZIP_CUST", RouteSeparatorSplit},
		// 조각 안에서 분해가 «기여»했으면 그쪽이 이긴다.
		{"주문__약어여부", "ORDR__약어_YN", RouteSeparatorSegmented},
		// 전부 매칭.
		{"고객__번호", "CUST__NO", RouteSeparatorSplit},
	} {
		got := d.Convert(tc.in)
		if got.Physical != tc.want {
			t.Fatalf("Convert(%q).Physical = %q; want %q", tc.in, got.Physical, tc.want)
		}
		if got.Route != tc.route {
			t.Fatalf("Convert(%q).Route = %v; want %v", tc.in, got.Route, tc.route)
		}
	}
}

// TestRouteValuesAreAppendOnly는 «새 Route는 끝에만 붙인다»는 관례를
// 기계가 지키게 한다.
//
// 주석만으로는 안 지켜졌다 — RouteWordWhole을 RouteUnmatched «앞에» 끼워
// 넣는 바람에 RouteUnmatched가 5에서 6으로 밀린 적이 있다. 지금은 직렬화하는
// 곳이 없어 무해했지만, 관례가 한 번 깨지면 다음 사람이 안 지킨다.
//
// 값을 바꿔야 할 진짜 이유가 생기면 이 표를 «의도적으로» 고치면 된다.
// 그때 이 테스트가 하는 일은 그 변경이 눈에 띄게 만드는 것뿐이다.
func TestRouteValuesAreAppendOnly(t *testing.T) {
	for _, tc := range []struct {
		route Route
		value int
		label string
	}{
		{RouteTermWhole, 0, "용어사전 통째"},
		{RouteTermJoined, 1, "용어사전(구분자제거)"},
		{RouteSegmented, 2, "분해"},
		{RouteSeparatorSplit, 3, "구분자분리"},
		{RouteSeparatorSegmented, 4, "구분자분리+분해"},
		{RouteUnmatched, 5, "미변환"},
		{RouteWordWhole, 6, "단어사전 통째"},
	} {
		if int(tc.route) != tc.value {
			t.Errorf("Route %q = %d; want %d (새 Route는 끝에만 붙인다)", tc.label, int(tc.route), tc.value)
		}
		// 실제로 부른 값(int(tc.route))을 찍는다. 기대 서수(tc.value)를
		// 찍으면 재정렬이 일어났을 때 «호출하지도 않은 Route»를 이름 대며
		// 실패한다 — 하필 그 재정렬을 막으려고 쓴 테스트다.
		if got := tc.route.String(); got != tc.label {
			t.Errorf("Route(%d).String() = %q; want %q", int(tc.route), got, tc.label)
		}
	}
	// 정의되지 않은 값도 문구를 낸다(호출자가 %s로 찍는다).
	if got := Route(99).String(); got != "알 수 없음" {
		t.Errorf("Route(99).String() = %q; want 알 수 없음", got)
	}
}
