package converty

import "testing"

func TestDecodeWebhookPrefersReference(t *testing.T) {
	body := []byte(`{
		"event": "order.update",
		"data": {
			"_id": "650fed789012345678901234",
			"reference": 1042,
			"barcode": "916749412151",
			"status": "confirmed",
			"customer": {"name": "Ahmed Ben Ali", "phone": "0555123456"}
		}
	}`)
	d := decodeWebhook(body)
	if d.OrderID != "1042" {
		t.Fatalf("OrderID = %q, want the human reference 1042", d.OrderID)
	}
	if d.Barcode != "916749412151" {
		t.Fatalf("Barcode = %q", d.Barcode)
	}
	if d.CustomerName != "Ahmed Ben Ali" || d.CustomerPhone != "0555123456" {
		t.Fatalf("customer = %q / %q", d.CustomerName, d.CustomerPhone)
	}
}

func TestDecodeWebhookFallsBackToID(t *testing.T) {
	d := decodeWebhook([]byte(`{"data": {"_id": "650fed789012345678901234", "status": "pending"}}`))
	if d.OrderID != "650fed789012345678901234" {
		t.Fatalf("OrderID = %q, want the _id fallback", d.OrderID)
	}
}

func TestPickReferenceSkipsZero(t *testing.T) {
	if got := pickReference(map[string]any{"data": map[string]any{"reference": float64(0)}}); got != "" {
		t.Fatalf("zero reference = %q, want empty", got)
	}
	if got := pickReference(map[string]any{"data": map[string]any{"reference": float64(1751)}}); got != "1751" {
		t.Fatalf("reference = %q, want 1751", got)
	}
}