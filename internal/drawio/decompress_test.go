// internal/drawio/decompress_test.go
package drawio

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"strings"
	"testing"
)

// 아래 상수는 testdata/relationship_logical.drawio(= 원본
// E:\devol\python\pyenv11\drawio\data\relationship_zipped.drawio) 안의 첫
// <diagram name="주문정보" ...> 요소의 텍스트를 실제로 복사한 것이다(직접
// 압축 해제해 mxGraphModel/shape=table이 들어있음을 검증 완료). 이 파일을
// 다시 열어 대조해도 좋지만, 아래 값 자체가 이미 실제 파일 내용이다.
const relationshipLogicalFirstDiagramRaw = `7Vttd6I4FP41OWf2Q3sgSIsfhep0z9hdt+3u7HxMJWJ2kHBCWrW/fhNIRF5sccbaKXKOx8LleiH3PrlP8miB5S1WnxmK5zfUxyGAhr8C1hWA0ISGI/5IyzqzOKaRGQJGfOWUG+7IM1ZG7fZIfJwUHDmlISdx0TilUYSnvGBDjNFl0W1Gw+JdYxTgiuFuisKq9Svx+VyNwjZy+zUmwVzf2TTUlQXSzsqQzJFPlwUTXvERjbh6xAlmCxThiIsrN4h9xwzYwznncqQDAEfiNZPe5wGlQYhRTJLzKV0I8zQRLqMZWpBQpnkrkKsCidtZQ2B5jFKeHS1WHg5lrXQZsmca7bi6yQOTcRt8wDt7Wo+/Gzy6Xlr/8JubYHL5cKaiPKHwUeUXDAfAtcDASA8M0L8CQw8MLOB6YOiCgQdchaCEr3VRsC9qpE7F4xC+vsUh4oRGw/yKiyN/IBEgnIa3z5jRe3qDIpEgN+GI8fwajZT7iIghWFemdlHnhjivDl+NJaGPbKqe69/JZIad9be7vyz2X/x1MLb838+gQi1iAeYv+JlqnHJwW3dQ2f2M6QJzthYOyxyHG7TNtzGojSxNylMRyUihLdgE3NxjQkkKPjV5rZ6Ko6bupYatDpENXX1qGwqlQL1SoItyoCw3lUDiYGvguSlF2j6ogzWwq4BMWJxeCkRPvju6vh8UdqbdEHfwV8NdzyjCxXR+FHcXxUDQsBvhLg+kHelsluCfxWZ9mZp0xDIQBZXE8pCjhxRyKWoUd1oSNoINOSKRaPwZrKY0DFGckNQ9s8xJ6I/Rmj5yHUifuTOywv5tRp3SVwB2LIIlCpOShPQ0kJdRSIJIHE8FRuUdXYYT8SxjlHDlsRPFT5hxvHoRdepqv4QJ3UK2QAntGlD2S9jZhl+hlvsWrqanvFimWznr3Tll5FkWR0/w7dKl50uyCAV5X2Pkl0wuTRdRaQlEh/BoSGV9o6yPFEssnXxG43s9+6UhlqhO82C74iUy4xnnNrDFs3ri3MzPxUu6M+7RKOEMkbRuWFR0iWVVXU5jFTTEMx2fqbTL4wfKuVib7Kr/i3PhdVAoFFgNQWC9FQisCggmX3bCQIyfExTeimUqioIwK1q6akV50WoqW5vrTX7LiS/PTirSOQtTypkT38dR83rAxvXYKoC1Z/5VsDwre0dDoWg7EeJigjxGfvIWXbrXpEuLlYQLgSNWEleg76TrViOjkT/+vJfvf4/H7wwO3aozXzeJ0ZREwTj75EUJPfax0LMCO2czPCiaGoU7ApzsjjsacUf9Cvgw3HHx3txx0RwEx2kO2Cc63k8Sh924GG0njsuGxDEYAMeWMtYcsU+28duHIo2jIOXkSMKsW3S8Ko2d4NawLC/1G3Z2syxDHay1mx3BH35z2GuMi19ld2hWKb5N20Ozo/lNKup4vqowdxvEH8TP6ZG/0zHIwbeIezPIu+8RzX4FBqMv1S+mPuo+cYPyjkF033mFQVopOh4HQSfHIXB3o+go460oo+nm880oo0Uy4h5SQOvpofpNY/1PWAQlSIs4GAKJRcMXz3UqbLC/dnRCbFAvJ77yk6dOToTQadjS305PhJ2eeHA9EX48PRG2W0+EnZ64SUW9nlhq1m3Y+x3yByednrg7N52eePDN4f4M8u56Imy3ngg7PXHzy9RmemILGOQ4eDk5xrA69fD4BPHu6qHVHvXQ6tTDTSrq1cPyduLE1cM9ANMiNhCn+T8FZ+75f3Jbw/8B`

