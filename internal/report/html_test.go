// internal/report/html_test.go
package report

import (
	"strings"
	"testing"
)

func TestTableDocHTML(t *testing.T) {
	html := TableDocHTML(sampleDoc(), Options{})
	for _, want := range []string{"<table", "Orders", "order_id", "</table>"} {
		if !strings.Contains(html, want) {
			t.Fatalf("expected html to contain %q, got:\n%s", want, html)
		}
	}
}

func TestTableDocHTML_ShowsPageName(t *testing.T) {
	// page_as_domain이 TableDocMarkdown에만 적용되고 HTML/PDF/xlsx는
	// 페이지명을 아예 안 보여줬다 — 같은 실행에서 나온 네 산출물이 서로
	// 다른 내용을 담고 있었다.
	plain := TableDocHTML(sampleDoc(), Options{})
	if !strings.Contains(plain, "페이지-1") {
		t.Errorf("expected page name in HTML, got:\n%s", plain)
	}
	if strings.Contains(plain, "도메인") {
		t.Errorf("expected no domain label when pageAsDomain=false, got:\n%s", plain)
	}

	domain := TableDocHTML(sampleDoc(), Options{PageAsDomain: true})
	if !strings.Contains(domain, "도메인") || !strings.Contains(domain, "페이지-1") {
		t.Errorf("expected domain label with page name when pageAsDomain=true, got:\n%s", domain)
	}
}
