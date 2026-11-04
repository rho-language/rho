package auth

import (
	"testing"
	"time"
)

func TestTicketRoundTrip(t *testing.T) {
	pair, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	token, err := SignTicket(pair.Private, TicketClaims{
		Subject: "miraur", Issuer: "keeper", Audience: "aquarium",
		TokenID: "one", IssuedAt: now.Unix(), ExpiresAt: now.Add(30 * time.Second).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := VerifyTicket(pair.Public, token, "keeper", "aquarium", now)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "miraur" || claims.TokenID != "one" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if _, err := VerifyTicket(pair.Public, token, "keeper", "mesh", now); err == nil {
		t.Fatal("a ticket for another audience was accepted")
	}
	if _, err := VerifyTicket(pair.Public, token, "keeper", "aquarium", now.Add(31*time.Second)); err == nil {
		t.Fatal("an expired ticket was accepted")
	}
	if _, err := VerifyTicket(pair.Public, token+"x", "keeper", "aquarium", now); err == nil {
		t.Fatal("a tampered ticket was accepted")
	}
}