func TestDecompress_RealDiagram(t *testing.T) {
	xml, err := Decompress(relationshipLogicalFirstDiagramRaw)
	if err != nil {
		t.Fatalf("decompress failed: %v", err)
	}
	if !strings.Contains(xml, "mxGraphModel") {
		t.Fatalf("decompressed xml missing mxGraphModel: %s", xml[:100])
	}
	if !strings.Contains(xml, "shape=table") {
		t.Fatalf("decompressed xml missing table shape")
	}
}

// deflateBase64는 draw.io의 압축 형식(raw deflate -> base64)으로 문자열을
// 인코딩한다. URL 인코딩 단계는 선택적이므로 여기서는 적용하지 않는다.
func deflateBase64(t *testing.T, s string) string {
	t.Helper()
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		t.Fatalf("flate.NewWriter: %v", err)
	}
	if _, err := w.Write([]byte(s)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestDecompress_PreservesLiteralPlus(t *testing.T) {
	// url.QueryUnescape는 "+"를 스페이스로 바꾼다. draw.io가 URL 인코딩
	// 단계를 생략한 페이로드(도형구성정보.md: 선택적 단계)에 리터럴 "+"가
	// 들어 있으면 값이 조용히 손상된다 — 에러가 안 나므로 기존 폴백 분기도
	// 타지 않는다. 스페이스 치환은 쿼리스트링에서만 유효한 규칙이라
	// 여기서는 적용하면 안 된다.
	const want = `<mxCell value="A+B 구분" style="shape=table;"/>`
	got, err := Decompress(deflateBase64(t, want))
	if err != nil {
		t.Fatalf("decompress failed: %v", err)
	}
	if got != want {
		t.Fatalf("literal '+' was corrupted:\n got: %s\nwant: %s", got, want)
	}
}

func TestDecompress_StillDecodesPercentEscapes(t *testing.T) {
	// draw.io의 일반 경로(encodeURIComponent 적용)는 그대로 동작해야 한다.
	got, err := Decompress(deflateBase64(t, `%3CmxCell%20value%3D%22A%2BB%22%2F%3E`))
	if err != nil {
		t.Fatalf("decompress failed: %v", err)
	}
	const want = `<mxCell value="A+B"/>`
	if got != want {
		t.Fatalf("percent escapes not decoded:\n got: %s\nwant: %s", got, want)
	}
}

func TestDecompress_RejectsBombPayload(t *testing.T) {
	// 압축 폭탄: 66KB짜리 .drawio 하나가 341MB를 먹는 것을 실제 바이너리로
	// 확인했다(deflate는 반복 데이터를 1000:1 가까이 줄인다). 상한이 없으면
	// 남이 보내준 ERD 파일 하나로 프로세스를 메모리째 넘어뜨릴 수 있다.
	// 상한은 인자로 받아 테스트에서 작게 줄인다 — 64MB를 실제로 만들어
	// 돌리면 테스트가 느려지고 CI 메모리를 그만큼 먹는다.
	payload := strings.Repeat("A", 4096)
	encoded := compressForTest(t, payload)

	if _, err := decompress(encoded, 1024); err == nil {
		t.Fatal("상한을 넘긴 payload는 에러여야 한다")
	}
	got, err := decompress(encoded, 8192)
	if err != nil {
		t.Fatalf("상한 안쪽은 그대로 해제돼야 한다: %v", err)
	}
	if got != payload {
		t.Fatalf("해제 결과가 원본과 다르다: %d bytes", len(got))
	}
}

func compressForTest(t *testing.T, s string) string {
	t.Helper()
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		t.Fatalf("flate writer: %v", err)
	}
	if _, err := w.Write([]byte(s)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}
