package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

type App struct {
	store          *Store
	startedAt      time.Time
	appJS          string
	receiptCounter atomic.Int64
}

const (
	cookieName        = "relampo_session"
	relampoTokenTTL   = 60 * time.Second
	payFlowTTL        = 5 * time.Minute
	bearerTTL         = 2 * time.Hour
	hintsHeader       = "X-Practice-Hints"
	correlationHeader = "X-Correlation-Id"
)

var eventCatalog = []struct {
	Name  string
	Venue string
	Date  string
}{
	{"Noche de Rock Nacional", "Estadio Centenario", "2026-08-15"},
	{"Clásico de Fútbol - Final", "Estadio Campeón del Siglo", "2026-08-22"},
	{"Festival Electrónico Voltaje", "Rural del Prado", "2026-09-05"},
	{"La Tormenta (teatro)", "Teatro Solís", "2026-09-12"},
	{"Stand Up: Riendo Bajo la Lluvia", "Sala del Museo", "2026-09-19"},
	{"Sinfónica: Beethoven 9", "Auditorio Nacional Sodre", "2026-10-03"},
	{"Feria Gamer Expo", "Antel Arena", "2026-10-17"},
	{"Jazz en el Puerto", "Teatro de Verano", "2026-10-24"},
}

// ---------------------------------------------------------------- helpers

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeErr emits a didactic error: it always names the variable that failed;
// with "X-Practice-Hints: true" it also says where to find/extract it.
func writeErr(w http.ResponseWriter, r *http.Request, status int, variable, msg, hint string) {
	body := map[string]any{
		"error":    msg,
		"variable": variable,
		"status":   status,
	}
	if hint != "" && r.Header.Get(hintsHeader) == "true" {
		body["hint"] = hint
	}
	writeJSON(w, status, body)
}

// preview truncates a value for error messages: enough to recognize what was
// sent/expected without handing over the full answer.
func preview(v string) string {
	if v == "" {
		return "(vacío o ausente)"
	}
	r := []rune(v)
	if len(r) > 24 {
		return string(r[:24]) + fmt.Sprintf("… (%d caracteres)", len(r))
	}
	return v
}

// writeCorrErr reports a failed correlation: the error states explicitly that
// the correlated value is wrong and shows received vs expected previews.
// Pass expected values raw; descriptive placeholders go in parentheses.
func writeCorrErr(w http.ResponseWriter, r *http.Request, status int, variable, msg, hint, received, expected string) {
	expShown := expected
	if !strings.HasPrefix(expected, "(") {
		expShown = preview(expected)
	}
	body := map[string]any{
		"error":            "valor correlacionado incorrecto: " + msg,
		"variable":         variable,
		"correlationError": true,
		"received":         preview(received),
		"expected":         expShown,
		"status":           status,
	}
	if hint != "" && r.Header.Get(hintsHeader) == "true" {
		body["hint"] = hint
	}
	writeJSON(w, status, body)
}

func (app *App) session(r *http.Request) *Session {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil
	}
	return app.store.Get(c.Value)
}

func (app *App) requireSession(w http.ResponseWriter, r *http.Request) *Session {
	s := app.session(r)
	if s == nil {
		writeErr(w, r, http.StatusUnauthorized, cookieName,
			"falta la cookie de sesión o la sesión expiró",
			"la cookie relampo_session se emite en el Set-Cookie de la primera respuesta de GET / y debe enviarse en todos los requests siguientes")
	}
	return s
}

func (app *App) requireBearer(w http.ResponseWriter, r *http.Request) *Session {
	s := app.requireSession(w, r)
	if s == nil {
		return nil
	}
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(auth) <= len(prefix) || auth[:len(prefix)] != prefix {
		writeErr(w, r, http.StatusUnauthorized, "bearer",
			"falta el header Authorization: Bearer <token>",
			"el token bearer se obtiene con jsonpath $.bearer del body JSON de POST /api/auth")
		return nil
	}
	token := auth[len(prefix):]
	if _, err := verifyJWT(token); err != nil {
		writeCorrErr(w, r, http.StatusUnauthorized, "bearer",
			"el token bearer enviado no es válido ("+err.Error()+")",
			"correlaciona el valor exacto devuelto por POST /api/auth; no lo edites ni lo recortes",
			token, s.Bearer)
		return nil
	}
	if token != s.Bearer {
		writeCorrErr(w, r, http.StatusUnauthorized, "bearer",
			"el token bearer enviado no corresponde al emitido para esta sesión",
			"cada sesión (cookie) recibe su propio bearer en POST /api/auth; no mezcles valores de otra sesión ni valores grabados",
			token, s.Bearer)
		return nil
	}
	return s
}

