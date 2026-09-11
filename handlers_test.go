package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	app.HealthHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado status 200, obtido %d", rec.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("erro ao decodificar resposta: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("esperado status=ok, obtido %q", body["status"])
	}
	if body["service"] != "donation-service" {
		t.Errorf("esperado service=donation-service, obtido %q", body["service"])
	}
}

func TestDonationHandler_MethodNotAllowed(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodDelete, "/donations", nil)
	rec := httptest.NewRecorder()

	app.DonationHandler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("esperado status 405, obtido %d", rec.Code)
	}
}

func TestDonationHandler_InvalidPayload(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodPost, "/donations", nil)
	rec := httptest.NewRecorder()

	app.DonationHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperado status 400 para payload vazio, obtido %d", rec.Code)
	}
}
