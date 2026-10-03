package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/tenant"
	"github.com/knadh/go-i18n"
	"github.com/knadh/stuffbin"
	"github.com/zerodha/fastglue"
)

const (
	defLang = "en-US"
)

// handleGetI18nLang returns the JSON language pack for the given language code.
func handleGetI18nLang(r *fastglue.Request) error {
	var (
		app  = r.Context.(*App)
		lang = r.RequestCtx.UserValue("lang").(string)
	)
	i, err := loadI18nLang(lang, app.fs)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendBytes(http.StatusOK, "application/json", i.JSON())
}

// handleGetAvailableLanguages returns the list of available languages
// by reading all JSON files from the /i18n/ directory in the embedded filesystem.
func handleGetAvailableLanguages(r *fastglue.Request) error {
	app := r.Context.(*App)

	files, err := app.fs.Glob("/i18n/*.json")
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, "error listing language files", nil))
	}

	type langInfo struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}

	var langs []langInfo
	for _, f := range files {
		code := strings.TrimSuffix(filepath.Base(f), ".json")
		b, err := app.fs.Read(f)
		if err != nil {
			continue
		}

		var meta map[string]string
		if err := json.Unmarshal(b, &meta); err != nil {
			continue
		}

		name := meta["_.name"]
		if name == "" {
			name = code
		}
		langs = append(langs, langInfo{Code: code, Name: name})
	}

	sort.Slice(langs, func(i, j int) bool {
		return langs[i].Name < langs[j].Name
	})

	return r.SendEnvelope(langs)
}

// localeI18n returns the language pack for the given locale code, matching
// available packs by exact code or language prefix, falling back to the
// installation's language.
func localeI18n(app *App, locale string) *i18n.I18n {
	return app.i18n.Lang(matchLangFile(app.fs, strings.ToLower(strings.TrimSpace(locale))))
}

// matchLangFile finds the language pack code for a locale: exact match first, then language prefix.
func matchLangFile(fs stuffbin.FileSystem, loc string) string {
	files, err := fs.Glob("/i18n/*.json")
	if err != nil {
		return defLang
	}
	lang := strings.SplitN(loc, "-", 2)[0]
	prefixMatch := ""
	for _, f := range files {
		code := strings.TrimSuffix(filepath.Base(f), ".json")
		lc := strings.ToLower(code)
		if lc == loc {
			return code
		}
		if prefixMatch == "" && (lc == lang || strings.HasPrefix(lc, lang+"-")) {
			prefixMatch = code
		}
	}
	if prefixMatch != "" {
		return prefixMatch
	}
	return defLang
}

// loadI18nLang loads the i18n language pack for the given language code.
func loadI18nLang(lang string, fs stuffbin.FileSystem) (*i18n.I18n, error) {
	// Helper function to read and initialize i18n language.
	readLang := func(lang string) ([]byte, error) {
		return fs.Read(fmt.Sprintf("/i18n/%s.json", lang))
	}

	// Read default language.
	b, err := readLang(defLang)
	if err != nil {
		return nil, envelope.NewError(envelope.GeneralError, "error reading default language", nil)
	}

	// Initialize with the default language.
	i, err := i18n.New(b)
	if err != nil {
		return nil, envelope.NewError(envelope.GeneralError, "error unmarshalling i18n language", nil)
	}

	// Load the selected language on top of it.
	if b, err = readLang(lang); err == nil {
		if err := i.Load(b); err != nil {
			return i, envelope.NewError(envelope.GeneralError, "error loading i18n language file", nil)
		}
	}

	return i, nil
}

// tenantLangs caches each tenant's language code; see tenantLang.
var tenantLangs sync.Map

type tenantLangEntry struct {
	lang string
	exp  time.Time
}

const tenantLangTTL = time.Minute

// tenantLang returns a tenant's own language, app.lang in its settings, or ""
// for the installation's language: the default tenant, single-tenant mode, or
// a tenant whose setting can't be read. It must not translate anything, since
// the catalog calls it to pick the language.
func tenantLang(app *App, tenantID int) string {
	if !app.tenant.Enabled() || tenantID == tenant.DefaultID {
		return ""
	}
	if e, ok := tenantLangs.Load(tenantID); ok && time.Now().Before(e.(tenantLangEntry).exp) {
		return e.(tenantLangEntry).lang
	}
	lang := ""
	if b, err := app.setting.Get(tenantID, "app.lang"); err == nil {
		json.Unmarshal(b, &lang)
	}
	tenantLangs.Store(tenantID, tenantLangEntry{lang: lang, exp: time.Now().Add(tenantLangTTL)})
	return lang
}
