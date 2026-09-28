package dbreverse

import "strings"

// MaskDSN은 DSN의 비밀번호 자리를 ***로 바꾼다. 접속에 실패한 순간이
// 비밀번호가 스크롤백에 박히는 순간이 되면 안 된다.
//
// net/url.Parse를 쓰지 않는다 — 비밀번호에 흔한 문자(@, /)가 들어가면
// 파싱이 실패하는데, 실패했을 때 «원문을 그대로 돌려주는» 것이 가장
// 위험한 동작이기 때문이다. 문자열만 보고 자른다.
func MaskDSN(dsn string) string {
	scheme := ""
	rest := dsn
	if i := strings.Index(dsn, "://"); i >= 0 {
		scheme = dsn[:i+3]
		rest = dsn[i+3:]
	}
	// 사용자정보는 «마지막» @ 앞까지다. 비밀번호에 @가 들어갈 수 있으므로
	// 첫 @가 아니라 마지막 @를 기준으로 삼는다.
	at := strings.LastIndex(rest, "@")
	if at < 0 {
		return dsn
	}
	userinfo := rest[:at]
	colon := strings.Index(userinfo, ":")
	if colon < 0 {
		return dsn // 비밀번호가 없다
	}
	return scheme + userinfo[:colon] + ":***" + rest[at:]
}
