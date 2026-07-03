package main

import (
	"html/template"
	"strings"
)

// All CSS lives here, in one place. To re-brand the app, change the variables
// in :root — nothing else references raw colors.
const baseCSS = `
:root {
  --bg: #0b0f1a;
  --bg-panel: #121829;
  --bg-inset: #0e1422;
  --border: #26304d;
  --text: #e8ecf6;
  --text-dim: #93a0bd;
  --accent: #ffd60a;
  --accent-strong: #ffc300;
  --accent-ink: #14180a;
  --ok: #4ade80;
  --err: #f87171;
  --mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
* { box-sizing: border-box; margin: 0; padding: 0; }
body {
  background: var(--bg);
  background-image: radial-gradient(ellipse 80% 50% at 50% -20%, rgba(255, 214, 10, 0.08), transparent);
  color: var(--text);
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  min-height: 100vh;
}
header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 16px 28px; border-bottom: 1px solid var(--border);
}
.logo { font-size: 20px; font-weight: 800; letter-spacing: -0.5px; text-decoration: none; color: var(--text); }
.logo .bolt { color: var(--accent); margin-right: 6px; }
.logo .tag { color: var(--accent); }
nav a { color: var(--text-dim); text-decoration: none; margin-left: 18px; font-size: 14px; }
nav a:hover { color: var(--accent); }
main { max-width: 860px; margin: 0 auto; padding: 40px 24px 80px; }
h1 { font-size: 30px; letter-spacing: -0.5px; margin-bottom: 8px; }
h1 .hl { color: var(--accent); }
.sub { color: var(--text-dim); margin-bottom: 28px; font-size: 15px; }
.panel {
  background: var(--bg-panel); border: 1px solid var(--border);
  border-radius: 12px; padding: 24px; margin-bottom: 20px;
}
.panel h2 { font-size: 17px; margin-bottom: 14px; }
label { display: block; font-size: 13px; color: var(--text-dim); margin: 12px 0 4px; }
input[type=text], input[type=password] {
  width: 100%; padding: 10px 12px; border-radius: 8px; border: 1px solid var(--border);
  background: var(--bg-inset); color: var(--text); font-size: 15px;
}
input:focus { outline: none; border-color: var(--accent); }
button, .btn {
  display: inline-block; margin-top: 18px; padding: 11px 22px; border: none; border-radius: 8px;
  background: var(--accent); color: var(--accent-ink); font-weight: 700; font-size: 15px;
  cursor: pointer; text-decoration: none;
}
button:hover, .btn:hover { background: var(--accent-strong); }
.status { margin-top: 14px; font-size: 14px; color: var(--text-dim); }
.status.err { color: var(--err); }
.kv { display: grid; grid-template-columns: 160px 1fr; gap: 6px 14px; font-size: 14px; }
.kv dt { color: var(--text-dim); }
.kv dd { font-family: var(--mono); word-break: break-all; }
code, .tok {
  font-family: var(--mono); font-size: 12.5px; background: var(--bg-inset);
  border: 1px solid var(--border); border-radius: 6px; padding: 2px 6px; word-break: break-all;
}
.note { font-size: 13px; color: var(--text-dim); line-height: 1.6; }
.note a { color: var(--accent); }
.badge {
  display: inline-block; font-size: 11px; font-weight: 700; letter-spacing: 0.6px;
  color: var(--accent); border: 1px solid var(--accent); border-radius: 999px;
  padding: 2px 10px; margin-bottom: 14px; text-transform: uppercase;
}
.receipt { font-size: 40px; margin: 6px 0 2px; color: var(--ok); font-weight: 800; }
pre.json {
  font-family: var(--mono); font-size: 12.5px; background: var(--bg-inset);
  border: 1px solid var(--border); border-radius: 8px; padding: 14px;
  overflow-x: auto; margin-top: 12px; color: var(--text);
}
footer { text-align: center; color: var(--text-dim); font-size: 12px; padding: 24px; }
`

