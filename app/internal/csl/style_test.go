package csl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimalCSL = `<?xml version="1.0" encoding="utf-8"?>
<style xmlns="http://purl.org/net/xbiblio/csl" class="in-text" version="1.0">
  <info><title>Test</title><id>https://litkit.local/test</id><updated>2026-01-01T00:00:00+00:00</updated></info>
  <citation><layout><text variable="title"/></layout></citation>
  <bibliography><layout><text variable="title"/></layout></bibliography>
</style>`

func TestResolve_Defaults(t *testing.T) {
	zh, err := Resolve("zh", "", "")
	if err != nil || zh.Name != "gb7714-2025" || !zh.Builtin {
		t.Fatalf("zh 默认 = %+v, err = %v", zh, err)
	}
	en, err := Resolve("en", "", "")
	if err != nil || en.Name != "apa" || !en.Builtin {
		t.Fatalf("en 默认 = %+v, err = %v", en, err)
	}
	if _, err := Resolve("fr", "", ""); err == nil {
		t.Fatal("未知语言应报错")
	}
}

func TestResolve_BuiltinAndPath(t *testing.T) {
	if st, err := Resolve("zh", "ieee", ""); err != nil || !st.Builtin {
		t.Fatalf("内置 ieee 解析失败: %+v, %v", st, err)
	}
	if st, err := Resolve("zh", "IEEE", ""); err != nil || st.Name != "ieee" {
		t.Fatalf("内置名应大小写不敏感: %+v, %v", st, err)
	}
	if _, err := Resolve("zh", "chicago", ""); err == nil {
		t.Fatal("未知内置样式应报错")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "custom.csl")
	if err := os.WriteFile(path, []byte(minimalCSL), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := Resolve("zh", path, "")
	if err != nil {
		t.Fatalf("路径样式解析失败: %v", err)
	}
	if st.Name != "custom" || st.Builtin {
		t.Errorf("路径样式 Name/Builtin = %q/%v", st.Name, st.Builtin)
	}

	bad := filepath.Join(dir, "bad.csl")
	if err := os.WriteFile(bad, []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve("zh", bad, ""); err == nil {
		t.Fatal("非 CSL 文件应报错")
	}

	badXML := filepath.Join(dir, "badxml.csl")
	if err := os.WriteFile(badXML, []byte(`<style><info></style>`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve("zh", badXML, ""); err == nil {
		t.Fatal("XML 非良构的 CSL 应报错")
	}
}

func TestResolve_UserStylesDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "journal-x.csl"), []byte(minimalCSL), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := Resolve("zh", "journal-x", dir)
	if err != nil {
		t.Fatalf("用户目录样式解析失败: %v", err)
	}
	if st.Name != "journal-x" || st.Builtin {
		t.Errorf("用户样式 = %+v", st)
	}

	// Linux 下 Glob("*.csl") 不会匹配 .CSL；ReadDir + EqualFold 必须能发现。
	if err := os.WriteFile(filepath.Join(dir, "Upper.CSL"), []byte(minimalCSL), 0o600); err != nil {
		t.Fatal(err)
	}
	if st, err := Resolve("zh", "upper", dir); err != nil || st.Name != "Upper" {
		t.Fatalf(".CSL 用户样式解析失败: %+v, %v", st, err)
	}

	// 内置名保留：用户目录中的同名文件不得覆盖内置样式。
	if err := os.WriteFile(filepath.Join(dir, "ieee.csl"), []byte(minimalCSL), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve("zh", "ieee", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Builtin {
		t.Fatal("用户目录的同名文件不应覆盖内置样式")
	}
}

func TestList_ConflictFlag(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ieee.csl"), []byte(minimalCSL), 0o600); err != nil {
		t.Fatal(err)
	}
	infos := List(dir)
	var conflict bool
	for _, info := range infos {
		if info.Conflict {
			conflict = true
		}
	}
	if !conflict {
		t.Fatalf("List 未标记内置名冲突: %+v", infos)
	}
}

func TestShow_And_Export(t *testing.T) {
	st, err := Show("apa", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(st.Content), "<style") {
		t.Fatal("Show 未返回 CSL 内容")
	}

	dir := t.TempDir()
	path, err := Export("apa", dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "apa-custom.csl" {
		t.Errorf("导出文件名 = %q", filepath.Base(path))
	}
	if _, err := Export("apa", dir); err == nil {
		t.Fatal("重复导出应报错，避免覆盖用户文件")
	}
	if _, err := Export("nope", dir); err == nil {
		t.Fatal("未知样式导出应报错")
	}
}
