# Guía de scripting — RelampoTickets

Cómo construir un script de performance sobre **https://practice.relampo.com**, paso a
paso, con think times, assertions y correlación de valores dinámicos.

Vale para **Relampo, JMeter, k6 y Gatling** — la lógica es la misma, solo cambia la
sintaxis. Los flujos y las grabaciones pueden variar; lo que no cambia son los
principios de esta guía.

---

## 1. Antes de grabar

| Punto | Por qué importa |
|---|---|
| **Grabá en ventana de incógnito** | Con caché limpia el navegador pide `/static/app.js`, que es donde nace el `Etag` (y donde se lee la función de encriptación). Con caché sucia ese request no aparece. |
| **Máximo 5 sesiones por nodo (IP)** | La app limita 5 sesiones concurrentes por máquina. La sexta recibe `429 vus_per_node`. Si abandonás un script sin `/logout`, el cupo se libera solo a los 90 s. |
| **Usuarios de prueba** | `user001` … `user500`, contraseñas `Pass001!` … `Pass500!`. Armá tu CSV: `for i in $(seq -w 1 500); do echo "user$i,Pass$i!"; done` |
| **Un usuario por VU** | Cada sesión tiene su propio catálogo y asientos; no hay colisiones entre usuarios virtuales. |
| **Hacé la grabación de corrido** | El token de pago expira a los 60 s. Si te demorás entre reservar y pagar, grabás un 403. |
| **Activá las pistas mientras depurás** | Enviá el header `X-Practice-Hints: true` y los errores te dicen exactamente de dónde extraer el valor que falló. Sacalo para la prueba final. |

---

## 2. El flujo base

Este recorrido de **12 acciones** captura prácticamente todos los valores dinámicos de
la app. Cada paso indica: qué hace el usuario, qué requests genera, cuánto esperar
(think time), qué validar (assertions) y qué valores nacen o se usan.

> **Think time**: tiempo que un usuario real tarda en leer/decidir/tipear. Va **después**
> de recibir la respuesta y **antes** de la siguiente acción. Sin think times, el script
> no simula usuarios: simula una inundación.

---

### Paso 1 — Abrir la home

**Acción:** el usuario entra a `https://practice.relampo.com`

**Requests:**
```
GET /                    → 200 (HTML)
GET /static/app.js       → 200 (JS)   ← solo con caché limpia
GET /favicon.svg         → 200
```

**Think time:** 5–8 s (lee la página, ve el formulario de login)

**Assertions:**
- Status `200`
- El body contiene `RelampoTickets`
- El body contiene `name="csrf_token"` ← si esto falla, no vas a poder terminar el flujo

**Valores que nacen:**
- `relampo_session` (cookie — **única de toda la app**, en el header `Set-Cookie`)
- `csrf_token` (input oculto en el HTML)
- `Etag` (header de la respuesta de `app.js`)

> ⚠️ **El request más importante del script.** Acá nace la cookie que necesitan TODOS
> los requests siguientes, y el `csrf_token` que recién se usa en el paso 10.
> Si tu herramienta maneja cookies automáticamente, dejalo activado.

---

### Paso 2 — Iniciar sesión

**Acción:** escribe usuario y contraseña, hace clic en "Entrar"

**Requests:**
```
POST /api/auth           → 200 (JSON)
```
Body: `{"username":"user001","password":"Pass001!"}`

**Think time:** 10–15 s (tipear usuario y contraseña)

**Assertions:**
- Status `200`
- El JSON contiene el campo `bearer`
- `$.user.username` es igual al usuario que enviaste

**Valores que nacen:** `bearer` (JWT, en `$.bearer`)

---

### Paso 3 — Ver "Mis eventos"

**Acción:** tras el login, la app lo lleva a su panel de eventos

**Requests:**
```
GET /manage              → 200 (HTML)
GET /api/manage/events   → 200 (JSON)   [Authorization: Bearer ...]
```

**Think time:** 4–6 s (mira sus eventos)

**Assertions:**
- Status `200` en ambos
- El HTML contiene `name="publish_token"`
- El JSON tiene `$.count >= 1` (todo usuario arranca con el evento *Relampo Fest*)

**Valores que nacen:** `publish_token` (input oculto — **un solo uso**)

**Valores que usa:** `bearer` (header `Authorization`)

---

### Paso 4 — Crear un evento

**Acción:** llena nombre, lugar y fecha, hace clic en "Publicar evento"

