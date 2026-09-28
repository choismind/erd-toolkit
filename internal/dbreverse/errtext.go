package dbreverse

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/korean"
)

// DecodeServerText는 DB 서버가 보낸 문구가 UTF-8이 아니면 CP949로 읽는다.
//
// 한국어 로케일 PostgreSQL은 인증 실패 같은 메시지를 서버 로케일 인코딩
// 그대로 보낸다(2026-08-31 실측). 접속에 실패한 순간이 사용자에게 가장
// 중요한 순간인데 그 문구가 스크롤백에 깨져 나오면 안 된다.
//
// 규칙은 하나다 — **유효한 UTF-8은 절대 건드리지 않는다.** 그래야 한글이
// 제대로 오는 서버나 우리가 직접 쓴 한국어 문구가 이 경로에 걸려도
// 무사하다. 추측은 UTF-8이 아닐 때만 한다.
func DecodeServerText(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	dec, err := korean.EUCKR.NewDecoder().String(s)
	// CP949 디코더는 못 읽는 바이트에 에러를 내지 않고 U+FFFD를 채워 넣는다.
	// 그래서 에러 대신 U+FFFD의 유무로 «추측이 틀렸다»를 판정한다 —
	// CP949에는 U+FFFD로 가는 자리가 없으므로 제대로 읽힌 문구에는 없다.
	if err != nil || strings.ContainsRune(dec, utf8.RuneError) {
		return s // CP949도 아니다. 지어내지 말고 원문을 그대로 둔다
	}
	return dec
}

// decodedError는 문구만 CP949로 고쳐 읽고 원인 사슬은 그대로 잇는다.
// errors.Is/As가 계속 통해야 호출자가 드라이버 에러를 분간할 수 있다.
type decodedError struct {
	cause error
	text  string
}

func (e *decodedError) Error() string { return e.text }
func (e *decodedError) Unwrap() error { return e.cause }

// DecodeError는 에러 문구에 DecodeServerText를 적용한다. 고칠 것이 없으면
// 원본 에러를 그대로 돌려준다 — 감쌀 이유가 없는데 감싸지 않는다.
func DecodeError(err error) error {
	if err == nil {
		return nil
	}
	text := DecodeServerText(err.Error())
	if text == err.Error() {
		return err
	}
	return &decodedError{cause: err, text: text}
}
