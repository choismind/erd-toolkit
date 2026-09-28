package dialect

import "testing"

func TestParse_표기흔들림을받는다(t *testing.T) {
	for _, in := range []string{"postgres", "PostgreSQL", " pg ", "POSTGRESQL"} {
		got, err := Parse(in)
		if err != nil || got != PostgreSQL {
			t.Errorf("Parse(%q) = %q, %v", in, got, err)
		}
	}
	if got, err := Parse(""); err != nil || got != ANSI {
		t.Errorf("빈 값은 ANSI다: %q, %v", got, err)
	}
}

func TestParse_모르는값은거부한다(t *testing.T) {
	// 조용히 ANSI로 떨어뜨리면 오타가 기본값으로 흘러가고, 사용자는 자기가
	// 지정한 대로 뽑혔다고 믿는다.
	_, err := Parse("postgre")
	if err == nil {
		t.Fatal("오타를 통과시켰다")
	}
	// 무엇을 칠 수 있는지 문구가 함께 말해야 한다.
	for _, want := range []string{"postgres", "mysql", "sqlite", "ansi"} {
		if !contains(err.Error(), want) {
			t.Errorf("에러 문구에 %q가 없다: %v", want, err)
		}
	}
}

func TestQuoteIdent_타깃DBMS마다감싸는문자가다르다(t *testing.T) {
	cases := map[Dialect]string{
		ANSI:       `"주문 정보"`,
		PostgreSQL: `"주문 정보"`,
		SQLite:     `"주문 정보"`,
		MySQL:      "`주문 정보`",
	}
	for d, want := range cases {
		if got := d.QuoteIdent("주문 정보"); got != want {
			t.Errorf("%s: got %s want %s", d, got, want)
		}
	}
}

func TestQuoteIdent_감싸는문자가이름안에있으면두번반복한다(t *testing.T) {
	if got := ANSI.QuoteIdent(`a"b`); got != `"a""b"` {
		t.Errorf("got %s", got)
	}
	if got := MySQL.QuoteIdent("a`b"); got != "`a``b`" {
		t.Errorf("got %s", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestDialect_제로값은ANSI다(t *testing.T) {
	// 설정 파일의 빈 dialect와 report.Options{}의 기본값 둘 다 여기로 온다.
	// «지정 안 함»이지 «타깃 DBMS가 없음»이 아니다.
	var zero Dialect
	if zero.Label() != ANSI.Label() {
		t.Errorf("Label: %q", zero.Label())
	}
	if zero.QuoteIdent("a") != ANSI.QuoteIdent("a") {
		t.Errorf("QuoteIdent: %q", zero.QuoteIdent("a"))
	}
}
