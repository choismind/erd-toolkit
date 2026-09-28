// internal/glossary/testsupport.go
package glossary

// MustLoadForTest는 메모리 위에 사전을 만든다. 다른 패키지의 테스트가
// xlsx 파일 없이 Dict를 얻기 위한 것이다 — 프로덕션 코드는 쓰지 않는다.
//
// «다른 패키지»가 쓰므로 _test.go에 둘 수 없고, 그래서 출하 바이너리에도
// 함께 실린다. 예전에는 *testing.T를 받아 «테스트에서만 불리도록 강제»
// 하려 했지만 그것은 강제가 아니었다 — nil을 넘기면 어디서든 컴파일된다.
// 그 인자가 실제로 한 일은 하나뿐이었다: testing 패키지를 출하 바이너리에
// 링크하는 것(그리고 testing이 끌고 오는 flag·regexp까지). CLI가 테스트
// 프레임워크를 짊어질 이유는 없으므로 인자를 뺐다.
//
// 남은 보호는 이름과 이 주석뿐이다. 그것으로 충분하다 — Load의 불변식을
// 건너뛰는 생성자라는 사실은 이름에 적혀 있고, 프로덕션 경로에서 이걸
// 부르는 변경은 리뷰에서 눈에 띈다.
func MustLoadForTest(terms, words map[string]string) *Dict {
	d := &Dict{terms: map[string]string{}, words: map[string]string{}}
	for k, v := range terms {
		d.terms[k] = v
	}
	for k, v := range words {
		d.words[k] = v
		if n := len([]rune(k)); n > d.maxWordRunes {
			d.maxWordRunes = n
		}
	}
	return d
}