**Requests:**
```
POST /api/manage/events  → 201 (JSON)   [Authorization: Bearer ...]
```
Body: `{"name":"...","venue":"...","date":"...","publishToken":"{{publish_token}}"}`

**Think time:** 15–25 s (llenar tres campos)

**Assertions:**
- Status `201`
- El JSON contiene `$.event.id`
- `$.event.name` es igual al nombre que enviaste

**Valores que nacen:** `event_id` (`$.event.id`), `event_rev` (`$.event.rev`)

**Valores que usa:** `publish_token`, `bearer`

> 💡 **Parametrizá el nombre del evento** (ej. `Evento-${VU}-${iteración}`) en vez de
> mandar siempre el mismo texto. Y recordá: máximo **5 eventos por usuario**; el sexto
> devuelve `409 events_limit`.

---

### Paso 5 — Abrir el evento para editar

**Acción:** hace clic en "editar" sobre su evento

**Requests:**
```
GET /manage/events/{{event_id}}/edit   → 200 (HTML)
```

**Think time:** 3–5 s

**Assertions:**
- Status `200`
- Existe el header `ETag`
- El HTML contiene el nombre del evento

**Valores que nacen:** `event_rev` (header **`ETag`**, entre comillas — hay que quitarlas)

---

### Paso 6 — Guardar cambios

**Acción:** cambia el nombre o el lugar, hace clic en "Guardar cambios"

**Requests:**
```
PUT /api/manage/events/{{event_id}}   → 200 (JSON)
     [Authorization: Bearer ...]  [If-Match: {{event_rev}}]
```

**Think time:** 10–15 s

**Assertions:**
- Status `200`
- `$.event.rev` **es distinta** de la que enviaste (cada update genera una versión nueva)

**Valores que usa:** `event_id`, `event_rev` (header `If-Match`), `bearer`
**Valores que nacen:** `event_rev_nueva` (`$.event.rev`) — necesaria para el paso 12

> ⚠️ **Bloqueo optimista.** Sin el header `If-Match` → `428`. Con una versión vieja →
> `412`. Cada modificación devuelve una `rev` nueva: siempre correlacioná la última.

---

### Paso 7 — Ir a comprar

**Acción:** hace clic en "comprar" en el menú

**Requests:**
```
GET /events              → 200 (HTML)
```

**Think time:** 3–5 s

**Assertions:**
- Status `200`
- El HTML contiene `data-config`

**Valores que nacen:** `catalog_id` (dentro de un **JSON escapado** en el atributo
`data-config`, con comillas como `&#34;`)

---

### Paso 8 — Ver el catálogo y los asientos

**Acción:** la página carga eventos y elige un asiento

**Requests:**
```
GET /api/events?catalogId={{catalog_id}}      → 200 (JSON)  [Bearer]
GET /api/events/{{event_id}}/seats            → 200 (JSON)  [Bearer]
```

**Think time:** 5–8 s (elegir asiento)

**Assertions:**
- Status `200` en ambos
- `$.events` tiene al menos 1 elemento
- `$.seats` tiene 12 elementos
- Existe el header `X-Correlation-Id` en la respuesta de seats

**Valores que nacen:**
- `event_id` alternativo (`$.events[*].id` — **elegí una ocurrencia aleatoria**)
- `seat_id` (`$.seats[*].id` — **aleatoria también**)
- `X-Correlation-Id` (**header de respuesta**, un solo uso)

> 💡 **No uses índices fijos** (`$.seats[3].id`) si podés evitarlo. En esta app funciona
> porque cada sesión tiene su propio stock, pero contra un sitio real es la causa #1 de
> scripts que fallan en la segunda iteración.

---

### Paso 9 — Reservar el asiento

**Acción:** la app reserva automáticamente al cargar la página

**Requests:**
```
POST /api/reservations   → 201 (JSON)
     [Authorization: Bearer ...]  [X-Correlation-Id: {{X-Correlation-Id}}]
```
Body: `{"eventId":"{{event_id}}","seatId":"{{seat_id}}"}`

**Think time:** 3–5 s (ve el resumen antes de pagar) — **⏱️ máximo 60 s**, el token expira

**Assertions:**
- Status `201`
- El JSON contiene `$.relampoToken`
- `$.tokenExpiresInSeconds` es `60`

**Valores que nacen:** `reservation_id` (`$.reservationId`), **`relampo_token_a`** (`$.relampoToken`)

**Valores que usa:** `X-Correlation-Id` (header), `bearer`

---

### Paso 10 — Pagar (⭐ el paso con encriptación)

