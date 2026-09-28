package report

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestTableDocPDF_CreatesNonEmptyFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "table_doc.pdf")
	if err := TableDocPDF(sampleDoc(), out, Options{}); err != nil {
		t.Fatalf("TableDocPDF failed: %v", err)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("output file missing: %v", err)
	}
	if info.Size() < 1000 {
		t.Fatalf("expected a real PDF file (>1KB), got %d bytes", info.Size())
	}
	header := make([]byte, 5)
	f, _ := os.Open(out)
	defer f.Close()
	f.Read(header)
	if string(header) != "%PDF-" {
		t.Fatalf("expected valid PDF header, got %q", header)
	}
}

func TestTableDocPDF_AcceptsPageAsDomain(t *testing.T) {
	// PDF도 다른 포맷과 같은 page_as_domain 옵션을 받아야 한다. PDF는
	// 본문을 문자열로 검사할 수 없으므로, 두 모드가 서로 다른 내용을
	// 만들어낸다는 것(= 옵션이 실제로 렌더링에 도달한다는 것)으로 검증한다.
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.pdf")
	domain := filepath.Join(dir, "domain.pdf")
	if err := TableDocPDF(sampleDoc(), plain, Options{}); err != nil {
		t.Fatalf("TableDocPDF failed: %v", err)
	}
	if err := TableDocPDF(sampleDoc(), domain, Options{PageAsDomain: true}); err != nil {
		t.Fatalf("TableDocPDF failed: %v", err)
	}
	a, _ := os.ReadFile(plain)
	b, _ := os.ReadFile(domain)
	if bytes.Equal(a, b) {
		t.Fatalf("pageAsDomain made no difference to the rendered PDF")
	}
}
