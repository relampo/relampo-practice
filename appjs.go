package main

import "strings"

// buildAppJS renders the single client-side script, served from
// /static/app.js with an ETag (the browser re-requests it with If-None-Match
// — that's the 304 correlation case). The crypto below is pure JS on purpose:
// Web Crypto needs a secure context, and this app must also work over plain
// HTTP behind the Relampo MITM recorder. Verified byte-identical against
// node:crypto and Go's crypto/hmac.
func buildAppJS(mode string) string {
	return strings.ReplaceAll(appJSTemplate, "__TOKEN_MODE__", mode)
}

const appJSTemplate = `(function () {
  'use strict';

  // La sal y el modo son publicos a proposito: el reto de la practica es
  // descubrir que el token viaja transformado y reimplementar la funcion
  // signRelampoToken() en el preprocesador de tu herramienta de carga.
  var RELAMPO_SALT = 'relampo-public-salt-v1';
  var TOKEN_MODE = '__TOKEN_MODE__';

  function sha256Bytes(msgBytes) {
    var K = [
      0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
      0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
      0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
      0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
      0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
      0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
      0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
      0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2
    ];
    var H = [0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19];
    var len = msgBytes.length;
    var bitLen = len * 8;
    var padded = msgBytes.slice();
    padded.push(0x80);
    while (padded.length % 64 !== 56) padded.push(0);
    var hi = Math.floor(bitLen / 0x100000000);
    var lo = bitLen >>> 0;
    padded.push((hi >>> 24) & 0xff, (hi >>> 16) & 0xff, (hi >>> 8) & 0xff, hi & 0xff);
    padded.push((lo >>> 24) & 0xff, (lo >>> 16) & 0xff, (lo >>> 8) & 0xff, lo & 0xff);
    var w = new Array(64);
    for (var i = 0; i < padded.length; i += 64) {
      for (var t = 0; t < 16; t++) {
        w[t] = (padded[i + t * 4] << 24) | (padded[i + t * 4 + 1] << 16) | (padded[i + t * 4 + 2] << 8) | (padded[i + t * 4 + 3]);
      }
      for (t = 16; t < 64; t++) {
        var s0 = ((w[t - 15] >>> 7) | (w[t - 15] << 25)) ^ ((w[t - 15] >>> 18) | (w[t - 15] << 14)) ^ (w[t - 15] >>> 3);
        var s1 = ((w[t - 2] >>> 17) | (w[t - 2] << 15)) ^ ((w[t - 2] >>> 19) | (w[t - 2] << 13)) ^ (w[t - 2] >>> 10);
        w[t] = (w[t - 16] + s0 + w[t - 7] + s1) | 0;
      }
      var a = H[0], b = H[1], c = H[2], d = H[3], e = H[4], f = H[5], g = H[6], h = H[7];
      for (t = 0; t < 64; t++) {
        var S1 = ((e >>> 6) | (e << 26)) ^ ((e >>> 11) | (e << 21)) ^ ((e >>> 25) | (e << 7));
        var ch = (e & f) ^ (~e & g);
        var temp1 = (h + S1 + ch + K[t] + w[t]) | 0;
        var S0 = ((a >>> 2) | (a << 30)) ^ ((a >>> 13) | (a << 19)) ^ ((a >>> 22) | (a << 10));
        var maj = (a & b) ^ (a & c) ^ (b & c);
        var temp2 = (S0 + maj) | 0;
        h = g; g = f; f = e; e = (d + temp1) | 0;
        d = c; c = b; b = a; a = (temp1 + temp2) | 0;
      }
      H[0] = (H[0] + a) | 0; H[1] = (H[1] + b) | 0; H[2] = (H[2] + c) | 0; H[3] = (H[3] + d) | 0;
      H[4] = (H[4] + e) | 0; H[5] = (H[5] + f) | 0; H[6] = (H[6] + g) | 0; H[7] = (H[7] + h) | 0;
    }
    var out = [];
    for (i = 0; i < 8; i++) {
      out.push((H[i] >>> 24) & 0xff, (H[i] >>> 16) & 0xff, (H[i] >>> 8) & 0xff, H[i] & 0xff);
    }
    return out;
  }

  function strToBytes(s) {
    var bytes = [];
    for (var i = 0; i < s.length; i++) {
      var c = s.charCodeAt(i);
      if (c < 0x80) bytes.push(c);
      else if (c < 0x800) bytes.push(0xc0 | (c >> 6), 0x80 | (c & 0x3f));
      else if (c < 0xd800 || c >= 0xe000) bytes.push(0xe0 | (c >> 12), 0x80 | ((c >> 6) & 0x3f), 0x80 | (c & 0x3f));
      else {
        var c2 = s.charCodeAt(++i);
        var cp = 0x10000 + (((c & 0x3ff) << 10) | (c2 & 0x3ff));
        bytes.push(0xf0 | (cp >> 18), 0x80 | ((cp >> 12) & 0x3f), 0x80 | ((cp >> 6) & 0x3f), 0x80 | (cp & 0x3f));
      }
    }
    return bytes;
  }

  function bytesToHex(bytes) {
    var hex = '';
    for (var i = 0; i < bytes.length; i++) hex += ((bytes[i] >>> 4).toString(16)) + ((bytes[i] & 0xf).toString(16));
    return hex;
  }

  function hmacSha256Hex(keyStr, msgStr) {
    var key = strToBytes(keyStr);
    if (key.length > 64) key = sha256Bytes(key);
    var ipad = [], opad = [];
    for (var i = 0; i < 64; i++) {
      var k = i < key.length ? key[i] : 0;
      ipad.push(k ^ 0x36);
      opad.push(k ^ 0x5c);
    }
    var inner = sha256Bytes(ipad.concat(strToBytes(msgStr)));
    return bytesToHex(sha256Bytes(opad.concat(inner)));
  }

  // Cifrado simple (modo por defecto): cada caracter del token se XORea con
  // la sal y se emite en hexadecimal. Reimplementable en ~5 lineas de JS puro.
  function xorSignToken(tokenA) {
    var out = '';
    for (var i = 0; i < tokenA.length; i++) {
      var x = tokenA.charCodeAt(i) ^ RELAMPO_SALT.charCodeAt(i % RELAMPO_SALT.length);
      out += (x < 16 ? '0' : '') + x.toString(16);
    }
    return out;
  }

  // valor B = transformacion del valor A segun el modo del servidor.
  function signRelampoToken(tokenA) {
    return TOKEN_MODE === 'hmac' ? hmacSha256Hex(RELAMPO_SALT, tokenA) : xorSignToken(tokenA);
  }

  // ------------------------------------------------------------ idioma
  // Switch visual ES/EN, 100% del lado del cliente: la preferencia vive en
  // sessionStorage y NO viaja en ningun request (no afecta la correlacion).
  var I18N = {
    es: {
      navBuy: 'comprar', navManage: 'mis eventos', navLogout: 'salir',
      footer: 'RelampoTickets — app de práctica para correlación en pruebas de performance',
      homeTitle: 'Entradas para tus eventos,', homeTitleHl: 'a la velocidad del rayo',
      homeSub: 'Iniciá sesión para comprar entradas.',
      loginTitle: 'Iniciar sesión', userLabel: 'Usuario', passLabel: 'Contraseña',
      userPh: 'usuario', passPh: 'contraseña', loginBtn: 'Entrar',
      eventsTitle: 'Elegí tu', eventsTitleHl: 'evento', eventsHello: 'Hola',
      eventsSub: 'La app elige un evento y un asiento al azar, reserva, y te deja listo el pago.',
      selTitle: 'Tu selección', kvEvent: 'Evento', kvVenue: 'Lugar', kvDate: 'Fecha',
      kvSeat: 'Asiento', kvPrice: 'Precio',
      payTitle: 'Pago seguro con RelampoPay', payBtn: 'Pagar ahora',
      manageTitle: 'Mis', manageTitleHl: 'eventos',
      manageSub: 'Creá tus propios eventos: aparecen en el catálogo y se pueden comprar.',
      createTitle: 'Crear evento', nameLabel: 'Nombre', venueLabel: 'Lugar', dateLabel: 'Fecha',
      namePh: 'Festival de Invierno', venuePh: 'Teatro Principal',
      publishBtn: 'Publicar evento', publishedTitle: 'Eventos publicados',
      editTitle: 'Editar', editTitleHl: 'evento', saveBtn: 'Guardar cambios', cancelBtn: 'Cancelar',
      cbBadge: 'RelampoPay — confirmación', cbTitle: 'Revisá tu', cbTitleHl: 'compra',
      summary: 'Resumen', confirmBtn: 'Confirmar compra',
      successBadge: 'compra confirmada', successBig: 'Compra confirmada',
      successSub: 'Tu entrada quedó emitida,', buyAnother: 'Comprar otra entrada',
      jsLoadingEvents: 'Cargando eventos…',
      jsReserved: 'Asiento reservado. Tenés 1 minuto para completar el pago.',
      jsVerifying: 'Verificando tu entrada…',
      jsVerified: 'Entrada verificada. Te la enviamos por correo.',
      jsVerifyFail: 'No pudimos verificar la entrada: ',
      jsNoEvents: 'Todavía no publicaste eventos.',
      jsEdit: 'editar', jsDelete: 'borrar', jsLoading: 'Cargando…'
    },
    en: {
      navBuy: 'buy tickets', navManage: 'my events', navLogout: 'logout',
      footer: 'RelampoTickets — practice app for correlation in performance testing',
      homeTitle: 'Tickets for your events,', homeTitleHl: 'at lightning speed',
      homeSub: 'Sign in to buy tickets.',
      loginTitle: 'Sign in', userLabel: 'Username', passLabel: 'Password',
      userPh: 'username', passPh: 'password', loginBtn: 'Sign in',
      eventsTitle: 'Choose your', eventsTitleHl: 'event', eventsHello: 'Hi',
      eventsSub: 'The app picks a random event and seat, reserves it, and gets your payment ready.',
      selTitle: 'Your selection', kvEvent: 'Event', kvVenue: 'Venue', kvDate: 'Date',
      kvSeat: 'Seat', kvPrice: 'Price',
      payTitle: 'Secure payment with RelampoPay', payBtn: 'Pay now',
      manageTitle: 'My', manageTitleHl: 'events',
      manageSub: 'Create your own events: they join the catalog and can be purchased.',
      createTitle: 'Create event', nameLabel: 'Name', venueLabel: 'Venue', dateLabel: 'Date',
      namePh: 'Winter Festival', venuePh: 'Main Theater',
      publishBtn: 'Publish event', publishedTitle: 'Published events',
      editTitle: 'Edit', editTitleHl: 'event', saveBtn: 'Save changes', cancelBtn: 'Cancel',
      cbBadge: 'RelampoPay — confirmation', cbTitle: 'Review your', cbTitleHl: 'purchase',
      summary: 'Summary', confirmBtn: 'Confirm purchase',
      successBadge: 'purchase confirmed', successBig: 'Purchase confirmed',
      successSub: 'Your ticket has been issued,', buyAnother: 'Buy another ticket',
      jsLoadingEvents: 'Loading events…',
      jsReserved: 'Seat reserved. You have 1 minute to complete the payment.',
      jsVerifying: 'Verifying your ticket…',
      jsVerified: 'Ticket verified. We emailed it to you.',
      jsVerifyFail: 'We could not verify your ticket: ',
      jsNoEvents: 'You have not published any events yet.',
      jsEdit: 'edit', jsDelete: 'delete', jsLoading: 'Loading…'
    }
  };

  function curLang() {
    var l = sessionStorage.getItem('relampo_lang');
    return l === 'en' ? 'en' : 'es';
  }

  function t(key) {
    var d = I18N[curLang()];
    return (d && d[key]) || I18N.es[key] || key;
  }

  function applyLang() {
    var lang = curLang();
    document.documentElement.lang = lang;
    var nodes = document.querySelectorAll('[data-i18n]');
    for (var i = 0; i < nodes.length; i++) {
      var key = nodes[i].getAttribute('data-i18n');
      if (I18N[lang][key]) nodes[i].textContent = I18N[lang][key];
    }
    var phs = document.querySelectorAll('[data-i18n-ph]');
    for (i = 0; i < phs.length; i++) {
      var pk = phs[i].getAttribute('data-i18n-ph');
      if (I18N[lang][pk]) phs[i].setAttribute('placeholder', I18N[lang][pk]);
    }
    var sw = document.querySelectorAll('[data-setlang]');
    for (i = 0; i < sw.length; i++) {
      sw[i].className = sw[i].getAttribute('data-setlang') === lang ? 'active' : '';
    }
  }

  function initLangSwitch() {
    var sw = document.querySelectorAll('[data-setlang]');
    for (var i = 0; i < sw.length; i++) {
      sw[i].addEventListener('click', function (ev) {
        ev.preventDefault();
        sessionStorage.setItem('relampo_lang', this.getAttribute('data-setlang'));
        applyLang();
      });
    }
    applyLang();
  }

  function el(id) { return document.getElementById(id); }

  function setStatus(msg, isErr) {
    var s = el('status');
    if (s) { s.textContent = msg; s.className = isErr ? 'status err' : 'status'; }
  }

  function authHeaders() {
    return { 'Authorization': 'Bearer ' + (sessionStorage.getItem('relampo_bearer') || '') };
  }

  function jsonOrThrow(r) {
    return r.json().then(function (j) {
      if (!r.ok) { throw new Error(j.error || ('HTTP ' + r.status)); }
      return j;
    });
  }

  function initHome() {
    el('loginForm').addEventListener('submit', function (ev) {
      ev.preventDefault();
      var form = ev.target;
      // El csrf_token emitido en esta pagina se necesita recien al final del
      // flujo (POST /pay/confirm); se conserva en sessionStorage.
      sessionStorage.setItem('relampo_csrf', form.querySelector('input[name="csrf_token"]').value);
      fetch('/api/auth', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: el('username').value, password: el('password').value })
      })
        .then(jsonOrThrow)
        .then(function (res) {
          sessionStorage.setItem('relampo_bearer', res.bearer);
          window.location.href = '/events';
        })
        .catch(function (e) { setStatus(String(e.message || e), true); });
    });
  }

  function initEvents() {
    var cfg = JSON.parse(el('app').getAttribute('data-config'));
    var chosen = {};
    setStatus(t('jsLoadingEvents'));
    fetch('/api/events?catalogId=' + encodeURIComponent(cfg.catalogId), { headers: authHeaders() })
      .then(jsonOrThrow)
      .then(function (data) {
        var pick = data.events[Math.floor(Math.random() * data.events.length)];
        chosen.event = pick;
        el('evName').textContent = pick.name;
        el('evVenue').textContent = pick.venue;
        el('evDate').textContent = pick.date;
        return fetch('/api/events/' + pick.id + '/seats', { headers: authHeaders() });
      })
      .then(function (r) {
        chosen.corr = r.headers.get('X-Correlation-Id');
        return jsonOrThrow(r);
      })
      .then(function (data) {
        var seat = data.seats[Math.floor(Math.random() * data.seats.length)];
        chosen.seat = seat;
        el('seatId').textContent = seat.id;
        el('seatPrice').textContent = '$ ' + seat.price;
        var h = authHeaders();
        h['Content-Type'] = 'application/json';
        h['X-Correlation-Id'] = chosen.corr;
        return fetch('/api/reservations', {
          method: 'POST', headers: h,
          body: JSON.stringify({ eventId: chosen.event.id, seatId: seat.id })
        });
      })
      .then(jsonOrThrow)
      .then(function (resv) {
        setStatus(t('jsReserved'));
        el('resvId').value = resv.reservationId;
        el('relampoToken').value = signRelampoToken(resv.relampoToken);
        el('payBox').style.display = 'block';
      })
      .catch(function (e) { setStatus(String(e.message || e), true); });
  }

  function initManage() {
    loadMyEvents();
    el('createForm').addEventListener('submit', function (ev) {
      ev.preventDefault();
      var h = authHeaders();
      h['Content-Type'] = 'application/json';
      fetch('/api/manage/events', {
        method: 'POST', headers: h,
        body: JSON.stringify({
          name: el('evname').value,
          venue: el('evvenue').value,
          date: el('evdate').value,
          publishToken: ev.target.querySelector('input[name="publish_token"]').value
        })
      })
        .then(jsonOrThrow)
        .then(function () {
          // el publish_token es de un solo uso: recargar para obtener otro
          window.location.reload();
        })
        .catch(function (e) { setStatus(String(e.message || e), true); });
    });
  }

  function loadMyEvents() {
    fetch('/api/manage/events', { headers: authHeaders() })
      .then(jsonOrThrow)
      .then(function (data) {
        var box = el('myEvents');
        if (data.events.length === 0) { box.textContent = t('jsNoEvents'); return; }
        box.textContent = '';
        data.events.forEach(function (evt) {
          var row = document.createElement('div');
          row.style.cssText = 'display:flex;align-items:center;justify-content:space-between;gap:12px;padding:10px 0;border-bottom:1px solid var(--border)';
          var label = document.createElement('span');
          label.textContent = evt.name + ' — ' + evt.venue + ' (' + evt.date + ')';
          var actions = document.createElement('span');
          var edit = document.createElement('a');
          edit.href = '/manage/events/' + evt.id + '/edit';
          edit.textContent = t('jsEdit');
          edit.style.cssText = 'color:var(--accent);margin-right:14px';
          var del = document.createElement('a');
          del.href = '#';
          del.textContent = t('jsDelete');
          del.style.color = 'var(--err)';
          del.addEventListener('click', function (clickEv) {
            clickEv.preventDefault();
            var h = authHeaders();
            h['If-Match'] = evt.rev;
            fetch('/api/manage/events/' + evt.id, { method: 'DELETE', headers: h })
              .then(jsonOrThrow)
              .then(loadMyEvents)
              .catch(function (e) { setStatus(String(e.message || e), true); });
          });
          actions.appendChild(edit);
          actions.appendChild(del);
          row.appendChild(label);
          row.appendChild(actions);
          box.appendChild(row);
        });
      })
      .catch(function (e) { el('myEvents').textContent = String(e.message || e); });
  }

  function initEdit() {
    el('editForm').addEventListener('submit', function (ev) {
      ev.preventDefault();
      var h = authHeaders();
      h['Content-Type'] = 'application/json';
      h['If-Match'] = el('eventRev').value;
      fetch('/api/manage/events/' + el('eventId').value, {
        method: 'PUT', headers: h,
        body: JSON.stringify({
          name: el('evname').value,
          venue: el('evvenue').value,
          date: el('evdate').value
        })
      })
        .then(jsonOrThrow)
        .then(function () { window.location.href = '/manage'; })
        .catch(function (e) { setStatus(String(e.message || e), true); });
    });
  }

  function initCallback() {
    el('csrfField').value = sessionStorage.getItem('relampo_csrf') || '';
  }

  function initSuccess() {
    var ticketId = document.body.getAttribute('data-ticket');
    el('ticketInfo').textContent = t('jsVerifying');
    fetch('/api/tickets/' + ticketId, { headers: authHeaders() })
      .then(jsonOrThrow)
      .then(function () { el('ticketInfo').textContent = t('jsVerified'); })
      .catch(function (e) { el('ticketInfo').textContent = t('jsVerifyFail') + String(e.message || e); });
  }

  initLangSwitch();

  var page = document.body.getAttribute('data-page');
  if (page === 'home') initHome();
  else if (page === 'events') initEvents();
  else if (page === 'manage') initManage();
  else if (page === 'edit') initEdit();
  else if (page === 'callback') initCallback();
  else if (page === 'success') initSuccess();
})();
`