**Acción:** hace clic en "Pagar ahora"

**Requests:**
```
POST /pay/start          → 302 (redirect)
```
Body (form): `reservation_id={{reservation_id}}&relampo_token={{relampo_token_b}}`

**Think time:** 2–3 s

**Assertions:**
- Status `302`
- El header `Location` contiene `state=`

**Valores que usa:** `reservation_id`, **`relampo_token_b`** ← ver sección 4
**Valores que nacen:** `state` (del header `Location`, **URL-encoded**)

> ⚠️ **Este request NO se puede correlacionar solo con extractores.** El token que
> recibiste en el paso 9 **no es** el que hay que enviar: el navegador lo transforma con
> JavaScript. Ver la **sección 4** para el método y el código en cada herramienta.
>
> ⚠️ **Desactivá "seguir redirects" en este request**, o tu herramienta se comerá el 302
> y no vas a poder leer el `state` del `Location`.

---

### Paso 11 — Pasarela de pago

**Acción:** la pasarela redirige automáticamente (formulario auto-submit)

**Requests:**
```
GET  /pay/authorize?state={{state}}                    → 200 (HTML)
POST /pay/continue                                     → 302
GET  /pay/callback?code={{code}}&state={{state}}       → 200 (HTML)
```
Body del POST (form): `request={{request}}&x_correlation_id={{x_correlation_id}}`

**Think time:** 0 s entre estos tres (los dispara la máquina, no la persona), y 5–8 s
después del callback (el usuario revisa el resumen antes de confirmar)

**Assertions:**
- `/pay/authorize` → 200, el HTML contiene `name="request"`
- `/pay/continue` → 302, el `Location` contiene `code=`
- `/pay/callback` → 200, el HTML contiene `name="view_state"`

**Valores que nacen:**
- `request` y `x_correlation_id` (2 inputs ocultos del formulario auto-submit)
- `code` (header `Location` del 302)
- `view_state` (input oculto, base64 de ~1.4 KB)
- `reservation_id` (re-emitido como input oculto)

---

### Paso 12 — Confirmar la compra

**Acción:** hace clic en "Confirmar compra"

**Requests:**
```
POST /pay/confirm        → 200 (HTML)
GET  /api/tickets/{{ticket_id}}   → 200 (JSON)  [Bearer]
```
Body (form): `view_state={{view_state}}&code={{code}}&reservation_id={{reservation_id}}&csrf_token={{csrf_token}}`

**Think time:** 3–5 s

**Assertions:**
- Status `200`
- El HTML contiene `data-ticket`
- El JSON del ticket contiene `"status":"CONFIRMED"` ← **la assertion más importante del script**

**Valores que usa:** `view_state`, `code`, `reservation_id`, **`csrf_token`** (¡del paso 1!)
**Valores que nacen:** `ticket_id` (atributo `data-ticket` del tag `<body>`)

---

### Paso 13 — Cerrar sesión (recomendado)

**Acción:** hace clic en "salir"

**Requests:**
```
DELETE /api/manage/events/{{event_id}}   → 200  [If-Match: {{event_rev_nueva}}]   (opcional: limpiar)
GET /logout                              → 302
```

**Think time:** 2–3 s

**Assertions:** status `302` con `Location: /`

> 💡 **Terminá siempre con `/logout`.** Libera el cupo de sesión de tu nodo al instante
> y evita que la siguiente iteración choque con el límite de 5.

---

## 3. Tabla maestra de valores dinámicos

