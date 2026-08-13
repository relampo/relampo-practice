# Scripting guide — RelampoTickets

How to build a performance script against **https://practice.relampo.com**, step by
step, with think times, assertions and dynamic-value correlation.

Applies to **Relampo, JMeter, k6 and Gatling** — the logic is the same, only the syntax
changes. Flows and recordings may vary; the principles in this guide do not.

*(Versión en español: [GUIA-SCRIPTING.md](GUIA-SCRIPTING.md))*

---

## 1. Before recording

| Point | Why it matters |
|---|---|
| **Record in an incognito window** | With a clean cache the browser requests `/static/app.js`, which is where the `Etag` is born (and where the encryption function can be read). With a dirty cache that request never appears. |
| **Maximum 5 sessions per node (IP)** | The application allows 5 concurrent sessions per machine. The sixth gets `429 vus_per_node`. If a script is abandoned without `/logout`, its slot is freed after 90 s. |
| **Test users** | `user001` … `user500`, passwords `Pass001!` … `Pass500!`. To generate the CSV: `for i in $(seq -w 1 500); do echo "user$i,Pass$i!"; done` |
| **One user per VU** | Each session has its own catalog and seats; there are no collisions between virtual users. |
| **Record in one go** | The payment token expires after 60 s. Any delay between reserving and paying records a 403. |
| **Turn on hints while debugging** | Sending the `X-Practice-Hints: true` header makes errors state exactly where to extract the failing value. Remove it in the final version. |

---

## 2. The base flow

These **13 actions** capture practically every dynamic value in the application. Each
step states what the user does, which main request it generates, how long to wait
(think time) and what to validate (assertions).

> **Think time**: the time a real user spends reading, deciding or typing. It goes
> **after** receiving the response and **before** the next action. Without think times a
> script does not simulate users: it simulates a flood.

---

### Step 1 — Open the home page

**Action:** the user goes to `https://practice.relampo.com`

**Request:** `GET /` → 200 (HTML)

**Think time:** 5–8 s (reads the page, sees the login form)

**Assertions:**
- Status `200`
- The body contains `RelampoTickets`
- The body contains `name="csrf_token"`

---

### Step 2 — Sign in

**Action:** types username and password, clicks "Entrar"

**Request:** `POST /api/auth` → 200 (JSON)

**Body:** `{"username":"user001","password":"Pass001!"}`

**Think time:** 10–15 s (typing username and password)

**Assertions:**
- Status `200`
- The JSON contains the `bearer` field
- `$.user.username` equals the username sent

---

### Step 3 — View "My events"

**Action:** after login the application lands on the events panel

**Requests:**
```
GET /manage              → 200 (HTML)
GET /api/manage/events   → 200 (JSON)
```

**Think time:** 4–6 s

**Assertions:**
- Status `200` on both
- The HTML contains `name="publish_token"`
- The JSON has `$.count >= 1` (every user starts with the *Relampo Fest* event)

---

### Step 4 — Create an event

**Action:** fills in name, venue and date, clicks "Publicar evento"

**Request:** `POST /api/manage/events` → 201 (JSON)

**Body:** `{"name":"...","venue":"...","date":"...","publishToken":"{{publish_token}}"}`

**Think time:** 15–25 s

**Assertions:**
- Status `201`
- The JSON contains `$.event.id`
- `$.event.name` equals the name sent

**Limit:** maximum 5 events per user; the sixth returns `409 events_limit`.

---

### Step 5 — Open the event for editing

**Action:** clicks "editar" on the event

**Request:** `GET /manage/events/{{event_id}}/edit` → 200 (HTML)

**Think time:** 3–5 s

**Assertions:**
- Status `200`
- The `ETag` header is present
- The HTML contains the event name

---

### Step 6 — Save changes

**Action:** changes the name or venue, clicks "Guardar cambios"

**Request:** `PUT /api/manage/events/{{event_id}}` → 200 (JSON)

**Headers:** `Authorization: Bearer …` · `If-Match: {{event_rev}}`

**Think time:** 10–15 s

**Assertions:**
- Status `200`
- `$.event.rev` is **different** from the one sent

> ⚠️ **Optimistic locking.** Without the `If-Match` header → `428`. With a stale version
> → `412`. Every update returns a new `rev`: always correlate the latest one.