const headerHTML = `
<header>
  <a class="logo" href="/"><span class="bolt">&#9889;</span>Relampo<span class="tag">Tickets</span></a>
  <nav>
    <a href="/events">comprar</a>
    <a href="/manage">mis eventos</a>
    <a href="/logout">salir</a>
  </nav>
</header>`

const footerHTML = `
<footer>RelampoTickets &mdash; app de pr&aacute;ctica para correlaci&oacute;n en pruebas de performance</footer>`

var homeTmpl = mustPage("home", `
<body data-page="home">`+headerHTML+`
<main>
  <h1>Entradas para tus eventos, <span class="hl">a la velocidad del rayo</span></h1>
  <p class="sub">Inici&aacute; sesi&oacute;n para comprar entradas.</p>
  <div class="panel">
    <h2>Iniciar sesi&oacute;n</h2>
    <form id="loginForm">
      <input type="hidden" name="csrf_token" value="{{.CSRF}}">
      <label for="username">Usuario</label>
      <input type="text" id="username" name="username" autocomplete="off" placeholder="usuario">
      <label for="password">Contrase&ntilde;a</label>
      <input type="password" id="password" name="password" placeholder="contrase&ntilde;a">
      <button type="submit">Entrar</button>
      <div id="status" class="status"></div>
    </form>
  </div>
</main>`+footerHTML+`
<script src="/static/app.js"></script>
</body>`)

var eventsTmpl = mustPage("events", `
<body data-page="events">`+headerHTML+`
<main>
  <h1>Eleg&iacute; tu <span class="hl">evento</span></h1>
  <p class="sub">Hola <strong>{{.User}}</strong>. La app elige un evento y un asiento al azar, reserva, y te deja listo el pago.</p>
  <div id="app" data-config="{{.ConfigJSON}}"></div>
  <div class="panel">
    <h2>Tu selecci&oacute;n</h2>
    <dl class="kv">
      <dt>Evento</dt><dd id="evName">&mdash;</dd>
      <dt>Lugar</dt><dd id="evVenue">&mdash;</dd>
      <dt>Fecha</dt><dd id="evDate">&mdash;</dd>
      <dt>Asiento</dt><dd id="seatId">&mdash;</dd>
      <dt>Precio</dt><dd id="seatPrice">&mdash;</dd>
    </dl>
    <div id="status" class="status">Cargando&hellip;</div>
  </div>
  <div class="panel" id="payBox" style="display:none">
    <h2>Pago seguro con RelampoPay</h2>
    <form id="payForm" method="POST" action="/pay/start">
      <input type="hidden" id="resvId" name="reservation_id" value="">
      <input type="hidden" id="relampoToken" name="relampo_token" value="">
      <button type="submit">Pagar ahora</button>
    </form>
  </div>
</main>`+footerHTML+`
<script src="/static/app.js"></script>
</body>`)

var manageTmpl = mustPage("manage", `
<body data-page="manage">`+headerHTML+`
<main>
  <h1>Mis <span class="hl">eventos</span></h1>
  <p class="sub">Cre&aacute; tus propios eventos: aparecen en el cat&aacute;logo y se pueden comprar.</p>
  <div class="panel">
    <h2>Crear evento</h2>
    <form id="createForm">
      <input type="hidden" name="publish_token" value="{{.PublishToken}}">
      <label for="evname">Nombre</label>
      <input type="text" id="evname" autocomplete="off" placeholder="Festival de Invierno">
      <label for="evvenue">Lugar</label>
      <input type="text" id="evvenue" autocomplete="off" placeholder="Teatro Principal">
      <label for="evdate">Fecha</label>
      <input type="text" id="evdate" autocomplete="off" placeholder="2026-12-01">
      <button type="submit">Publicar evento</button>
      <div id="status" class="status"></div>
    </form>
  </div>
  <div class="panel">
    <h2>Eventos publicados</h2>
    <div id="myEvents" class="note">Cargando&hellip;</div>
  </div>
</main>`+footerHTML+`
<script src="/static/app.js"></script>
</body>`)