| Valor | Nace en | Ubicación en la respuesta | Se envía en | Ubicación en el request | Extractor sugerido |
|---|---|---|---|---|---|
| `relampo_session` | `GET /` | Header `Set-Cookie` | Todos | Header `Cookie` | Cookie manager (automático) |
| `csrf_token` | `GET /` | HTML, input oculto | `POST /pay/confirm` | Form body | `(?is)name=["']csrf_token["'][^>]*value=["']([^"']+)["']` |
| `Etag` | `GET /static/app.js` | Header `Etag` | mismo request | Header `If-None-Match` | `(?im)^Etag:\s*([^\r\n]+)$` — *opcional, no se valida* |
| `bearer` | `POST /api/auth` | JSON | Toda la API | Header `Authorization: Bearer` | jsonpath `$.bearer` |
| `publish_token` | `GET /manage` | HTML, input oculto | `POST /api/manage/events` | JSON, campo `publishToken` | `(?is)name=["']publish_token["'][^>]*value=["']([^"']+)["']` |
| `event_id` | `POST /api/manage/events` | JSON | edit / PUT / DELETE / seats | **Path** de la URL | jsonpath `$.event.id` |
| `event_rev` | `GET .../edit` o el PUT | Header `ETag` / JSON | PUT y DELETE | Header **`If-Match`** | `(?im)^ETag:\s*"?([^"\r\n]+)"?$` o jsonpath `$.event.rev` |
| `catalog_id` | `GET /events` | HTML, JSON escapado en `data-config` | `GET /api/events` | **Query param** `catalogId` | `(?is)data-config="[^"]*?catalogId(?:&#34;\|&quot;):(?:&#34;\|&quot;)([A-Za-z0-9-]+)` |
| `seat_id` | `GET .../seats` | JSON, array | `POST /api/reservations` | JSON body | jsonpath `$.seats[*].id` → **aleatorio** |
| `X-Correlation-Id` | `GET .../seats` | **Header de respuesta** | `POST /api/reservations` | **Header de request** | `(?im)^X-Correlation-Id:\s*([^\r\n]+)$` |
| `reservation_id` | `POST /api/reservations` | JSON | `/pay/start` y `/pay/confirm` | Form body | jsonpath `$.reservationId` |
| **`relampo_token`** ⭐ | `POST /api/reservations` | JSON (`$.relampoToken`) | `POST /pay/start` | Form body **transformado** | jsonpath + **preprocesador** (sección 4) |
| `state` | `POST /pay/start` | Header `Location` | authorize y callback | Query param | `(?im)^Location:\s*[^\r\n]*?[?&]state=([^&#\r\n]+)` + **url_decode** |
| `request` | `GET /pay/authorize` | HTML, input oculto | `POST /pay/continue` | Form body | `(?is)name=["']request["'][^>]*value=["']([^"']+)["']` |
| `x_correlation_id` | `GET /pay/authorize` | HTML, input oculto | `POST /pay/continue` | Form body | `(?is)name=["']x_correlation_id["'][^>]*value=["']([^"']+)["']` |
| `code` | `POST /pay/continue` | Header `Location` | callback y confirm | Query param + form body | `(?im)^Location:\s*[^\r\n]*?[?&]code=([^&#\r\n]+)` |
| `view_state` | `GET /pay/callback` | HTML, input oculto | `POST /pay/confirm` | Form body | `(?is)name=["']view_state["'][^>]*value=["']([^"']+)["']` |
| `ticket_id` | `POST /pay/confirm` | HTML, atributo del `<body>` | `GET /api/tickets/{id}` | **Path** de la URL | `(?is)data-ticket=["']([^"']+)["']` |

### Valores de un solo uso o con vencimiento

| Valor | Regla | Consecuencia si no la respetás |
|---|---|---|
| `publish_token` | 1 solo uso | Hay que volver a `GET /manage` antes de cada creación |
| `X-Correlation-Id` | 1 solo uso | Hay que pedir seats antes de cada reserva |
| `relampo_token` | 1 uso + **60 s** | Reserva nueva en cada iteración, y pagar rápido |
| `state` / flujo de pago | 5 minutos | Completar el pago dentro de ese margen |
| `event_rev` | cambia en cada update | Re-correlacionar después de cada PUT |
| `bearer` | 2 horas | Suficiente para cualquier prueba |
| Sesión | 30 min de inactividad | Con think times normales no molesta |

### Sintaxis de extractores por herramienta

**Desde el body HTML (regex):**

| Herramienta | Cómo |
|---|---|
| Relampo | `- type: regex` / `from: body` / `var: csrf_token` / `pattern: ...` / `group: 1` |
| JMeter | *Regular Expression Extractor* → Field to check: **Body**, Template: `$1$`, Match No: `1` |
| k6 | `const csrf = res.body.match(/name="csrf_token" value="([^"]+)"/)[1];` |
| Gatling | `.check(regex("""name="csrf_token" value="([^"]+)"""").saveAs("csrf_token"))` |

**Desde el body JSON:**

| Herramienta | Cómo |
|---|---|
| Relampo | `- type: jsonpath` / `expression: $.bearer` |
| JMeter | *JSON Extractor* → JSON Path: `$.bearer` |
| k6 | `const bearer = res.json().bearer;` |
| Gatling | `.check(jsonPath("$.bearer").saveAs("bearer"))` |

**Desde un header de respuesta:**

