package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestManageEventsFlow covers the create/edit/delete flow with its two extra
// correlation patterns: single-use publish_token (HTML → JSON body) and
// optimistic locking (ETag response header → If-Match request header).
func TestManageEventsFlow(t *testing.T) {
	app := &App{store: NewStore(30 * time.Minute), startedAt: time.Now()}
	srv := httptest.NewServer(newMux(app))
	defer srv.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	do := func(method, path, body string, hdr map[string]string) (*http.Response, string) {
		var rdr *strings.Reader
		if body == "" {
			rdr = strings.NewReader("")
		} else {
			rdr = strings.NewReader(body)
		}
		req, _ := http.NewRequest(method, srv.URL+path, rdr)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
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
			t.Fatalf("no pude extraer %s de: %.200s", what, s)
		}
		return m[1]
	}

	// login
	do("GET", "/", "", nil)
	resp, body := do("POST", "/api/auth", `{"username":"user100","password":"Pass100!"}`,
		map[string]string{"Content-Type": "application/json"})
	if resp.StatusCode != 200 {
		t.Fatalf("auth: %d %s", resp.StatusCode, body)
	}
	var authResp struct{ Bearer string }
	json.Unmarshal([]byte(body), &authResp)
	auth := map[string]string{"Authorization": "Bearer " + authResp.Bearer}
	authJSON := map[string]string{
		"Authorization": "Bearer " + authResp.Bearer,
		"Content-Type":  "application/json",
	}

	// GET /manage → publish_token oculto en HTML
	_, body = do("GET", "/manage", "", nil)
	pubToken := rx(`name="publish_token" value="([^"]+)"`, body, "publish_token")

	// crear SIN publishToken → 403 didáctico
	resp, body = do("POST", "/api/manage/events",
		`{"name":"Mi Festival","venue":"Mi Teatro","date":"2026-12-01"}`, authJSON)
	if resp.StatusCode != 403 || !strings.Contains(body, "publish_token") {
		t.Fatalf("sin publishToken debía dar 403: %d %s", resp.StatusCode, body)
	}

	// crear con publishToken → 201 con id + rev
	resp, body = do("POST", "/api/manage/events",
		fmt.Sprintf(`{"name":"Mi Festival","venue":"Mi Teatro","date":"2026-12-01","publishToken":%q}`, pubToken),
		authJSON)
	if resp.StatusCode != 201 {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	var created struct {
		Event struct{ ID, Rev string }
	}
	json.Unmarshal([]byte(body), &created)

	// el publish_token es de un solo uso
	resp, body = do("POST", "/api/manage/events",
		fmt.Sprintf(`{"name":"Otro","venue":"Otro","date":"2026-12-02","publishToken":%q}`, pubToken),
		authJSON)
	if resp.StatusCode != 403 {
		t.Fatalf("reusar publishToken debía dar 403: %d %s", resp.StatusCode, body)
	}

	// GET edit → rev vigente en el header ETag
	resp, body = do("GET", "/manage/events/"+created.Event.ID+"/edit", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("edit page: %d %s", resp.StatusCode, body)
	}
	etagRev := strings.Trim(resp.Header.Get("Etag"), `"`)
	if etagRev != created.Event.Rev {
		t.Fatalf("ETag %q != rev %q", etagRev, created.Event.Rev)
	}

	// PUT sin If-Match → 428
	resp, body = do("PUT", "/api/manage/events/"+created.Event.ID,
		`{"name":"Festival Editado"}`, authJSON)
	if resp.StatusCode != 428 {
		t.Fatalf("sin If-Match debía dar 428: %d %s", resp.StatusCode, body)
	}

	// PUT con If-Match correcto → 200 y rev nueva
	h := map[string]string{}
	for k, v := range authJSON {
		h[k] = v
	}
	h["If-Match"] = etagRev
	resp, body = do("PUT", "/api/manage/events/"+created.Event.ID,
		`{"name":"Festival Editado"}`, h)
	if resp.StatusCode != 200 {
		t.Fatalf("update: %d %s", resp.StatusCode, body)
	}
	var updated struct {
		Event struct{ Name, Rev string }
	}
	json.Unmarshal([]byte(body), &updated)
	if updated.Event.Name != "Festival Editado" || updated.Event.Rev == etagRev {
		t.Fatalf("update no aplicado: %+v", updated)
	}

	// PUT con la rev VIEJA → 412 (correlación de valor obsoleto)
	resp, body = do("PUT", "/api/manage/events/"+created.Event.ID,
		`{"name":"Otra vez"}`, h)
	if resp.StatusCode != 412 {
		t.Fatalf("rev vieja debía dar 412: %d %s", resp.StatusCode, body)
	}

	// el evento creado aparece en el catálogo de compra
	_, body = do("GET", "/events", "", nil)
	rawCfg := rx(`data-config="([^"]+)"`, body, "data-config")
	unescaped := strings.NewReplacer("&#34;", `"`, "&quot;", `"`, "&amp;", "&").Replace(rawCfg)
	var cfg struct {
		CatalogID string `json:"catalogId"`
	}
	json.Unmarshal([]byte(unescaped), &cfg)
	resp, body = do("GET", "/api/events?catalogId="+cfg.CatalogID, "", auth)
	if resp.StatusCode != 200 || !strings.Contains(body, "Festival Editado") {
		t.Fatalf("el evento creado no aparece en el catálogo: %d %.300s", resp.StatusCode, body)
	}

	// DELETE con If-Match vigente → 200 y desaparece de la lista
	h["If-Match"] = updated.Event.Rev
	resp, body = do("DELETE", "/api/manage/events/"+created.Event.ID, "", h)
	if resp.StatusCode != 200 {
		t.Fatalf("delete: %d %s", resp.StatusCode, body)
	}
	resp, body = do("GET", "/api/manage/events", "", auth)
	if resp.StatusCode != 200 || strings.Contains(body, created.Event.ID) {
		t.Fatalf("el evento borrado sigue listado: %s", body)
	}
}
