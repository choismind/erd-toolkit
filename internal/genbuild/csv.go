package genbuild

import (
	"bytes"
	"encoding/csv"
	"fmt"
)

// utf8BOM은 엑셀의 "CSV UTF-8(쉼표로 분리)" 내보내기가 파일 맨 앞에 붙이는
// 바이트 순서 표시(BOM)다. 그대로 두면 첫 헤더 셀의 첫 글자 앞에 이 세
// 바이트가 그대로 붙어 checkHeader가 "테이블명"과 다르다며 거부한다 —
// 사용자가 헤더를 잘못 쓴 것처럼 보이지만 실은 엑셀이 그렇게 저장한
// 것뿐이다(리뷰에서 발견). 파싱 전에 조용히 벗겨낸다.
var utf8BOM = []byte("\xef\xbb\xbf")

// ParseCSV는 csv 파일 하나를 페이지 하나로 취급한다(원장: "CSV는 탭
// 개념이 없으므로 파일 하나 = 페이지 하나"). 테이블 블록(7열)과 관계
// 블록(10열)의 열 개수가 서로 다르므로 FieldsPerRecord 검사를 끈다.
func ParseCSV(pageName string, src []byte) ([]PageDef, error) {
	src = bytes.TrimPrefix(src, utf8BOM)
	r := csv.NewReader(bytes.NewReader(src))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv 읽기 실패: %w", err)
	}
	page, err := ParseSheet(pageName, rows)
	if err != nil {
		return nil, err
	}
	return []PageDef{page}, nil
}
