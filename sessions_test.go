package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSessionsPerNodeLimit: máximo 5 sesiones concurrentes por nodo (IP),
// con desalojo de sesiones zombi (VUs abandonados sin /logout).
func TestSessionsPerNodeLimit(t *testing.T) {
	app := &App{store: NewStore(30 * time.Minute), startedAt: time.Now(), appJS: buildAppJS(tokenMode)}
	srv := httptest.NewServer(newMux(app))
	defer srv.Close()

	home := func(xff string) (*http.Response, string) {
		req, _ := http.NewRequest("GET", srv.URL+"/", nil)
		req.Header.Set("X-Forwarded-For", xff)
		resp, err := http.DefaultTransport.RoundTrip(req) // sin cookies: cada GET / crea sesión
		if err != nil {
			t.Fatal(err)
		}
		var sb strings.Builder
		buf := make([]byte, 8*1024)
		for {
			n, err := resp.Body.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				break
			}
		}
		resp.Body.Close()
		return resp, sb.String()
	}

	// 5 sesiones desde el nodo 10.0.0.1 → OK
	for i := 0; i < 5; i++ {
		if resp, _ := home("10.0.0.1"); resp.StatusCode != 200 {
			t.Fatalf("sesión %d del nodo debía ser 200: %d", i+1, resp.StatusCode)
		}
	}

	// la sexta desde el mismo nodo → 429 didáctico
	resp, body := home("10.0.0.1")
	if resp.StatusCode != 429 || !strings.Contains(body, "vus_per_node") {
		t.Fatalf("la 6ª sesión debía dar 429 vus_per_node: %d %s", resp.StatusCode, body)
	}

	// otro nodo (otra IP) no está afectado
	if resp, _ := home("10.0.0.2"); resp.StatusCode != 200 {
		t.Fatalf("otro nodo debía poder crear sesión: %d", resp.StatusCode)
	}

	// zombi: un VU del nodo 1 dejó de pegar requests hace 2 minutos
	app.store.mu.Lock()
	for _, s := range app.store.sessions {
		if s.IP == "10.0.0.1" {
			s.LastSeen = time.Now().Add(-2 * time.Minute)
			break
		}
	}
	app.store.mu.Unlock()

	// ahora la sesión nueva desaloja al zombi y entra
	if resp, _ := home("10.0.0.1"); resp.StatusCode != 200 {
		t.Fatalf("con un zombi desalojable debía entrar: %d", resp.StatusCode)
	}
	if got := app.store.CountByIP("10.0.0.1"); got != 5 {
		t.Fatalf("el nodo debía seguir en 5 sesiones, tiene %d", got)
	}

	// /logout libera el cupo al instante
	// (tomar una cookie válida: crear sesión desde nodo 3 y cerrarla)
	req, _ := http.NewRequest("GET", srv.URL+"/", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.3")
	r3, _ := http.DefaultTransport.RoundTrip(req)
	r3.Body.Close()
	cookie := r3.Header.Get("Set-Cookie")
	if before := app.store.CountByIP("10.0.0.3"); before != 1 {
		t.Fatalf("nodo 3 debía tener 1 sesión, tiene %d", before)
	}
	lo, _ := http.NewRequest("GET", srv.URL+"/logout", nil)
	lo.Header.Set("Cookie", strings.Split(cookie, ";")[0])
	lo.Header.Set("X-Forwarded-For", "10.0.0.3")
	rl, _ := http.DefaultTransport.RoundTrip(lo)
	rl.Body.Close()
	if after := app.store.CountByIP("10.0.0.3"); after != 0 {
		t.Fatalf("tras logout el nodo 3 debía tener 0 sesiones, tiene %d", after)
	}
}
