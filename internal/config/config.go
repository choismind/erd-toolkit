package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type validationOverride struct {
	ANSISQLTypes     *bool `yaml:"ansi_sql_types,omitempty"`
	NamingConvention *bool `yaml:"naming_convention,omitempty"`
}

type PageOverride struct {
	Validation *validationOverride `yaml:"validation,omitempty"`
}

type Config struct {
	Outputs struct {
		RelationDoc bool `yaml:"relation_doc"`
		SQLDDL      bool `yaml:"sql_ddl"`
	} `yaml:"outputs"`
	Validation struct {
		ANSISQLTypes     bool `yaml:"ansi_sql_types"`
		NamingConvention bool `yaml:"naming_convention"`
	} `yaml:"validation"`
	PageAsDomain bool `yaml:"page_as_domain"`
	// Dialect는 뽑을 SQL이 겨누는 DBMS다(ansi/postgres/mysql/sqlite).
	// 비어 있으면 ANSI다 — «아무 데나 맞춘다»가 아니라 «표준을 겨눈다»는 뜻이고,
	// 그 사실이 산출물 머리에 「ANSI (지정 안 함)」으로 박힌다.
	//
	// CLI의 --dialect가 그 실행 1회에 한해 이 값을 덮어쓴다.
	Dialect string `yaml:"dialect"`
	// Recursive는 폴더를 지정했을 때 하위 폴더까지 읽을지 정한다. 기본은
	// 끔 — 큰 트리를 잘못 지정했을 때 조용히 오래 도는 쪽보다, 필요할 때만
	// 켜는 쪽이 사고가 없다. CLI의 --recursive로도 켤 수 있다.
	Recursive bool                    `yaml:"recursive"`
	OutputDir string                  `yaml:"output_dir"`
	Pages     map[string]PageOverride `yaml:"pages"`
	// Dictionary는 표준용어사전 xlsx 경로다(convert 전용). 비어 있으면
	// 대상 폴더에서 자동탐색한다.
	//
	// 상대 경로는 «설정 파일이 있는 곳»이 아니라 «실행한 작업 디렉터리»를
	// 기준으로 푼다. 이 저장소의 경로 필드 중 설정 파일 위치를 기준으로
	// 삼는 것은 하나도 없다 — OutputDir은 «입력 파일이 있는 폴더» 기준이고
	// (internal/pipeline/generate.go), convert의 --out은 작업 디렉터리
	// 기준이다. 설정 파일 기준으로 바꾸면 --dictionary로 준 경로와 설정에
	// 적은 경로가 서로 다른 기준을 갖게 되므로, 관례를 일부러 그대로 둔다.
	Dictionary string `yaml:"dictionary"`
}

func Load(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// EffectiveValidation은 전역 옵션에 페이지별 오버라이드를 적용한 최종값을
// 반환한다.
func (c Config) EffectiveValidation(pageName string) (ansiSQL, naming bool) {
	ansiSQL = c.Validation.ANSISQLTypes
	naming = c.Validation.NamingConvention
	pg, ok := c.Pages[pageName]
	if !ok || pg.Validation == nil {
		return
	}
	if pg.Validation.ANSISQLTypes != nil {
		ansiSQL = *pg.Validation.ANSISQLTypes
	}
	if pg.Validation.NamingConvention != nil {
		naming = *pg.Validation.NamingConvention
	}
	return
}

// discoveryNames는 자동탐색이 찾는 파일명이다. 앞에 있는 것이 이긴다 —
// 둘 다 있을 때 실행마다 결과가 달라지면 안 된다. .yaml/.yml 중 어느 쪽을
// 쓰는지는 사람마다 갈리므로 둘 다 받는다.
var discoveryNames = []string{"erdtool.yaml", "erdtool.yml"}

// Discover는 dir에서 설정 파일을 찾는다. 상위 폴더로 거슬러 올라가지는
// 않는다 — 어느 설정이 먹었는지 눈으로 바로 알 수 있어야 하기 때문이다.
func Discover(dir string) (string, bool) {
	return discoverIn(dir, discoveryNames)
}

// dictionaryNames는 사전 자동탐색이 찾는 파일명이다. 앞에 있는 것이 이긴다.
var dictionaryNames = []string{"표준용어사전.xlsx"}

// DiscoverDictionary는 dir에서 표준용어사전을 찾는다. 설정 파일 자동탐색(Discover)과
// 규칙을 discoverIn으로 공유한다 — 두 벌로 따로 짜면 한쪽만 케이스를 늘렸을 때
// (예: 대소문자 무시, 다른 확장자 폴백) "같은 규칙"이라는 주장이 조용히 거짓이 된다.
func DiscoverDictionary(dir string) (string, bool) {
	return discoverIn(dir, dictionaryNames)
}

// connectionNames는 접속 설정 자동탐색이 찾는 파일명이다. 앞에 있는 것이 이긴다.
var connectionNames = []string{"erdtool.connections.yaml", "erdtool.connections.yml"}

// DiscoverConnections는 dir에서 접속 설정 파일을 찾는다. 규칙은
// Discover/DiscoverDictionary와 discoverIn으로 공유한다.
func DiscoverConnections(dir string) (string, bool) {
	return discoverIn(dir, connectionNames)
}

// discoverIn은 dir에서 names를 차례로 찾는다. 앞에 있는 이름이 이긴다.
// 상위 폴더로는 거슬러 올라가지 않는다 — 어느 파일이 먹었는지 눈으로 바로
// 알 수 있어야 하기 때문이다.
func discoverIn(dir string, names []string) (string, bool) {
	for _, name := range names {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, true
		}
	}
	return "", false
}
