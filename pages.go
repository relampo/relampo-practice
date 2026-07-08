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
nav { display: flex; align-items: center; }
nav a { color: var(--text-dim); text-decoration: none; margin-left: 18px; font-size: 14px; }
nav a:hover { color: var(--accent); }
.lang { margin-left: 22px; display: inline-flex; gap: 6px; }
.lang a {
  margin-left: 0; font-size: 12px; font-weight: 700; padding: 3px 9px;
  border: 1px solid var(--border); border-radius: 6px; color: var(--text-dim);
}
.lang a.active { background: var(--accent); border-color: var(--accent); color: var(--accent-ink); }
.userchip {
  margin-left: 18px; font-size: 12px; font-weight: 700; font-family: var(--mono);
  color: var(--accent); border: 1px solid var(--accent); border-radius: 999px;
  padding: 3px 10px;
}
.cols { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; align-items: start; }
@media (max-width: 720px) { .cols { grid-template-columns: 1fr; } }
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
.features-head {
  margin-top: 44px; margin-bottom: 14px; font-size: 12px; letter-spacing: 1.4px;
  text-transform: uppercase; color: var(--accent); font-weight: 700;
}
.features { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
@media (max-width: 640px) { .features { grid-template-columns: 1fr; } }
.feature {
  background: var(--bg-panel); border: 1px solid var(--border);
  border-radius: 12px; padding: 18px 18px 16px;
}
.feature .ico { display: block; margin-bottom: 12px; }
.feature .ico svg { width: 22px; height: 22px; stroke: var(--accent); }
.feature h3 { font-size: 15px; margin-bottom: 6px; }
.feature p { font-size: 13px; color: var(--text-dim); line-height: 1.55; }
.features-link { display: block; margin-top: 16px; font-size: 13px; color: var(--accent); text-decoration: none; }
.features-link:hover { text-decoration: underline; }
`

// Nav links are server-driven: nothing but the language switch shows before
// login, and "comprar" only shows once the user has created events.
const headerHTML = `
<header>
  <a class="logo" href="/"><span class="bolt">&#9889;</span>Relampo<span class="tag">Tickets</span></a>
  <nav>
    {{if .LoggedIn}}{{if .HasEvents}}<a href="/events" data-i18n="navBuy">comprar</a>{{end}}<a href="/manage" data-i18n="navManage">mis eventos</a>
    <span class="userchip">{{.NavUser}}</span>
    <a href="/logout" data-i18n="navLogout">salir</a>{{end}}
    <span class="lang"><a href="#" data-setlang="es">ES</a><a href="#" data-setlang="en">EN</a></span>
  </nav>
</header>`

const footerHTML = `
<footer data-i18n="footer">RelampoTickets &mdash; app de pr&aacute;ctica para correlaci&oacute;n en pruebas de rendimiento</footer>`

var homeTmpl = mustPage("home", `
<body data-page="home">`+headerHTML+`
<main>
  <h1><span data-i18n="homeTitle">Entradas para tus eventos,</span> <span class="hl" data-i18n="homeTitleHl">a la velocidad del rayo</span></h1>
<p class="sub" data-i18n="homeSub">Inicia sesi&oacute;n para comprar entradas.</p>
  <div class="panel">
    <h2 data-i18n="loginTitle">Iniciar sesi&oacute;n</h2>
    <form id="loginForm">
      <input type="hidden" name="csrf_token" value="{{.CSRF}}">
      <label for="username" data-i18n="userLabel">Usuario</label>
      <input type="text" id="username" name="username" autocomplete="off" placeholder="usuario" data-i18n-ph="userPh">
      <label for="password" data-i18n="passLabel">Contrase&ntilde;a</label>
      <input type="password" id="password" name="password" placeholder="contrase&ntilde;a" data-i18n-ph="passPh">
      <button type="submit" data-i18n="loginBtn">Entrar</button>
      <div id="status" class="status"></div>
    </form>
  </div>

  <div class="features-head" data-i18n="featHead">&iquest;Por qu&eacute; Relampo?</div>
  <div class="features">
    <div class="feature">
      <span class="ico"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg></span>
      <h3 data-i18n="feat1t">Correlaci&oacute;n autom&aacute;tica completa</h3>
<p data-i18n="feat1d">Relampo detecta y correlaciona los valores din&aacute;micos por ti al grabar: menos scripting manual, pruebas listas antes.</p>
    </div>
    <div class="feature">
      <span class="ico"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="16" y="16" width="6" height="6" rx="1"/><rect x="2" y="16" width="6" height="6" rx="1"/><rect x="9" y="2" width="6" height="6" rx="1"/><path d="M5 16v-3a1 1 0 0 1 1-1h12a1 1 0 0 1 1 1v3"/><path d="M12 12V8"/></svg></span>
      <h3 data-i18n="feat2t">500 usuarios virtuales, 4 nodos</h3>
<p data-i18n="feat2d">Genera hasta 500 usuarios virtuales distribuidos en 4 nodos de carga con la integraci&oacute;n nativa de Relampo con GitHub.</p>
    </div>
    <div class="feature">
      <span class="ico"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/><path d="m10 12-2 2 2 2"/><path d="m14 16 2-2-2-2"/></svg></span>
      <h3 data-i18n="feat3t">Scripts YAML declarativos</h3>
<p data-i18n="feat3d">F&aacute;ciles de leer, intuitivos y versionables en Git: revisas tus pruebas de rendimiento como revisas c&oacute;digo.</p>
    </div>
    <div class="feature">
      <span class="ico"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m12 14 4-4"/><path d="M3.34 19a10 10 0 1 1 17.32 0"/></svg></span>
      <h3 data-i18n="feat4t">Motor de carga en Go</h3>
      <p data-i18n="feat4d">Generaci&oacute;n de carga de alto rendimiento con la eficiencia y la concurrencia nativa de Go.</p>
    </div>
  </div>
<a class="features-link" href="https://relampo.com" target="_blank" rel="noopener" data-i18n="featLink">Conoce m&aacute;s en relampo.com &rarr;</a>
</main>`+footerHTML+`
<script src="/static/app.js"></script>
</body>`)

var eventsTmpl = mustPage("events", `
<body data-page="events">`+headerHTML+`
<main>
<h1><span data-i18n="eventsTitle">Elige tu</span> <span class="hl" data-i18n="eventsTitleHl">evento</span></h1>
  <p class="sub"><span data-i18n="eventsHello">Hola</span> <strong>{{.User}}</strong>. <span data-i18n="eventsSub">La app elige un evento y un asiento al azar, reserva, y te deja listo el pago.</span></p>
  <div id="app" data-config="{{.ConfigJSON}}"></div>
  <div class="panel">
    <h2 data-i18n="selTitle">Tu selecci&oacute;n</h2>
    <dl class="kv">
      <dt data-i18n="kvEvent">Evento</dt><dd id="evName">&mdash;</dd>
      <dt data-i18n="kvVenue">Lugar</dt><dd id="evVenue">&mdash;</dd>
      <dt data-i18n="kvDate">Fecha</dt><dd id="evDate">&mdash;</dd>
      <dt data-i18n="kvSeat">Asiento</dt><dd id="seatId">&mdash;</dd>
      <dt data-i18n="kvPrice">Precio</dt><dd id="seatPrice">&mdash;</dd>
    </dl>
    <div id="status" class="status">&hellip;</div>
  </div>
  <div class="panel" id="payBox" style="display:none">
    <h2 data-i18n="payTitle">Pago seguro con RelampoPay</h2>
    <form id="payForm" method="POST" action="/pay/start">
      <input type="hidden" id="resvId" name="reservation_id" value="">
      <input type="hidden" id="relampoToken" name="relampo_token" value="">
      <button type="submit" data-i18n="payBtn">Pagar ahora</button>
    </form>
  </div>
</main>`+footerHTML+`
<script src="/static/app.js"></script>
</body>`)

var manageTmpl = mustPage("manage", `
<body data-page="manage">`+headerHTML+`
<main>
  <h1><span data-i18n="manageTitle">Mis</span> <span class="hl" data-i18n="manageTitleHl">eventos</span></h1>
<p class="sub" data-i18n="manageSub">Crea tus propios eventos: aparecen en el cat&aacute;logo y se pueden comprar.</p>
  <div class="cols">
  <div class="panel">
    <h2 data-i18n="createTitle">Crear evento</h2>
{{if .AtLimit}}<p class="note" data-i18n="limitNote">Has alcanzado el l&iacute;mite de 5 eventos. Borra alguno para poder crear otro.</p>{{end}}
    <form id="createForm"{{if .AtLimit}} style="display:none"{{end}}>
      <input type="hidden" name="publish_token" value="{{.PublishToken}}">
      <label for="evname" data-i18n="nameLabel">Nombre</label>
      <input type="text" id="evname" autocomplete="off" placeholder="Festival de Invierno" data-i18n-ph="namePh">
      <label for="evvenue" data-i18n="venueLabel">Lugar</label>
      <input type="text" id="evvenue" autocomplete="off" placeholder="Teatro Principal" data-i18n-ph="venuePh">
      <label for="evdate" data-i18n="dateLabel">Fecha</label>
      <input type="text" id="evdate" autocomplete="off" placeholder="2026-12-01">
      <button type="submit" data-i18n="publishBtn">Publicar evento</button>
      <div id="status" class="status"></div>
    </form>
  </div>
  <div class="panel">
    <h2 data-i18n="publishedTitle">Eventos publicados</h2>
    <div id="myEvents" class="note">&hellip;</div>
  </div>
  </div>
</main>`+footerHTML+`
<script src="/static/app.js"></script>
</body>`)

var editTmpl = mustPage("edit", `
<body data-page="edit">`+headerHTML+`
<main>
  <h1><span data-i18n="editTitle">Editar</span> <span class="hl" data-i18n="editTitleHl">evento</span></h1>
  <div class="panel">
    <h2>{{.Name}}</h2>
    <form id="editForm">
      <input type="hidden" id="eventId" value="{{.ID}}">
      <input type="hidden" id="eventRev" name="rev" value="{{.Rev}}">
      <label for="evname" data-i18n="nameLabel">Nombre</label>
      <input type="text" id="evname" value="{{.Name}}">
      <label for="evvenue" data-i18n="venueLabel">Lugar</label>
      <input type="text" id="evvenue" value="{{.Venue}}">
      <label for="evdate" data-i18n="dateLabel">Fecha</label>
      <input type="text" id="evdate" value="{{.Date}}">
      <button type="submit" data-i18n="saveBtn">Guardar cambios</button>
      <a class="btn" style="background:transparent;color:var(--text-dim);border:1px solid var(--border)" href="/manage" data-i18n="cancelBtn">Cancelar</a>
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
    <p class="note" style="margin-top:10px">Redirecting to the payment processor&hellip;</p>
    <form id="payform" method="POST" action="/pay/continue">
      <input type="hidden" name="request" value="{{.Request}}">
      <input type="hidden" name="x_correlation_id" value="{{.XCorr}}">
      <noscript><button type="submit">Continuar / Continue</button></noscript>
    </form>
  </div>
</main>
<script>document.getElementById('payform').submit();</script>
</body>`)

var callbackTmpl = mustPage("callback", `
<body data-page="callback">`+headerHTML+`
<main>
  <div class="badge" data-i18n="cbBadge">RelampoPay &mdash; confirmaci&oacute;n</div>
<h1><span data-i18n="cbTitle">Revisa tu</span> <span class="hl" data-i18n="cbTitleHl">compra</span></h1>
  <div class="panel">
    <h2 data-i18n="summary">Resumen</h2>
    <dl class="kv">
      <dt data-i18n="kvEvent">Evento</dt><dd>{{.EventName}}</dd>
      <dt data-i18n="kvSeat">Asiento</dt><dd>{{.SeatID}}</dd>
      <dt data-i18n="kvPrice">Precio</dt><dd>$ {{.Price}}</dd>
    </dl>
    <form id="confirmForm" method="POST" action="/pay/confirm">
      <input type="hidden" name="view_state" value="{{.ViewState}}">
      <input type="hidden" name="code" value="{{.Code}}">
      <input type="hidden" name="reservation_id" value="{{.ReservationID}}">
      <input type="hidden" id="csrfField" name="csrf_token" value="">
      <button type="submit" data-i18n="confirmBtn">Confirmar compra</button>
    </form>
  </div>
</main>`+footerHTML+`
<script src="/static/app.js"></script>
</body>`)

var successTmpl = mustPage("success", `
<body data-page="success" data-ticket="{{.TicketID}}">`+headerHTML+`
<main>
  <div class="badge" data-i18n="successBadge">compra confirmada</div>
  <div class="panel" style="text-align:center">
    <div class="receipt">&#9889; <span data-i18n="successBig">Compra confirmada</span></div>
    <p class="sub"><span data-i18n="successSub">Tu entrada qued&oacute; emitida,</span> {{.User}}.</p>
    <dl class="kv" style="text-align:left">
      <dt data-i18n="kvEvent">Evento</dt><dd>{{.EventName}}</dd>
      <dt data-i18n="kvSeat">Asiento</dt><dd>{{.SeatID}}</dd>
    </dl>
    <p id="ticketInfo" class="status">&hellip;</p>
    <a class="btn" href="/events" data-i18n="buyAnother">Comprar otra entrada</a>
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
<link rel="icon" type="image/svg+xml" href="/favicon.svg">
<title>RelampoTickets</title>
<style>%CSS%</style>
</head>
%BODY%
</html>`
	page := strings.ReplaceAll(skeleton, "%CSS%", baseCSS)
	page = strings.ReplaceAll(page, "%BODY%", body)
	return template.Must(template.New(name).Parse(page))
}
