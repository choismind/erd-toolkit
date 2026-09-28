package dbreverse

import "testing"

func TestMaskDSN(t *testing.T) {
	cases := []struct{ in, want string }{
		{"postgres://chois:s3cret@localhost/mydb", "postgres://chois:***@localhost/mydb"},
		{"postgres://chois@localhost/mydb", "postgres://chois@localhost/mydb"},
		{"mysql://chois:p@ss@localhost:3306/shop", "mysql://chois:***@localhost:3306/shop"},
		{"./app.db", "./app.db"},
		{"", ""},
	}
	for _, c := range cases {
		if got := MaskDSN(c.in); got != c.want {
			t.Errorf("MaskDSN(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

// 마스킹이 실패해도 원문을 흘리면 안 된다.
func TestMaskDSN_NeverLeaksOnMalformedInput(t *testing.T) {
	got := MaskDSN("postgres://user:pw@@@broken")
	if got == "postgres://user:pw@@@broken" {
		t.Errorf("망가진 DSN에서도 비밀번호가 그대로 나왔다: %q", got)
	}
}