---

### Step 7 — Go to buy

**Action:** clicks "comprar" in the menu

**Request:** `GET /events` → 200 (HTML)

**Think time:** 3–5 s

**Assertions:** status `200` · the HTML contains `data-config`

---

### Step 8 — Browse catalog and seats

**Action:** the page loads the events and a seat is chosen

**Requests:**
```
GET /api/events?catalogId={{catalog_id}}      → 200 (JSON)
GET /api/events/{{event_id}}/seats            → 200 (JSON)
```

**Think time:** 5–8 s

**Assertions:**
- Status `200` on both
- `$.events` has at least 1 element
- `$.seats` has 12 elements
- The `X-Correlation-Id` header is present in the seats response

---

### Step 9 — Reserve the seat

**Action:** the application reserves automatically when the page loads

**Request:** `POST /api/reservations` → 201 (JSON)

**Headers:** `Authorization: Bearer …` · `X-Correlation-Id: {{X-Correlation-Id}}`

**Body:** `{"eventId":"{{event_id}}","seatId":"{{seat_id}}"}`

**Think time:** 3–5 s — **60 s maximum**, the token expires

**Assertions:**
- Status `201`
- The JSON contains `$.relampoToken`
- `$.tokenExpiresInSeconds` is `60`

---

### Step 10 — Pay

**Action:** clicks "Pagar ahora"

**Request:** `POST /pay/start` → 302 (redirect)

**Body (form):** `reservation_id={{reservation_id}}&relampo_token={{relampo_token_b}}`

**Think time:** 2–3 s

**Assertions:**
- Status `302`
- The `Location` header contains `state=`

> ⚠️ **This step cannot be solved with extractors alone.** The token received in step 9
> is **not** the one to send: the browser transforms it with JavaScript. See **section 5**
> for the method and the code in each tool.
>
> ⚠️ Also **disable "follow redirects"** on this request, or the tool consumes the 302
> and the `state` in the `Location` header becomes unreachable.

---

### Step 11 — Payment gateway

**Action:** the gateway redirects on its own (auto-submit form)

**Requests:**
```
GET  /pay/authorize?state={{state}}                    → 200 (HTML)
POST /pay/continue                                     → 302
GET  /pay/callback?code={{code}}&state={{state}}       → 200 (HTML)
```

**POST body (form):** `request={{request}}&x_correlation_id={{x_correlation_id}}`

**Think time:** 0 s between these three (the machine fires them), and 5–8 s after the
callback (the user reviews the summary before confirming)

**Assertions:**
- `/pay/authorize` → 200, the HTML contains `name="request"`
- `/pay/continue` → 302, the `Location` contains `code=`
- `/pay/callback` → 200, the HTML contains `name="view_state"`

---

### Step 12 — Confirm the purchase

**Action:** clicks "Confirmar compra"

**Requests:**
```
POST /pay/confirm                 → 200 (HTML)
GET  /api/tickets/{{ticket_id}}   → 200 (JSON)
```

**Body (form):** `view_state={{view_state}}&code={{code}}&reservation_id={{reservation_id}}&csrf_token={{csrf_token}}`

**Think time:** 3–5 s

**Assertions:**
- Status `200`
- The HTML contains `data-ticket`
- The ticket JSON contains `"status":"CONFIRMED"` ← **the most important assertion in the script**

---

### Step 13 — Sign out

**Action:** deletes the test event and clicks "salir"

**Requests:**
```
DELETE /api/manage/events/{{event_id}}   → 200   (optional: cleanup)
GET /logout                              → 302
```

**Headers:** `If-Match: {{event_rev_new}}` on the DELETE

**Think time:** 2–3 s

**Assertions:** status `302` with `Location: /`

---

## 3. Dynamic values table