func seatPrice(seatID string) int {
	sum := 0
	for _, b := range []byte(seatID) {
		sum += int(b)
	}
	return 1200 + (sum%9)*150
}

// ---------------------------------------------------------------- portal

// GET / — the ONLY endpoint that ever sets a cookie. One cookie for the whole
// app, born on the first home response, sent back on everything else.
func (app *App) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	s := app.session(r)
	if s == nil {
		s = app.store.Create()
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    s.ID,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	homeTmpl.Execute(w, map[string]any{"CSRF": s.CSRFToken})
}

// GET /static/app.js — served with an ETag so browsers revalidate with
// If-None-Match and get a 304 (the Etag correlation case).
func (app *App) staticAppJS(w http.ResponseWriter, r *http.Request) {
	sum := sha256.Sum256([]byte(app.appJS))
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("Etag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Write([]byte(app.appJS))
}

// GET /events — página del catálogo; siembra catálogo y eventos por sesión.
func (app *App) eventsPage(w http.ResponseWriter, r *http.Request) {
	s := app.requireSession(w, r)
	if s == nil {
		return
	}
	if s.User == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	app.store.Lock()
	if s.CatalogID == "" {
		s.CatalogID = "CAT-" + randomHex(6)
		for _, ec := range eventCatalog {
			ev := &Event{
				ID:    "EV-" + randomUUID(),
				Name:  ec.Name,
				Venue: ec.Venue,
				Date:  ec.Date,
			}
			for row := 0; row < 3; row++ {
				for n := 1; n <= 4; n++ {
					ev.Seats = append(ev.Seats, fmt.Sprintf("S-%c%d-%s", 'A'+row, n, randomHex(3)))
				}
			}
			s.Events = append(s.Events, ev)
		}
	}
	s.Step = "browsing"
	cfg, _ := json.Marshal(map[string]any{
		"catalogId": s.CatalogID,
		"currency":  "UYU",
		"locale":    "es-UY",
	})
	app.store.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	eventsTmpl.Execute(w, map[string]any{
		"User":       s.User,
		"ConfigJSON": string(cfg),
	})
}

// GET /logout — invalida server-side; no emite Set-Cookie (la única cookie de
// la app nace en GET / y nada más la toca).
func (app *App) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		app.store.Delete(c.Value)
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (app *App) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"app":           "RelampoTickets",
		"uptimeSeconds": int(time.Since(app.startedAt).Seconds()),
		"users":         userCount,
	})
}

func (app *App) debugSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sessions": app.store.Snapshot()})
}

// ---------------------------------------------------------------- API

// POST /api/auth — login JSON; devuelve el bearer (jsonpath $.bearer).
func (app *App) apiAuth(w http.ResponseWriter, r *http.Request) {
	s := app.requireSession(w, r)
	if s == nil {
		return
	}
	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		writeErr(w, r, http.StatusBadRequest, "body",
			"body JSON inválido", "envía {\"username\":\"user001\",\"password\":\"Pass001!\"}")
		return
	}
	if !checkCredentials(creds.Username, creds.Password) {
		writeErr(w, r, http.StatusUnauthorized, "credentials",
			"usuario o contraseña incorrectos",
			"usuarios válidos: user001…user500 con Pass001!…Pass500!; descarga /users.csv")
		return
	}
	bearer := issueJWT(creds.Username, bearerTTL)
	app.store.Lock()
	s.User = creds.Username
	s.Bearer = bearer
	s.Step = "logged_in"
	app.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"bearer":     bearer,
		"tokenType":  "Bearer",
		"expSeconds": int(bearerTTL.Seconds()),
		"issuedAt":   time.Now().UTC().Format(time.RFC3339),
		"user": map[string]any{
			"username":    creds.Username,
			"displayName": "Usuario " + creds.Username[4:],
		},
	})
}

