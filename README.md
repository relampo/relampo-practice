# ⚡ RelampoTickets

App de práctica para **correlación de valores dinámicos en pruebas de performance**.
Simula una venta de entradas para eventos con un gateway de pago interno (RelampoPay),
diseñada para grabarse con el recorder MITM de Relampo y scriptearse en cualquier
herramienta de carga (Relampo, JMeter, k6, Gatling, LoadRunner).

Un solo binario Go, **sin base de datos ni dependencias externas**: 500 usuarios
hardcodeados, estado en memoria con TTL, todo el JavaScript servido desde la misma app
(cero CDNs — funciona sin internet y el MITM captura el 100% del tráfico).

Dos flujos de negocio encadenados: **gestionar eventos propios** (crear → editar →
borrar, máximo **5 por usuario**) y **comprar una entrada** (catálogo → reserva →
gateway de pago → ticket). El catálogo de compra son exactamente los eventos creados
por el usuario, así que el flujo típico es: **login → crear evento → comprar entrada**.

Reglas de UI: los links del menú (comprar / mis eventos / salir) solo aparecen con un
usuario logueado; "comprar" solo cuando el usuario tiene eventos creados. Tras el
login, la primera pantalla es **Mis eventos**: si no hay ninguno, el formulario de
crear es lo primero que se ve.

## Cómo correrla

```bash
go run .                 # escucha en :8080 (variable PORT para cambiarlo)
go test ./...            # test end-to-end del flujo correlacionado completo
```

Docker:

```bash
docker build -t relampo-tickets .
docker run -p 8080:8080 relampo-tickets
```

## Usuarios

500 usuarios generados en código: `user001`…`user500` con contraseña `Pass001!`…`Pass500!`.
El patrón es predecible a propósito: el data pool CSV se arma a mano en un minuto
(o con un one-liner: `for i in $(seq -w 1 500); do echo "user$i,Pass$i!"; done`).
Las credenciales no se muestran en ninguna pantalla de la app.

## Cookies

**Una sola cookie en toda la app**: `relampo_session`, emitida en el `Set-Cookie` de la
primera respuesta de `GET /` (home). Ningún otro endpoint emite cookies; todos los demás
requests la exigen como header `Cookie`.

## El flujo (lo que graba el recorder)

```
[1]  GET  /                                → cookie única + csrf_token (HTML oculto)
[2]  GET  /static/app.js                   → Etag (→ If-None-Match / 304)
[3]  POST /api/auth                        → bearer JWT (JSON)
     (requiere ≥1 evento creado: sin eventos, /events redirige a /manage)
[4]  GET  /events                          → catalogId (JSON escapado en atributo HTML)
[5]  GET  /api/events?catalogId=…          → array con TUS eventos (elegir uno al azar)
[6]  GET  /api/events/{id}/seats           → asientos + header X-Correlation-Id
[7]  POST /api/reservations                → reservationId + relampo_token (valor A)
[8]  POST /pay/start                       → 302 con state en Location  ⭐ exige valor B
[9]  GET  /pay/authorize?state=…           → auto-submit form (request, x_correlation_id)
[10] POST /pay/continue                    → 302 con code en Location
[11] GET  /pay/callback?code=…&state=…     → view_state (base64 grande, HTML oculto)
[12] POST /pay/confirm                     → página de éxito con data-ticket
[13] GET  /api/tickets/{ticket_id}         → verificación final (assertions)
[14] GET  /logout                          → invalida la sesión
```

## Flujo 2: gestión de eventos (crear / editar / borrar)

```
[15] GET  /manage                          → publish_token (input oculto, UN SOLO USO)
[16] POST /api/manage/events               → crea evento; exige publishToken en el JSON
                                             devuelve $.event.id + $.event.rev
[17] GET  /manage/events/{id}/edit         → rev vigente en el header ETag de la respuesta
[18] PUT  /api/manage/events/{id}          → exige header If-Match con la rev vigente
                                             (falta → 428, rev vieja → 412) devuelve rev nueva
[19] GET  /api/manage/events               → lista de eventos propios (con sus rev)
[20] DELETE /api/manage/events/{id}        → borra, también con If-Match
```

