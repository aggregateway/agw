package agw

import (
	"net/http"
	"strings"
)

type configView struct {
	Index            int
	Name             string
	URL              string
	AuthType         string
	AuthValue        string
	AuthIsSecret     bool
	AppSelectorsText string
	HasAuth          bool
}

type pageView struct {
	Debug        bool
	AllowDebug   bool
	AppSelectors []AppSelector
	OGURL        string
	OGImage      string
}

func stringSliceContains(list []string, item string) bool {
	for _, s := range list {
		if s == item {
			return true
		}
	}
	return false
}

func configViews(upstreams []Upstream) []configView {
	views := make([]configView, 0, len(upstreams))
	for i, upstream := range upstreams {
		view := configView{Index: i + 1, Name: upstream.Name, URL: upstream.URL, AppSelectorsText: strings.Join(upstream.AppSelectors, ", ")}
		if upstream.Authorization != nil {
			view.HasAuth = true
			view.AuthType = upstream.Authorization.Type
			// secret:<key> references are resolved by the browser from its own
			// localStorage, never from shared server memory, so credentials
			// injected by another browser are not exposed here.
			if strings.HasPrefix(upstream.Authorization.Value, "secret:") {
				view.AuthIsSecret = true
				view.AuthValue = upstream.Authorization.Value
			} else if resolved, err := resolveAuthValue(upstream.Authorization.Value, nil); err == nil {
				view.AuthValue = resolved
			} else {
				view.AuthValue = upstream.Authorization.Value
			}
		}
		views = append(views, view)
	}
	return views
}

func serveConfigPage(w http.ResponseWriter, r *http.Request, selectors []AppSelector, debug, allowDebug bool) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	base := scheme + "://" + r.Host
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := getTemplate("page.html").Execute(w, pageView{Debug: debug, AllowDebug: allowDebug, AppSelectors: selectors, OGURL: base + "/", OGImage: base + "/icon-512.png"}); err != nil {
		http.Error(w, "failed to render page", http.StatusInternalServerError)
	}
}

func serveConfigFragment(w http.ResponseWriter, upstreams []Upstream) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := getTemplate("fragment.html").Execute(w, configViews(upstreams)); err != nil {
		http.Error(w, "failed to render config", http.StatusInternalServerError)
	}
}