// GET /api/events?catalogId=… — el catalogId viaja como query parameter.
func (app *App) apiEvents(w http.ResponseWriter, r *http.Request) {
	s := app.requireBearer(w, r)
	if s == nil {
		return
	}
	catalogID := r.URL.Query().Get("catalogId")
	if catalogID == "" || catalogID != s.CatalogID {
		writeCorrErr(w, r, http.StatusBadRequest, "catalog_id",
			"el catalogId enviado no coincide con el emitido para esta sesión",
			"extrae el catalogId del JSON escapado en el atributo data-config del div#app en GET /events (regex + unescape)",
			catalogID, s.CatalogID)
		return
	}
	type evJSON struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Venue     string `json:"venue"`
		Date      string `json:"date"`
		PriceFrom int    `json:"priceFrom"`
	}
	out := make([]evJSON, 0, len(s.Events))
	for _, ev := range s.Events {
		out = append(out, evJSON{ev.ID, ev.Name, ev.Venue, ev.Date, seatPrice(ev.Seats[0])})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"catalogId": catalogID,
		"count":     len(out),
		"events":    out,
	})
}

// GET /api/events/{id}/seats — el event_id viaja en el PATH de la URL; la
// respuesta entrega X-Correlation-Id como header.
func (app *App) apiSeats(w http.ResponseWriter, r *http.Request) {
	s := app.requireBearer(w, r)
	if s == nil {
		return
	}
	ev := s.findEvent(r.PathValue("id"))
	if ev == nil {
		writeCorrErr(w, r, http.StatusNotFound, "event_id",
			"el event_id enviado en el path no corresponde a ningún evento de esta sesión",
			"extrae un id del array $.events[*].id de GET /api/events (elige una ocurrencia aleatoria)",
			r.PathValue("id"), "(uno de los id de $.events[*].id de GET /api/events)")
		return
	}
	app.store.Lock()
	s.CorrelationID = randomUUID()
	s.Step = "choosing_seat"
	corr := s.CorrelationID
	app.store.Unlock()
	type seatJSON struct {
		ID    string `json:"id"`
		Row   string `json:"row"`
		Price int    `json:"price"`
	}
	seats := make([]seatJSON, 0, len(ev.Seats))
	for _, id := range ev.Seats {
		seats = append(seats, seatJSON{id, id[2:4], seatPrice(id)})
	}
	w.Header().Set(correlationHeader, corr)
	writeJSON(w, http.StatusOK, map[string]any{
		"eventId": ev.ID,
		"seats":   seats,
	})
}