Los eventos creados SON el catálogo de compra: el flujo 2 alimenta al flujo 1
(en cada iteración: crear evento → comprarle una entrada). Límite de negocio:
**máximo 5 eventos por usuario** — el sexto `POST /api/manage/events` responde
`409` con `variable: events_limit` (hay que borrar uno para liberar lugar).

## Mapa de correlación

| Variable | Nace en | Extractor | Viaja en el siguiente request como |
|---|---|---|---|
| `relampo_session` | `Set-Cookie` de `GET /` | cookie manager (auto) | header `Cookie` (todos los requests) |
| `csrf_token` | input oculto en `GET /` | regex | form body de `POST /pay/confirm` (¡al final del flujo!) |
| `Etag` | header de `GET /static/app.js` | regex sobre headers | header `If-None-Match` |
| `bearer` | body JSON de `POST /api/auth` | jsonpath `$.bearer` | header `Authorization: Bearer …` |
| `catalogId` | JSON escapado en `data-config` de `GET /events` | regex + unescape | query param `?catalogId=` |
| `event_id` | `$.events[*].id` de `GET /api/events` | jsonpath (ocurrencia aleatoria) | **path** `/api/events/{id}/seats` |
| `seat_id` | `$.seats[*].id` de seats | jsonpath (ocurrencia aleatoria) | body JSON de `POST /api/reservations` |
| `X-Correlation-Id` | **header de respuesta** de seats | regex sobre headers | **header de request** de `POST /api/reservations` |
| `reservation_id` | `$.reservationId` | jsonpath | form body de `POST /pay/start` |
| `relampo_token` ⭐ | `$.relampoToken` (valor A) | jsonpath | form body de `POST /pay/start`, **transformado** (ver abajo) |
| `state` | header `Location` del 302 de `/pay/start` | regex sobre headers + **url_decode** | query param de `GET /pay/authorize` |
| `request`, `x_correlation_id` | inputs ocultos de `/pay/authorize` | 2 regex | form body de `POST /pay/continue` |
| `code` | header `Location` del 302 de `/pay/continue` | regex sobre headers | query param del callback + form body de confirm |
| `view_state` | input oculto de `/pay/callback` (~1.4 KB base64) | regex | form body de `POST /pay/confirm` |
| `ticket_id` | atributo `data-ticket` del HTML de éxito | regex | path de `GET /api/tickets/{id}` |
| `publish_token` | input oculto en `GET /manage` (un solo uso) | regex | campo `publishToken` del **body JSON** de create |
| `event_id` (propio) | `$.event.id` de create | jsonpath | path de edit/update/delete |
| `rev` | header **`ETag`** de la página de edit, o `$.event.rev` | regex sobre headers / jsonpath | header **`If-Match`** de PUT/DELETE (sin él → 428; obsoleta → 412) |

## ⭐ El `relampo_token` (mismo nombre, dos valores: A ≠ B)

El token que **recibes** en la respuesta (`$.relampoToken`, valor A) **no** es el que
debes **enviar**: el navegador lo transforma con JavaScript antes de mandarlo en
`POST /pay/start`. Para scriptear el flujo hay que aplicar la misma transformación en
un **preprocesador** del request. La función `signRelampoToken()` está visible en
`/static/app.js`.

### Modo `simple` (por defecto) — cifrado XOR, ~5 líneas de JS puro

Cada carácter de A se XORea con la sal pública y se emite en hexadecimal.
Funciona en cualquier motor JavaScript, sin librerías:

```javascript
function signRelampoToken(a) {
  var salt = 'relampo-public-salt-v1', out = '';
  for (var i = 0; i < a.length; i++) {
    var x = a.charCodeAt(i) ^ salt.charCodeAt(i % salt.length);
    out += (x < 16 ? '0' : '') + x.toString(16);
  }
  return out;
}
```

En Groovy/JMeter la misma idea son 4 líneas con `charAt`/`String.format("%02x", …)`.

### Modo `hmac` (avanzado) — arrancar con `RELAMPO_TOKEN_MODE=hmac`