| Name | Value | Captured in | Location in the response | Sent back in | Extractor type | Suggested extractor |
|---|---|---|---|---|---|---|
| `relampo_session` | `7b37cfacc836a31d…` | `GET /` | `Set-Cookie` header | Every request · `Cookie` header | Cookie manager | Automatic |
| `csrf_token` | `0f66a320060901667dab…` | `GET /` | HTML body, hidden input | `POST /pay/confirm` · form body | regex | `(?is)name=["']csrf_token["'][^>]*value=["']([^"']+)["']` |
| `Etag` | `"735c697234df2c9f"` | `GET /static/app.js` | `Etag` header | Same request · `If-None-Match` header | regex over headers | `(?im)^Etag:\s*([^\r\n]+)$` |
| `bearer` | `eyJhbGciOiJIUzI1NiIs…` | `POST /api/auth` | JSON body | Whole API · `Authorization` header | jsonpath | `$.bearer` |
| `publish_token` | `PUB-6480098c33938000…` | `GET /manage` | HTML body, hidden input | `POST /api/manage/events` · `publishToken` field of the JSON | regex | `(?is)name=["']publish_token["'][^>]*value=["']([^"']+)["']` |
| `event_id` | `EV-09929966-2806-4898…` | `POST /api/manage/events` | JSON body | edit · PUT · DELETE · seats · URL **path** | jsonpath | `$.event.id` |
| `event_rev` | `1-d3fd5c00075746c5` | `GET …/edit` or the PUT | `ETag` header or JSON body | PUT and DELETE · `If-Match` header | regex over headers · jsonpath | `(?im)^ETag:\s*"?([^"\r\n]+)"?$` · `$.event.rev` |
| `catalog_id` | `CAT-44ef1118c40a` | `GET /events` | HTML body, escaped JSON inside the `data-config` attribute | `GET /api/events` · **query param** `catalogId` | regex | `(?is)data-config="[^"]*?catalogId(?:&#34;\|&quot;):(?:&#34;\|&quot;)([A-Za-z0-9-]+)` |
| `seat_id` | `S-C3-315883` | `GET …/seats` | JSON body, array of 12 | `POST /api/reservations` · JSON body | jsonpath, random occurrence | `$.seats[*].id` |
| `X-Correlation-Id` | `cfc9769c-ae22-4969-9cb7…` | `GET …/seats` | Response header | `POST /api/reservations` · request **header** | regex over headers | `(?im)^X-Correlation-Id:\s*([^\r\n]+)$` |
| `reservation_id` | `RES-d908e32724a9` | `POST /api/reservations` | JSON body | `/pay/start` and `/pay/confirm` · form body | jsonpath | `$.reservationId` |
| **`relampo_token`** | `8a23a2b1…` → `4a045e52…` | `POST /api/reservations` | JSON body | `POST /pay/start` · form body, **transformed** | jsonpath + preprocessor | `$.relampoToken` then XOR with the salt (section 5) |
| `state` | `Xjv42dJuWHnm9Rz0Ehfkai…` | `POST /pay/start` | `Location` header of the 302 | authorize and callback · query param | regex over headers + url_decode | `(?im)^Location:\s*[^\r\n]*?[?&]state=([^&#\r\n]+)` |
| `request` | `zCz3b7z59IBchOq2DjUCav…` | `GET /pay/authorize` | HTML body, hidden input | `POST /pay/continue` · form body | regex | `(?is)name=["']request["'][^>]*value=["']([^"']+)["']` |
| `x_correlation_id` | `a5c999c1-591a-45ca-8cd5…` | `GET /pay/authorize` | HTML body, hidden input | `POST /pay/continue` · form body | regex | `(?is)name=["']x_correlation_id["'][^>]*value=["']([^"']+)["']` |
| `code` | `debb45abd0a956d94d8b…` | `POST /pay/continue` | `Location` header of the 302 | callback · query param — confirm · form body | regex over headers | `(?im)^Location:\s*[^\r\n]*?[?&]code=([^&#\r\n]+)` |
| `view_state` | `LcyOk5nIhyOquHux1fsO3R…` | `GET /pay/callback` | HTML body, hidden input (~1.4 KB) | `POST /pay/confirm` · form body | regex | `(?is)name=["']view_state["'][^>]*value=["']([^"']+)["']` |
| `ticket_id` | `TCK-38dd517074a589c9` | `POST /pay/confirm` | HTML body, attribute of the `<body>` tag | `GET /api/tickets/{id}` · URL **path** | regex | `(?is)data-ticket=["']([^"']+)["']` |

### Single-use or expiring values

