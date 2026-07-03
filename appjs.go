package main

// appJS is the single client-side script, served from /static/app.js with an
// ETag (the browser re-requests it with If-None-Match — that's the 304
// correlation case). The SHA-256/HMAC below is pure JS on purpose: Web Crypto
// needs a secure context, and this app must also work over plain HTTP behind
// the Relampo MITM recorder. Verified byte-identical against node:crypto and
// Go's crypto/hmac.
const appJS = `(function () {
  'use strict';

  // La sal es publica a proposito: el reto de la practica es descubrir que el
  // token viaja transformado y reimplementar esta funcion en tu herramienta
  // de carga (JSR223 en JMeter, modulo crypto en k6, C en LoadRunner).
  var RELAMPO_SALT = 'relampo-public-salt-v1';

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

  // valor B = HMAC-SHA256(valor A, sal publica), en hexadecimal.
  function signRelampoToken(tokenA) {
    return hmacSha256Hex(RELAMPO_SALT, tokenA);
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
    setStatus('Cargando eventos…');
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
        setStatus('Asiento reservado. Tenés 1 minuto para completar el pago.');
        el('resvId').value = resv.reservationId;
        el('relampoToken').value = signRelampoToken(resv.relampoToken);
        el('payBox').style.display = 'block';
      })
      .catch(function (e) { setStatus(String(e.message || e), true); });
  }

  function initCallback() {
    el('csrfField').value = sessionStorage.getItem('relampo_csrf') || '';
  }

  function initSuccess() {
    var ticketId = document.body.getAttribute('data-ticket');
    fetch('/api/tickets/' + ticketId, { headers: authHeaders() })
      .then(jsonOrThrow)
      .then(function () { el('ticketInfo').textContent = 'Entrada verificada. Te la enviamos por correo.'; })
      .catch(function (e) { el('ticketInfo').textContent = 'No pudimos verificar la entrada: ' + String(e.message || e); });
  }

  var page = document.body.getAttribute('data-page');
  if (page === 'home') initHome();
  else if (page === 'events') initEvents();
  else if (page === 'callback') initCallback();
  else if (page === 'success') initSuccess();
})();
`