```
B = hex( HMAC-SHA256( A, "relampo-public-salt-v1" ) )
```

- **JMeter** (JSR223/Groovy):
  ```groovy
  import javax.crypto.Mac
  import javax.crypto.spec.SecretKeySpec
  def mac = Mac.getInstance("HmacSHA256")
  mac.init(new SecretKeySpec("relampo-public-salt-v1".bytes, "HmacSHA256"))
  vars.put("relampo_token_b", mac.doFinal(vars.get("relampo_token_a").bytes).encodeHex().toString())
  ```
- **k6**: `import crypto from 'k6/crypto'; crypto.hmac('sha256', salt, tokenA, 'hex')`
- **curl / bash**: `printf '%s' "$A" | openssl dgst -sha256 -hmac "relampo-public-salt-v1" -hex`

El `/static/app.js` servido se adapta al modo activo, así que lo que graba el recorder
siempre coincide con lo que valida el servidor.

Reglas del token (en ambos modos): **un solo uso** (reusarlo → 403) y **expira a los
60 segundos** (scripts lentos o valores grabados → 403). Enviar el valor A crudo → 403
con mensaje explícito.

## Límite de carga: 5 sesiones concurrentes por nodo (IP)

Cada usuario virtual abre una sesión en `GET /`. Un mismo nodo de carga (una IP,
una máquina) admite **máximo 5 sesiones vivas a la vez**; la sexta recibe **429**
con `variable: vus_per_node`. Con la ejecución distribuida de Relampo (4 nodos),
el máximo natural es 4 × 5 = 20 VUs.

No hace falta desloguearse para liberar cupos: si un script se corta sin
`/logout`, sus sesiones quedan "zombis" (dejan de hacer requests) y el servidor
las desaloja automáticamente cuando el mismo nodo necesita el lugar (tras ~90 s
de inactividad). `GET /logout` lo libera al instante (buena práctica del flujo).

Configuración:

- `RELAMPO_MAX_SESSIONS_PER_IP` — sesiones concurrentes por nodo (default `5`; `0` = sin límite).
- `RELAMPO_ZOMBIE_IDLE_SECONDS` — segundos de inactividad para considerar una
  sesión desalojable (default `90`; mínimo `5`). Bajarlo (ej. `30`) hace más
  fluido el ciclo de debug corrida-tras-corrida sin logout.

Detrás de un proxy/App Runner, la IP se toma del header `X-Forwarded-For`.

Tip para depurar en tu máquina sin límites: `RELAMPO_MAX_SESSIONS_PER_IP=0 go run .`

## Errores de correlación (para aprender)

Cuando un valor correlacionado llega mal, el servidor responde un 4xx que lo dice
explícitamente y muestra **lo que recibió vs lo que esperaba** (en vista previa
truncada, suficiente para diagnosticar sin regalar el valor completo):

```json
{
  "error": "valor correlacionado incorrecto: el catalogId enviado no coincide con el emitido para esta sesión",
  "variable": "catalog_id",
  "correlationError": true,
  "received": "CAT-inventado",
  "expected": "CAT-3982ca4dd1c9",
  "status": 400
}
```

Si además envías el header **`X-Practice-Hints: true`**, la respuesta incluye un campo
`hint` que dice exactamente de dónde extraer el valor.

```bash
curl -s -H "X-Practice-Hints: true" http://localhost:8080/api/events | jq
```

## Endpoints auxiliares

- `GET /health` — estado y uptime.
- `GET /debug/sessions` — sesiones activas en vivo (usuario, paso del flujo, edad):
  útil para monitorear durante la prueba de carga.

## Deploy en AWS (después de practicar local)

El mismo binario corre en cualquier lado. Rutas simples:

1. **App Runner / ECS Fargate**: `docker build` + push a ECR. Cero servidores.
2. **EC2**: `GOOS=linux go build -o relampo-tickets .` y correr el binario.

El estado vive en memoria: usa **una sola instancia** (es lo correcto para una app de
práctica; si algún día escalas horizontal, necesitarás sticky sessions).
