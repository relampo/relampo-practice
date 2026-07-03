package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// relampoSalt is deliberately public: it ships inside /static/app.js so the
// person scripting the load test can re-implement the transformation. The
// challenge is the correlation + client-side transform, not secrecy.
const relampoSalt = "relampo-public-salt-v1"

var jwtSecret = []byte("relampo-tickets-jwt-secret")

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// randomStateB64 uses standard base64 (with + / =) so the value needs URL
// encoding inside the Location header — forcing an url_decode transform on
// the extractor side, same as real OAuth state values.
func randomStateB64(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func randomUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// signRelampoToken is the server-side twin of signRelampoToken() in app.js:
// value B = hex(HMAC-SHA256(value A, relampoSalt)).
func signRelampoToken(tokenA string) string {
	mac := hmac.New(sha256.New, []byte(relampoSalt))
	mac.Write([]byte(tokenA))
	return hex.EncodeToString(mac.Sum(nil))
}

// --- Minimal HS256 JWT (issue + verify), no external dependencies ---

func b64url(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func issueJWT(username string, ttl time.Duration) string {
	header := b64url([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{
		"iss":   "RelampoTickets",
		"sub":   username,
		"scope": "tickets.browse tickets.buy",
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(ttl).Unix(),
	})
	signing := header + "." + b64url(claims)
	mac := hmac.New(sha256.New, jwtSecret)
	mac.Write([]byte(signing))
	return signing + "." + b64url(mac.Sum(nil))
}

func verifyJWT(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("malformed token")
	}
	signing := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, jwtSecret)
	mac.Write([]byte(signing))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return "", fmt.Errorf("invalid signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid payload")
	}
	var claims struct {
		Sub string `json:"sub"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("invalid claims")
	}
	if time.Now().Unix() > claims.Exp {
		return "", fmt.Errorf("token expired")
	}
	return claims.Sub, nil
}

// newViewState builds a large opaque base64 blob (~1.4 KB), in the spirit of
// JSF/ASP.NET view state: painful enough that it must be correlated, never typed.
func newViewState() string {
	b := make([]byte, 1024)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}
