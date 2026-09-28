package dbreverse

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// 한국어 로케일 PostgreSQL이 실제로 보낸 바이트다 — 2026-08-31에 이 기계의
// 서버에 잘못된 계정으로 붙어 `od -c`로 뜬 원문 그대로다.
// 손으로 옮겨 적지 말 것: 처음 적었을 때 "했"을 "하였"으로 틀렸다.
const pgKoreanAuthFailureHex = "" +
	"c4a1b8edc0fbbfc0b7f9" + // 치명적오류
	"3a20" + // ": "
	"bbe7bfebc0da" + // 사용자
	"20226e6f7375636875736572 22" + // ` "nosuchuser"`
	"c0c7" + // 의
	"2070617373776f726420" + // " password "
	"c0cec1f5c0bb" + // 인증을
	"20" +
	"bdc7c6d0c7dfbdc0b4cfb4d9" // 실패했습니다

const pgKoreanAuthFailureUTF8 = `치명적오류: 사용자 "nosuchuser"의 password 인증을 실패했습니다`

func pgKoreanAuthFailure(t *testing.T) string {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(pgKoreanAuthFailureHex, " ", ""))
	if err != nil {
		t.Fatalf("픽스처 hex가 깨졌다: %v", err)
	}
	return string(b)
}

func TestDecodeServerText_DecodesCP949(t *testing.T) {
	got := DecodeServerText(pgKoreanAuthFailure(t))
	if got != pgKoreanAuthFailureUTF8 {
		t.Errorf("DecodeServerText = %q; want %q", got, pgKoreanAuthFailureUTF8)
	}
}

// 이미 UTF-8인 문구는 한 바이트도 건드리면 안 된다.
func TestDecodeServerText_LeavesUTF8Untouched(t *testing.T) {
	for _, s := range []string{
		"",
		`connection refused`,
		`접속 실패 (postgres://chois:***@localhost/mydb)`,
		"�", // 치환 문자 자체도 유효한 UTF-8이다
	} {
		if got := DecodeServerText(s); got != s {
			t.Errorf("DecodeServerText(%q) = %q; 원문 그대로여야 한다", s, got)
		}
	}
}

// CP949도 아닌 바이트열이면 원문을 그대로 둔다 — 멋대로 지어내지 않는다.
func TestDecodeServerText_KeepsUndecodableBytes(t *testing.T) {
	s := string([]byte{0xFF, 0xFE, 0x00, 0x41})
	if got := DecodeServerText(s); got != s {
		t.Errorf("DecodeServerText = %q; 디코딩 못 하면 원문이어야 한다", got)
	}
}

// 에러를 감싸도 원인 사슬은 살아 있어야 한다 — errors.Is/As가 계속 통해야 한다.
func TestDecodeError_PreservesCause(t *testing.T) {
	cause := errors.New(pgKoreanAuthFailure(t))
	wrapped := DecodeError(cause)

	if wrapped.Error() != pgKoreanAuthFailureUTF8 {
		t.Errorf("Error() = %q; want %q", wrapped.Error(), pgKoreanAuthFailureUTF8)
	}
	if !errors.Is(wrapped, cause) {
		t.Error("errors.Is가 원인을 못 찾는다 — Unwrap이 없다")
	}
}

func TestDecodeError_NilStaysNil(t *testing.T) {
	if err := DecodeError(nil); err != nil {
		t.Errorf("DecodeError(nil) = %v; want nil", err)
	}
}

// 이미 UTF-8인 에러는 감싸지 않고 그대로 돌려준다.
func TestDecodeError_ReturnsSameErrorWhenAlreadyUTF8(t *testing.T) {
	cause := errors.New("connection refused")
	if got := DecodeError(cause); got != cause {
		t.Errorf("DecodeError = %v (%T); 원본 에러 그대로여야 한다", got, got)
	}
}
