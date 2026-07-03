package main

import (
	"sync"
	"time"
)

// Reservation is a seat hold created before payment. Its RelampoToken is the
// "value A" the client receives; the server only accepts the HMAC-transformed
// "value B" on /pay/start, exactly once, within TokenTTL.
type Reservation struct {
	ID           string
	EventID      string
	SeatID       string
	RelampoToken string
	TokenExpires time.Time
	TokenUsed    bool
}

// PayFlow tracks one pass through the fake payment gateway redirect chain.
type PayFlow struct {
	State         string
	ReservationID string
	RequestBlob   string
	XCorrelation  string
	Code          string
	ViewState     string
	Expires       time.Time
}

// Ticket is the final artifact of a completed purchase.
type Ticket struct {
	ID            string
	ReceiptNumber string
	EventName     string
	SeatID        string
	User          string
	IssuedAt      time.Time
}

// Event lives only inside a session: every virtual user gets its own copy of
// the catalog so 500 concurrent users never contend for the same seats.
type Event struct {
	ID    string
	Name  string
	Venue string
	Date  string
	Price int
	Seats []string
}

// Session is all server-side state bound to the single relampo_session cookie.
type Session struct {
	ID            string
	CreatedAt     time.Time
	LastSeen      time.Time
	Step          string
	CSRFToken     string
	User          string
	Bearer        string
	CatalogID     string
	Events        []*Event
	CorrelationID string
	Reservations  map[string]*Reservation
	PayFlows      map[string]*PayFlow
	Tickets       map[string]*Ticket
}

func (s *Session) findEvent(id string) *Event {
	for _, ev := range s.Events {
		if ev.ID == id {
			return ev
		}
	}
	return nil
}

// Store is the in-memory session store. No database by design: state is
// ephemeral and a restart leaves the app clean for the next practice run.
type Store struct {
	mu       sync.Mutex
	sessions map[string]*Session
	ttl      time.Duration
}

func NewStore(ttl time.Duration) *Store {
	st := &Store{sessions: make(map[string]*Session), ttl: ttl}
	go st.janitor()
	return st
}

func (st *Store) janitor() {
	for range time.Tick(time.Minute) {
		now := time.Now()
		st.mu.Lock()
		for id, s := range st.sessions {
			if now.Sub(s.LastSeen) > st.ttl {
				delete(st.sessions, id)
			}
		}
		st.mu.Unlock()
	}
}

func (st *Store) Create() *Session {
	s := &Session{
		ID:           randomHex(32),
		CreatedAt:    time.Now(),
		LastSeen:     time.Now(),
		Step:         "home",
		CSRFToken:    randomHex(20),
		Reservations: make(map[string]*Reservation),
		PayFlows:     make(map[string]*PayFlow),
		Tickets:      make(map[string]*Ticket),
	}
	st.mu.Lock()
	st.sessions[s.ID] = s
	st.mu.Unlock()
	return s
}

func (st *Store) Get(id string) *Session {
	st.mu.Lock()
	defer st.mu.Unlock()
	s := st.sessions[id]
	if s != nil {
		s.LastSeen = time.Now()
	}
	return s
}

func (st *Store) Delete(id string) {
	st.mu.Lock()
	delete(st.sessions, id)
	st.mu.Unlock()
}

// Snapshot returns a copy of coarse session info for /debug/sessions.
func (st *Store) Snapshot() []map[string]any {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := make([]map[string]any, 0, len(st.sessions))
	for _, s := range st.sessions {
		out = append(out, map[string]any{
			"session":      s.ID[:8] + "…",
			"user":         s.User,
			"step":         s.Step,
			"ageSeconds":   int(time.Since(s.CreatedAt).Seconds()),
			"reservations": len(s.Reservations),
			"tickets":      len(s.Tickets),
		})
	}
	return out
}

// Lock exposes the store mutex so handlers can mutate a session's nested
// state safely; keep critical sections short.
func (st *Store) Lock()   { st.mu.Lock() }
func (st *Store) Unlock() { st.mu.Unlock() }