| Value | Rule | What it means for the script |
|---|---|---|
| `publish_token` | single use | Go back to `GET /manage` before every create |
| `X-Correlation-Id` | single use | Request seats before every reservation |
| `relampo_token` | single use + **60 s** | New reservation on every iteration, and pay quickly |
| `state` / payment flow | 5 minutes | Complete the payment within that window |
| `event_rev` | changes on every update | Re-correlate after each PUT |
| `bearer` | 2 hours | Enough for any test |
| Session | 30 min of inactivity | Not an issue with normal think times |

---

## 4. Extractors per tool

**From the HTML body (regex):**

| Tool | How |
|---|---|
| Relampo | `- type: regex` / `from: body` / `var: csrf_token` / `pattern: ...` / `group: 1` |
| JMeter | *Regular Expression Extractor* → Field to check: **Body**, Template: `$1$`, Match No: `1` |
| k6 | `const csrf = res.body.match(/name="csrf_token" value="([^"]+)"/)[1];` |
| Gatling | `.check(regex("""name="csrf_token" value="([^"]+)"""").saveAs("csrf_token"))` |

**From the JSON body:**

| Tool | How |
|---|---|
| Relampo | `- type: jsonpath` / `expression: $.bearer` |
| JMeter | *JSON Extractor* → JSON Path: `$.bearer` |
| k6 | `const bearer = res.json().bearer;` |
| Gatling | `.check(jsonPath("$.bearer").saveAs("bearer"))` |

**From a response header:**

| Tool | How |
|---|---|
| Relampo | `- type: regex` / `from: headers` / `pattern: (?im)^X-Correlation-Id:\s*([^\r\n]+)$` |
| JMeter | *Regular Expression Extractor* → Field to check: **Response Headers** |
| k6 | `const corr = res.headers['X-Correlation-Id'];` |
| Gatling | `.check(header("X-Correlation-Id").saveAs("corr"))` |

**From the `Location` of a redirect (302):**

⚠️ Automatic redirect following must be disabled first; otherwise the 302 is never visible.

| Tool | How |
|---|---|
| Relampo | `follow_redirects: false` + `- type: regex` / `from: headers` / `transform: url_decode` |
| JMeter | Untick *Follow Redirects* + Regex Extractor over Response Headers + `__urldecode()` |
| k6 | `const res = http.post(url, body, { redirects: 0 }); const state = decodeURIComponent(res.headers['Location'].match(/state=([^&]+)/)[1]);` |
| Gatling | `.disableFollowRedirect.check(header("Location").transform(...).saveAs("state"))` |

---

## 5. The encrypted value: `relampo_token`

### The problem

```
Response of POST /api/reservations:
  "relampoToken": "9788897b868175dc..."          ← value A (64 characters)

Request of POST /pay/start:
  relampo_token=4b5254595549584f...              ← value B (128 characters)
```

**A ≠ B.** Value B **does not appear in any response**: the browser's JavaScript computes
it before submitting the form. Correlating A and sending it as-is makes the server
answer `403 you sent the RAW relampo_token (value A)`.

No automatic correlator can solve this: it takes **code**.

### The method

```
B = for each character of A:  (character code  XOR  code of the matching salt character)
                              written as 2 hexadecimal digits
```

With `salt = "relampo-public-salt-v1"` (public, it lives in `/static/app.js`). When the
salt runs out (22 characters) it starts over. Since each character becomes 2 hex digits,
**B is always twice as long as A**.

Example of the first two rounds:

| Character of A | Salt character | XOR | In hex |
|---|---|---|---|
| `9` (57) | `r` (114) | 75 | `4b` |
| `7` (55) | `e` (101) | 82 | `52` |

→ B starts with `4b52...`

### The code, per tool

**Relampo** — `spark` block on the `POST /pay/start` request:

```javascript
// STEP 1 — Read the token extracted in the previous request
var tokenA = vars.get("relampo_token_a");

// STEP 2 — Transform it (same logic as app.js in the browser)
var salt = "relampo-public-salt-v1";
var tokenB = "";

for (var i = 0; i < tokenA.length; i++) {
  var x = tokenA.charCodeAt(i) ^ salt.charCodeAt(i % salt.length);
  tokenB = tokenB + (x < 16 ? "0" : "") + x.toString(16);
}

// STEP 3 — Save it to use in the body as {{relampo_token_b}}
vars.set("relampo_token_b", tokenB);
```

