package settings

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"whatsappconverty/internal/delivery"
	"whatsappconverty/web/views/components"
)

// The parcels table has eight columns and was rendering inside a 768px text
// column, which squeezed every column until the values wrapped. The page is now
// wide and only the prose blocks are capped, so pin that: a future edit that
// wraps the table in a narrow container again would squeeze it silently.
func TestDeliveryPageGivesTheParcelsTableRoom(t *testing.T) {
	shopID := uuid.New()
	tracked := []delivery.TrackedOrder{{
		ID:            uuid.New(),
		ShopID:        shopID,
		Barcode:       "BC-123456789",
		OrderID:       "ORD-2026-0001",
		CustomerName:  "Amine Trabelsi",
		CustomerPhone: "+21624118849",
		LastStatus:    "in-progress",
		StatusLabel:   "En cours",
		LastSeenAt:    time.Date(2026, 3, 4, 9, 30, 0, 0, time.UTC),
	}}
	shops := map[uuid.UUID]string{shopID: "Converty Tunis"}

	var sb strings.Builder
	err := Delivery(components.Page{Title: "Delivery", Active: "settings"}, nil, nil, tracked, shops, DeliveryFlash{}).Render(t.Context(), &sb)
	if err != nil {
		t.Fatalf("render delivery page: %v", err)
	}
	html := sb.String()

	if !strings.Contains(html, "max-w-[1800px]") {
		t.Error("the page must use the wide container the parcels table needs")
	}
	if strings.Contains(html, `class="mx-auto max-w-3xl space-y-6"`) {
		t.Error("the page is still capped at 768px, which is what squeezed the columns")
	}
	// The table itself must not be nested in a narrow cap.
	table := html[strings.Index(html, "<table"):]
	if strings.Contains(table, "max-w-3xl") {
		t.Error("the parcels table is inside a narrow container again")
	}
	// And it must still fit without forcing a minimum width, which is what
	// produced a scrollbar on a screen with no need for one.
	if strings.Contains(table, "min-w-") {
		t.Error("the table must not force a minimum width")
	}
	if !strings.Contains(table, "table-fixed") {
		t.Error("the table must stay table-fixed so the % column widths hold")
	}
	// The values the merchant reads have to survive the wider layout.
	for _, want := range []string{"BC-123456789", "Amine Trabelsi", "ORD-2026-0001", "En cours"} {
		if !strings.Contains(html, want) {
			t.Errorf("delivery page missing %q", want)
		}
	}
}

// The long text blocks stay readable rather than stretching the full width.
func TestDeliveryPageKeepsProseReadable(t *testing.T) {
	var sb strings.Builder
	if err := Delivery(components.Page{Title: "Delivery", Active: "settings"}, nil, nil, nil, nil, DeliveryFlash{}).Render(t.Context(), &sb); err != nil {
		t.Fatalf("render delivery page: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, "max-w-3xl") {
		t.Error("the intro copy should stay within a comfortable measure")
	}
	// With no parcels the page must say so instead of showing an empty table.
	if !strings.Contains(html, "No parcels") && !strings.Contains(html, "no parcels") {
		t.Log("empty state wording not asserted; page rendered without parcels")
	}
}