var editTmpl = mustPage("edit", `
<body data-page="edit">`+headerHTML+`
<main>
  <h1>Editar <span class="hl">evento</span></h1>
  <div class="panel">
    <h2>{{.Name}}</h2>
    <form id="editForm">
      <input type="hidden" id="eventId" value="{{.ID}}">
      <input type="hidden" id="eventRev" name="rev" value="{{.Rev}}">
      <label for="evname">Nombre</label>
      <input type="text" id="evname" value="{{.Name}}">
      <label for="evvenue">Lugar</label>
      <input type="text" id="evvenue" value="{{.Venue}}">
      <label for="evdate">Fecha</label>
      <input type="text" id="evdate" value="{{.Date}}">
      <button type="submit">Guardar cambios</button>
      <a class="btn" style="background:transparent;color:var(--text-dim);border:1px solid var(--border)" href="/manage">Cancelar</a>
      <div id="status" class="status"></div>
    </form>
  </div>
</main>`+footerHTML+`
<script src="/static/app.js"></script>
</body>`)

var authorizeTmpl = mustPage("authorize", `
<body data-page="authorize">
<main style="max-width:520px">
  <div class="panel" style="margin-top:60px;text-align:center">
    <div class="badge">RelampoPay</div>
    <h2>Redirigiendo al procesador de pagos&hellip;</h2>
    <p class="note" style="margin-top:10px">No cierres esta ventana.</p>
    <form id="payform" method="POST" action="/pay/continue">
      <input type="hidden" name="request" value="{{.Request}}">
      <input type="hidden" name="x_correlation_id" value="{{.XCorr}}">
      <noscript><button type="submit">Continuar</button></noscript>
    </form>
  </div>
</main>
<script>document.getElementById('payform').submit();</script>
</body>`)

var callbackTmpl = mustPage("callback", `
<body data-page="callback">`+headerHTML+`
<main>
  <div class="badge">RelampoPay &mdash; confirmaci&oacute;n</div>
  <h1>Revis&aacute; tu <span class="hl">compra</span></h1>
  <div class="panel">
    <h2>Resumen</h2>
    <dl class="kv">
      <dt>Evento</dt><dd>{{.EventName}}</dd>
      <dt>Asiento</dt><dd>{{.SeatID}}</dd>
      <dt>Precio</dt><dd>$ {{.Price}}</dd>
    </dl>
    <form id="confirmForm" method="POST" action="/pay/confirm">
      <input type="hidden" name="view_state" value="{{.ViewState}}">
      <input type="hidden" name="code" value="{{.Code}}">
      <input type="hidden" name="reservation_id" value="{{.ReservationID}}">
      <input type="hidden" id="csrfField" name="csrf_token" value="">
      <button type="submit">Confirmar compra</button>
    </form>
  </div>
</main>`+footerHTML+`
<script src="/static/app.js"></script>
</body>`)

var successTmpl = mustPage("success", `
<body data-page="success" data-ticket="{{.TicketID}}">`+headerHTML+`
<main>
  <div class="badge">compra confirmada</div>
  <div class="panel" style="text-align:center">
    <div class="receipt">&#9889; Compra confirmada</div>
    <p class="sub">Tu entrada qued&oacute; emitida, {{.User}}.</p>
    <dl class="kv" style="text-align:left">
      <dt>Evento</dt><dd>{{.EventName}}</dd>
      <dt>Asiento</dt><dd>{{.SeatID}}</dd>
    </dl>
    <p id="ticketInfo" class="status">Verificando tu entrada&hellip;</p>
    <a class="btn" href="/events">Comprar otra entrada</a>
  </div>
</main>`+footerHTML+`
<script src="/static/app.js"></script>
</body>`)

func mustPage(name, body string) *template.Template {
	const skeleton = `<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>RelampoTickets</title>
<style>%CSS%</style>
</head>
%BODY%
</html>`
	page := strings.ReplaceAll(skeleton, "%CSS%", baseCSS)
	page = strings.ReplaceAll(page, "%BODY%", body)
	return template.Must(template.New(name).Parse(page))
}