| Herramienta | Cómo |
|---|---|
| Relampo | `- type: regex` / `from: headers` / `pattern: (?im)^X-Correlation-Id:\s*([^\r\n]+)$` |
| JMeter | *Regular Expression Extractor* → Field to check: **Response Headers** |
| k6 | `const corr = res.headers['X-Correlation-Id'];` |
| Gatling | `.check(header("X-Correlation-Id").saveAs("corr"))` |

**Desde el `Location` de un redirect (302):**

⚠️ Primero desactivá el seguimiento automático de redirects, si no, no vas a ver el 302.

| Herramienta | Cómo |
|---|---|
| Relampo | `follow_redirects: false` + `- type: regex` / `from: headers` / `transform: url_decode` |
| JMeter | Destildar *Follow Redirects* + Regex Extractor sobre Response Headers + `__urldecode()` |
| k6 | `const res = http.post(url, body, { redirects: 0 }); const state = decodeURIComponent(res.headers['Location'].match(/state=([^&]+)/)[1]);` |
| Gatling | `.disableFollowRedirect.check(header("Location").transform(...).saveAs("state"))` |

---

## 4. ⭐ El caso especial: `relampo_token` (valor encriptado)

### El problema

```
Respuesta de POST /api/reservations:
  "relampoToken": "9788897b868175dc..."          ← valor A (64 caracteres)

Request de POST /pay/start:
  relampo_token=4b5254595549584f...              ← valor B (128 caracteres)
```

**A ≠ B.** El valor B **no aparece en ninguna respuesta**: lo calcula el JavaScript del
navegador antes de enviar el formulario. Si correlacionás A y lo reenviás tal cual, el
servidor responde `403 you sent the RAW relampo_token (value A)`.

Ningún correlador automático puede resolver esto: hay que **escribir código**.

### El método

```
B = por cada carácter de A:  (código del carácter  XOR  código del carácter de la sal)
                             escrito en hexadecimal de 2 dígitos
```

Con `sal = "relampo-public-salt-v1"` (pública, está en `/static/app.js`). Cuando la sal
se termina (22 caracteres), vuelve a empezar desde el principio. Como cada carácter se
convierte en 2 dígitos hex, **B siempre mide el doble que A**.

Ejemplo de las primeras dos vueltas:

| Carácter de A | Carácter de la sal | XOR | En hex |
|---|---|---|---|
| `9` (57) | `r` (114) | 75 | `4b` |
| `7` (55) | `e` (101) | 82 | `52` |

→ B empieza con `4b52...`

### El código, por herramienta

**Relampo** — bloque `spark` en el request `POST /pay/start`:

```javascript
// PASO 1 — Leer el token que extrajimos en el request anterior
var tokenA = vars.get("relampo_token_a");

// PASO 2 — Transformarlo (misma lógica que app.js en el navegador)
var salt = "relampo-public-salt-v1";
var tokenB = "";

for (var i = 0; i < tokenA.length; i++) {
  var x = tokenA.charCodeAt(i) ^ salt.charCodeAt(i % salt.length);
  tokenB = tokenB + (x < 16 ? "0" : "") + x.toString(16);
}

// PASO 3 — Guardarlo para usarlo en el body como {{relampo_token_b}}
vars.set("relampo_token_b", tokenB);
```

**JMeter** — *JSR223 PreProcessor* (lenguaje Groovy) colgado del sampler de pago:

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

**k6** — función normal de JavaScript:

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

// uso
const tokenA = res.json().relampoToken;
const tokenB = signRelampoToken(tokenA);
```

**Gatling** — función Scala + `session.set`:

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

// uso
.exec(session => session.set("tokenB", signRelampoToken(session("tokenA").as[String])))
```

### Cómo verificar que lo hiciste bien

1. El valor B debe medir **exactamente el doble** que A (128 vs 64 caracteres).
2. El `POST /pay/start` debe responder **302** con `Location: /pay/authorize?state=...`
3. Corré **dos iteraciones seguidas**: si la segunda también pasa, la transformación está
   bien (el token es de un solo uso, así que un valor grabado fallaría).

### Modo avanzado (opcional)

Si la app corre con `RELAMPO_TOKEN_MODE=hmac`, la transformación pasa a ser
`B = hex(HMAC-SHA256(A, salt))`. En k6: `crypto.hmac('sha256', salt, tokenA, 'hex')`.
En JMeter: `javax.crypto.Mac` con `HmacSHA256`.

---

## 5. Assertions: qué validar en cada tipo de respuesta

