package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "erdreport.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoad_GlobalDefaults(t *testing.T) {
	path := writeTempConfig(t, `
outputs:
  relation_doc: true
  sql_ddl: false
validation:
  ansi_sql_types: true
  naming_convention: false
page_as_domain: true
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !cfg.Outputs.RelationDoc || cfg.Outputs.SQLDDL {
		t.Fatalf("unexpected outputs: %+v", cfg.Outputs)
	}
	if !cfg.PageAsDomain {
		t.Fatalf("expected page_as_domain=true")
	}
	ansi, naming := cfg.EffectiveValidation("아무페이지")
	if !ansi || naming {
		t.Fatalf("expected global validation defaults to apply: ansi=%v naming=%v", ansi, naming)
	}
}

func TestLoad_PageOverride(t *testing.T) {
	path := writeTempConfig(t, `
validation:
  ansi_sql_types: true
  naming_convention: false
pages:
  "논리ERD":
    level: relational
    validation:
      ansi_sql_types: false
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	ansi, _ := cfg.EffectiveValidation("논리ERD")
	if ansi {
		t.Fatalf("expected page override to disable ansi_sql_types for 논리ERD")
	}
}

func TestDiscover_FindsErdtoolYaml(t *testing.T) {
	// 지금은 --config로만 설정을 줄 수 있어서, .drawio 옆에 설정을 두고
	// 그냥 `erdtool generate .`을 치면 설정이 조용히 무시됐다.
	dir := t.TempDir()
	want := filepath.Join(dir, "erdtool.yaml")
	if err := os.WriteFile(want, []byte("page_as_domain: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, found := Discover(dir)
	if !found {
		t.Fatal("erdtool.yaml을 찾지 못했다")
	}
	if got != want {
		t.Fatalf("경로가 다르다: got %q want %q", got, want)
	}
}

func TestDiscover_FallsBackToYml(t *testing.T) {
	// .yaml/.yml 중 어느 쪽을 쓰는지는 사람마다 갈린다. 한쪽만 지원하면
	// 다른 쪽을 쓴 사용자에게는 "설정이 그냥 안 먹는" 것으로 보인다.
	dir := t.TempDir()
	want := filepath.Join(dir, "erdtool.yml")
	if err := os.WriteFile(want, []byte("page_as_domain: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, found := Discover(dir)
	if !found || got != want {
		t.Fatalf("erdtool.yml 폴백 실패: got %q found=%v", got, found)
	}
}

func TestDiscover_PrefersYamlOverYml(t *testing.T) {
	// 둘 다 있으면 어느 쪽이 이기는지 정해져 있어야 한다 — 실행할 때마다
	// 달라지면 안 된다.
	dir := t.TempDir()
	yaml := filepath.Join(dir, "erdtool.yaml")
	for _, p := range []string{yaml, filepath.Join(dir, "erdtool.yml")} {
		if err := os.WriteFile(p, []byte("page_as_domain: true\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if got, _ := Discover(dir); got != yaml {
		t.Fatalf(".yaml이 이겨야 한다: got %q", got)
	}
}

func TestDiscover_NothingThere(t *testing.T) {
	if got, found := Discover(t.TempDir()); found {
		t.Fatalf("설정이 없으면 found=false여야 한다: got %q", got)
	}
}

func TestConfig_RecursiveKey(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "erdtool.yaml")
	if err := os.WriteFile(p, []byte("recursive: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Recursive {
		t.Fatal("recursive: true가 읽히지 않았다")
	}
}

func TestDiscoverDictionary(t *testing.T) {
	dir := t.TempDir()
	if _, ok := DiscoverDictionary(dir); ok {
		t.Fatal("빈 폴더에서 사전을 찾았다고 나왔다")
	}

	p := filepath.Join(dir, "표준용어사전.xlsx")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, ok := DiscoverDictionary(dir)
	if !ok || got != p {
		t.Fatalf("DiscoverDictionary = %q, %v; want %q, true", got, ok, p)
	}
}

func TestConfig_DictionaryKey(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "erdtool.yaml")
	if err := os.WriteFile(p, []byte("dictionary: ./사전.xlsx\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Dictionary != "./사전.xlsx" {
		t.Fatalf("Dictionary = %q", cfg.Dictionary)
	}
}

func TestDiscoverConnections(t *testing.T) {
	dir := t.TempDir()
	if _, ok := DiscoverConnections(dir); ok {
		t.Fatal("없는데 찾았다고 한다")
	}
	p := filepath.Join(dir, "erdtool.connections.yaml")
	if err := os.WriteFile(p, []byte("connections: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok := DiscoverConnections(dir)
	if !ok || got != p {
		t.Errorf("DiscoverConnections = (%q, %v); want (%q, true)", got, ok, p)
	}
}
