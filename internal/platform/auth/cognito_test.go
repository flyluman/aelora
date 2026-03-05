package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestRSAKeyFromJWKValidation(t *testing.T) {
	if _, err := rsaKeyFromJWK("", ""); err == nil {
		t.Fatal("expected decode error")
	}

	n := base64.RawURLEncoding.EncodeToString([]byte{0x01, 0x00, 0x01})
	e := base64.RawURLEncoding.EncodeToString([]byte{0x01, 0x00, 0x01})
	key, err := rsaKeyFromJWK(n, e)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.N.Sign() == 0 || key.E == 0 {
		t.Fatal("expected valid rsa key values")
	}
}

func TestValidateTokenRejectsMissingInputs(t *testing.T) {
	v := NewCognitoValidator("us-east-1", "pool", "client")
	if _, err := v.ValidateToken(context.Background(), "", "d1"); err != ErrUnauthorized {
		t.Fatalf("expected unauthorized for empty token, got %v", err)
	}
	if _, err := v.ValidateToken(context.Background(), "Bearer abc", ""); err != ErrUnauthorized {
		t.Fatalf("expected unauthorized for empty device id, got %v", err)
	}
}

func TestLookupKeyUsesFreshCache(t *testing.T) {
	v := NewCognitoValidator("us-east-1", "pool", "client")
	v.keys["kid1"] = &rsaPublicKeyForTest
	v.fetchedAt = time.Now()
	key, err := v.lookupKey(context.Background(), "kid1")
	if err != nil || key == nil {
		t.Fatalf("expected cached key, got key=%v err=%v", key, err)
	}
}

func TestRefreshKeysNon200(t *testing.T) {
	v := NewCognitoValidator("us-east-1", "pool", "client")
	v.httpClient = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}),
	}
	if err := v.refreshKeys(context.Background()); err != ErrUnauthorized {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

func TestValidateTokenSuccessWithCachedKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	v := NewCognitoValidator("us-east-1", "pool", "client")
	v.keys["kid1"] = &key.PublicKey
	v.fetchedAt = time.Now()

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":              v.issuer,
		"aud":              "client",
		"token_use":        "access",
		"sub":              "u1",
		"cognito:username": "user-one",
	})
	tok.Header["kid"] = "kid1"
	raw, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	id, err := v.ValidateToken(context.Background(), "Bearer "+raw, "d1")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if id.UserID != "u1" || id.DeviceID != "d1" {
		t.Fatalf("unexpected identity: %+v", id)
	}
}

func TestValidateTokenRejectsInvalidClaims(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	v := NewCognitoValidator("us-east-1", "pool", "client")
	v.keys["kid1"] = &key.PublicKey
	v.fetchedAt = time.Now()

	cases := []jwt.MapClaims{
		{"iss": "bad", "aud": "client", "token_use": "access", "sub": "u1"},
		{"iss": v.issuer, "aud": "wrong", "token_use": "access", "sub": "u1"},
		{"iss": v.issuer, "aud": "client", "token_use": "id", "sub": "u1"},
		{"iss": v.issuer, "aud": "client", "token_use": "access", "sub": ""},
	}
	for i, claims := range cases {
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		tok.Header["kid"] = "kid1"
		raw, err := tok.SignedString(key)
		if err != nil {
			t.Fatalf("sign token %d: %v", i, err)
		}
		if _, err := v.ValidateToken(context.Background(), "Bearer "+raw, "d1"); err != ErrUnauthorized {
			t.Fatalf("case %d expected unauthorized, got %v", i, err)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

var rsaPublicKeyForTest = rsa.PublicKey{N: big.NewInt(3), E: 3}
