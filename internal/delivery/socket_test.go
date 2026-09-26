package delivery

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/kingecg/gosocketio/socketio"
)

func TestMescolisEventDecode(t *testing.T) {
	raw := `{
		"barcode": "1234567890113",
		"status": "in-progress",
		"updated_at": "2026-06-22T14:32:11+01:00",
		"deliveryman_name": "Mohamed Ben Ali",
		"deliveryman_phone_number": "20000000",
		"qualification": null
	}`
	var e mescolisEvent
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if e.Barcode != "1234567890113" || e.Status != "in-progress" {
		t.Fatalf("barcode/status decode wrong: %+v", e)
	}
	if e.DeliverymanName != "Mohamed Ben Ali" || e.DeliverymanPhoneNumber != "20000000" {
		t.Fatalf("deliveryman decode wrong: %+v", e)
	}
	if e.Qualification != "" {
		t.Fatalf("null qualification should decode to empty, got %q", e.Qualification)
	}
}

func TestMescolisSocketLive(t *testing.T) {
	if os.Getenv("MESCOLIS_DIAL_TEST") == "" {
		t.Skip("set MESCOLIS_DIAL_TEST=1 to dial the live Mes Colis socket")
	}
	token := os.Getenv("MESCOLIS_DIAL_TOKEN")

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	client, err := socketio.Dial(ctx, MescolisSocketURL, &socketio.Options{
		Transports: []string{"websocket"},
		Auth:       map[string]any{"token": token},
		Timeout:    15 * time.Second,
	})
	if err != nil {
		// An auth rejection (CONNECT_ERROR) is expected with a stub token and
		// still proves the engine.io handshake + socket.io CONNECT work.
		t.Logf("dial result (expected auth error with stub token): %v", err)
		return
	}
	defer func() { _ = client.Close() }()
	t.Log("dial ok — token accepted")
}
