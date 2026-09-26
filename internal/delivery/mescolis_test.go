package delivery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"whatsappconverty/internal/config"
)

func newTestClient(apiKey string, baseURL string) *MescolisClient {
	return &MescolisClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

func TestGetOrdersSendsTokenAndDecodes(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get(mescolisAccessTokenHdr)
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1,"orders":[{"barcode":"AB-100","status":"in-progress","status_label":"En cours"},{"barcode":"AB-101","status":"delivered"}],"not_found":["AB-999"]}`))
	}))
	defer srv.Close()

	client := newTestClient("secret-token", srv.URL)
	resp, err := client.GetOrders(context.Background(), []string{"AB-100", "AB-101", "AB-999"})
	if err != nil {
		t.Fatalf("GetOrders: %v", err)
	}

	if gotAuth != "secret-token" {
		t.Fatalf("x-access-token header = %q, want secret-token", gotAuth)
	}
	barcodes := gotBody["barcodes"].([]any)
	if len(barcodes) != 3 {
		t.Fatalf("barcodes payload = %v, want 3", barcodes)
	}

	if len(resp.Orders) != 2 {
		t.Fatalf("orders = %d, want 2", len(resp.Orders))
	}
	if resp.Orders[0].StatusLabel == "" {
		t.Fatal("expected status_label to decode")
	}
	if resp.Orders[1].Status != "delivered" {
		t.Fatalf("second order status = %q", resp.Orders[1].Status)
	}
	if len(resp.NotFound) != 1 || resp.NotFound[0] != "AB-999" {
		t.Fatalf("not_found = %v", resp.NotFound)
	}
}

func TestGetOrdersEmptyBarcodesNoRequest(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := newTestClient("t", srv.URL)
	resp, err := client.GetOrders(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetOrders: %v", err)
	}
	if resp.Orders != nil {
		t.Fatal("expected empty orders")
	}
	if hit {
		t.Fatal("no HTTP request expected for empty barcodes")
	}
}

func TestGetOrdersAPIErrorDecoded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"status":0,"code":"INVALID_TOKEN","message":"bad credentials"}`))
	}))
	defer srv.Close()

	client := newTestClient("t", srv.URL)
	_, err := client.GetOrders(context.Background(), []string{"X"})
	if err == nil {
		t.Fatal("expected an error for HTTP 401")
	}
	if err.Error() != "mescolis api error: bad credentials (INVALID_TOKEN)" {
		t.Fatalf("unexpected error text: %v", err)
	}
}

func TestProbeAcceptsUnknownParcel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"barcode":"__platform_probe__","status":"0"}`))
	}))
	defer srv.Close()

	client := newTestClient("t", srv.URL)
	if err := client.Probe(context.Background()); err != nil {
		t.Fatalf("Probe should accept an ok response for an unknown barcode: %v", err)
	}
}

func TestProbeAcceptsUnknownParcelAsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"No order in data base","status":404}`))
	}))
	defer srv.Close()

	client := newTestClient("t", srv.URL)
	if err := client.Probe(context.Background()); err != nil {
		t.Fatalf("Probe should accept a missing-order error as a valid token: %v", err)
	}
}

func TestProbeRejectsInvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Invalid Token!","status":501,"error":"INVALID_TOKEN"}`))
	}))
	defer srv.Close()

	client := newTestClient("t", srv.URL)
	if err := client.Probe(context.Background()); err == nil {
		t.Fatal("Probe should reject an invalid token")
	}
}

func TestNewMescolisClientUsesConfigBaseURL(t *testing.T) {
	cfg := config.Config{MescolisBaseURL: "https://m.example.test/api"}
	client := NewMescolisClient("k", false, "", cfg, nil)
	if client.baseURL != "https://m.example.test/api" {
		t.Fatalf("baseURL = %q", client.baseURL)
	}
	if client.apiKey != "k" {
		t.Fatalf("apiKey = %q", client.apiKey)
	}
}

func TestStatusTerminal(t *testing.T) {
	for _, terminal := range []string{"delivered", "delivered-and-paid", "return-sender", "final-return"} {
		if !StatusTerminal(terminal) {
			t.Fatalf("%q should be terminal", terminal)
		}
	}
	for _, active := range []string{"", "pending", "in-progress", "inter-depot", "at-agency", "cancelled-by-sender"} {
		if StatusTerminal(active) {
			t.Fatalf("%q should NOT be terminal", active)
		}
	}
}

func TestLabelFor(t *testing.T) {
	if got := LabelFor("in-progress"); got != "Out for delivery" {
		t.Fatalf("LabelFor(in-progress) = %q", got)
	}
	if got := LabelFor("unknown-status"); got != "unknown-status" {
		t.Fatalf("LabelFor(unknown) = %q", got)
	}
}
