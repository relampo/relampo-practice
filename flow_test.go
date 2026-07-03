package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestFullPurchaseFlow walks the complete correlated flow exactly like a load
// testing script would: extracting every dynamic value from the previous
// response and injecting it into the next request.
func TestFullPurchaseFlow(t *testing.T) {
	app := &App{store: NewStore(30 * time.Minute), startedAt: time.Now()}
	srv := httptest.NewServer(withLogging(newMux(app)))
	defer srv.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		// no seguir redirects: el "script" debe extraer state/code del Location
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	get := func(path string, hdr map[string]string) (*http.Response, string) {
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		var sb strings.Builder
		buf := make([]byte, 32*1024)
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

	post := func(path, contentType, body string, hdr map[string]string) (*http.Response, string) {
		req, _ := http.NewRequest("POST", srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", contentType)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		var sb strings.Builder
		buf := make([]byte, 32*1024)
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

	rx := func(pattern, s, what string) string {
		m := regexp.MustCompile(pattern).FindStringSubmatch(s)
		if m == nil {
			t.Fatalf("no pude extraer %s con %q de: %.300s", what, pattern, s)
		}
		return m[1]
	}

	// [1] GET / — única Set-Cookie de toda la app + csrf_token en HTML
	resp, body := get("/", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("home: %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Set-Cookie"), cookieName+"=") {
		t.Fatalf("home no emitió la cookie %s", cookieName)
	}
	csrf := rx(`name="csrf_token" value="([^"]+)"`, body, "csrf_token")

	// [2] GET /static/app.js — Etag + revalidación 304
	resp, _ = get("/static/app.js", nil)
	etag := resp.Header.Get("Etag")
	if etag == "" {
		t.Fatal("app.js sin Etag")
	}
	resp, _ = get("/static/app.js", map[string]string{"If-None-Match": etag})
	if resp.StatusCode != 304 {
		t.Fatalf("esperaba 304 con If-None-Match, recibí %d", resp.StatusCode)
	}

	// [3] POST /api/auth — bearer por jsonpath
	resp, body = post("/api/auth", "application/json",
		`{"username":"user042","password":"Pass042!"}`, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("auth: %d %s", resp.StatusCode, body)
	}
	var authResp struct{ Bearer string }
	json.Unmarshal([]byte(body), &authResp)
	if authResp.Bearer == "" {
		t.Fatal("sin bearer")
	}
	auth := map[string]string{"Authorization": "Bearer " + authResp.Bearer}

	// [4] GET /events — catalogId en JSON escapado dentro de atributo HTML
	_, body = get("/events", nil)
	rawCfg := rx(`data-config="([^"]+)"`, body, "data-config")
	var cfg struct {
		CatalogID string `json:"catalogId"`
	}
	unescaped := strings.NewReplacer("&#34;", `"`, "&quot;", `"`, "&amp;", "&").Replace(rawCfg)
	if err := json.Unmarshal([]byte(unescaped), &cfg); err != nil {
		t.Fatalf("config embebida ilegible: %v (%s)", err, unescaped)
	}

	// [5] GET /api/events?catalogId= — query param + array JSON
	resp, body = get("/api/events?catalogId="+url.QueryEscape(cfg.CatalogID), auth)
	if resp.StatusCode != 200 {
		t.Fatalf("events: %d %s", resp.StatusCode, body)
	}
	var evList struct {
		Events []struct{ ID string }
	}
	json.Unmarshal([]byte(body), &evList)
	eventID := evList.Events[3].ID

	// [6] GET /api/events/{id}/seats — path param, X-Correlation-Id en header
	resp, body = get("/api/events/"+eventID+"/seats", auth)
	if resp.StatusCode != 200 {
		t.Fatalf("seats: %d %s", resp.StatusCode, body)
	}
	corr := resp.Header.Get(correlationHeader)
	if corr == "" {
		t.Fatal("sin X-Correlation-Id")
	}
	var seatList struct {
		Seats []struct{ ID string }
	}
	json.Unmarshal([]byte(body), &seatList)
	seatID := seatList.Seats[7].ID

	// [7] POST /api/reservations — exige el header correlacionado
	hdrs := map[string]string{
		"Authorization":   "Bearer " + authResp.Bearer,
		correlationHeader: corr,
	}
	resp, body = post("/api/reservations", "application/json",
		fmt.Sprintf(`{"eventId":%q,"seatId":%q}`, eventID, seatID), hdrs)
	if resp.StatusCode != 201 {
		t.Fatalf("reservations: %d %s", resp.StatusCode, body)
	}
	var resv struct {
		ReservationID string `json:"reservationId"`
		RelampoToken  string `json:"relampoToken"`
	}
	json.Unmarshal([]byte(body), &resv)

	// [8a] POST /pay/start con el token CRUDO → debe rechazar con 403
	form := url.Values{"reservation_id": {resv.ReservationID}, "relampo_token": {resv.RelampoToken}}
	resp, body = post("/pay/start", "application/x-www-form-urlencoded", form.Encode(),
		map[string]string{hintsHeader: "true"})
	if resp.StatusCode != 403 || !strings.Contains(body, "CRUDO") {
		t.Fatalf("el token crudo debía rechazarse con 403: %d %s", resp.StatusCode, body)
	}

	// [8b] POST /pay/start con B = HMAC(A) → 302 con state en el Location
	form.Set("relampo_token", signRelampoToken(resv.RelampoToken))
	resp, _ = post("/pay/start", "application/x-www-form-urlencoded", form.Encode(), nil)
	if resp.StatusCode != 302 {
		t.Fatalf("pay/start: %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	stateEnc := rx(`[?&]state=([^&#]+)`, loc, "state")
	state, _ := url.QueryUnescape(stateEnc) // transform url_decode
	if state == stateEnc && strings.ContainsAny(state, "+/=") == false {
		t.Logf("aviso: state sin caracteres que exijan url_decode: %q", state)
	}

	// [8c] reutilizar el token debe fallar (un solo uso)
	resp, body = post("/pay/start", "application/x-www-form-urlencoded", form.Encode(), nil)
	if resp.StatusCode != 403 || !strings.Contains(body, "usado") {
		t.Fatalf("la reutilización debía dar 403: %d %s", resp.StatusCode, body)
	}

	// [9] GET /pay/authorize?state= — auto-submit form con 2 inputs ocultos
	resp, body = get("/pay/authorize?state="+url.QueryEscape(state), nil)
	if resp.StatusCode != 200 {
		t.Fatalf("authorize: %d %s", resp.StatusCode, body)
	}
	reqBlob := rx(`name="request" value="([^"]+)"`, body, "request")
	xcorr := rx(`name="x_correlation_id" value="([^"]+)"`, body, "x_correlation_id")
	// los valores en HTML viajan escapados (base64 puede traer + / =); des-escapar
	htmlUnescape := strings.NewReplacer("&#43;", "+", "&#61;", "=", "&#47;", "/", "&amp;", "&")
	reqBlob = htmlUnescape.Replace(reqBlob)

	// [10] POST /pay/continue — 302 con code en el Location
	form = url.Values{"request": {reqBlob}, "x_correlation_id": {xcorr}}
	resp, body = post("/pay/continue", "application/x-www-form-urlencoded", form.Encode(), nil)
	if resp.StatusCode != 302 {
		t.Fatalf("continue: %d %s", resp.StatusCode, body)
	}
	loc = resp.Header.Get("Location")
	code := rx(`[?&]code=([^&#]+)`, loc, "code")

	// [11] GET /pay/callback — view_state gigante en input oculto
	resp, body = get("/pay/callback?code="+code+"&state="+url.QueryEscape(state), nil)
	if resp.StatusCode != 200 {
		t.Fatalf("callback: %d %s", resp.StatusCode, body)
	}
	viewState := htmlUnescape.Replace(rx(`name="view_state" value="([^"]+)"`, body, "view_state"))
	if len(viewState) < 1000 {
		t.Fatalf("view_state sospechosamente corto: %d", len(viewState))
	}

	// [12a] confirmar SIN csrf → 403 didáctico
	form = url.Values{
		"view_state":     {viewState},
		"code":           {code},
		"reservation_id": {resv.ReservationID},
	}
	resp, body = post("/pay/confirm", "application/x-www-form-urlencoded", form.Encode(), nil)
	if resp.StatusCode != 403 || !strings.Contains(body, "csrf_token") {
		t.Fatalf("sin csrf debía dar 403 nombrando csrf_token: %d %s", resp.StatusCode, body)
	}

	// [12b] confirmar con todo → página de éxito con data-ticket
	form.Set("csrf_token", csrf)
	resp, body = post("/pay/confirm", "application/x-www-form-urlencoded", form.Encode(), nil)
	if resp.StatusCode != 200 {
		t.Fatalf("confirm: %d %s", resp.StatusCode, body)
	}
	ticketID := rx(`data-ticket="([^"]+)"`, body, "ticket_id")

	// [13] GET /api/tickets/{id} — verificación final
	resp, body = get("/api/tickets/"+ticketID, auth)
	if resp.StatusCode != 200 || !strings.Contains(body, "CONFIRMED") {
		t.Fatalf("ticket: %d %s", resp.StatusCode, body)
	}

	// [14] logout y sesión muerta
	get("/logout", nil)
	resp, _ = get("/events", nil)
	if resp.StatusCode == 200 {
		t.Fatal("la sesión debía quedar invalidada tras logout")
	}
}

// TestJSAndGoHMACMatch pins the contract between signRelampoToken() in Go and
// the JS twin embedded in app.js (verified externally against node:crypto).
func TestJSAndGoHMACMatch(t *testing.T) {
	got := signRelampoToken("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	want := "714c0204af3371804c1df9b31acd262b76e945ef2e3bc84f352e8dbe8d27f11e"
	if got != want {
		t.Fatalf("HMAC Go≠JS: got %s want %s", got, want)
	}
}

func TestCredentials(t *testing.T) {
	if !checkCredentials("user001", "Pass001!") || !checkCredentials("user250", "Pass250!") ||
		!checkCredentials("user500", "Pass500!") {
		t.Fatal("credenciales válidas rechazadas")
	}
	if checkCredentials("user501", "Pass501!") || checkCredentials("user001", "Pass002!") {
		t.Fatal("credenciales inválidas aceptadas")
	}
}
