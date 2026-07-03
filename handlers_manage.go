package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Event management ("Mis eventos"): create + edit + delete with two extra
// correlation patterns — a single-use publish_token (HTML hidden input →
// JSON body field) and optimistic locking (response ETag → If-Match header,
// 428/412 on mistakes).

// Regla de negocio: un usuario no puede tener más de 5 eventos a la vez.
const maxEventsPerUser = 5

func stripQuotes(s string) string {
	return strings.Trim(s, `"`)
}

// GET /manage — página de gestión; siembra un publish_token de UN SOLO USO.
func (app *App) managePage(w http.ResponseWriter, r *http.Request) {
	s := app.requireSession(w, r)
	if s == nil {
		return
	}
	if s.User == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	app.store.Lock()
	s.PublishToken = "PUB-" + randomHex(16)
	s.Step = "managing"
	app.store.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	manageTmpl.Execute(w, pageData(s, map[string]any{
		"PublishToken": s.PublishToken,
		"AtLimit":      s.ownedCount() >= maxEventsPerUser,
	}))
}

// GET /api/manage/events — lista SOLO los eventos creados por el usuario.
func (app *App) apiManageList(w http.ResponseWriter, r *http.Request) {
	s := app.requireBearer(w, r)
	if s == nil {
		return
	}
	type evJSON struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Venue string `json:"venue"`
		Date  string `json:"date"`
		Rev   string `json:"rev"`
	}
	out := []evJSON{}
	for _, ev := range s.Events {
		if ev.Owned {
			out = append(out, evJSON{ev.ID, ev.Name, ev.Venue, ev.Date, ev.Rev})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(out), "events": out})
}

