// internal/drawio/decompress.go
package drawio

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
)

// maxDecompressedBytes는 diagram 하나를 해제할 때 받아들이는 상한이다.
// 실제 draw.io ERD는 해제 후 수백 KB 수준이고(이 저장소의 픽스처는 전부
// 100KB 미만), 64MB면 비정상적으로 큰 다이어그램도 넉넉히 통과한다.
//
// 상한이 필요한 이유: deflate는 반복 데이터를 1000:1 가까이 줄인다. 상한
// 없이 io.ReadAll을 하면 남이 보내준 66KB짜리 .drawio 하나가 341MB를
// 먹는 것을 실제 바이너리로 확인했다 — 파일을 조금만 키우면 프로세스가
// 메모리째 넘어간다. 이 도구의 입력은 «남에게서 받은 ERD»가 정상이므로
// 신뢰할 수 없는 입력으로 다뤄야 한다.
const maxDecompressedBytes = 64 << 20

// Decompress는 draw.io가 diagram 요소에 저장하는 base64 + raw-deflate +
// URL-encode 압축을 해제한다 (도형구성정보.md에서 실측 확인된 알고리즘).
func Decompress(encoded string) (string, error) {
	return decompress(encoded, maxDecompressedBytes)
}

// decompress는 상한을 인자로 받는다 — 테스트가 64MB짜리 payload를 실제로
// 만들지 않고도 상한 동작을 검사할 수 있게 하기 위해서다.
func decompress(encoded string, limit int64) (string, error) {
	if encoded == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	r := flate.NewReader(bytes.NewReader(raw))
	defer r.Close()
	// limit+1까지만 읽어서, 넘겼는지 판단하면서도 상한 이상은 절대 담지
	// 않는다. 상한이 없으면 66KB짜리 파일 하나가 수백 MB를 먹는다(실측).
	out, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return "", fmt.Errorf("raw deflate decompress: %w", err)
	}
	if int64(len(out)) > limit {
		return "", fmt.Errorf("decompressed diagram exceeds the %d byte limit (decompression bomb or corrupt file)", limit)
	}
	// QueryUnescape가 아니라 PathUnescape를 쓴다(M8): 둘의 유일한 차이는
	// QueryUnescape가 "+"를 스페이스로 바꾼다는 점인데, 그 규칙은
	// application/x-www-form-urlencoded 쿼리스트링에서만 유효하다. draw.io는
	// encodeURIComponent를 쓰므로 리터럴 "+"는 %2B로 들어오고, URL 인코딩
	// 단계를 아예 생략한 페이로드(도형구성정보.md: 선택적 단계)에는 리터럴
	// "+"가 그대로 들어있다 — QueryUnescape면 후자가 에러 없이 조용히
	// 스페이스로 손상된다(에러가 없으니 아래 폴백도 타지 않는다).
	decoded, err := url.PathUnescape(string(out))
	if err != nil {
		// URL 인코딩이 안 된 경우도 있으므로(도형구성정보.md: 선택적 단계),
		// 실패하면 원본 해제 결과를 그대로 반환한다.
		return string(out), nil
	}
	return decoded, nil
}
