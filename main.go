package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func newMux(app *App) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", app.home)
	mux.HandleFunc("GET /static/app.js", app.staticAppJS)
	mux.HandleFunc("GET /favicon.svg", app.favicon)
	mux.HandleFunc("GET /health", app.health)
	mux.HandleFunc("GET /debug/sessions", app.debugSessions)
	mux.HandleFunc("GET /events", app.eventsPage)
	mux.HandleFunc("GET /logout", app.logout)

	mux.HandleFunc("POST /api/auth", app.apiAuth)
	mux.HandleFunc("GET /api/events", app.apiEvents)
	mux.HandleFunc("GET /api/events/{id}/seats", app.apiSeats)
	mux.HandleFunc("POST /api/reservations", app.apiReservations)
	mux.HandleFunc("GET /api/tickets/{id}", app.apiTicket)

	mux.HandleFunc("GET /manage", app.managePage)
	mux.HandleFunc("GET /manage/events/{id}/edit", app.manageEditPage)
	mux.HandleFunc("GET /api/manage/events", app.apiManageList)
	mux.HandleFunc("POST /api/manage/events", app.apiManageCreate)
	mux.HandleFunc("PUT /api/manage/events/{id}", app.apiManageUpdate)
	mux.HandleFunc("DELETE /api/manage/events/{id}", app.apiManageDelete)

	mux.HandleFunc("POST /pay/start", app.payStart)
	mux.HandleFunc("GET /pay/authorize", app.payAuthorize)
	mux.HandleFunc("POST /pay/continue", app.payContinue)
	mux.HandleFunc("GET /pay/callback", app.payCallback)
	mux.HandleFunc("POST /pay/confirm", app.payConfirm)
	return mux
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sr, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, sr.status, time.Since(start).Round(time.Millisecond))
	})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if m := os.Getenv("RELAMPO_TOKEN_MODE"); m == "hmac" || m == "simple" {
		tokenMode = m
	}
	app := &App{
		store:     NewStore(30 * time.Minute),
		startedAt: time.Now(),
		appJS:     buildAppJS(tokenMode),
	}
	addr := ":" + port
	fmt.Printf("⚡ RelampoTickets — app de práctica para correlación\n")
	fmt.Printf("   http://localhost:%s  (usuarios: user001…user%03d / Pass001!…Pass%03d!)\n", port, userCount, userCount)
	fmt.Printf("   relampo_token: modo %s (RELAMPO_TOKEN_MODE=simple|hmac)\n", tokenMode)
	log.Fatal(http.ListenAndServe(addr, withLogging(newMux(app))))
}
