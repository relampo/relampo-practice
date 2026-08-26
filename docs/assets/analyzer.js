(function () {
  var state = { load: null, cpuRaw: null, memoryRaw: null, cpu: null, memory: null, combined: [], zoom: null, bucketMs: 2000, bucketOrigin: null };
  var tableState = {
    valuesTable: { page: 0, size: 25 },
    errorsTable: { page: 0, size: 25 },
    slowTable: { page: 0, size: 25 }
  };
  var colors = { rps: '#38bdf8', avg: '#fb923c', p95: '#ffd60a', vus: '#e8ecf6', errors: '#f87171', cpu: '#4ade80', memory: '#a78bfa' };
  var labels = { rps: 'RPS', avg: 'Avg ms', p95: 'P95 ms', vus: 'VUs', errors: 'Errores', cpu: 'CPU %', memory: 'Memoria %' };

  function $(id) { return document.getElementById(id); }
  function bucketKey(ms) {
    return Math.floor(ms / state.bucketMs) * state.bucketMs;
  }
  function average(values) { return values.length ? values.reduce(function (a, b) { return a + b; }, 0) / values.length : 0; }
  function wholeNumber(value) {
    value = Number(value);
    return Number.isFinite(value) ? Math.max(0, Math.round(value)) : 0;
  }
  function minValue(values, mapper) {
    var min = Infinity;
    values.forEach(function (item) {
      var value = mapper ? mapper(item) : item;
      if (value != null && Number.isFinite(value) && value < min) min = value;
    });
    return min === Infinity ? 0 : min;
  }
  function maxValue(values, mapper) {
    var max = -Infinity;
    values.forEach(function (item) {
      var value = mapper ? mapper(item) : item;
      if (value != null && Number.isFinite(value) && value > max) max = value;
    });
    return max === -Infinity ? 0 : max;
  }
  function fmtTime(ms) { return new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }); }
  function fmtFull(ms) { return new Date(ms).toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }); }
  function fmtInterval(rowOrMs) {
    var start = typeof rowOrMs === 'number' ? rowOrMs : rowOrMs.t;
    var end = typeof rowOrMs === 'number' ? start + state.bucketMs : (rowOrMs.end || start + state.bucketMs);
    return fmtFull(start) + ' - ' + fmtFull(end);
  }
  function fmtNum(n, digits) {
    if (n == null || Number.isNaN(n)) return '-';
    return Number(n).toLocaleString(undefined, { maximumFractionDigits: digits == null ? 1 : digits });
  }
  function fmtMetric(key, value) {
    if (value == null || Number.isNaN(value)) return '-';
    if (key === 'avg' || key === 'p95') return fmtNum(value, 0) + ' ms';
    if (key === 'cpu' || key === 'memory') return fmtNum(value, 1) + '%';
    if (key === 'rps') return fmtNum(value, 1);
    return fmtNum(value, 0);
  }
  function percentile(values, p) {
    if (!values.length) return 0;
    var sorted = values.slice().sort(function (a, b) { return a - b; });
    return sorted[Math.max(0, Math.ceil(p * sorted.length) - 1)];
  }
  function parseCSVLine(line) {
    var out = [], cur = '', quoted = false;
    for (var i = 0; i < line.length; i++) {
      var c = line[i];
      if (c === '"') {
        if (quoted && line[i + 1] === '"') { cur += '"'; i++; } else quoted = !quoted;
      } else if (c === ',' && !quoted) { out.push(cur); cur = ''; } else cur += c;
    }
    out.push(cur);
    return out;
  }
  function esc(s) { return String(s).replace(/[&<>"']/g, function (c) { return ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]; }); }

  function parseLoadFile(text, name) {
    var trimmed = text.trim();
    if (!trimmed) throw new Error('Archivo vacio');
    if (/\.jsonl$/i.test(name)) return parseK6Jsonl(trimmed);
    if (/\.json$/i.test(name)) {
      try { return parseGenericJson(JSON.parse(trimmed)); } catch (_) {}
    }
    return parseJtlCsv(trimmed);
  }

  function parseJtlCsv(text) {
    var lines = text.split(/\r?\n/).filter(Boolean);
    var header = parseCSVLine(lines[0]);
    var idx = {};
    header.forEach(function (h, i) { idx[h] = i; });
    if (idx.timeStamp == null || idx.elapsed == null) throw new Error('No encontre columnas timeStamp y elapsed');
    var rows = [];
    lines.slice(1).forEach(function (line) {
      var c = parseCSVLine(line);
      var ts = Number(c[idx.timeStamp]);
      if (ts < 10000000000) ts *= 1000;
      var elapsed = Number(c[idx.elapsed]) || 0;
      var code = String(c[idx.responseCode] || '');
      var success = String(c[idx.success] || '');
      rows.push({
        ts: ts,
        elapsed: elapsed,
        label: c[idx.label] || 'request',
        code: code,
        ok: success ? success === 'true' && /^[23]/.test(code) : /^[23]/.test(code),
        threads: wholeNumber(c[idx.allThreads] || c[idx.grpThreads]),
        message: c[idx.responseMessage] || c[idx.failureMessage] || '',
        url: c[idx.URL] || ''
      });
    });
    return summarizeRows(rows);
  }

  function parseK6Jsonl(text) {
    var rows = [];
    text.split(/\r?\n/).forEach(function (line) {
      try {
        var item = JSON.parse(line);
        if (item.type !== 'Point' || item.metric !== 'http_req_duration') return;
        var status = item.data.tags && item.data.tags.status || '';
        rows.push({
          ts: new Date(item.data.time).getTime(),
          elapsed: Number(item.data.value) || 0,
          label: item.data.tags && (item.data.tags.name || item.data.tags.url) || 'request',
          code: status,
          ok: !status || /^[23]/.test(String(status)),
          threads: wholeNumber(item.data.tags && item.data.tags.vu),
          message: item.data.tags && (item.data.tags.error || item.data.tags.error_code) || '',
          url: item.data.tags && item.data.tags.url || ''
        });
      } catch (_) {}
    });
    if (!rows.length) throw new Error('No pude detectar puntos k6 http_req_duration');
    return summarizeRows(rows);
  }

  function parseGenericJson(json) {
    if (json.metricData) throw new Error('Ese JSON parece metrica de servidor, cargalo en CPU o Memoria');
    var arr = Array.isArray(json) ? json : (json.requests || json.samples || []);
    if (!arr.length) throw new Error('JSON de prueba no reconocido');
    return summarizeRows(arr.map(normalizeJsonRow).filter(Boolean));
  }

  function normalizeJsonRow(r) {
    var ts = r.timeStamp || r.timestamp || r.time || r.ts;
    var elapsed = r.elapsed || r.duration || r.responseTime || r.value;
    if (ts == null || elapsed == null) return null;
    ts = Number.isFinite(Number(ts)) ? Number(ts) : new Date(ts).getTime();
    if (ts < 10000000000) ts *= 1000;
    var code = String(r.responseCode || r.status || '');
    return {
      ts: ts,
      elapsed: Number(elapsed) || 0,
      label: r.label || r.name || r.url || 'request',
      code: code,
      ok: r.success === false ? false : !code || /^[23]/.test(code),
      threads: wholeNumber(r.allThreads || r.vus),
      message: r.responseMessage || r.failureMessage || r.error || r.message || '',
      url: r.url || ''
    };
  }

  function summarizeRows(rows) {
    if (!rows.length) throw new Error('No encontre samples de prueba');
    rows.sort(function (a, b) { return a.ts - b.ts; });
    if (!state.bucketOrigin) state.bucketOrigin = rows[0].ts;
    var buckets = new Map(), endpoints = new Map(), errorEndpoints = new Map(), end = 0, maxVus = 0, maxVusAt = rows[0].ts, errors = [];
    rows.forEach(function (r) {
      end = Math.max(end, r.ts + r.elapsed);
      var key = bucketKey(r.ts);
      var b = buckets.get(key) || { t: key, count: 0, errors: 0, elapsed: [], vusLast: null, vusMax: 0 };
      b.count++;
      if (!r.ok) b.errors++;
      if (!r.ok) errors.push({
        ts: r.ts,
        label: r.label,
        code: r.code || '-',
        elapsed: r.elapsed,
        threads: r.threads || 0,
        message: r.message || '',
        url: r.url || ''
      });
      if (!r.ok) {
        var epKey = r.label + '|' + (r.code || '-') + '|' + (r.message || r.url || '');
        var ep = errorEndpoints.get(epKey) || { label: r.label, code: r.code || '-', message: r.message || '', url: r.url || '', count: 0, elapsed: [], first: r.ts, last: r.ts };
        ep.count++;
        ep.elapsed.push(r.elapsed);
        ep.first = Math.min(ep.first, r.ts);
        ep.last = Math.max(ep.last, r.ts);
        errorEndpoints.set(epKey, ep);
      }
      b.elapsed.push(r.elapsed);
      b.vusMax = Math.max(b.vusMax, wholeNumber(r.threads));
      b.vusLast = wholeNumber(r.threads);
      buckets.set(key, b);
      if (b.vusMax > maxVus) { maxVus = b.vusMax; maxVusAt = key; }
      var e = endpoints.get(r.label) || { label: r.label, count: 0, errors: 0, elapsed: [] };
      e.count++;
      if (!r.ok) e.errors++;
      e.elapsed.push(r.elapsed);
      endpoints.set(r.label, e);
    });
    var elapsedAll = rows.map(function (r) { return r.elapsed; });
    var durationSec = Math.max(1, (end - rows[0].ts) / 1000);
    var minutes = Array.from(buckets.values()).sort(function (a, b) { return a.t - b.t; }).map(function (b) {
      var bucketStart = Math.max(b.t, rows[0].ts);
      var bucketEnd = Math.min(b.t + state.bucketMs, end);
      var seconds = Math.max(0.001, (bucketEnd - bucketStart) / 1000);
      return { t: bucketStart, end: bucketEnd, rps: b.count / seconds, errors: b.errors, avg: average(b.elapsed), p95: percentile(b.elapsed, 0.95), vus: wholeNumber(b.vusLast), vusMax: wholeNumber(b.vusMax) };
    });
    var endpointStats = Array.from(endpoints.values()).map(function (e) {
      return { label: e.label, count: e.count, errors: e.errors, min: minValue(e.elapsed), max: maxValue(e.elapsed), avg: average(e.elapsed), p95: percentile(e.elapsed, 0.95) };
    }).sort(function (a, b) { return b.p95 - a.p95; });
    var errorStats = Array.from(errorEndpoints.values()).map(function (e) {
      return { label: e.label, code: e.code, count: e.count, min: minValue(e.elapsed), max: maxValue(e.elapsed), avg: average(e.elapsed), p95: percentile(e.elapsed, 0.95), first: e.first, last: e.last, message: e.message, url: e.url };
    }).sort(function (a, b) { return b.p95 - a.p95; });
    return { rows: rows, minutes: minutes, endpoints: endpointStats, errors: errors, errorStats: errorStats, summary: { samples: rows.length, start: rows[0].ts, end: end, durationSec: durationSec, rps: rows.length / durationSec, errors: errors.length, avg: average(elapsedAll), p95: percentile(elapsedAll, 0.95), maxVus: maxVus, maxVusAt: maxVusAt } };
  }

  function parseMetricJson(text) {
    var json = JSON.parse(text);
    var data = json.metricData || json.data || [];
    return data.map(function (p) {
      var ts = p.timestamp;
      ts = Number.isFinite(Number(ts)) ? Number(ts) : new Date(ts).getTime();
      if (ts < 10000000000) ts *= 1000;
      return { t: ts, value: Number(p.average != null ? p.average : p.value) || 0 };
    }).sort(function (a, b) { return a.t - b.t; });
  }

  function bucketMetrics(rows) {
    if (!rows) return null;
    if (state.load) return bucketServerMetrics(rows);
    var buckets = new Map();
    rows.forEach(function (m) {
      var key = bucketKey(m.t);
      var b = buckets.get(key) || { t: key, values: [] };
      b.values.push(m.value);
      buckets.set(key, b);
    });
    return Array.from(buckets.values()).map(function (b) {
      return { t: b.t, value: average(b.values) };
    }).sort(function (a, b) { return a.t - b.t; });
  }

  function bucketServerMetrics(rows) {
    var start = state.load.summary.start;
    var end = state.load.summary.end;
    var buckets = new Map();
    rows.forEach(function (m) {
      if (m.t < start || m.t > end) return;
      var t = state.bucketMs >= 60000 ? Math.floor(m.t / state.bucketMs) * state.bucketMs : m.t;
      var b = buckets.get(t) || { t: t, values: [], lastSeen: m.t };
      b.values.push(m.value);
      b.lastSeen = Math.max(b.lastSeen, m.t);
      buckets.set(t, b);
    });
    return Array.from(buckets.values()).map(function (b) {
      return { t: b.lastSeen, value: average(b.values) };
    }).sort(function (a, b) { return a.t - b.t; });
  }

  function combine() {
    var map = new Map();
    if (!state.bucketOrigin) {
      var metricStarts = []
        .concat(state.cpuRaw || [])
        .concat(state.memoryRaw || [])
        .map(function (m) { return m.t; });
      if (metricStarts.length) state.bucketOrigin = minValue(metricStarts);
    }
    state.cpu = bucketMetrics(state.cpuRaw);
    state.memory = bucketMetrics(state.memoryRaw);
    function row(t) {
      var r = map.get(t);
      if (!r) { r = { t: t, end: null, rps: null, avg: null, p95: null, vus: null, vusMax: null, errors: null, cpu: null, memory: null }; map.set(t, r); }
      return r;
    }
    if (state.load) state.load.minutes.forEach(function (m) {
      var r = row(m.t);
      r.end = m.end;
      r.rps = m.rps; r.avg = m.avg; r.p95 = m.p95; r.errors = m.errors; r.vus = wholeNumber(m.vus); r.vusMax = wholeNumber(m.vusMax);
    });
    if (state.cpu) state.cpu.forEach(function (m) { row(m.t).cpu = m.value; });
    if (state.memory) state.memory.forEach(function (m) { row(m.t).memory = m.value; });
    state.combined = Array.from(map.values()).sort(function (a, b) { return a.t - b.t; });
    if (state.load && state.load.minutes.length) {
      var start = state.load.summary.start;
      var end = state.load.summary.end;
      state.combined = state.combined.filter(function (r) { return r.t >= start && r.t <= end; });
    }
  }

  function visibleRows() {
    var bounds = visibleBounds();
    if (!bounds) return state.combined;
    return state.combined.filter(function (r) { return r.t >= bounds.start && r.t <= bounds.end; });
  }

  function visibleBounds() {
    if (state.zoom) return state.zoom;
    return null;
  }

  function render() {
    combine();
    seedZoomInputs();
    renderKpis();
    renderTables();
    drawAllCharts();
  }

  function seedZoomInputs() {}
  function maxSeries(key) {
    var vals = visibleRows().map(function (r) { return r[key]; }).filter(function (v) { return v != null; });
    return vals.length ? maxValue(vals) : null;
  }
  function bestRowBy(key) {
    return visibleRows().filter(function (r) { return r[key] != null; }).sort(function (a, b) { return b[key] - a[key]; })[0] || null;
  }

  function renderKpis() {
    var s = visibleLoadSummary();
    $('kSamples').textContent = s ? fmtNum(s.samples, 0) : '-';
    $('kRps').textContent = s ? fmtNum(s.rps, 1) : '-';
    $('kP95').textContent = s ? fmtNum(s.p95, 0) + ' ms' : '-';
    $('kErrors').textContent = s ? fmtNum(s.errors, 0) : '-';
    $('kVus').textContent = s ? fmtNum(s.maxVus, 0) : '-';
    var cpuMax = maxSeries('cpu'), memMax = maxSeries('memory');
    $('kCpu').textContent = cpuMax == null ? '-' : fmtNum(cpuMax, 1) + '%';
    $('kMemory').textContent = memMax == null ? '-' : fmtNum(memMax, 1) + '%';
    $('kDuration').textContent = s ? formatDuration(s.durationSec) : '-';
    $('summaryText').textContent = buildInsight(bestRowBy('p95'), s);
  }

  function visibleLoadSummary() {
    if (!state.load) return null;
    if (!visibleBounds()) return state.load.summary;
    var bounds = visibleBounds();
    var rows = visibleLoadRows();
    if (!rows.length) return { samples: 0, durationSec: bounds ? Math.max(1, (bounds.end - bounds.start) / 1000) : 0, rps: 0, errors: 0, p95: 0, maxVus: 0 };
    var elapsed = rows.map(function (r) { return r.elapsed; });
    var start = bounds ? bounds.start : rows[0].ts;
    var end = bounds ? bounds.end : maxValue(rows, function (r) { return r.ts + r.elapsed; });
    var durationSec = Math.max(1, (end - start) / 1000);
    return {
      samples: rows.length,
      durationSec: durationSec,
      rps: rows.length / durationSec,
      errors: rows.filter(function (r) { return !r.ok; }).length,
      p95: percentile(elapsed, 0.95),
      maxVus: maxValue(rows, function (r) { return r.threads || 0; })
    };
  }

  function visibleLoadRows() {
    if (!state.load) return [];
    var bounds = visibleBounds();
    return bounds ? state.load.rows.filter(function (r) { return r.ts >= bounds.start && r.ts <= bounds.end; }) : state.load.rows;
  }

  function endpointStatsForRows(rows) {
    var endpoints = new Map();
    rows.forEach(function (r) {
      var e = endpoints.get(r.label) || { label: r.label, count: 0, errors: 0, elapsed: [] };
      e.count++;
      if (!r.ok) e.errors++;
      e.elapsed.push(r.elapsed);
      endpoints.set(r.label, e);
    });
    return Array.from(endpoints.values()).map(function (e) {
      return { label: e.label, count: e.count, errors: e.errors, min: minValue(e.elapsed), max: maxValue(e.elapsed), avg: average(e.elapsed), p95: percentile(e.elapsed, 0.95) };
    }).sort(function (a, b) { return b.p95 - a.p95; });
  }

  function errorStatsForRows(rows) {
    var errorEndpoints = new Map();
    rows.filter(function (r) { return !r.ok; }).forEach(function (r) {
      var epKey = r.label + '|' + (r.code || '-') + '|' + (r.message || r.url || '');
      var ep = errorEndpoints.get(epKey) || { label: r.label, code: r.code || '-', message: r.message || '', url: r.url || '', count: 0, elapsed: [], first: r.ts, last: r.ts };
      ep.count++;
      ep.elapsed.push(r.elapsed);
      ep.first = Math.min(ep.first, r.ts);
      ep.last = Math.max(ep.last, r.ts);
      errorEndpoints.set(epKey, ep);
    });
    return Array.from(errorEndpoints.values()).map(function (e) {
      return { label: e.label, code: e.code, count: e.count, min: minValue(e.elapsed), max: maxValue(e.elapsed), avg: average(e.elapsed), p95: percentile(e.elapsed, 0.95), first: e.first, last: e.last, message: e.message, url: e.url };
    }).sort(function (a, b) { return b.p95 - a.p95; });
  }

  function formatDuration(seconds) {
    if (seconds < 60) return fmtNum(seconds, 0) + ' s';
    return fmtNum(seconds / 60, 1) + ' min';
  }

  function buildInsight(row, summary) {
    if (!summary) return 'Los hallazgos aparecen cuando cargues datos.';
    var bounds = visibleBounds();
    var range = bounds ? fmtTime(bounds.start) + ' - ' + fmtTime(bounds.end) : fmtTime(state.load.summary.start) + ' - ' + fmtTime(state.load.summary.end);
    var parts = ['Vista ' + range, 'duracion ' + formatDuration(summary.durationSec), 'agrupado ' + formatDuration(state.bucketMs / 1000)];
    if (!row) return parts.join(' | ');
    parts.push('punto sensible ' + fmtTime(row.t));
    if (row.vus != null) parts.push('VUs ' + fmtNum(row.vus, 0));
    if (row.rps != null) parts.push('RPS ' + fmtNum(row.rps, 1));
    if (row.errors != null) parts.push('errores ' + fmtNum(row.errors, 0));
    if (row.avg != null) parts.push('avg ' + fmtNum(row.avg, 0) + ' ms');
    if (row.p95 != null) parts.push('P95 ' + fmtNum(row.p95, 0) + ' ms');
    if (row.cpu != null) parts.push('CPU ' + fmtNum(row.cpu, 1) + '%');
    if (row.memory != null) parts.push('memoria ' + fmtNum(row.memory, 1) + '%');
    var note = '. ';
    if ((row.cpu || 0) > 80 && (row.p95 || 0) > 500) note += 'Posible saturacion de CPU correlacionada con latencia.';
    else if ((row.errors || 0) > 0 && (row.cpu || 0) < 75) note += 'Errores sin CPU saturada: revisar cierre de prueba, red, timeouts o aplicacion.';
    else if ((row.p95 || 0) > 500 && (row.cpu || 0) < 75) note += 'Latencia alta sin CPU saturada: revisar endpoint, IO, locks, DB o cliente de carga.';
    else note += 'No se ve saturacion clara en ese punto.';
    return parts.join(' | ') + note;
  }

  function renderTables() {
    if (state.load) {
      var bounds = visibleBounds();
      var rows = bounds ? visibleLoadRows() : state.load.rows;
      var endpoints = bounds ? endpointStatsForRows(rows) : state.load.endpoints;
      renderEndpointTable('valuesTable', endpoints);
      renderEndpointTable('slowTable', endpoints.slice().sort(function (a, b) { return b.p95 - a.p95; }));
      renderErrorsTable();
    }
  }

  function renderEndpointTable(id, rows) {
    renderPagedTable(id, ['Request', 'Samples', 'Errores', 'Error %', 'Min', 'Average', 'P95', 'Max'], rows.map(function (e) {
      var errPct = e.count ? e.errors / e.count * 100 : 0;
      return [e.label, fmtNum(e.count, 0), fmtNum(e.errors, 0), fmtNum(errPct, 2) + '%', fmtNum(e.min, 0) + ' ms', fmtNum(e.avg, 0) + ' ms', fmtNum(e.p95, 0) + ' ms', fmtNum(e.max, 0) + ' ms'];
    }));
  }

  function renderErrorsTable() {
    var rows = state.load ? (visibleBounds() ? errorStatsForRows(visibleLoadRows()) : state.load.errorStats) : [];
    if (!rows.length) {
      $('errorsTable').innerHTML = '<div class="empty">No encontre errores en el archivo de prueba.</div>';
      return;
    }
    renderPagedTable('errorsTable', ['Request', 'Codigo', 'Errores', 'Primero', 'Ultimo', 'Min', 'Average', 'P95', 'Max', 'Mensaje / URL'], rows.map(function (e) {
      var detail = e.message || e.url || '-';
      if (e.message && e.url) detail = e.message + ' | ' + e.url;
      return [e.label, e.code, fmtNum(e.count, 0), fmtFull(e.first), fmtFull(e.last), fmtNum(e.min, 0) + ' ms', fmtNum(e.avg, 0) + ' ms', fmtNum(e.p95, 0) + ' ms', fmtNum(e.max, 0) + ' ms', detail];
    }));
  }

  function table(headers, rows) {
    return '<table><thead><tr>' + headers.map(function (h) { return '<th>' + esc(h) + '</th>'; }).join('') + '</tr></thead><tbody>' +
      rows.map(function (r) { return '<tr>' + r.map(function (c) { return '<td>' + esc(c) + '</td>'; }).join('') + '</tr>'; }).join('') +
      '</tbody></table>';
  }

  function renderPagedTable(id, headers, rows) {
    var cfg = tableState[id] || { page: 0, size: 25 };
    var pages = Math.max(1, Math.ceil(rows.length / cfg.size));
    cfg.page = Math.min(cfg.page, pages - 1);
    tableState[id] = cfg;
    var start = cfg.page * cfg.size;
    var pageRows = rows.slice(start, start + cfg.size);
    var end = Math.min(rows.length, start + cfg.size);
    var rangeText = rows.length ? 'Mostrando ' + fmtNum(start + 1, 0) + '-' + fmtNum(end, 0) + ' de ' + fmtNum(rows.length, 0) : '0 filas';
    var pager = '<div class="pager"><span>' + rangeText + '</span>' +
      '<button type="button" class="icon-btn" data-page="' + id + '" data-dir="-1">Anterior</button>' +
      '<span>' + (cfg.page + 1) + ' / ' + pages + '</span>' +
      '<button type="button" class="icon-btn" data-page="' + id + '" data-dir="1">Siguiente</button></div>';
    $(id).innerHTML = table(headers, pageRows) + pager;
    $(id).querySelectorAll('[data-page]').forEach(function (button) {
      button.addEventListener('click', function () {
        var target = button.getAttribute('data-page');
        tableState[target].page = Math.max(0, Math.min(pages - 1, tableState[target].page + Number(button.getAttribute('data-dir'))));
        renderTables();
      });
    });
  }

  function drawAllCharts() {
    drawChart('timeline', ['rps', 'avg', 'p95', 'vus', 'errors', 'cpu', 'memory'], true);
    drawChart('chartRps', ['rps'], false);
    drawChart('chartP95', ['p95'], false);
    drawChart('chartAvg', ['avg'], false);
    drawChart('chartErrors', ['errors'], false);
    drawChart('chartCpu', ['cpu'], false);
    drawChart('chartMemory', ['memory'], false);
  }

  function drawChart(id, series, combined) {
    var canvas = $(id), ctx = canvas.getContext('2d'), width = canvas.width, height = canvas.height;
    ctx.clearRect(0, 0, width, height);
    ctx.fillStyle = '#0e1422';
    ctx.fillRect(0, 0, width, height);
    var rows = visibleRows();
    if (!rows.length) {
      ctx.fillStyle = '#93a0bd';
      ctx.font = '14px -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif';
      ctx.fillText('Sin datos para este rango.', 20, 32);
      return;
    }
    var active = {};
    document.querySelectorAll('[data-series]').forEach(function (input) { active[input.dataset.series] = input.checked; });
    if (combined) series = series.filter(function (s) { return active[s]; });
    var pad = combined ? { l: 132, r: 94, t: 34, b: 42 } : { l: 62, r: 18, t: 18, b: 42 };
    var minX = rows[0].t, maxX = rows[rows.length - 1].t;
    if (minX === maxX) maxX += 60000;
    var scales = buildScales(rows, series);
    drawGrid(ctx, width, height, pad, minX, maxX, rows);
    drawAxes(ctx, width, height, pad, series, scales, combined);
    drawOrder(series, combined).forEach(function (key) { drawSeries(ctx, rows, key, minX, maxX, pad, width, height, scales[key]); });
    drawLegend(ctx, series, combined ? 12 : 10, pad.l);
  }

  function drawOrder(series, combined) {
    if (!combined) return series;
    var priority = ['memory', 'cpu', 'vus', 'errors', 'avg', 'p95', 'rps'];
    return priority.filter(function (key) { return series.indexOf(key) >= 0; });
  }

  function buildScales(rows, series) {
    var scales = {};
    series.forEach(function (key) {
      var vals = rows.map(function (r) { return r[key]; }).filter(function (v) { return v != null && Number.isFinite(v); });
      if (!vals.length) return;
      var maxY = maxValue(vals);
      if (maxY <= 0) maxY = 1;
      scales[key] = maxY * 1.08;
    });
    return scales;
  }

  function drawGrid(ctx, width, height, pad, minX, maxX, rows) {
    ctx.strokeStyle = '#26304d';
    ctx.lineWidth = 1;
    for (var i = 0; i < 5; i++) {
      var y = pad.t + (height - pad.t - pad.b) * i / 4;
      ctx.beginPath(); ctx.moveTo(pad.l, y); ctx.lineTo(width - pad.r, y); ctx.stroke();
    }
    ctx.fillStyle = '#93a0bd';
    ctx.font = '12px -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif';
    var ticks = timeTicks(rows);
    ticks.forEach(function (tm) {
      var x = pad.l + (tm - minX) / (maxX - minX) * (width - pad.l - pad.r);
      ctx.fillText(fmtTime(tm), x - 18, height - 16);
    });
  }

  function timeTicks(rows) {
    if (rows.length <= 8) return rows.map(function (r) { return r.t; });
    var step = Math.ceil((rows.length - 1) / 6);
    var ticks = [];
    for (var i = 0; i < rows.length; i += step) ticks.push(rows[i].t);
    var last = rows[rows.length - 1].t;
    if (ticks[ticks.length - 1] !== last) ticks.push(last);
    return ticks;
  }

  function drawAxes(ctx, width, height, pad, series, scales, combined) {
    var left = combined ? series.filter(function (s) { return ['memory', 'cpu', 'avg', 'p95'].indexOf(s) >= 0; }) : series;
    var right = combined ? series.filter(function (s) { return ['vus', 'rps', 'errors'].indexOf(s) >= 0; }) : [];
    left.forEach(function (key, i) { drawAxisTicks(ctx, pad.l - 12 - i * 30, height, pad, key, scales[key], 'left'); });
    right.forEach(function (key, i) { drawAxisTicks(ctx, width - pad.r + 10 + i * 28, height, pad, key, scales[key], 'right'); });
  }

  function drawAxisTicks(ctx, x, height, pad, key, maxY, side) {
    if (!maxY) return;
    ctx.fillStyle = colors[key];
    ctx.font = '11px -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif';
    ctx.textAlign = side === 'right' ? 'left' : 'right';
    ctx.textBaseline = 'middle';
    for (var i = 0; i < 5; i++) {
      var y = pad.t + (height - pad.t - pad.b) * i / 4;
      var value = maxY - (maxY * i / 4);
      ctx.fillText(axisValue(key, value), x, y);
    }
    ctx.textAlign = 'left';
    ctx.textBaseline = 'alphabetic';
  }

  function axisValue(key, value) {
    if (key === 'cpu' || key === 'memory' || key === 'rps') return fmtNum(value, 1);
    return fmtNum(value, 0);
  }

  function drawLegend(ctx, series, y, startX) {
    var x = startX;
    ctx.font = '12px -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif';
    series.forEach(function (key) {
      ctx.fillStyle = colors[key];
      ctx.fillRect(x, y, 10, 10);
      ctx.fillStyle = '#e8ecf6';
      ctx.fillText(labels[key], x + 14, y + 10);
      x += Math.max(78, labels[key].length * 8 + 30);
    });
  }

  function drawSeries(ctx, rows, key, minX, maxX, pad, width, height, maxY) {
    if (!maxY) return;
    var points = rows.map(function (r) {
      var v = r[key];
      if (v == null || !Number.isFinite(v)) return null;
      return {
        x: pad.l + (r.t - minX) / (maxX - minX) * (width - pad.l - pad.r),
        y: height - pad.b - (v / maxY) * (height - pad.t - pad.b),
        t: r.t
      };
    }).filter(Boolean);
    if (!points.length) return;
    ctx.strokeStyle = colors[key];
    ctx.lineWidth = key === 'rps' || key === 'avg' || key === 'p95' ? 2.8 : 2.2;
    ctx.lineJoin = 'round';
    ctx.lineCap = 'round';
    ctx.globalAlpha = key === 'memory' || key === 'cpu' ? 0.82 : 0.95;
    splitPointRuns(points).forEach(function (run) {
      if (!run.length) return;
      if (run.length === 1) {
        ctx.beginPath();
        ctx.arc(run[0].x, run[0].y, 2.5, 0, Math.PI * 2);
        ctx.fill();
        return;
      }
      drawSmoothPath(ctx, run, pad.t, height - pad.b);
      ctx.stroke();
    });
    ctx.globalAlpha = 1;
    ctx.fillStyle = colors[key];
    points.forEach(function (p) {
      ctx.beginPath();
      ctx.arc(p.x, p.y, 2.5, 0, Math.PI * 2);
      ctx.fill();
    });
  }

  function splitPointRuns(points) {
    var runs = [], current = [];
    var gaps = [];
    for (var i = 1; i < points.length; i++) gaps.push(points[i].t - points[i - 1].t);
    gaps.sort(function (a, b) { return a - b; });
    var naturalGap = gaps.length ? gaps[Math.floor(gaps.length / 2)] : state.bucketMs;
    var gapLimit = Math.max(state.bucketMs * 1.5, naturalGap * 2.2);
    points.forEach(function (point) {
      var prev = current[current.length - 1];
      if (prev && point.t - prev.t > gapLimit) {
        runs.push(current);
        current = [];
      }
      current.push(point);
    });
    if (current.length) runs.push(current);
    return runs;
  }

  function drawSmoothPath(ctx, points, minY, maxY) {
    function clampY(y) { return Math.max(minY, Math.min(maxY, y)); }
    ctx.beginPath();
    ctx.moveTo(points[0].x, points[0].y);
    if (points.length === 2) {
      ctx.lineTo(points[1].x, points[1].y);
      return;
    }
    for (var i = 0; i < points.length - 1; i++) {
      var p0 = points[Math.max(0, i - 1)];
      var p1 = points[i];
      var p2 = points[i + 1];
      var p3 = points[Math.min(points.length - 1, i + 2)];
      var cp1x = p1.x + (p2.x - p0.x) / 6;
      var cp1y = clampY(p1.y + (p2.y - p0.y) / 6);
      var cp2x = p2.x - (p3.x - p1.x) / 6;
      var cp2y = clampY(p2.y - (p3.y - p1.y) / 6);
      ctx.bezierCurveTo(cp1x, cp1y, cp2x, cp2y, p2.x, p2.y);
    }
  }

  function nearestMetric(rows, key, target) {
    var best = null, bestDistance = Infinity;
    rows.forEach(function (r) {
      if (r[key] == null || !Number.isFinite(r[key])) return;
      var distance = Math.abs(r.t - target);
      if (distance < bestDistance) {
        best = r;
        bestDistance = distance;
      }
    });
    return best;
  }

  function metricCadence(rows, key) {
    var times = rows.filter(function (r) { return r[key] != null && Number.isFinite(r[key]); }).map(function (r) { return r.t; });
    if (times.length < 2) return state.bucketMs;
    var gaps = [];
    for (var i = 1; i < times.length; i++) gaps.push(times[i] - times[i - 1]);
    gaps.sort(function (a, b) { return a - b; });
    return gaps[Math.floor(gaps.length / 2)] || state.bucketMs;
  }

  function loadMetricAt(rows, key, target) {
    for (var i = 0; i < rows.length; i++) {
      var r = rows[i];
      var end = r.end || r.t + state.bucketMs;
      if (r[key] != null && target >= r.t && target < end) return r;
    }
    return nearestMetric(rows.filter(function (r) {
      return r[key] != null && Math.abs(r.t - target) <= state.bucketMs / 2;
    }), key, target);
  }

  function serverMetricAt(rows, key, target) {
    var sourceRows = rows.filter(function (r) { return r[key] != null && Number.isFinite(r[key]); });
    if (!sourceRows.length) return null;
    var cadence = metricCadence(rows, key);
    var best = null;
    sourceRows.forEach(function (r) {
      if (r.t <= target && (!best || r.t > best.t)) best = r;
    });
    if (!best || target - best.t > cadence) return null;
    return best;
  }

  function tooltipMetricAt(rows, key, target) {
    if (key === 'cpu' || key === 'memory') return serverMetricAt(rows, key, target);
    return loadMetricAt(rows, key, target);
  }

  function rowFromEvent(canvas, ev) {
    var preferredKey = chartKey(canvas.id);
    var rows = visibleRows();
    if (preferredKey) rows = rows.filter(function (r) { return r[preferredKey] != null; });
    if (!rows.length) return null;
    var rect = canvas.getBoundingClientRect();
    var x = (ev.clientX - rect.left) / rect.width * canvas.width;
    var padL = canvas.id === 'timeline' ? 132 : 62;
    var padR = canvas.id === 'timeline' ? 94 : 18;
    var minX = rows[0].t, maxX = rows[rows.length - 1].t || rows[0].t + 60000;
    var ratio = Math.max(0, Math.min(1, (x - padL) / (canvas.width - padL - padR)));
    var target = minX + ratio * (maxX - minX);
    if (preferredKey) return nearestMetric(rows, preferredKey, target);
    var allRows = visibleRows();
    var base = loadMetricAt(allRows, 'rps', target) || nearestMetric(allRows, 'rps', target) || allRows.slice().sort(function (a, b) { return Math.abs(a.t - target) - Math.abs(b.t - target); })[0];
    var row = { t: base.t, end: base.end || base.t + state.bucketMs };
    ['memory', 'cpu', 'vus', 'vusMax', 'rps', 'errors', 'avg', 'p95'].forEach(function (key) {
      var source = tooltipMetricAt(allRows, key, target);
      row[key] = source ? source[key] : null;
    });
    return row;
  }

  function chartKey(id) {
    return {
      chartRps: 'rps',
      chartP95: 'p95',
      chartAvg: 'avg',
      chartErrors: 'errors',
      chartCpu: 'cpu',
      chartMemory: 'memory'
    }[id] || null;
  }

  function setupTooltip() {
    var tip = $('tooltip');
    document.querySelectorAll('canvas').forEach(function (canvas) {
    canvas.addEventListener('mousemove', function (ev) {
      var row = rowFromEvent(canvas, ev);
      if (!row) return;
      tip.innerHTML = '<strong>' + fmtInterval(row) + '</strong>' +
        tooltipLine('memory', 'Memoria', row.memory) +
        tooltipLine('cpu', 'CPU', row.cpu) +
        tooltipLine('vus', 'VUs', row.vus) +
        tooltipLine('vus', 'VUs max', row.vusMax) +
        tooltipLine('rps', 'RPS', row.rps) +
        tooltipLine('errors', 'Errors', row.errors) +
        tooltipLine('avg', 'AVE', row.avg) +
        tooltipLine('p95', 'P95', row.p95);
      tip.hidden = false;
      tip.style.left = Math.min(window.innerWidth - 380, Math.max(10, ev.clientX + 14)) + 'px';
      tip.style.top = Math.min(window.innerHeight - 150, Math.max(10, ev.clientY + 14)) + 'px';
    });
    canvas.addEventListener('mouseleave', function () { tip.hidden = true; });
    });
  }

  function tooltipLine(key, label, value) {
    if (value == null || Number.isNaN(value)) return '';
    return '<div class="tip-row"><span class="swatch" style="background:' + colors[key] + '"></span><span>' + esc(label) + ':</span><b>' + esc(fmtMetric(key, value)) + '</b></div>';
  }

  function setupDragZoom() {
    var canvas = $('timeline');
    var wrap = canvas.parentElement;
    var box = document.createElement('div');
    box.className = 'selection';
    box.hidden = true;
    wrap.appendChild(box);
    var startX = null;

    function dataTimeFromClientX(clientX) {
      var rows = visibleRows();
      if (!rows.length) return null;
      var rect = canvas.getBoundingClientRect();
      var padL = 132, padR = 94;
      var x = Math.max(padL, Math.min(canvas.width - padR, (clientX - rect.left) / rect.width * canvas.width));
      var ratio = (x - padL) / (canvas.width - padL - padR);
      var minX = rows[0].t, maxX = rows[rows.length - 1].t;
      if (minX === maxX) maxX += 60000;
      return minX + ratio * (maxX - minX);
    }

    canvas.addEventListener('mousedown', function (ev) {
      if (!visibleRows().length) return;
      startX = ev.clientX;
      var rect = wrap.getBoundingClientRect();
      box.hidden = false;
      box.style.left = Math.max(14, ev.clientX - rect.left) + 'px';
      box.style.width = '0px';
    });

    window.addEventListener('mousemove', function (ev) {
      if (startX == null) return;
      var rect = wrap.getBoundingClientRect();
      var left = Math.max(14, Math.min(startX, ev.clientX) - rect.left);
      var right = Math.min(rect.width - 14, Math.max(startX, ev.clientX) - rect.left);
      box.style.left = left + 'px';
      box.style.width = Math.max(0, right - left) + 'px';
    });

    window.addEventListener('mouseup', function (ev) {
      if (startX == null) return;
      var delta = Math.abs(ev.clientX - startX);
      var a = dataTimeFromClientX(startX);
      var b = dataTimeFromClientX(ev.clientX);
      startX = null;
      box.hidden = true;
      if (delta < 12 || a == null || b == null) return;
      var start = bucketKey(Math.min(a, b));
      var end = bucketKey(Math.max(a, b));
      if (end <= start) end = start + state.bucketMs;
      state.zoom = { start: start, end: end };
      renderKpis(); renderTables(); drawAllCharts();
    });
  }

  function setupDownloads() {
    document.querySelectorAll('[data-download]').forEach(function (button) {
      button.addEventListener('click', function () {
        var id = button.getAttribute('data-download');
        var canvas = $(id);
        if (!canvas) return;
        var link = document.createElement('a');
        link.download = 'relampo-' + id + '.png';
        link.href = canvas.toDataURL('image/png');
        link.click();
      });
    });
  }

  function readFile(input) {
    return new Promise(function (resolve, reject) {
      if (!input.files || !input.files[0]) return resolve(null);
      var reader = new FileReader();
      reader.onload = function () { resolve({ name: input.files[0].name, text: reader.result }); };
      reader.onerror = function () { reject(reader.error); };
      reader.readAsText(input.files[0]);
    });
  }

  function resetTablePages() {
    Object.keys(tableState).forEach(function (key) { tableState[key].page = 0; });
  }

  function setupTableControls() {
    document.querySelectorAll('[data-page-size]').forEach(function (select) {
      select.addEventListener('change', function () {
        var id = select.getAttribute('data-page-size');
        tableState[id].size = Number(select.value) || 25;
        tableState[id].page = 0;
        renderTables();
      });
    });
  }

  function setupBucketControl() {
    $('bucketSize').addEventListener('change', function () {
      state.bucketMs = Number($('bucketSize').value) || 60000;
      if (state.load) state.load = summarizeRows(state.load.rows);
      state.zoom = null;
      resetTablePages();
      render();
    });
  }

  $('analyzeBtn').addEventListener('click', function () {
    $('analyzeBtn').disabled = true;
    $('status').textContent = 'Analizando archivos... si son grandes puede tardar unos segundos.';
    $('status').className = 'status';
    Promise.all([readFile($('loadFile')), readFile($('cpuFile')), readFile($('memoryFile'))]).then(function (files) {
      state.bucketOrigin = null;
      if (files[0]) state.load = parseLoadFile(files[0].text, files[0].name);
      if (files[1]) state.cpuRaw = parseMetricJson(files[1].text);
      if (files[2]) state.memoryRaw = parseMetricJson(files[2].text);
      state.zoom = null;
      resetTablePages();
      render();
      $('status').textContent = 'Analisis listo. Granularidad: ' + formatDuration(state.bucketMs / 1000) + '.';
      $('status').className = 'status';
    }).catch(function (err) {
      $('status').textContent = err.message || String(err);
      $('status').className = 'status err';
    }).finally(function () {
      $('analyzeBtn').disabled = false;
    });
  });

  $('resetZoom').addEventListener('click', function () {
    state.zoom = null;
    renderKpis(); renderTables(); drawAllCharts();
  });
  document.querySelectorAll('[data-series]').forEach(function (input) { input.addEventListener('change', drawAllCharts); });
  setupTableControls();
  setupBucketControl();
  setupTooltip();
  setupDragZoom();
  setupDownloads();
  drawAllCharts();
})();
