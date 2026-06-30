package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tralljf/open-telemetry-cep-google-cloud-run/internal/telemetry"
	"github.com/tralljf/open-telemetry-cep-google-cloud-run/internal/zipcode"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var (
	errZipcodeNotFound = errors.New("zipcode not found")
	errWeatherNotFound = errors.New("weather not found")
)

type requestPayload struct {
	CEP string `json:"cep"`
}

type responsePayload struct {
	City  string  `json:"city"`
	TempC float64 `json:"temp_C"`
	TempF float64 `json:"temp_F"`
	TempK float64 `json:"temp_K"`
}

func main() {
	ctx := context.Background()
	shutdownTelemetry, err := telemetry.Init(ctx, "service-b")
	if err != nil {
		log.Fatalf("failed to initialize telemetry: %v", err)
	}
	defer func() {
		if err := shutdownTelemetry(context.Background()); err != nil {
			log.Printf("failed to shutdown telemetry: %v", err)
		}
	}()

	port := getenv("PORT", "8081")
	app := &application{
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
		log.Printf("service-b listening on :%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("service-b failed: %v", err)
		}
	}()

	waitForShutdown(server)
}

type application struct {
	client *http.Client
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

	city, err := app.fetchCity(r.Context(), payload.CEP)
	if err != nil {
		if errors.Is(err, errZipcodeNotFound) {
			writeError(w, http.StatusNotFound, "can not find zipcode")
			return
		}
		log.Printf("failed to fetch city: %v", err)
		writeError(w, http.StatusBadGateway, "can not fetch zipcode")
		return
	}

	tempC, err := app.fetchTemperature(r.Context(), city)
	if err != nil {
		if errors.Is(err, errWeatherNotFound) {
			writeError(w, http.StatusNotFound, "can not find zipcode")
			return
		}
		log.Printf("failed to fetch weather: %v", err)
		writeError(w, http.StatusBadGateway, "can not fetch weather")
		return
	}

	response := responsePayload{
		City:  city,
		TempC: round(tempC),
		TempF: round(tempC*1.8 + 32),
		TempK: round(tempC + 273.15),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

func (app *application) fetchCity(ctx context.Context, cep string) (string, error) {
	tracer := otel.Tracer("service-b")
	ctx, span := tracer.Start(ctx, "Busca de CEP")
	defer span.End()

	endpoint := fmt.Sprintf("https://viacep.com.br/ws/%s/json/", cep)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}

	resp, err := app.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", errZipcodeNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("viacep returned status %d", resp.StatusCode)
	}

	var data struct {
		Localidade string `json:"localidade"`
		Erro       bool   `json:"erro"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}
	if data.Erro || data.Localidade == "" {
		return "", errZipcodeNotFound
	}

	span.SetAttributes(attribute.String("zipcode.city", data.Localidade))
	return data.Localidade, nil
}

func (app *application) fetchTemperature(ctx context.Context, city string) (float64, error) {
	tracer := otel.Tracer("service-b")
	ctx, span := tracer.Start(ctx, "Busca de temperatura")
	defer span.End()

	location, err := app.geocodeCity(ctx, city)
	if err != nil {
		return 0, err
	}

	forecastURL := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f&current=temperature_2m&timezone=auto",
		location.Latitude,
		location.Longitude,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, forecastURL, nil)
	if err != nil {
		return 0, err
	}

	resp, err := app.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return 0, fmt.Errorf("open-meteo forecast returned status %d", resp.StatusCode)
	}

	var data struct {
		Current struct {
			Temperature float64 `json:"temperature_2m"`
		} `json:"current"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, err
	}

	span.SetAttributes(
		attribute.String("weather.city", city),
		attribute.Float64("weather.temperature_celsius", data.Current.Temperature),
	)
	return data.Current.Temperature, nil
}

type geocodeLocation struct {
	Latitude  float64
	Longitude float64
}

func (app *application) geocodeCity(ctx context.Context, city string) (geocodeLocation, error) {
	geocodeURL := fmt.Sprintf(
		"https://geocoding-api.open-meteo.com/v1/search?name=%s&count=1&language=pt&format=json",
		url.QueryEscape(city),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, geocodeURL, nil)
	if err != nil {
		return geocodeLocation{}, err
	}

	resp, err := app.client.Do(req)
	if err != nil {
		return geocodeLocation{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return geocodeLocation{}, fmt.Errorf("open-meteo geocoding returned status %d", resp.StatusCode)
	}

	var data struct {
		Results []struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return geocodeLocation{}, err
	}
	if len(data.Results) == 0 {
		return geocodeLocation{}, errWeatherNotFound
	}

	return geocodeLocation{
		Latitude:  data.Results[0].Latitude,
		Longitude: data.Results[0].Longitude,
	}, nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(message))
}

func round(value float64) float64 {
	return math.Round(value*100) / 100
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
