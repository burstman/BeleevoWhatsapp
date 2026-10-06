package viewsdashboard

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"whatsappconverty/internal/whatsapp"
	"whatsappconverty/web/views/components"
)

// The two-pane inbox must render a single nav badge id (the full page gets it
// from the nav, not from the sidebar fragment) and keep the sidebar + thread
// pane regions present.
func TestInboxTwoPaneRender(t *testing.T) {
	shopID := uuid.New()
	thread := whatsapp.Conversation{
		ID:             uuid.New(),
		ShopID:         shopID,
		CustomerPhone:  "21654116584",
		CustomerName:   "Ahmed",
		LastBody:       "Bonjour",
		LastMessageAt:  time.Now().Add(-time.Minute),
		UnreadCount:    2,
		WindowOpen:     true,
		LastDirection:  "inbound",
	}
	page := components.Page{Title: "Inbox", Active: "inbox", WhatsAppConnected: true, UnreadInbox: 2}

	var b strings.Builder
	if err := InboxPage(page, []whatsapp.Conversation{thread}, map[uuid.UUID]string{shopID: "Boutique"}, nil, "").Render(context.Background(), &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := b.String()

	if n := strings.Count(out, `id="inbox-nav-badge"`); n != 1 {
		t.Errorf("nav badge rendered %d times, want exactly 1 (no OOB strays on a full page)", n)
	}
	if !strings.Contains(out, `id="inbox-sidebar"`) {
		t.Error("sidebar container missing")
	}
	if !strings.Contains(out, `id="inbox-thread-pane"`) {
		t.Error("thread pane missing")
	}
	if !strings.Contains(out, "Select a conversation") {
		t.Error("no-thread placeholder missing")
	}
	if !strings.Contains(out, "Ahmed") || !strings.Contains(out, "Bonjour") {
		t.Error("conversation identity missing from messenger list")
	}
	if !strings.Contains(out, `hx-get="/inbox/sidebar"`) {
		t.Error("sidebar does not carry its polling trigger")
	}
}

// A full thread page must keep the sidebar and a single nav badge, with the
// active row highlighted and the thread card present.
func TestInboxThreadPageRender(t *testing.T) {
	shopID := uuid.New()
	conv := whatsapp.Conversation{
		ID:            uuid.New(),
		ShopID:        shopID,
		CustomerPhone: "21654116584",
		CustomerName:  "Ahmed",
		WindowOpen:    true,
	}
	msgs := []whatsapp.ChatMessage{
		{Direction: "inbound", Body: "Bonjour", CreatedAt: time.Now().Add(-time.Minute)},
		{Direction: "outbound", Body: "Oui ?", CreatedAt: time.Now()},
	}
	page := components.Page{Title: "Inbox", Active: "inbox", WhatsAppConnected: true}

	var b strings.Builder
	if err := InboxPage(page, []whatsapp.Conversation{conv}, map[uuid.UUID]string{shopID: "Boutique"}, &ActiveInboxThread{Conv: conv, Msgs: msgs}, "").Render(context.Background(), &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := b.String()

	if n := strings.Count(out, `id="inbox-nav-badge"`); n != 1 {
		t.Errorf("nav badge rendered %d times, want 1", n)
	}
	if !strings.Contains(out, `id="thread-card"`) {
		t.Error("thread card missing")
	}
	if !strings.Contains(out, `id="thread-bubbles"`) {
		t.Error("bubble area missing")
	}
	if !strings.Contains(out, "Bonjour") || !strings.Contains(out, "Oui ?") {
		t.Error("bubbles missing")
	}
	if !strings.Contains(out, "+21654116584") {
		t.Error("thread header phone missing")
	}
	if strings.Contains(out, `hx-swap-oob="outerHTML"`) {
		t.Error("full page must not emit OOB swaps (AJAX responses only)")
	}
}