package i18n

import (
	"fmt"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"chinese", "zh", LanguageChinese},
		{"chinese cn", "zh-cn", LanguageChinese},
		{"chinese hans", "zh-hans", LanguageChinese},
		{"chinese uppercase", "ZH-CN", LanguageChinese},
		{"chinese mixed case hans", "Zh-Hans", LanguageChinese},
		{"chinese padded", "  zh  ", LanguageChinese},
		{"english", "en", LanguageEnglish},
		{"english uppercase", "EN", LanguageEnglish},
		{"english padded", " en ", LanguageEnglish},
		{"system", "system", LanguageSystem},
		{"unknown", "fr", LanguageSystem},
		{"empty", "", LanguageSystem},
		{"whitespace only", "   ", LanguageSystem},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Normalize(tt.value); got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name          string
		preference    string
		systemLocales []string
		want          string
	}{
		{"explicit zh", "zh", nil, LanguageChinese},
		{"explicit zh-cn", "zh-cn", nil, LanguageChinese},
		{"explicit en uppercase", "EN", nil, LanguageEnglish},
		{"unknown falls back to system then en", "fr", nil, LanguageEnglish},
		{"system with zh locale", "system", []string{"zh-CN", "en"}, LanguageChinese},
		{"system with zh-Hans locale", "system", []string{"zh-Hans"}, LanguageChinese},
		{"system with only en locale", "system", []string{"en-US"}, LanguageEnglish},
		{"system with nil locales", "system", nil, LanguageEnglish},
		{"system with empty locales", "system", []string{}, LanguageEnglish},
		{"system skips blank locales", "system", []string{"", "  ", "zh-CN"}, LanguageChinese},
		{"system with non-zh first locale", "system", []string{"fr-FR", "zh-CN"}, LanguageEnglish},
		{"system with blank non-zh locale", "system", []string{"   ", "fr"}, LanguageEnglish},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Resolve(tt.preference, tt.systemLocales); got != tt.want {
				t.Errorf("Resolve(%q, %v) = %q, want %q", tt.preference, tt.systemLocales, got, tt.want)
			}
		})
	}
}

func TestLookup(t *testing.T) {
	tests := []struct {
		name   string
		lang   string
		key    string
		want   string
		wantOK bool
	}{
		{"nav.home en", LanguageEnglish, "nav.home", "Home", true},
		{"nav.home zh", LanguageChinese, "nav.home", "首页", true},
		{"home.status.title en", LanguageEnglish, "home.status.title", "Current status", true},
		{"home.status.title zh", LanguageChinese, "home.status.title", "当前状态", true},
		{"settings.apiKeys.curseforge.label en", LanguageEnglish, "settings.apiKeys.curseforge.label", "CurseForge API Key", true},
		{"settings.apiKeys.curseforge.label zh", LanguageChinese, "settings.apiKeys.curseforge.label", "CurseForge API Key", true},
		{"manage.cleanup.scan en", LanguageEnglish, "manage.cleanup.scan", "Scan Unused Dependencies", true},
		{"manage.cleanup.scan zh", LanguageChinese, "manage.cleanup.scan", "扫描未使用依赖", true},
		{"download.confirmReplace.conflictFile placeholder", LanguageChinese, "download.confirmReplace.conflictFile", "冲突文件：{file}", true},
		{"missing key", LanguageEnglish, "does.not.exist", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Lookup(tt.lang, tt.key)
			if ok != tt.wantOK {
				t.Fatalf("Lookup(%q, %q) ok = %v, want %v", tt.lang, tt.key, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("Lookup(%q, %q) = %q, want %q", tt.lang, tt.key, got, tt.want)
			}
		})
	}
}

