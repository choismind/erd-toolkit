// internal/drawio/compress.go
package drawio

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"fmt"
	"strings"
)

// Compress는 Decompress의 짝이다. draw.io가 diagram 요소에 저장하는
// «encodeURIComponent -> raw deflate -> base64» 순서로 감싼다.
//
// draw.io는 읽을 때 decodeURIComponent(pako.inflateRaw(atob(text)))를 하므로,
// URL 인코딩 단계를 생략하면 payload에 들어 있는 % 문자에서 draw.io 쪽
// decodeURIComponent가 터진다. 우리 Decompress는 인코딩 안 된 payload도
// 받아주지만(폴백), 우리가 «쓸» 때는 draw.io 규칙을 정확히 지켜야 한다.
func Compress(plain string) (string, error) {
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		return "", fmt.Errorf("new deflate writer: %w", err)
	}
	if _, err := w.Write([]byte(encodeURIComponent(plain))); err != nil {
		return "", fmt.Errorf("deflate write: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("deflate close: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// encodeURIComponent는 JavaScript의 encodeURIComponent와 «같은 집합»을
// 남긴다: A-Z a-z 0-9 - _ . ! ~ * ' ( )
//
// Go 표준 라이브러리에는 대응물이 없다. url.QueryEscape는 스페이스를 +로
// 바꾸고(그 규칙은 form-urlencoded 쿼리스트링에서만 유효하다),
// url.PathEscape는 encodeURIComponent가 이스케이프하는 문자 일부를 그대로
// 둔다. 어느 쪽도 draw.io가 기대하는 형식이 아니다.
func encodeURIComponent(s string) string {
	const unreserved = "-_.!~*'()"
	var b strings.Builder
	b.Grow(len(s))
	for _, c := range []byte(s) {
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteByte(c)
		case strings.IndexByte(unreserved, c) >= 0:
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte("0123456789ABCDEF"[c>>4])
			b.WriteByte("0123456789ABCDEF"[c&0x0F])
		}
	}
	return b.String()
}
