package drawio

import (
	"os"
	"strings"
	"testing"
)

func TestCompress_RoundTrip(t *testing.T) {
	cases := []string{
		`<mxGraphModel><root><mxCell id="0"/></root></mxGraphModel>`,
		`<mxCell value="고객번호 int NOT NULL"/>`,
		`<mxCell value="a+b 100% &amp; &lt;x&gt;"/>`,
		"줄바꿈\n탭\t끝",
	}
	for _, in := range cases {
		enc, err := Compress(in)
		if err != nil {
			t.Fatalf("Compress(%q): %v", in, err)
		}
		got, err := Decompress(enc)
		if err != nil {
			t.Fatalf("Decompress after Compress(%q): %v", in, err)
		}
		if got != in {
			t.Fatalf("round trip: got %q; want %q", got, in)
		}
	}
}

// TestCompress_RealFixtureRoundTrip은 draw.io가 실제로 만든 압축 payload를
// 풀었다가 우리 Compress로 다시 감싸고 또 풀어서, 같은 내용이 나오는지 본다.
//
// testdata의 .drawio 10개 중 compressed="true"인 것은 relationship_logical.drawio
// 하나뿐이다(실측). relationship_physical.drawio는 compressed="false"라서
// 그 파일을 읽으면 이 테스트의 목적(실제 draw.io 압축 payload 검증)이
// 항상 SKIP으로 사라진다.
func TestCompress_RealFixtureRoundTrip(t *testing.T) {
	data, err := os.ReadFile("testdata/relationship_logical.drawio")
	if err != nil {
		t.Skipf("압축 픽스처가 없어 건너뛴다: %v", err)
	}
	payload := extractDiagramText(t, string(data))
	if payload == "" {
		t.Skip("이 픽스처는 압축되어 있지 않다")
	}

	plain, err := Decompress(payload)
	if err != nil {
		t.Fatalf("원본 해제: %v", err)
	}
	again, err := Compress(plain)
	if err != nil {
		t.Fatalf("재압축: %v", err)
	}
	back, err := Decompress(again)
	if err != nil {
		t.Fatalf("재해제: %v", err)
	}
	if back != plain {
		t.Fatal("재압축 후 해제한 내용이 원본과 다르다")
	}
}

// extractDiagramText는 <diagram ...>텍스트</diagram>의 텍스트를 꺼낸다.
// 자식 요소가 있으면(비압축) 빈 문자열을 돌려준다.
func extractDiagramText(t *testing.T, s string) string {
	t.Helper()
	i := strings.Index(s, "<diagram")
	if i < 0 {
		return ""
	}
	j := strings.Index(s[i:], ">")
	if j < 0 {
		return ""
	}
	start := i + j + 1
	k := strings.Index(s[start:], "</diagram>")
	if k < 0 {
		return ""
	}
	body := strings.TrimSpace(s[start : start+k])
	if strings.HasPrefix(body, "<") {
		return ""
	}
	return body
}
