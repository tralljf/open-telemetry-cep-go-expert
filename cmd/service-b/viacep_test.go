package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newViaCEPServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/01001003/json/" {
			t.Errorf("unexpected viacep path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestFetchCityZipcodeNotFound(t *testing.T) {
	tests := map[string]string{
		"erro as string": `{"erro":"true"}`,
		"erro as bool":   `{"erro":true}`,
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			server := newViaCEPServer(t, http.StatusOK, body)
			app := &application{viaCEPBaseURL: server.URL, client: server.Client()}

			_, err := app.fetchCity(t.Context(), "01001003")
			if !errors.Is(err, errZipcodeNotFound) {
				t.Fatalf("err = %v, want %v", err, errZipcodeNotFound)
			}
		})
	}
}

func TestFetchCityFound(t *testing.T) {
	tests := map[string]string{
		"without erro":         `{"cep":"01001-003","localidade":"São Paulo"}`,
		"erro false as bool":   `{"localidade":"São Paulo","erro":false}`,
		"erro false as string": `{"localidade":"São Paulo","erro":"false"}`,
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			server := newViaCEPServer(t, http.StatusOK, body)
			app := &application{viaCEPBaseURL: server.URL, client: server.Client()}

			city, err := app.fetchCity(t.Context(), "01001003")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if city != "São Paulo" {
				t.Fatalf("city = %q, want %q", city, "São Paulo")
			}
		})
	}
}

func TestHandleWeatherZipcodeNotFound(t *testing.T) {
	tests := map[string]string{
		"erro as string": `{"erro":"true"}`,
		"erro as bool":   `{"erro":true}`,
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			server := newViaCEPServer(t, http.StatusOK, body)
			app := &application{viaCEPBaseURL: server.URL, client: server.Client()}

			req := httptest.NewRequest(http.MethodPost, "/weather", strings.NewReader(`{"cep":"01001003"}`))
			rec := httptest.NewRecorder()
			app.handleWeather(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
			}
			if got := rec.Body.String(); got != "can not find zipcode" {
				t.Fatalf("body = %q, want %q", got, "can not find zipcode")
			}
		})
	}
}