// POST /api/manage/events — crea un evento; exige el publish_token extraído
// del HTML de GET /manage dentro del body JSON. Un solo uso.
func (app *App) apiManageCreate(w http.ResponseWriter, r *http.Request) {
	s := app.requireBearer(w, r)
	if s == nil {
		return
	}
	var body struct {
		Name         string `json:"name"`
		Venue        string `json:"venue"`
		Date         string `json:"date"`
		PublishToken string `json:"publishToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, r, http.StatusBadRequest, "body", "body JSON inválido", "")
		return
	}
	if body.PublishToken == "" || body.PublishToken != s.PublishToken {
		expected := s.PublishToken
		if expected == "" {
			expected = "(ya consumido: es de un solo uso, vuelve a GET /manage para obtener otro)"
		}
		writeCorrErr(w, r, http.StatusForbidden, "publish_token",
			"el publishToken enviado no coincide con el emitido en GET /manage",
			"extrae el input oculto publish_token del HTML de GET /manage (regex) y envíalo en el campo publishToken del body JSON; cada creación requiere volver a GET /manage",
			body.PublishToken, expected)
		return
	}
	if body.Name == "" || body.Venue == "" || body.Date == "" {
		writeErr(w, r, http.StatusBadRequest, "name/venue/date",
			"name, venue y date son obligatorios", "")
		return
	}
	if s.ownedCount() >= maxEventsPerUser {
		writeErr(w, r, http.StatusConflict, "events_limit",
			fmt.Sprintf("límite alcanzado: un usuario no puede tener más de %d eventos; borra alguno para crear otro", maxEventsPerUser),
			"usa DELETE /api/manage/events/{id} (con If-Match) para liberar lugar")
		return
	}
	ev := &Event{
		ID:    "EV-" + randomUUID(),
		Name:  body.Name,
		Venue: body.Venue,
		Date:  body.Date,
		Owned: true,
		Rev:   "1-" + randomHex(8),
	}
	for row := 0; row < 3; row++ {
		for n := 1; n <= 4; n++ {
			ev.Seats = append(ev.Seats, fmt.Sprintf("S-%c%d-%s", 'A'+row, n, randomHex(3)))
		}
	}
	app.store.Lock()
	s.Events = append(s.Events, ev)
	s.PublishToken = "" // un solo uso
	app.store.Unlock()
	writeJSON(w, http.StatusCreated, map[string]any{
		"event": map[string]any{
			"id": ev.ID, "name": ev.Name, "venue": ev.Venue, "date": ev.Date, "rev": ev.Rev,
		},
	})
}

func (app *App) ownedEvent(w http.ResponseWriter, r *http.Request, s *Session) *Event {
	ev := s.findEvent(r.PathValue("id"))
	if ev == nil || !ev.Owned {
		writeCorrErr(w, r, http.StatusNotFound, "event_id",
			"el event_id enviado no corresponde a un evento creado por este usuario",
			"usa el id devuelto en $.event.id de POST /api/manage/events o en $.events[*].id de GET /api/manage/events",
			r.PathValue("id"), "(el $.event.id devuelto por POST /api/manage/events)")
		return nil
	}
	return ev
}

// GET /manage/events/{id}/edit — formulario de edición; la rev vigente viaja
// como header ETag de esta respuesta (y como input oculto).
func (app *App) manageEditPage(w http.ResponseWriter, r *http.Request) {
	s := app.requireSession(w, r)
	if s == nil {
		return
	}
	if s.User == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	ev := app.ownedEvent(w, r, s)
	if ev == nil {
		return
	}
	w.Header().Set("Etag", `"`+ev.Rev+`"`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	editTmpl.Execute(w, pageData(s, map[string]any{
		"ID": ev.ID, "Name": ev.Name, "Venue": ev.Venue, "Date": ev.Date, "Rev": ev.Rev,
	}))
}

func (app *App) checkIfMatch(w http.ResponseWriter, r *http.Request, ev *Event) bool {
	ifMatch := stripQuotes(r.Header.Get("If-Match"))
	if ifMatch == "" {
		writeCorrErr(w, r, http.StatusPreconditionRequired, "if_match",
			"falta el header If-Match con la rev vigente del evento",
			"extrae la rev del header ETag de GET /manage/events/{id}/edit (regex sobre headers) o de $.event.rev y envíala como header If-Match",
			ifMatch, ev.Rev)
		return false
	}
	if ifMatch != ev.Rev {
		writeCorrErr(w, r, http.StatusPreconditionFailed, "if_match",
			"la rev enviada en If-Match ya no es la vigente (el evento cambió); vuelve a leerlo y correlaciona la última",
			"cada actualización devuelve una rev nueva en $.event.rev; correlaciona siempre la última, no reutilices valores viejos",
			ifMatch, ev.Rev)
		return false
	}
	return true
}

// PUT /api/manage/events/{id} — actualiza con bloqueo optimista.
func (app *App) apiManageUpdate(w http.ResponseWriter, r *http.Request) {
	s := app.requireBearer(w, r)
	if s == nil {
		return
	}
	ev := app.ownedEvent(w, r, s)
	if ev == nil {
		return
	}
	if !app.checkIfMatch(w, r, ev) {
		return
	}
	var body struct {
		Name  string `json:"name"`
		Venue string `json:"venue"`
		Date  string `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, r, http.StatusBadRequest, "body", "body JSON inválido", "")
		return
	}
	app.store.Lock()
	if body.Name != "" {
		ev.Name = body.Name
	}
	if body.Venue != "" {
		ev.Venue = body.Venue
	}
	if body.Date != "" {
		ev.Date = body.Date
	}
	rev := revBump(ev.Rev)
	ev.Rev = rev
	app.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"event": map[string]any{
			"id": ev.ID, "name": ev.Name, "venue": ev.Venue, "date": ev.Date, "rev": rev,
		},
	})
}

// DELETE /api/manage/events/{id} — borra con bloqueo optimista.
func (app *App) apiManageDelete(w http.ResponseWriter, r *http.Request) {
	s := app.requireBearer(w, r)
	if s == nil {
		return
	}
	ev := app.ownedEvent(w, r, s)
	if ev == nil {
		return
	}
	if !app.checkIfMatch(w, r, ev) {
		return
	}
	app.store.Lock()
	for i, cand := range s.Events {
		if cand.ID == ev.ID {
			s.Events = append(s.Events[:i], s.Events[i+1:]...)
			break
		}
	}
	app.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": ev.ID})
}

// revBump: "3-a1b2c3" → "4-<nuevo hex>".
func revBump(rev string) string {
	n := 0
	if i := strings.IndexByte(rev, '-'); i > 0 {
		fmt.Sscanf(rev[:i], "%d", &n)
	}
	return fmt.Sprintf("%d-%s", n+1, randomHex(8))
}
