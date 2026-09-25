package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"whatsappconverty/internal/config"
)

const (
	mescolisDefaultBaseURL = "https://api.mescolis.tn/api"
	mescolisAccessTokenHdr = "x-access-token"
)

// MescolisClient talks to the Mes Colis Express shipping API. All requests are
// authenticated with the shop's access token in the x-access-token header.
// The API is a trimmed port of the client in the reference e-commerce app:
// only the status-lookup endpoints are needed here because the Converty
// platform creates the parcels itself — this platform only watches them.
type MescolisClient struct {
	apiKey          string
	allowSubAccount bool
	accountCode     string
	baseURL         string
	http            *http.Client
}

// NewMescolisClient builds a client. baseURL comes from config so tests and
// mirrors can point elsewhere.
func NewMescolisClient(apiKey string, allowSubAccount bool, accountCode string, cfg config.Config, httpClient *http.Client) *MescolisClient {
	base := mescolisDefaultBaseURL
	if cfg.MescolisBaseURL != "" {
		base = cfg.MescolisBaseURL
	}
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &MescolisClient{
		apiKey:          apiKey,
		allowSubAccount: allowSubAccount,
		accountCode:     accountCode,
		baseURL:         base,
		http:            client,
	}
}

// MescolisOrderStatus is a single entry returned by POST /orders/GetOrders.
type MescolisOrderStatus struct {
	Barcode     string `json:"barcode"`
	Status      string `json:"status"`
	StatusLabel string `json:"status_label"`
	CreatedAt   string `json:"created_at"`
}

// GetOrdersResponse is the response of POST /orders/GetOrders.
type GetOrdersResponse struct {
	Status   int                   `json:"status"`
	Orders   []MescolisOrderStatus `json:"orders"`
	NotFound []string              `json:"not_found"`
}

// mescolisAPIError is the standard error body returned by the Mes Colis API.
type mescolisAPIError struct {
	Status  int    `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e mescolisAPIError) Error() string {
	return fmt.Sprintf("mescolis api error: %s (%s)", e.Message, e.Code)
}

// GetOrders fetches the current status of multiple parcels in one call.
// An empty barcode list short-circuits to an empty response without a request.
func (c *MescolisClient) GetOrders(ctx context.Context, barcodes []string) (*GetOrdersResponse, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("mescolis api key is not configured")
	}
	if len(barcodes) == 0 {
		return &GetOrdersResponse{}, nil
	}

	payload := map[string]any{
		"barcodes": barcodes,
	}
	if c.allowSubAccount {
		payload["allow_sub_account"] = true
		if c.accountCode != "" {
			payload["account_code"] = c.accountCode
		}
	}

	var out GetOrdersResponse
	if err := c.do(ctx, "/orders/GetOrders", payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetOrder fetches the current status of a single parcel by barcode. It is the
// single-parcel counterpart of GetOrders, used by the manual "test" check.
func (c *MescolisClient) GetOrder(ctx context.Context, barcode string) (*MescolisOrderStatus, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("mescolis api key is not configured")
	}

	var out struct {
		Barcode                string `json:"barcode"`
		Status                 string `json:"status"`
		StatusLabel            string `json:"status_label"`
		DeliverymanName        string `json:"deliveryman_name,omitempty"`
		DeliverymanPhoneNumber string `json:"deliveryman_phone_number,omitempty"`
	}
	if err := c.do(ctx, "/orders/GetOrder", map[string]string{"barcode": barcode}, &out); err != nil {
		return nil, err
	}
	return &MescolisOrderStatus{
		Barcode:     out.Barcode,
		Status:      out.Status,
		StatusLabel: out.StatusLabel,
	}, nil
}

// Probe verifies the token reaches the API by calling the single-order
// endpoint with a sentinel barcode: any HTTP/parse success means the token is
// accepted, even when the parcel itself does not exist ("0" or not_found).
func (c *MescolisClient) Probe(ctx context.Context) error {
	if _, err := c.GetOrder(ctx, "__platform_probe__"); err != nil {
		return err
	}
	return nil
}

func (c *MescolisClient) do(ctx context.Context, path string, payload any, out any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(mescolisAccessTokenHdr, c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("mescolis: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeMescolisError(resp)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("mescolis: failed to decode response: %w", err)
	}
	return nil
}

func decodeMescolisError(resp *http.Response) error {
	var e mescolisAPIError
	data, err := io.ReadAll(resp.Body)
	if err == nil {
		if uerr := json.Unmarshal(data, &e); uerr == nil && e.Message != "" {
			return e
		}
		return fmt.Errorf("mescolis api error: %s", string(data))
	}
	return fmt.Errorf("mescolis api error: http %d", resp.StatusCode)
}

// StatusTerminal reports whether a Mes Colis status is terminal: no further
// progress is expected, so the poller stops watching the parcel.
func StatusTerminal(status string) bool {
	switch status {
	case "delivered", "delivered-and-paid", "return-sender", "final-return":
		return true
	}
	return false
}

// LabelFor maps a Mes Colis status to a human label when the API response does
// not carry status_label.
func LabelFor(status string) string {
	switch status {
	case "pending", "confirmed":
		return "Order accepted"
	case "in-progress":
		return "Out for delivery"
	case "delivered":
		return "Delivered"
	case "delivered-and-paid":
		return "Delivered and paid"
	case "return-sender":
		return "Return to sender"
	case "final-return":
		return "Returned"
	}
	if status == "" {
		return "Unknown"
	}
	return status
}

// KnownStatuses lists the delivery statuses the automation UI offers for
// Mes Colis, exactly as documented in "Documentation Socket" (Statuts list).
func KnownStatuses() []string {
	return []string{
		"pending",
		"to-be-picked-up",
		"picked-up",
		"at-agency",
		"return-agency",
		"in-progress",
		"to-be-verified",
		"delivered",
		"delivered-and-paid",
		"exchanged",
		"refunded",
		"final-return",
		"return-inter-agency",
		"return-sender",
		"return-received",
		"inter-depot",
		"unavailable-1",
		"unavailable-2",
		"paiement-received",
		"order-refund",
		"saisie-douane",
		"anomalie",
	}
}
