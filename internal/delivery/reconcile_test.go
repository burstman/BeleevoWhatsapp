package delivery

import (
	"testing"

	"github.com/google/uuid"
)

// The Mes Colis token belongs to the operator, not to one shop, so a parcel that
// shop B registered must still be polled through the account shop A connected.
// Getting this wrong means the poller silently watches nothing.
func TestPickConnectionUsesSharedAccountAcrossShops(t *testing.T) {
	owner := uuid.New()
	other := uuid.New()

	own := map[uuid.UUID]connection{
		owner: {shop: owner, token: "shared-token"},
	}
	shared := []connection{{shop: owner, token: "shared-token"}}

	c, ok := pickConnection(other, own, shared)
	if !ok {
		t.Fatal("a parcel of a shop without its own connection must still be polled")
	}
	if c.token != "shared-token" {
		t.Fatalf("polled with token %q, want the operator's shared account", c.token)
	}

	c, ok = pickConnection(owner, own, shared)
	if !ok || c.token != "shared-token" {
		t.Fatalf("the owning shop must poll its own connection, got ok=%v", ok)
	}
}

func TestPickConnectionPrefersTheShopsOwnAccount(t *testing.T) {
	owner := uuid.New()
	own := uuid.New()

	ownMap := map[uuid.UUID]connection{
		owner: {shop: owner, token: "operator-token"},
		own:   {shop: own, token: "own-token"},
	}

	c, ok := pickConnection(own, ownMap, []connection{ownMap[owner]})
	if !ok {
		t.Fatal("a shop with its own connection must be polled with it")
	}
	if c.token != "own-token" {
		t.Fatalf("polled with token %q, want the shop's own account", c.token)
	}
}

func TestPickConnectionWithoutAnyAccount(t *testing.T) {
	if _, ok := pickConnection(uuid.New(), map[uuid.UUID]connection{}, nil); ok {
		t.Fatal("no connection means nothing to poll")
	}
}

func TestRemovedUpstreamIsTerminalButNotTriggerable(t *testing.T) {
	if !StatusTerminal(StatusRemovedUpstream) {
		t.Fatal("a parcel the carrier dropped must stop being polled")
	}
	for _, s := range KnownStatuses() {
		if s == StatusRemovedUpstream {
			t.Fatal("removed-upstream must never be offered as an automation trigger")
		}
	}
	if got := LabelFor(StatusRemovedUpstream); got != "Removed at carrier" {
		t.Fatalf("label = %q", got)
	}
}

func TestMissesBeforeSettledSurvivesASighting(t *testing.T) {
	// The threshold only exists to ride out replication lag; a value of 1 would
	// end tracking on a single unlucky answer.
	if MissesBeforeSettled < 2 {
		t.Fatalf("MissesBeforeSettled = %d, want at least 2", MissesBeforeSettled)
	}
}
