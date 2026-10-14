package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"hinatracer/internal/history"
	"hinatracer/internal/webserver"
)

// backend is where the GUI's data and actions live: the in-process engines
// (local) or a remote WebUI (remote). Calls block, so the GUI makes them in
// goroutines and applies results on the UI thread.
type backend interface {
	// Post runs a mutating API call.
	Post(path string, body any) (apiResult, error)
	// Get fetches a GET API path.
	Get(path string) ([]byte, error)
	// Remote reports whether this is a remote WebUI.
	Remote() bool
}

type localBackend struct{ a *app }

func (l localBackend) call() mcpCall {
	return mcpCall{mux: l.a.localAPI(), role: webserver.RoleAdmin}
}

func (l localBackend) Post(path string, body any) (apiResult, error) {
	if body == nil {
		body = struct{}{}
	}
	var r apiResult
	err := l.call().do("POST", path, body, &r)
	return r, err
}

func (l localBackend) Get(path string) ([]byte, error) { return l.call().raw("GET", path) }
func (l localBackend) Remote() bool                    { return false }

// localAPI is the unguarded API mux used for in-process calls.
func (a *app) localAPI() http.Handler {
	a.apiOnce.Do(func() {
		if a.apiMux.Load() == nil {
			a.webHandler()
		}
	})
	return a.apiMux.Load()
}

// be returns the active backend.
func (a *app) be() backend {
	if rc := a.remote; rc != nil {
		return rc
	}
	return localBackend{a}
}

// readOnly reports whether the GUI must hide edit controls (remote guest).
func (a *app) readOnly() bool {
	return a.remote != nil && a.remote.Role() == string(webserver.RoleGuest)
}

// beAsync runs a backend POST off the UI thread; done (optional) runs on
// the UI thread with the result.
func (a *app) beAsync(path string, body any, done func(apiResult, error)) {
	b := a.be()
	safeGo("backend", func() {
		r, err := b.Post(path, body)
		if err == nil && !r.OK && r.Message != "" {
			err = errors.New(r.Message)
		}
		if done != nil {
			_ = a.run(func() { done(r, err) })
		} else if err != nil {
			_ = a.run(func() { a.showToast(err.Error()) })
		}
		a.requestUIUpdate()
	})
}

// historyVia fetches chart points from a backend.
func historyVia(b backend, q url.Values) ([]history.Point, error) {
	data, err := b.Get("/api/history?" + q.Encode())
	if err != nil {
		return nil, err
	}
	var r struct {
		Data struct {
			Points []history.Point `json:"points"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return r.Data.Points, nil
}

// showToast shows a short notice in the top bar.
func (a *app) showToast(msg string) {
	a.notice = msg
	a.noticeAt = time.Now()
}
