package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tralljf/open-telemetry-cep-google-cloud-run/internal/telemetry"
	"github.com/tralljf/open-telemetry-cep-google-cloud-run/internal/zipcode"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type requestPayload struct {
	CEP string `json:"cep"`
}

func main() {
	ctx := context.Background()
	shutdownTelemetry, err := telemetry.Init(ctx, "service-a")
	if err != nil {
		log.Fatalf("failed to initialize telemetry: %v", err)
	}
	defer func() {
		if err := shutdownTelemetry(context.Background()); err != nil {
			log.Printf("failed to shutdown telemetry: %v", err)
		}
	}()

	serviceBURL := getenv("SERVICE_B_URL", "http://service-b:8081/weather")
	port := getenv("PORT", "8080")

	app := &application{
		serviceBURL: serviceBURL,
		client: &http.Client{
			Timeout:   10 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}

	mux := http.NewServeMux()
	mux.Handle("/weather", otelhttp.NewHandler(http.HandlerFunc(app.handleWeather), "POST /weather"))
	mux.Handle("/", otelhttp.NewHandler(http.HandlerFunc(app.handleWeather), "POST /"))

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("service-a listening on :%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("service-a failed: %v", err)
		}
	}()

	waitForShutdown(server)
}

type application struct {
	serviceBURL string
	client      *http.Client
}

func (app *application) handleWeather(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var payload requestPayload
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil || !zipcode.Valid(payload.CEP) {
		writeError(w, http.StatusUnprocessableEntity, "invalid zipcode")
		return
	}

	body, err := json.Marshal(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, app.serviceBURL, bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.client.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "service-b unavailable")
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("failed to copy service-b response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(message))
}

func waitForShutdown(server *http.Server) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("server shutdown failed: %v", err)
	}
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