func TestTInterpolation(t *testing.T) {
	Apply(LanguageChinese)

	got := T("download.confirmReplace.conflictFile", map[string]string{"file": "old.jar"})
	if want := "冲突文件：old.jar"; got != want {
		t.Errorf("T zh interpolation = %q, want %q", got, want)
	}

	Apply(LanguageEnglish)
	got = T("favorites.resultSummary", map[string]string{"added": "3", "updated": "1", "skipped": "2"})
	if want := "Added 3 · Updated 1 · Skipped 2"; got != want {
		t.Errorf("T en interpolation = %q, want %q", got, want)
	}

	// Multiple param maps merge, last wins.
	got = T("favorites.resultSummary",
		map[string]string{"added": "1", "updated": "1", "skipped": "1"},
		map[string]string{"added": "9"},
	)
	if want := "Added 9 · Updated 1 · Skipped 1"; got != want {
		t.Errorf("T merged interpolation = %q, want %q", got, want)
	}

	// A placeholder with no matching param is left untouched.
	got = T("download.confirmReplace.conflictFile")
	if want := "Conflicting file: {file}"; got != want {
		t.Errorf("T unresolved placeholder = %q, want %q", got, want)
	}
}

func TestTFallbackToEnglish(t *testing.T) {
	// Introduce a key that only exists in the English dictionary, then verify
	// the Chinese lookup falls back to it.
	const key = "__test.onlyEnglish"
	enMessages[key] = "english only"
	defer delete(enMessages, key)

	Apply(LanguageChinese)
	if got, want := T(key), "english only"; got != want {
		t.Errorf("T fallback to en = %q, want %q", got, want)
	}
}

func TestTMissingKeyReturnsKey(t *testing.T) {
	Apply(LanguageEnglish)
	const key = "this.key.does.not.exist"
	if got := T(key); got != key {
		t.Errorf("T missing key = %q, want %q", got, key)
	}

	Apply(LanguageChinese)
	if got := T(key); got != key {
		t.Errorf("T missing key (zh) = %q, want %q", got, key)
	}
}

// TestKeyParity asserts that zhMessages and enMessages expose exactly the same
// set of dotted keys; it fails if a key exists in one language but not the other.
func TestKeyParity(t *testing.T) {
	zhKeys := make(map[string]struct{}, len(zhMessages))
	for k := range zhMessages {
		zhKeys[k] = struct{}{}
	}
	enKeys := make(map[string]struct{}, len(enMessages))
	for k := range enMessages {
		enKeys[k] = struct{}{}
	}

	for k := range zhKeys {
		if _, ok := enKeys[k]; !ok {
			t.Errorf("key %q exists in zhMessages but is missing from enMessages", k)
		}
	}
	for k := range enKeys {
		if _, ok := zhKeys[k]; !ok {
			t.Errorf("key %q exists in enMessages but is missing from zhMessages", k)
		}
	}

	if len(zhKeys) != len(enKeys) {
		t.Errorf("key count mismatch: zh=%d en=%d", len(zhKeys), len(enKeys))
	}
	if len(zhKeys) == 0 {
		t.Fatal("dictionary is empty")
	}
	t.Logf("zhMessages: %d keys, enMessages: %d keys", len(zhKeys), len(enKeys))
}

func TestApplyAndCurrent(t *testing.T) {
	defer Apply(LanguageSystem)

	if got := Apply(LanguageChinese); got != LanguageChinese {
		t.Errorf("Apply(zh) = %q, want %q", got, LanguageChinese)
	}
	if got := Current(); got != LanguageChinese {
		t.Errorf("Current() after Apply(zh) = %q, want %q", got, LanguageChinese)
	}

	if got := Apply("fr"); got != LanguageEnglish {
		t.Errorf("Apply(fr) = %q, want %q", got, LanguageEnglish)
	}
	if got := Current(); got != LanguageEnglish {
		t.Errorf("Current() after Apply(fr) = %q, want %q", got, LanguageEnglish)
	}
}

func ExampleT() {
	Apply(LanguageEnglish)
	fmt.Println(T("nav.home"))
	fmt.Println(T("download.confirmReplace.conflictFile", map[string]string{"file": "a.jar"}))
	// Output:
	// Home
	// Conflicting file: a.jar
}