// POST /api/reservations — exige el X-Correlation-Id recibido como header en
// la respuesta de seats; devuelve el relampo_token (valor A, un solo uso, 60s).
func (app *App) apiReservations(w http.ResponseWriter, r *http.Request) {
	s := app.requireBearer(w, r)
	if s == nil {
		return
	}
	if got := r.Header.Get(correlationHeader); got == "" || got != s.CorrelationID {
		writeCorrErr(w, r, http.StatusForbidden, "x_correlation_id",
			"el header X-Correlation-Id enviado no coincide con el emitido en la respuesta de seats",
			"correlaciona el header X-Correlation-Id de la RESPUESTA de GET /api/events/{id}/seats y reenvíalo como header de este request",
			got, s.CorrelationID)
		return
	}
	var body struct {
		EventID string `json:"eventId"`
		SeatID  string `json:"seatId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, r, http.StatusBadRequest, "body", "body JSON inválido", "")
		return
	}
	ev := s.findEvent(body.EventID)
	if ev == nil {
		writeCorrErr(w, r, http.StatusBadRequest, "event_id",
			"el eventId enviado no corresponde a ningún evento de esta sesión", "",
			body.EventID, "(uno de los id de $.events[*].id de GET /api/events)")
		return
	}
	seatOK := false
	for _, id := range ev.Seats {
		if id == body.SeatID {
			seatOK = true
			break
		}
	}
	if !seatOK {
		writeCorrErr(w, r, http.StatusBadRequest, "seat_id",
			"el seatId enviado no pertenece al evento",
			"extrae un id del array $.seats[*].id de GET /api/events/{id}/seats",
			body.SeatID, "(uno de los id de $.seats[*].id de GET /api/events/{id}/seats)")
		return
	}
	res := &Reservation{
		ID:           "RES-" + randomHex(6),
		EventID:      ev.ID,
		SeatID:       body.SeatID,
		RelampoToken: randomHex(32),
		TokenExpires: time.Now().Add(relampoTokenTTL),
	}
	app.store.Lock()
	s.Reservations[res.ID] = res
	s.CorrelationID = "" // un solo uso
	s.Step = "reserved"
	app.store.Unlock()
	writeJSON(w, http.StatusCreated, map[string]any{
		"reservationId":         res.ID,
		"eventId":               res.EventID,
		"seatId":                res.SeatID,
		"relampoToken":          res.RelampoToken,
		"tokenExpiresInSeconds": int(relampoTokenTTL.Seconds()),
		"note":                  "relampoToken debe enviarse TRANSFORMADO en /pay/start: hex(HMAC-SHA256(token, sal pública de app.js))",
	})
}

// GET /api/tickets/{id} — verificación final del ticket emitido.
func (app *App) apiTicket(w http.ResponseWriter, r *http.Request) {
	s := app.requireBearer(w, r)
	if s == nil {
		return
	}
	t := s.Tickets[r.PathValue("id")]
	if t == nil {
		writeErr(w, r, http.StatusNotFound, "ticket_id",
			"ticket inexistente en esta sesión",
			"el ticket_id aparece en el atributo data-ticket del body de la página de éxito de POST /pay/confirm")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticketId":      t.ID,
		"receiptNumber": t.ReceiptNumber,
		"event":         t.EventName,
		"seat":          t.SeatID,
		"user":          t.User,
		"issuedAt":      t.IssuedAt.UTC().Format(time.RFC3339),
		"status":        "CONFIRMED",
	})
}

// ---------------------------------------------------------------- RelampoPay

// POST /pay/start — form con reservation_id + relampo_token TRANSFORMADO (B).
// Valida el HMAC, marca el token como usado y arranca la cadena de redirects.
func (app *App) payStart(w http.ResponseWriter, r *http.Request) {
	s := app.requireSession(w, r)
	if s == nil {
		return
	}
	if err := r.ParseForm(); err != nil {
		writeErr(w, r, http.StatusBadRequest, "form", "formulario inválido", "")
		return
	}
	res := s.Reservations[r.PostFormValue("reservation_id")]
	if res == nil {
		writeCorrErr(w, r, http.StatusNotFound, "reservation_id",
			"el reservation_id enviado no corresponde a ninguna reserva de esta sesión",
			"correlaciona $.reservationId del JSON de POST /api/reservations",
			r.PostFormValue("reservation_id"), "(el $.reservationId de POST /api/reservations)")
		return
	}
	sent := r.PostFormValue("relampo_token")
	hint := "el valor A viene en $.relampoToken de POST /api/reservations; debes enviar B = XOR de cada carácter de A con la sal 'relampo-public-salt-v1', en hex — mira signRelampoToken() en /static/app.js y reimpleméntala en el preprocesador de tu herramienta"
	if tokenMode == "hmac" {
		hint = "el valor A viene en $.relampoToken de POST /api/reservations; debes enviar B = hex(HMAC-SHA256(A, 'relampo-public-salt-v1')) — mira signRelampoToken() en /static/app.js y reimpleméntala en tu herramienta"
	}
	expected := signRelampoToken(res.RelampoToken)
	switch {
	case sent == "":
		writeCorrErr(w, r, http.StatusBadRequest, "relampo_token",
			"falta el relampo_token transformado", hint, sent, expected)
		return
	case sent == res.RelampoToken:
		writeCorrErr(w, r, http.StatusForbidden, "relampo_token",
			"enviaste el relampo_token CRUDO (valor A); el servidor solo acepta el valor transformado (B)",
			hint, sent, expected)
		return
	case res.TokenUsed:
		writeErr(w, r, http.StatusForbidden, "relampo_token",
			"este relampo_token ya fue usado (es de un solo uso); crea una nueva reserva en cada iteración", hint)
		return
	case time.Now().After(res.TokenExpires):
		writeErr(w, r, http.StatusForbidden, "relampo_token",
			"relampo_token expirado (TTL 60s); reduce el tiempo entre la reserva y el pago", hint)
		return
	case sent != expected:
		writeCorrErr(w, r, http.StatusForbidden, "relampo_token",
			"el valor transformado enviado no coincide con el esperado (revisa la transformación de signRelampoToken)",
			hint, sent, expected)
		return
	}
	pf := &PayFlow{
		State:         randomStateB64(24),
		ReservationID: res.ID,
		RequestBlob:   randomStateB64(48),
		Expires:       time.Now().Add(payFlowTTL),
	}
	app.store.Lock()
	res.TokenUsed = true
	s.PayFlows[pf.State] = pf
	s.Step = "paying"
	app.store.Unlock()
	// state usa base64 estándar (+ / =) a propósito: en el header Location va
	// URL-encoded y el extractor necesita transform url_decode.
	http.Redirect(w, r, "/pay/authorize?state="+url.QueryEscape(pf.State), http.StatusFound)
}

func (app *App) payFlowByState(w http.ResponseWriter, r *http.Request, s *Session, state string) *PayFlow {
	pf := s.PayFlows[state]
	if pf == nil || time.Now().After(pf.Expires) {
		writeCorrErr(w, r, http.StatusForbidden, "state",
			"el state enviado no coincide con ninguno vigente emitido para esta sesión",
			"extrae state del header Location del 302 de POST /pay/start (regex sobre headers + url_decode)",
			state, "(el state del Location del 302 de POST /pay/start, url-decodificado)")
		return nil
	}
	return pf
}

// GET /pay/authorize?state=… — página del gateway con auto-submit form.
func (app *App) payAuthorize(w http.ResponseWriter, r *http.Request) {
	s := app.requireSession(w, r)
	if s == nil {
		return
	}
	pf := app.payFlowByState(w, r, s, r.URL.Query().Get("state"))
	if pf == nil {
		return
	}
	app.store.Lock()
	pf.XCorrelation = randomUUID()
	app.store.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	authorizeTmpl.Execute(w, map[string]any{
		"Request": pf.RequestBlob,
		"XCorr":   pf.XCorrelation,
	})
}

// POST /pay/continue — recibe los campos del auto-submit form y redirige con
// el code en el Location.
func (app *App) payContinue(w http.ResponseWriter, r *http.Request) {
	s := app.requireSession(w, r)
	if s == nil {
		return
	}
	if err := r.ParseForm(); err != nil {
		writeErr(w, r, http.StatusBadRequest, "form", "formulario inválido", "")
		return
	}
	reqBlob := r.PostFormValue("request")
	xcorr := r.PostFormValue("x_correlation_id")
	var pf *PayFlow
	for _, cand := range s.PayFlows {
		if cand.RequestBlob == reqBlob && cand.XCorrelation != "" && cand.XCorrelation == xcorr {
			pf = cand
			break
		}
	}
	if pf == nil {
		writeCorrErr(w, r, http.StatusForbidden, "request/x_correlation_id",
			"los valores enviados no coinciden con los emitidos en el formulario del gateway",
			"extrae los inputs ocultos 'request' y 'x_correlation_id' del HTML de GET /pay/authorize (2 regex) y envíalos como form body",
			"request="+preview(reqBlob)+", x_correlation_id="+preview(xcorr),
			"(los inputs ocultos del HTML de GET /pay/authorize)")
		return
	}
	if time.Now().After(pf.Expires) {
		writeErr(w, r, http.StatusForbidden, "state", "flujo de pago expirado", "")
		return
	}
	app.store.Lock()
	pf.Code = randomHex(20)
	app.store.Unlock()
	http.Redirect(w, r,
		"/pay/callback?code="+pf.Code+"&state="+url.QueryEscape(pf.State),
		http.StatusFound)
}

// GET /pay/callback?code=…&state=… — confirmación con view_state gigante.
func (app *App) payCallback(w http.ResponseWriter, r *http.Request) {
	s := app.requireSession(w, r)
	if s == nil {
		return
	}
	pf := app.payFlowByState(w, r, s, r.URL.Query().Get("state"))
	if pf == nil {
		return
	}
	if code := r.URL.Query().Get("code"); code == "" || code != pf.Code {
		writeCorrErr(w, r, http.StatusForbidden, "code",
			"el code enviado no coincide con el emitido para este flujo de pago",
			"extrae code del header Location del 302 de POST /pay/continue (regex sobre headers)",
			code, pf.Code)
		return
	}
	app.store.Lock()
	pf.ViewState = newViewState()
	app.store.Unlock()
	res := s.Reservations[pf.ReservationID]
	ev := s.findEvent(res.EventID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	callbackTmpl.Execute(w, map[string]any{
		"ViewState":     pf.ViewState,
		"Code":          pf.Code,
		"ReservationID": res.ID,
		"EventName":     ev.Name,
		"SeatID":        res.SeatID,
		"Price":         seatPrice(res.SeatID),
	})
}

// POST /pay/confirm — cierre del flujo: exige view_state + code + csrf_token
// (el csrf nació en GET /, al principio de todo).
func (app *App) payConfirm(w http.ResponseWriter, r *http.Request) {
	s := app.requireSession(w, r)
	if s == nil {
		return
	}
	if err := r.ParseForm(); err != nil {
		writeErr(w, r, http.StatusBadRequest, "form", "formulario inválido", "")
		return
	}
	code := r.PostFormValue("code")
	var pf *PayFlow
	for _, cand := range s.PayFlows {
		if cand.Code != "" && cand.Code == code {
			pf = cand
			break
		}
	}
	if pf == nil {
		writeCorrErr(w, r, http.StatusForbidden, "code",
			"el code enviado no coincide con ningún flujo de pago vigente",
			"extrae code del Location del 302 de POST /pay/continue",
			code, "(el code del Location del 302 de POST /pay/continue)")
		return
	}
	if vs := r.PostFormValue("view_state"); vs == "" || vs != pf.ViewState {
		writeCorrErr(w, r, http.StatusForbidden, "view_state",
			"el view_state enviado no coincide con el emitido en la página de confirmación",
			"extrae el input oculto view_state del HTML de GET /pay/callback (regex; es un base64 largo)",
			vs, pf.ViewState)
		return
	}
	if csrf := r.PostFormValue("csrf_token"); csrf == "" || csrf != s.CSRFToken {
		writeCorrErr(w, r, http.StatusForbidden, "csrf_token",
			"el csrf_token enviado no coincide con el emitido al inicio de la sesión",
			"el csrf_token es el input oculto del formulario de login en GET / (el primer request del flujo); guárdalo hasta el final",
			csrf, s.CSRFToken)
		return
	}
	res := s.Reservations[pf.ReservationID]
	if r.PostFormValue("reservation_id") != res.ID {
		writeCorrErr(w, r, http.StatusForbidden, "reservation_id",
			"el reservation_id enviado no coincide con el del flujo de pago", "",
			r.PostFormValue("reservation_id"), res.ID)
		return
	}
	ev := s.findEvent(res.EventID)
	t := &Ticket{
		ID:            "TCK-" + randomHex(8),
		ReceiptNumber: fmt.Sprintf("R-%d-%06d", time.Now().Year(), app.receiptCounter.Add(1)),
		EventName:     ev.Name,
		SeatID:        res.SeatID,
		User:          s.User,
		IssuedAt:      time.Now(),
	}
	app.store.Lock()
	s.Tickets[t.ID] = t
	delete(s.PayFlows, pf.State)
	s.Step = "completed"
	app.store.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	successTmpl.Execute(w, map[string]any{
		"TicketID":      t.ID,
		"ReceiptNumber": t.ReceiptNumber,
		"EventName":     t.EventName,
		"SeatID":        t.SeatID,
		"User":          t.User,
	})
}