**JMeter** — *JSR223 PreProcessor* (Groovy) on the payment sampler:

```groovy
def tokenA = vars.get("relampo_token_a")
def salt = "relampo-public-salt-v1"
def tokenB = new StringBuilder()

for (int i = 0; i < tokenA.length(); i++) {
    int x = ((int) tokenA.charAt(i)) ^ ((int) salt.charAt(i % salt.length()))
    tokenB.append(String.format("%02x", x))
}

vars.put("relampo_token_b", tokenB.toString())
```

**k6** — a plain JavaScript function:

```javascript
function signRelampoToken(tokenA) {
  const salt = "relampo-public-salt-v1";
  let tokenB = "";
  for (let i = 0; i < tokenA.length; i++) {
    const x = tokenA.charCodeAt(i) ^ salt.charCodeAt(i % salt.length);
    tokenB += x.toString(16).padStart(2, "0");
  }
  return tokenB;
}

// usage
const tokenA = res.json().relampoToken;
const tokenB = signRelampoToken(tokenA);
```

**Gatling** — a Scala function + `session.set`:

```scala
def signRelampoToken(tokenA: String): String = {
  val salt = "relampo-public-salt-v1"
  val sb = new StringBuilder
  for (i <- tokenA.indices) {
    val x = tokenA(i) ^ salt(i % salt.length)
    sb.append(f"$x%02x")
  }
  sb.toString
}

// usage
.exec(session => session.set("tokenB", signRelampoToken(session("tokenA").as[String])))
```

### How to verify it is right

1. Value B must be **exactly twice as long** as A (128 against 64 characters).
2. `POST /pay/start` must answer **302** with `Location: /pay/authorize?state=...`
3. Run **two consecutive iterations**: if the second one also passes, the transform is
   correct (the token is single-use, so a recorded value would fail).

---

## 6. Application errors

| Status | `variable` | What it means |
|---|---|---|
| 401 | `relampo_session` | Missing cookie or expired session |
| 401 | `bearer` | Token missing, badly copied or from another session |
| 400 | `catalog_id` | The `catalogId` is not the one for this session |
| 403 | `x_correlation_id` | The header from the seats response was not sent back |
| 403 | `relampo_token` | Raw value sent, badly transformed, already used or expired |
| 403 | `state` / `code` | Wrong extraction from `Location` (was the 302 consumed?) |
| 403 | `csrf_token` | The value from step 1 was lost |
| 428 / 412 | `if_match` | Missing `rev`, or a stale one was sent |
| 409 | `events_limit` | The user already has 5 events |
| 429 | `vus_per_node` | More than 5 concurrent sessions from the same IP |

Every error carries `received` and `expected` for comparison. With the
`X-Practice-Hints: true` header they also include a `hint` field with the exact
extraction location.

---

## 7. Think times and load profile

Apply a ±30 % variation so virtual users do not stay in lockstep.

| Moment | Time |
|---|---|
| Reading a page | 3–8 s |
| Filling a short form (login) | 10–15 s |
| Filling a long form (create event) | 15–25 s |
| Choosing from a list | 5–8 s |
| Between automatic requests (redirects, fetches from the same page) | 0 s |
| **Between reserving and paying** | **60 s maximum** |

With the 5-sessions-per-node limit, a reasonable test is **5 VUs per node** with a 30 s
ramp-up and 10–15 minutes of duration. Using Relampo's distributed execution across
4 nodes, that reaches 20 VUs.

---

## 8. Checklist before calling the script done

- [ ] Runs **two consecutive iterations** without errors (the real test of correlation)
- [ ] No dynamic value left hardcoded (search for `EV-`, `CAT-`, `TCK-`, `RES-`, `PUB-` in the script)
- [ ] The `csrf_token` from step 1 reaches step 12
- [ ] The `relampo_token` is transformed by the preprocessor, not sent raw
- [ ] The payment redirects have "follow redirects" disabled where required
- [ ] There are assertions on every step, including `"status":"CONFIRMED"` at the end
- [ ] There are think times between user actions
- [ ] User and data are parameterized (CSV), not hardcoded
- [ ] The flow ends with `/logout`
- [ ] The `X-Practice-Hints` header was removed from the final version
