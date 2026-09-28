package genbuild

import "testing"

const validCSV = `테이블명,순번,컬럼명,컬럼유형,색인여부,널허용,단일값
고객,1,고객번호,int,PK,False,False
고객,2,고객명,char(50),,False,False
주문,1,주문번호,int,PK,False,False
주문,2,주문고객번호,int,FK1,False,False
## 관계
순번,원천테이블명,원천컬럼명,원천카디널리티,원천색인,연관명,목표카디널리티,목표색인,목표테이블명,목표컬럼명
1,고객,고객번호,ERone,PK,고객주문,ERzeroToMany,FK1,주문,주문고객번호
`

func TestParseCSV_SinglePage(t *testing.T) {
	pages, err := ParseCSV("주문정보", []byte(validCSV))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pages) != 1 || pages[0].Name != "주문정보" {
		t.Fatalf("got %+v", pages)
	}
	if len(pages[0].Tables) != 2 || len(pages[0].Relations) != 1 {
		t.Fatalf("got %d tables, %d relations", len(pages[0].Tables), len(pages[0].Relations))
	}
}

// 엑셀의 "CSV UTF-8(쉼표로 분리)" 내보내기는 파일 맨 앞에 BOM(EF BB BF)을
// 붙인다. 벗겨내지 않으면 첫 헤더 셀이 "테이블명"과 바이트 단위로 달라져
// checkHeader가 거부한다 — 사용자는 헤더를 제대로 썼는데도 "헤더가
// 틀렸다"는 오해를 사는 에러를 본다(리뷰에서 발견).
func TestParseCSV_StripsUTF8BOM(t *testing.T) {
	withBOM := append([]byte("\xef\xbb\xbf"), []byte(validCSV)...)
	pages, err := ParseCSV("주문정보", withBOM)
	if err != nil {
		t.Fatalf("BOM이 있으면 안 되는데: %v", err)
	}
	if len(pages) != 1 || len(pages[0].Tables) != 2 {
		t.Fatalf("got %+v", pages)
	}
}

func TestParseCSV_PropagatesParseError(t *testing.T) {
	if _, err := ParseCSV("p", []byte("잘못된,헤더\n")); err == nil {
		t.Fatal("헤더가 틀리면 에러여야 한다")
	}
}
