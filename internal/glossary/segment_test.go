package glossary

import (
	"strings"
	"testing"
)

func joinSeg(parts []segmentPart) string {
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = p.Text
		if !p.Matched {
			out[i] += "!"
		}
	}
	return strings.Join(out, "+")
}

func segTestDict(t *testing.T) *Dict {
	t.Helper()
	p := writeTestDict(t,
		[][2]string{{"주문일자", "ORDR_YMD"}},
		[][2]string{
			{"주문", "ORDR"}, {"고객", "CUST"}, {"번호", "NO"},
			{"일자", "YMD"}, {"명", "NM"}, {"선적", "SHPMNT"},
		},
	)
	d, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return d
}

func TestSegment_SplitsCompound(t *testing.T) {
	d := segTestDict(t)
	if got := joinSeg(d.segment("주문고객번호")); got != "주문+고객+번호" {
		t.Fatalf("got %q", got)
	}
}

func TestSegment_PrefersFewerPieces(t *testing.T) {
	// 이름과 달리 이 테스트가 실질적으로 검증하는 건 2순위(조각 수 최소화)가
	// 아니라 1순위다: "선"과 "적"은 둘 다 사전에 없으므로 선+적으로 쪼개면
	// 미매칭 2글자가 생긴다. 선적(1조각, 미매칭 0)이 이겨야 하는 건 1순위만
	// 으로도 결정되고, 2순위는 이 테스트를 통과시키는 데 필요하지 않다.
	// 2순위를 미매칭 0개인 후보끼리 격리해서 검사하는 건 아래
	// TestSegment_PrefersFewerPiecesWhenFullyMatched다.
	d := segTestDict(t)
	if got := joinSeg(d.segment("선적번호")); got != "선적+번호" {
		t.Fatalf("got %q", got)
	}
}

func TestSegment_PrefersFewerPiecesWhenFullyMatched(t *testing.T) {
	// 2순위(조각 수 최소화 = 최장일치)만 격리해서 검사한다. 두 분해 후보가
	// 둘 다 미매칭 0개여야 1순위로는 안 갈리고 2순위만 남는다.
	//
	// 사전: 가나다(3글자), 라마(2글자), 가(1글자), 나(1글자), 다라마(3글자).
	// 입력 "가나다라마"를 완전매칭(미매칭 0)으로 덮는 조합은 둘뿐이다.
	//   후보1(2조각): 가나다 + 라마
	//   후보2(3조각): 가 + 나 + 다라마
	// 이 사전은 일부러 비대칭으로 짰다: 후보2의 마지막 조각(다라마)은
	// 위치 2에서 시작해 DP가 위치 5에 훨씬 일찍 도달하고, 후보1의 마지막
	// 조각(라마)은 위치 3에서 시작해 더 늦게 도달한다. 그래서 «조각 수를
	// 세지 않고 완전매칭이면 먼저 도착한 쪽이 이긴다»는 식으로 비용을
	// 잘못 바꾸면(예: 조각당 비용을 없애면) 늦게 도착하는 후보1이 이미
	// 확정된 후보2를 뒤집지 못해 오답(후보2)이 나온다. 조각 수를 실제로
	// 세는 비용식(cost+1)이 있어야만 더 큰 비용(3)인 후보2를 더 작은
	// 비용(2)인 후보1이 뒤집는다.
	p := writeTestDict(t, nil, [][2]string{
		{"가나다", "GND"}, {"라마", "RM"}, {"가", "G"}, {"나", "N"}, {"다라마", "DRM"},
	})
	d, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := joinSeg(d.segment("가나다라마")); got != "가나다+라마" {
		t.Fatalf("got %q; want 가나다+라마 (2조각) — 2순위(조각 수 최소화)가 깨졌다", got)
	}
}

func TestSegment_MarksUnmatchedRunes(t *testing.T) {
	// 약, 어는 사전에 없다. 붙어 있는 미매칭 글자는 한 조각으로 합쳐지고
	// Matched=false다 — 없던 경계를 만들지 않는다.
	d := segTestDict(t)
	if got := joinSeg(d.segment("약어명")); got != "약어!+명" {
		t.Fatalf("got %q", got)
	}
}

func TestSegment_MinimizesUnmatchedFirst(t *testing.T) {
	// 미매칭 글자를 줄이는 쪽이 조각 수를 줄이는 쪽보다 먼저다.
	d := segTestDict(t)
	if got := joinSeg(d.segment("고객X번호")); got != "고객+X!+번호" {
		t.Fatalf("got %q", got)
	}
}

func TestSegment_Empty(t *testing.T) {
	d := segTestDict(t)
	if got := d.segment(""); len(got) != 0 {
		t.Fatalf("빈 문자열은 빈 조각 목록이어야 한다: %v", got)
	}
}
