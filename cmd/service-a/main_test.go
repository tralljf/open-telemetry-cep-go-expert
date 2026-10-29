package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleWeatherPropagatesServiceBResponse(t *testing.T) {
	serviceB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("can not find zipcode"))
	}))
	defer serviceB.Close()

	app := &application{serviceBURL: serviceB.URL, client: serviceB.Client()}

	req := httptest.NewRequest(http.MethodPost, "/weather", strings.NewReader(`{"cep":"01001003"}`))
	rec := httptest.NewRecorder()
	app.handleWeather(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if got := rec.Body.String(); got != "can not find zipcode" {
		t.Fatalf("body = %q, want %q", got, "can not find zipcode")
	}
}

func TestHandleWeatherInvalidZipcode(t *testing.T) {
	app := &application{serviceBURL: "http://unused", client: http.DefaultClient}

	req := httptest.NewRequest(http.MethodPost, "/weather", strings.NewReader(`{"cep":"123"}`))
	rec := httptest.NewRecorder()
	app.handleWeather(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if got := rec.Body.String(); got != "invalid zipcode" {
		t.Fatalf("body = %q, want %q", got, "invalid zipcode")
	}
}