Un script sin assertions **miente**: reporta éxito aunque el servidor esté devolviendo
errores. Reglas prácticas:

| Tipo de request | Qué validar siempre | Qué validar además |
|---|---|---|
| Página HTML | Status `200` | Un texto que solo aparece si la página cargó bien (ej. `Mis eventos`) y el input oculto que vas a extraer |
| API JSON | Status esperado (`200`/`201`) | Que exista el campo que vas a correlacionar |
| Redirect (302) | Status `302` | Que el `Location` contenga el parámetro que vas a extraer |
| Confirmación final | Status `200` | `"status":"CONFIRMED"` en el ticket |

**Assertion negativa recomendada** en todos los requests: que el body **NO** contenga
`correlationError`. Si aparece, algún valor se correlacionó mal y el script está
"pasando" sobre un flujo roto.

**Errores que devuelve la app y qué significan:**

| Status | `variable` | Qué te está diciendo |
|---|---|---|
| 401 | `relampo_session` | Falta la cookie o expiró la sesión |
| 401 | `bearer` | Token ausente, mal copiado o de otra sesión |
| 400 | `catalog_id` | El `catalogId` no es el de esta sesión |
| 403 | `x_correlation_id` | No reenviaste el header de la respuesta de seats |
| 403 | `relampo_token` | Mandaste el crudo, o mal transformado, o ya usado, o expirado |
| 403 | `state` / `code` | Extrajiste mal el `Location` (¿te comiste el 302?) |
| 403 | `csrf_token` | Perdiste el valor del paso 1 |
| 428 / 412 | `if_match` | Falta la `rev`, o mandaste una vieja |
| 409 | `events_limit` | El usuario ya tiene 5 eventos |
| 429 | `vus_per_node` | Más de 5 sesiones concurrentes desde tu IP |

Todos los errores traen `received` y `expected` para que compares. Con el header
`X-Practice-Hints: true` agregan un campo `hint` con el lugar exacto de extracción.

---

## 6. Think times y perfil de carga

**Think times sugeridos** (usá una variación de ±30 % para que no queden sincronizados):

| Momento | Tiempo |
|---|---|
| Leer una página | 3–8 s |
| Llenar un formulario corto (login) | 10–15 s |
| Llenar un formulario largo (crear evento) | 15–25 s |
| Elegir de una lista | 5–8 s |
| Entre requests automáticos (redirects, fetch de la misma página) | 0 s |
| **Entre reservar y pagar** | **máximo 60 s** ⏱️ |

**Perfil de carga para esta app:** con el límite de 5 sesiones por nodo, una prueba
razonable es 5 VUs por nodo con rampa de 30 s y 10–15 minutos de duración. Si usás la
ejecución distribuida de Relampo (4 nodos), llegás a 20 VUs.

---

## 7. Checklist antes de dar el script por terminado

- [ ] Corre **dos iteraciones seguidas** sin errores (la prueba real de la correlación)
- [ ] Ningún valor dinámico quedó escrito a mano (buscá `EV-`, `CAT-`, `TCK-`, `RES-`, `PUB-` en el script)
- [ ] El `csrf_token` del paso 1 llega hasta el paso 12
- [ ] El `relampo_token` se transforma con el preprocesador (no se envía crudo)
- [ ] Los redirects del pago están con "seguir redirects" desactivado donde hace falta
- [ ] Hay assertions en todos los pasos, incluida `"status":"CONFIRMED"` al final
- [ ] Hay think times entre las acciones del usuario
- [ ] Usuario y datos están parametrizados (CSV), no hardcodeados
- [ ] El flujo termina en `/logout`
- [ ] Sacaste el header `X-Practice-Hints` de la versión final

---

## 8. Ejercicios por nivel

| Nivel | Objetivo | Valores que hay que resolver |
|---|---|---|
| **1 — Básico** | Login y navegar | `relampo_session`, `csrf_token`, `bearer` |
| **2 — Intermedio** | Crear y editar un evento | `publish_token` (1 uso), `event_id`, `event_rev` (`If-Match`, 428/412) |
| **3 — Avanzado** | Comprar una entrada | `catalog_id` (JSON escapado), `X-Correlation-Id`, `state` y `code` (redirects), `view_state` |
| **4 — Experto** | Pago completo | **`relampo_token`** con preprocesador + las reglas de un solo uso y 60 s |
| **5 — Maestro** | Prueba de carga real | Todo lo anterior + datos parametrizados, ocurrencias aleatorias, think times variables y assertions completas |
