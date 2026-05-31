package jwt

import (
	"testing"
	"time"
)

func TestSignParse_RoundTrip(t *testing.T) {
	secret := []byte("test-secret")
	token, exp, err := Sign("user-123", time.Hour, secret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if exp.Before(time.Now()) {
		t.Errorf("expiresAt should be in the future")
	}
	sub, err := Parse(token, secret)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sub != "user-123" {
		t.Errorf("subject = %q, want user-123", sub)
	}
}

func TestParse_WrongSecret(t *testing.T) {
	token, _, _ := Sign("user-123", time.Hour, []byte("real-secret"))
	if _, err := Parse(token, []byte("other-secret")); err == nil {
		t.Fatal("expected error with wrong secret")
	}
}

func TestParse_Expired(t *testing.T) {
	token, _, _ := Sign("user-123", -time.Minute, []byte("s"))
	if _, err := Parse(token, []byte("s")); err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestParse_Garbage(t *testing.T) {
	if _, err := Parse("not-a-jwt", []byte("s")); err == nil {
		t.Fatal("expected error for garbage token")
	}
}
