package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrUnauthorized = errors.New("unauthorized")

type Identity struct {
	UserID   string
	Username string
	DeviceID string
}

type Validator interface {
	ValidateToken(ctx context.Context, rawToken, deviceID string) (Identity, error)
}

type CognitoValidator struct {
	httpClient *http.Client
	issuer     string
	appClient  string
	jwksURL    string

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

func NewCognitoValidator(region, userPoolID, appClientID string) *CognitoValidator {
	issuer := fmt.Sprintf("https://cognito-idp.%s.amazonaws.com/%s", region, userPoolID)
	return &CognitoValidator{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		issuer:     issuer,
		appClient:  appClientID,
		jwksURL:    issuer + "/.well-known/jwks.json",
		keys:       make(map[string]*rsa.PublicKey),
	}
}

func (v *CognitoValidator) ValidateToken(ctx context.Context, rawToken, deviceID string) (Identity, error) {
	token := strings.TrimSpace(strings.TrimPrefix(rawToken, "Bearer "))
	if token == "" {
		return Identity{}, ErrUnauthorized
	}
	if strings.TrimSpace(deviceID) == "" {
		return Identity{}, ErrUnauthorized
	}

	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, ErrUnauthorized
		}
		key, err := v.lookupKey(ctx, kid)
		if err != nil {
			return nil, err
		}
		return key, nil
	}, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil || !parsed.Valid {
		return Identity{}, ErrUnauthorized
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return Identity{}, ErrUnauthorized
	}

	iss, _ := claims["iss"].(string)
	if iss != v.issuer {
		return Identity{}, ErrUnauthorized
	}

	if v.appClient != "" {
		aud, _ := claims["aud"].(string)
		clientID, _ := claims["client_id"].(string)
		if aud != v.appClient && clientID != v.appClient {
			return Identity{}, ErrUnauthorized
		}
	}
	tokenUse, _ := claims["token_use"].(string)
	if tokenUse != "access" {
		return Identity{}, ErrUnauthorized
	}

	sub, _ := claims["sub"].(string)
	username, _ := claims["cognito:username"].(string)
	if sub == "" {
		return Identity{}, ErrUnauthorized
	}
	return Identity{UserID: sub, Username: username, DeviceID: deviceID}, nil
}

func (v *CognitoValidator) lookupKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	if key, ok := v.keys[kid]; ok && time.Since(v.fetchedAt) < 10*time.Minute {
		v.mu.RUnlock()
		return key, nil
	}
	v.mu.RUnlock()

	if err := v.refreshKeys(ctx); err != nil {
		return nil, err
	}

	v.mu.RLock()
	defer v.mu.RUnlock()
	key, ok := v.keys[kid]
	if !ok {
		return nil, ErrUnauthorized
	}
	return key, nil
}

func (v *CognitoValidator) refreshKeys(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ErrUnauthorized
	}

	var payload struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}

	next := make(map[string]*rsa.PublicKey)
	for _, key := range payload.Keys {
		if key.Kty != "RSA" || key.Kid == "" {
			continue
		}
		pub, err := rsaKeyFromJWK(key.N, key.E)
		if err != nil {
			continue
		}
		next[key.Kid] = pub
	}
	if len(next) == 0 {
		return ErrUnauthorized
	}

	v.mu.Lock()
	v.keys = next
	v.fetchedAt = time.Now()
	v.mu.Unlock()
	return nil
}

func rsaKeyFromJWK(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, err
	}

	n := new(big.Int).SetBytes(nBytes)
	e := 0
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}
	if n.Sign() == 0 || e == 0 {
		return nil, errors.New("invalid rsa jwk")
	}
	return &rsa.PublicKey{N: n, E: e}, nil
}
