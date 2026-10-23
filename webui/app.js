'use strict';
// HinaTracer WebUI — self-contained, no external resources.
(function () {
  const $ = (s, r) => (r || document).querySelector(s);
  // handlers that call these change server state (guests may not)
  const WRITE_RE = /\b(api|editAlias|delIDs|importDlg|saveUpd|saveStartup|httpEdit|httpBatch|httpDel|httpTrace|resetStats)\(/;
  let GUEST = false;
  const H = (tag, attrs, ...kids) => {
    const e = document.createElement(tag);
    if (attrs) for (const k in attrs) {
      const v = attrs[k];
      if (v == null || v === false) continue;
      if (k === 'class') e.className = v;
      else if (k === 'text') e.textContent = v;
      else if (k.startsWith('on')) e.addEventListener(k.slice(2), v);
      else if (k === 'value') e.value = v;
      else if (k === 'checked') e.checked = !!v;
      else if (k === 'disabled') e.disabled = !!v;
      else e.setAttribute(k, v === true ? '' : v);
    }
    for (const k of kids.flat()) if (k != null && k !== false) e.append(k.nodeType ? k : document.createTextNode(String(k)));
    // controls that change server state are hidden for read-only guests
    if (attrs && WRITE_RE.test(String(attrs.onclick || '') + String(attrs.onchange || ''))) e.classList.add('w');
    return e;
  };
  const W = el => { el.classList.add('w'); return el; };

  // ---------- color theme ----------
  // Like the language: a default in the config (admin only, S.settings.palette
  // and S.settings.theme) and this browser's own pick in localStorage
  // (ht.palette, ht.mode; empty = follow the default). Guests may pick too.
  const PALETTES = ['sakura', 'mint', 'ocean', 'sunset', 'graphite', 'matcha'];
  const okPalette = id => PALETTES.includes(id) ? id : '';
  const myPalette = () => okPalette(localStorage.getItem('ht.palette') || '');
  const myMode = () => { const m = localStorage.getItem('ht.mode') || ''; return ['light', 'dark', 'system'].includes(m) ? m : ''; };
  const defPalette = () => okPalette((S && S.settings.palette) || '') || PALETTES[0];
  const defMode = () => (S && S.settings.theme) || 'system';
  function applyLook() {
    const root = document.documentElement;
    root.dataset.palette = myPalette() || defPalette();
    const m = myMode() || defMode();
    if (m === 'light' || m === 'dark') root.dataset.theme = m; else delete root.dataset.theme;
  }
  { // before the first state (S not yet declared): this browser's pick only
    const root = document.documentElement, m = myMode();
    root.dataset.palette = myPalette() || PALETTES[0];
    if (m === 'light' || m === 'dark') root.dataset.theme = m;
  }

  // ---------- i18n ----------
  let STR = {}, LANGS = [], LANG = '', DEFAULT_LANG = '';
  function T(key, ...args) {
    let s = STR[key];
    if (s == null) return key;
    args.forEach((a, i) => {
      s = s.split('{' + i + '}').join(String(a));
      s = s.replace(new RegExp('\\{' + i + ':\\.(\\d)f\\}', 'g'), (_, d) => Number(a).toFixed(+d));
    });
    return s;
  }
  async function loadI18n() {
    const want = localStorage.getItem('ht.lang') || '';
    const r = await fetch('/api/i18n' + (want ? ('?lang=' + encodeURIComponent(want)) : ''), { cache: 'no-store' });
    const j = await r.json();
    STR = j.strings || {}; LANGS = j.langs || []; LANG = j.lang; DEFAULT_LANG = j.default || j.lang;
    document.documentElement.lang = LANG;
    document.querySelectorAll('[data-t]').forEach(e => { e.textContent = T(e.dataset.t); });
  }

  // ---------- API ----------
  async function api(path, body) {
    if (GUEST && path !== 'logout') { toast(T('web.guest.tip')); return {}; }
    const r = await fetch('/api/' + path, {
      method: 'POST', credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', 'X-HinaTracer': '1' },
      body: JSON.stringify(body || {}),
    });
    if (r.status === 401) { showLogin(); throw new Error('auth'); }
    let j = {};
    try { j = await r.json(); } catch (e) { /* empty */ }
    if (!r.ok && j.message) toast(j.message);
    return j;
  }

  // ---------- helpers ----------
  let toastTimer;
  function toast(msg) {
    if (!msg) return;
    const t = $('#toast'); t.textContent = msg; t.hidden = false;
    clearTimeout(toastTimer); toastTimer = setTimeout(() => { t.hidden = true; }, 2600);
  }
  function copy(text) {
    if (!text) return;
    const done = () => toast(T('web.copied'));
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(text).then(done, () => fallbackCopy(text, done));
    } else fallbackCopy(text, done);
  }
  function fallbackCopy(text, done) {
    const ta = H('textarea', { style: 'position:fixed;opacity:0' }); ta.value = text;
    document.body.append(ta); ta.select();
    try { document.execCommand('copy'); done(); } catch (e) { /* ignore */ }
    ta.remove();
  }
  const dash = v => (v == null || v === '' ? '—' : v);
  const ms = (v, ok) => (ok ? Number(v).toFixed(1) + ' ms' : '—');
  const pct = v => Number(v).toFixed(1) + '%';
  function fmtTime(t) {
    if (!t) return '—';
    const d = new Date(t), now = new Date();
    const p = n => String(n).padStart(2, '0');
    const hms = p(d.getHours()) + ':' + p(d.getMinutes()) + ':' + p(d.getSeconds());
    if (d.toDateString() === now.toDateString()) return hms;
    return d.getFullYear() + '-' + p(d.getMonth() + 1) + '-' + p(d.getDate()) + ' ' + hms;
  }
  function fmtBytes(n) {
    if (n < 1024) return n + ' B';
    if (n < 1048576) return (n / 1024).toFixed(1) + ' KB';
    return (n / 1048576).toFixed(2) + ' MB';
  }
  const placeholder = v => !v || v === '—' || v === '…' || v === T('cell.loading') || v === T('cell.resolving');
  function flagCell(iso, name) {
    const sig = (iso || '') + '|' + (name || '');
    return [sig, () => {
      const s = H('span', { class: 'flag' + (placeholder(name) ? ' muted' : '') });
      if (iso && /^[A-Za-z]{2}$/.test(iso)) {
        const img = H('img', { src: '/flags/' + iso.toLowerCase() + '.png', alt: iso, loading: 'lazy' });
        img.onerror = () => img.remove();
        s.append(img);
      }
      s.append(dash(name));
      return s;
    }];
  }
  function lossCell(v, any) {
    if (!any) return '—';
    return [v.toFixed(1), () => H('span', null, H('span', { class: 'lossbar' }, H('i', { style: 'width:' + Math.min(100, v) + '%' })), pct(v))];
  }

  // ---------- context menu & modal ----------
  let menuEl = null, menuPrefix = null;
  function closeMenu() { if (menuEl) { menuEl.remove(); menuEl = null; } }
  function openMenu(x, y, items) {
    closeMenu();
    // A table puts its "Copy column" submenu in front of every row menu.
    if (menuPrefix) { items = [...menuPrefix, ...items]; menuPrefix = null; }
    if (GUEST) {
      items = items.filter(it => it === '-' || it && !WRITE_RE.test(String(it.run)));
      items = items.filter((it, i, a) => it !== '-' || (i > 0 && i < a.length - 1 && a[i - 1] !== '-'));
      if (!items.length) return;
    }
    const m = H('div', { class: 'menu' });
    items.forEach(it => {
      if (it === '-') { m.append(H('hr')); return; }
      if (!it) return;
      if (it.sub) {
        const fly = H('div', { class: 'menu sub' });
        it.sub.forEach(s => fly.append(s === '-' ? H('hr') : H('button', { disabled: s.disabled, onclick: () => { closeMenu(); s.run(); } }, s.label)));
        const b = H('button', { class: 'has-sub', disabled: it.disabled }, it.label, H('span', { class: 'sub-arrow', text: '›' }));
        const show = () => {
          if (b.disabled) return;
          m.querySelectorAll('.menu.sub').forEach(x => { if (x !== fly) x.remove(); });
          if (!fly.isConnected) m.append(fly);
          const br = b.getBoundingClientRect(), fr = fly.getBoundingClientRect();
          let left = br.right - 2;
          if (left + fr.width > innerWidth - 6) left = br.left - fr.width + 2;
          fly.style.left = left + 'px';
          fly.style.top = Math.max(4, Math.min(br.top - 4, innerHeight - fr.height - 6)) + 'px';
        };
        b.onmouseenter = show; b.onclick = e => { e.stopPropagation(); show(); };
        m.append(b);
        return;
      }
      const btn = H('button', { class: it.danger ? 'danger' : '', disabled: it.disabled, onclick: () => { closeMenu(); it.run(); } }, it.label);
      btn.onmouseenter = () => m.querySelectorAll('.menu.sub').forEach(x => x.remove());
      m.append(btn);
    });
    document.body.append(m);
    const r = m.getBoundingClientRect();
    m.style.left = Math.max(4, Math.min(x, innerWidth - r.width - 6)) + 'px';
    m.style.top = Math.max(4, Math.min(y, innerHeight - r.height - 6)) + 'px';
    menuEl = m;
  }
  document.addEventListener('mousedown', e => { if (menuEl && !menuEl.contains(e.target)) closeMenu(); });
  document.addEventListener('keydown', e => { if (e.key === 'Escape') { closeMenu(); if (trendView) closeTrendView(); else closeModal(); } });
  window.addEventListener('blur', closeMenu);

  let modal = null; // {bg, update}
  function openModal(build, wide) {
    closeModal();
    const box = H('div', { class: 'modal' + (wide ? ' wide' : '') });
    const bg = H('div', { class: 'modal-bg', onmousedown: e => { if (e.target === bg) closeModal(); } }, box);
    document.body.append(bg);
    modal = { bg, update: null };
    modal.update = build(box) || null;
    const f = box.querySelector('input,textarea');
    if (f) setTimeout(() => f.focus(), 30);
  }
  function closeModal() { if (modal) { modal.bg.remove(); modal = null; } }
  // resetStats clears stats; while they are saved to disk it asks first,
  // because the saved totals are cleared for good.
  function resetStats(kind, ids, n) {
    if (!n) return;
    if (!S.settings.historySave) { api(kind + '/reset', { ids }); return; }
    confirmBox(T('reset.confirm.title'), T('reset.confirm.desc', n), T('reset.confirm.ok'),
      () => api(kind + '/reset', { ids, confirm: true }), true);
  }
  function confirmBox(title, text, okLabel, onOk, danger, extra) {
    openModal(box => {
      box.append(H('h2', { text: title }), H('div', { class: 'desc', text: text }));
      if (extra) box.append(extra);
      box.append(H('div', { class: 'actions' },
        H('button', { class: 'btn', onclick: closeModal }, T('alias.cancel')),
        H('button', { class: 'btn ' + (danger ? 'danger' : 'primary'), onclick: () => { closeModal(); onOk(); } }, okLabel)));
    });
  }

  // ---------- keyed, sortable, multi-select table ----------
  // cols: {id,label,num,mono,cls,sort(row)->value,cell(row)->string|[sig,fn]}
  function Table(o) {
    const wrap = H('div', { class: 'table-wrap' });
    const table = H('table'), thead = H('thead'), tbody = H('tbody');
    const empty = H('div', { class: 'empty', text: o.empty || '' });
    table.append(thead, tbody); wrap.append(table, empty);
    const sort = o.sortState || { col: o.defaultSort, desc: false };
    const sel = new Set();
    let anchor = null, rows = [], display = [];
    const rowEls = new Map();
    const hr = H('tr');
    let allChk = null;
    if (o.check) {
      allChk = H('input', { type: 'checkbox', onclick: e => { e.stopPropagation(); if (allChk.checked) display.forEach(r => sel.add(o.key(r))); else sel.clear(); paintSel(); } });
      hr.append(H('th', { class: 'chk' }, allChk));
    }
    o.cols.forEach(c => {
      const th = H('th', { class: c.num ? 'num' : '' }, c.label, H('span', { class: 'arrow' }));
      th.onclick = () => {
        if (sort.col === c.id) sort.desc = !sort.desc; else { sort.col = c.id; sort.desc = false; }
        render(); if (o.onSort) o.onSort(sort);
      };
      c.th = th; hr.append(th);
      if (o.id) th.append(grip(c));
    });
    thead.append(hr);
    // Column widths: drag the right edge of a header; double-click it to
    // reset. Saved per table in localStorage (ht.cols.<id>).
    const wKey = 'ht.cols.' + o.id;
    let widths = {};
    try { widths = JSON.parse(localStorage.getItem(wKey) || '{}') || {}; } catch (_) { widths = {}; }
    function applyWidth(c) {
      const w = widths[c.id];
      for (const p of ['width', 'minWidth', 'maxWidth']) c.th.style[p] = w ? w + 'px' : '';
      table.classList.toggle('sized', Object.keys(widths).length > 0);
    }
    function saveWidths() {
      if (Object.keys(widths).length) localStorage.setItem(wKey, JSON.stringify(widths)); else localStorage.removeItem(wKey);
    }
    function grip(c) {
      const g = H('span', { class: 'grip', title: '' });
      g.onclick = e => e.stopPropagation();
      g.ondblclick = e => { e.stopPropagation(); delete widths[c.id]; applyWidth(c); saveWidths(); };
      g.onpointerdown = e => {
        e.preventDefault(); e.stopPropagation();
        const x0 = e.clientX, w0 = c.th.getBoundingClientRect().width;
        g.setPointerCapture(e.pointerId);
        g.onpointermove = ev => { widths[c.id] = Math.max(36, Math.round(w0 + ev.clientX - x0)); applyWidth(c); };
        g.onpointerup = () => { g.onpointermove = g.onpointerup = null; saveWidths(); };
      };
      return g;
    }
    if (o.id) o.cols.forEach(applyWidth);

    function cmp(a, b) {
      if (a == null || a === '') return b == null || b === '' ? 0 : 1;
      if (b == null || b === '') return -1;
      if (typeof a === 'number' && typeof b === 'number') return a - b;
      return String(a).localeCompare(String(b), undefined, { numeric: true, sensitivity: 'base' });
    }
    function sorted() {
      const c = o.cols.find(x => x.id === sort.col);
      const idx = rows.map((r, i) => i);
      if (c && c.sort) {
        idx.sort((i, j) => {
          let v = cmp(c.sort(rows[i]), c.sort(rows[j]));
          if (sort.desc) v = -v;
          return v || i - j;
        });
      } else if (sort.desc) idx.reverse();
      return idx.map(i => rows[i]);
    }
    function paintSel() {
      for (const [k, el] of rowEls) {
        const on = sel.has(k);
        el.classList.toggle('sel', on);
        if (el._chk) el._chk.checked = on;
      }
      if (allChk) allChk.checked = display.length > 0 && display.every(r => sel.has(o.key(r)));
      if (o.onSelect) o.onSelect();
    }
    function clickRow(e, k) {
      if (e.shiftKey && anchor != null) {
        const keys = display.map(o.key);
        let a = keys.indexOf(anchor), b = keys.indexOf(k);
        if (a > b) [a, b] = [b, a];
        if (!(e.ctrlKey || e.metaKey)) sel.clear();
        for (let i = a; i <= b; i++) sel.add(keys[i]);
      } else if (e.ctrlKey || e.metaKey) {
        if (sel.has(k)) sel.delete(k); else sel.add(k);
        anchor = k;
      } else { sel.clear(); sel.add(k); anchor = k; }
      paintSel();
    }
    function makeRow(k) {
      const tr = H('tr');
      tr._k = k; tr._sig = [];
      if (o.check) {
        const cb = H('input', { type: 'checkbox', onclick: e => { e.stopPropagation(); if (cb.checked) sel.add(k); else sel.delete(k); anchor = k; paintSel(); } });
        tr._chk = cb; tr.append(H('td', { class: 'chk' }, cb));
      }
      o.cols.forEach(c => tr.append(H('td', { class: [c.num ? 'num' : '', c.mono ? 'mono' : '', c.cls || ''].join(' ').trim() })));
      tr.onclick = e => clickRow(e, k);
      tr.ondblclick = () => { const r = rows.find(x => o.key(x) === k); if (r && o.onOpen) o.onOpen(r); };
      tr.oncontextmenu = e => {
        e.preventDefault();
        if (!sel.has(k)) { sel.clear(); sel.add(k); anchor = k; paintSel(); }
        const r = rows.find(x => o.key(x) === k);
        if (r && o.onMenu) { menuPrefix = copyColsMenu(api_selected()); o.onMenu(e, r, api_selected()); menuPrefix = null; }
      };
      return tr;
    }
    function api_selected() { return display.filter(r => sel.has(o.key(r))); }
    // colText is a cell as plain text: the string a cell shows, else the
    // column's copy() or its sort value.
    function colText(c, r, i) {
      if (c.copy) return c.copy(r);
      const v = c.cell(r, i);
      if (!Array.isArray(v)) return v;
      const sv = c.sort ? c.sort(r) : null;
      return typeof sv === 'string' ? sv : String(v[0] ?? '');
    }
    // copyColsMenu: "Copy column" submenu for the selected rows, in the
    // order shown; one value per line, empty values left out.
    function copyColsMenu(selRows) {
      if (!selRows.length) return null;
      const sub = o.cols.filter(c => c.id !== '#').map(c => {
        const vals = selRows.map(r => String(colText(c, r, display.indexOf(r)) ?? '').trim()).filter(v => v && v !== '—' && v !== '*' && v !== '…');
        return { label: c.label, disabled: !vals.length, run: () => copy(vals.join('\n')) };
      });
      return [{ label: T('table.copy_cols', selRows.length), sub }, '-'];
    }
    // Ctrl/Cmd+A selects every row of the table on screen (not while typing).
    document.addEventListener('keydown', e => {
      if (!(e.ctrlKey || e.metaKey) || e.key.toLowerCase() !== 'a' || e.shiftKey || e.altKey) return;
      const t = e.target;
      if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))) return;
      if (!wrap.isConnected || !wrap.offsetParent || modal || !display.length) return;
      e.preventDefault();
      display.forEach(r => sel.add(o.key(r)));
      paintSel();
    });
    function render() {
      o.cols.forEach(c => {
        c.th.classList.toggle('sorted', sort.col === c.id);
        c.th.lastChild.textContent = sort.col === c.id && c.sort ? (sort.desc ? '▼' : '▲') : '';
      });
      display = sorted();
      const live = new Set(display.map(o.key));
      for (const [k, el] of rowEls) if (!live.has(k)) { el.remove(); rowEls.delete(k); sel.delete(k); }
      let prev = null;
      display.forEach((r, i) => {
        const k = o.key(r);
        let tr = rowEls.get(k);
        if (!tr) { tr = makeRow(k); rowEls.set(k, tr); }
        const want = prev ? prev.nextSibling : tbody.firstChild;
        if (want !== tr) tbody.insertBefore(tr, want);
        prev = tr;
        if (o.rowClass) tr.className = (o.rowClass(r) + (sel.has(k) ? ' sel' : '')).trim();
        else tr.classList.toggle('sel', sel.has(k));
        const off = o.check ? 1 : 0;
        o.cols.forEach((c, ci) => {
          const v = c.cell(r, i);
          const sig = Array.isArray(v) ? v[0] : v;
          if (tr._sig[ci] === sig) return;
          tr._sig[ci] = sig;
          const td = tr.children[ci + off];
          if (Array.isArray(v)) { td.replaceChildren(v[1]()); td.title = ''; }
          else { td.textContent = v; td.title = String(v).length > 28 ? v : ''; }
        });
      });
      empty.hidden = display.length > 0;
      table.hidden = false;
      if (allChk) allChk.checked = display.length > 0 && display.every(r => sel.has(o.key(r)));
    }
    return {
      el: wrap, sort, sel, cols: o.cols,
      update(rs) { rows = rs || []; render(); },
      selected: api_selected,
      display: () => display,
      clear() { sel.clear(); paintSel(); },
      relabel(empt) { empty.textContent = empt; },
    };
  }

  // ---------- latency chart (probe + aggregated ranges) ----------
  const RANGES = ['probe', 'minute', 'hour', 'day', 'week', 'month'];
  function niceCeil(v) {
    if (!(v > 0) || !isFinite(v)) return 1;
    const e = Math.pow(10, Math.floor(Math.log10(v)));
    for (const m of [1, 2, 5, 10]) if (m * e >= v) return m * e;
    return 10 * e;
  }
  function axisMs(v) { return (v >= 10 || v === 0 ? v.toFixed(0) : v.toFixed(1)) + ' ms'; }
  function sparkSvg(draw) {
    const W = 600, Hh = 100;
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('viewBox', `0 0 ${W} ${Hh}`); svg.setAttribute('preserveAspectRatio', 'none'); svg.classList.add('spark');
    const mk = (tag, attrs, cls) => { const e = document.createElementNS(svg.namespaceURI, tag); for (const k in attrs) e.setAttribute(k, attrs[k]); if (cls) e.classList.add(cls); svg.append(e); return e; };
    [0, 0.5, 1].forEach(f => mk('line', { x1: 0, x2: W, y1: Hh - f * Hh, y2: Hh - f * Hh, 'vector-effect': 'non-scaling-stroke' }, 'g'));
    draw(mk, W, Hh);
    return svg;
  }
  const p2 = x => String(x).padStart(2, '0');
  function fmtRangeTime(v, range) {
    const d = new Date(v);
    const md = `${p2(d.getMonth() + 1)}-${p2(d.getDate())}`, ymd = `${d.getFullYear()}-${md}`;
    switch (range) {
      case 'probe': return `${md} ${p2(d.getHours())}:${p2(d.getMinutes())}:${p2(d.getSeconds())}`;
      case 'minute': return `${md} ${p2(d.getHours())}:${p2(d.getMinutes())}`;
      case 'hour': return `${md} ${p2(d.getHours())}:00`;
      case 'month': return `${d.getFullYear()}-${p2(d.getMonth() + 1)}`;
      default: return ymd;
    }
  }
  function histToPts(hist) {
    return (hist || []).slice(-120).map(v => v < 0 ? { samples: 1, lost: 1, lossPct: 100, ok: false } : { samples: 1, avg: v, min: v, max: v, lossPct: 0, ok: true });
  }
  // nearest point index for x in [0,w] among n evenly spaced points
  function chartNearest(n, w, x) {
    if (n <= 0 || w <= 0) return -1;
    if (n === 1) return 0;
    return Math.max(0, Math.min(n - 1, Math.round(x / w * (n - 1))));
  }
  function msTxt(v) { return (v >= 100 ? v.toFixed(0) : v.toFixed(1)) + ' ms'; }
  // tipLines: [text, kind] with kind '' (time), 'v' value, 'l' loss, 'i' info.
  function tipLines(pts, i, range) {
    const p = pts[i], out = [[p.at ? fmtRangeTime(p.at, range) : T('chart.tip.index', i + 1), '']];
    if (range === 'probe') out.push(p.ok ? [T('chart.tip.latency', msTxt(p.avg)), 'v'] : [T('chart.tip.lost'), 'l']);
    else {
      if (p.ok) out.push([T('chart.tip.avg', msTxt(p.avg)), 'v'], [T('chart.tip.min', msTxt(p.min)), 'i'], [T('chart.tip.max', msTxt(p.max)), 'i']);
      out.push([T('chart.tip.loss', (p.lossPct || 0).toFixed(1) + '%'), 'l'], [T('chart.tip.samples', p.samples), 'i']);
    }
    if (p.code) out.push([T('chart.tip.code', p.code), 'i']);
    return out;
  }
  // ---- time axis (same rules as the desktop chart, chart_time.go) ----
  const BUCKET_MS = { minute: 60000, hour: 3600000, day: 86400000, week: 604800000, month: 2629746000 };
  const CHART_DEF_BUCKETS = 120, CHART_MIN_BUCKETS = 4, CHART_FETCH = 5000, CHART_MAX_BAR = 24;
  function medianStep(ts) {
    const d = [];
    for (let i = 1; i < ts.length; i++) if (ts[i] > ts[i - 1]) d.push(ts[i] - ts[i - 1]);
    if (!d.length) return 0;
    d.sort((a, b) => a - b);
    return d[d.length >> 1];
  }
  // layoutTimes: each point's axis time (buckets in their middle), the
  // gaps before each point (the line breaks there) and the axis limits.
  // from/to (ms or null) is the time filter, which sets the span.
  function layoutTimes(pts, range, from, to, now) {
    const n = pts.length, probe = range === 'probe';
    const timed = n > 0 && pts.every(p => p.at);
    let bucket = BUCKET_MS[range] || 0;
    const ts = pts.map((p, i) => {
      if (!timed) return now - (n - 1 - i) * 1000;
      const t = new Date(p.at).getTime();
      if (probe) return t;
      if (range === 'month') { const d = new Date(p.at); return (t + new Date(d.getFullYear(), d.getMonth() + 1, d.getDate(), d.getHours(), d.getMinutes()).getTime()) / 2; }
      return t + bucket / 2;
    });
    const med = medianStep(ts);
    if (probe || !bucket) bucket = med || 1000;
    // buckets sit on a fixed grid: a missing bucket is a gap (the median
    // spacing of scattered buckets must not join them)
    let gap = probe ? Math.max(3 * med, 1500) : bucket * 1.5;
    if (!timed) gap = Infinity;
    const brk = ts.map((t, i) => i > 0 && t - ts[i - 1] > gap);
    const half = probe ? 0 : bucket / 2;
    let lo = n ? ts[0] - half : now, hi = n ? Math.max(ts[n - 1] + half, now) : now;
    if (from != null) lo = from;
    // a range still running ends at the live edge (data end or now)
    if (to != null) hi = to <= now ? to : Math.min(hi, to);
    if (hi < lo) hi = lo;
    const minSpan = Math.max(CHART_MIN_BUCKETS * bucket, 1000);
    const def = Math.max(minSpan, from != null || to != null ? hi - lo : Math.min(hi - lo, CHART_DEF_BUCKETS * bucket));
    return { ts, brk, ax: { lo, hi, bucket, gap, def, timed, minSpan, maxSpan: Math.max(hi - lo, def) } };
  }
  // chart view: span/end/panned, K-line style. Not panned = live edge.
  function newView() { return { span: 0, end: 0, panned: false, wantOlder: false }; }
  const vSpan = (v, ax) => Math.max(ax.minSpan, Math.min(v.span > 0 ? v.span : ax.def, ax.maxSpan));
  function vWindow(v, ax) {
    const span = vSpan(v, ax), full = ax.hi - ax.lo;
    if (span >= full) { const t0 = ax.lo - (span - full) / 2; return [t0, t0 + span]; }
    const end = v.panned ? Math.max(ax.lo + span, Math.min(v.end, ax.hi)) : ax.hi;
    return [end - span, end];
  }
  function vSetEnd(v, ax, end, more) {
    const span = vSpan(v, ax);
    if (span >= ax.hi - ax.lo) { v.panned = false; return; }
    end = Math.max(ax.lo + span, Math.min(end, ax.hi));
    v.panned = end < ax.hi; v.end = end;
    if (end - span <= ax.lo && more) v.wantOlder = true;
  }
  function vPan(v, ax, frac, more) { const [t0, t1] = vWindow(v, ax); vSetEnd(v, ax, t1 + frac * (t1 - t0), more); }
  function vZoom(v, ax, f, anchor, more) {
    const [t0, t1] = vWindow(v, ax), old = t1 - t0;
    v.span = Math.max(ax.minSpan, Math.min(old * f, ax.maxSpan));
    if (!v.panned) return;
    anchor = Math.max(0, Math.min(1, anchor));
    const ta = t0 + anchor * old;
    vSetEnd(v, ax, ta - anchor * v.span + v.span, more);
  }
  // chartSnap: the nearest point within a run; in a gap only one within snapMs.
  function chartSnap(ts, brk, t, snapMs) {
    const n = ts.length;
    if (!n) return -1;
    let lo = 0, hi = n;
    while (lo < hi) { const m = (lo + hi) >> 1; if (ts[m] >= t) hi = m; else lo = m + 1; }
    const j = lo, near = i => Math.abs(ts[i] - t);
    if (j === 0) return near(0) <= snapMs ? 0 : -1;
    if (j === n) return near(n - 1) <= snapMs ? n - 1 : -1;
    const i = near(j) < near(j - 1) ? j : j - 1;
    return !brk[j] || near(i) <= snapMs ? i : -1;
  }
  function fmtSpan(ms) {
    const u = [[86400000, 'd'], [3600000, 'h'], [60000, 'min'], [1000, 's']];
    for (const [k, s] of u) if (ms >= k) { const v = ms / k; return (v >= 10 ? v.toFixed(0) : v.toFixed(1).replace(/\.0$/, '')) + ' ' + s; }
    return Math.round(ms) + ' ms';
  }
  function popIcon() {
    const s = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    s.setAttribute('viewBox', '0 0 16 16'); s.setAttribute('width', '14'); s.setAttribute('height', '14');
    s.innerHTML = '<path d="M9.5 2.5h4v4M13.5 2.5 8 8M6.5 3.5h-3v9h9v-3" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>';
    return s;
  }
  // rangeChart draws probes (line, lost bars) or buckets (avg line,
  // min–max band, loss% bars) on a real time axis: lines break at gaps,
  // bars sit at their time. o: {view, from, to, more, areaPx, onPop, latest, big}.
  function rangeChart(pts, range, o) {
    pts = pts || []; o = o || {};
    const probe = range === 'probe', now = Date.now();
    const { ts, brk, ax } = layoutTimes(pts, range, o.from, o.to, now);
    const view = o.view || newView();
    const [t0, t1] = vWindow(view, ax);
    let i0 = 0; while (i0 < ts.length && ts[i0] < t0 - ax.bucket) i0++;
    let i1 = i0; while (i1 < ts.length && ts[i1] <= t1 + ax.bucket) i1++;
    i0 = Math.max(0, i0 - 1); i1 = Math.min(ts.length, i1 + 1);
    const vis = [];
    for (let i = i0; i < i1; i++) vis.push(i);
    const lost = vis.reduce((a, i) => a + (ts[i] >= t0 && ts[i] <= t1 ? pts[i].lost || 0 : 0), 0);
    const top = niceCeil(Math.max(0, ...vis.filter(i => pts[i].ok).map(i => pts[i].max)) * 1.1) || 1;
    const areaPx = o.areaPx || 600;
    const pxPerMs = t1 > t0 ? areaPx / (t1 - t0) : 0;
    const barPx = Math.max(2, Math.min(CHART_MAX_BAR, ax.bucket * pxPerMs * (probe ? 0.6 : 0.7)));
    const wrap = H('div', { class: 'chart' + (o.big ? ' big' : '') });
    const head = H('div', { class: 'chart-head' }, H('b', { text: T('chart.title') }));
    if (ax.timed && vis.length) head.append(H('span', { class: 'chart-span', text: fmtRangeTime(t0, probe ? 'probe' : 'minute') + ' – ' + fmtRangeTime(t1, probe ? 'probe' : 'minute') + ' · ' + fmtSpan(t1 - t0) }));
    head.append(H('span', { class: 'sp' }));
    if (probe) { if (lost) head.append(H('span', { class: 'chart-lost', text: T('chart.lost', lost) })); }
    else head.append(H('span', { class: 'chart-legend', text: T('chart.legend.band') + ' · ' + T('chart.legend.loss') }));
    if (view.panned && o.latest) head.append(H('button', { class: 'btn sm chart-latest', type: 'button', title: T('chart.latest.tip'), onclick: o.latest }, T('chart.latest') + ' ⇥'));
    if (o.onPop) {
      const b = H('button', { class: 'chart-pop', type: 'button', title: T('chart.pop'), 'aria-label': T('chart.pop'), onclick: o.onPop });
      b.append(popIcon()); head.append(b);
    }
    const plot = H('div', { class: 'chart-plot' });
    const svg = sparkSvg((mk, W, Hh) => {
      const x = t => t1 > t0 ? ((t - t0) / (t1 - t0)) * W : W / 2, y = v => Hh - (v / top) * Hh;
      const bw = barPx * W / areaPx;
      const bar = (t, pct) => {
        const h = probe ? Hh : (pct / 100) * Hh;
        const x0 = Math.max(0, x(t) - bw / 2), x1 = Math.min(W, x(t) + bw / 2);
        if (x1 > x0) mk('rect', { x: x0, y: Hh - h, width: x1 - x0, height: h }, 'loss');
      };
      // columns of (avg, min, max, loss) when there are more points than pixels
      let cols = null;
      if (vis.length > 2 * areaPx) {
        const nc = Math.ceil(areaPx), cw = (t1 - t0) / nc;
        cols = new Array(nc).fill(null);
        for (const i of vis) {
          const c = Math.floor((ts[i] - t0) / cw);
          if (c < 0 || c >= nc) continue;
          const p = pts[i];
          let k = cols[c];
          if (!k) k = cols[c] = { t: t0 + (c + 0.5) * cw, s: 0, n: 0, mn: Infinity, mx: -Infinity, loss: 0, cut: false, ok: false };
          if (brk[i] && k.n) k.cut = true;
          k.loss = Math.max(k.loss, p.lossPct || 0);
          if (p.ok) { k.ok = true; k.s += p.avg; k.n++; k.mn = Math.min(k.mn, probe ? p.avg : p.min); k.mx = Math.max(k.mx, probe ? p.avg : p.max); }
        }
      }
      let line = '', band = '', run = [];
      const flush = () => {
        if (!run.length) return;
        const f = run[0];
        line += ` M${x(f.t).toFixed(1)} ${y(f.avg).toFixed(1)}` + run.slice(1).map(r => ` L${x(r.t).toFixed(1)} ${y(r.avg).toFixed(1)}`).join('');
        band += ` M${x(f.t).toFixed(1)} ${y(f.max).toFixed(1)}` + run.slice(1).map(r => ` L${x(r.t).toFixed(1)} ${y(r.max).toFixed(1)}`).join('');
        for (let k = run.length - 1; k >= 0; k--) band += ` L${x(run[k].t).toFixed(1)} ${(probe && !cols ? Hh : y(run[k].min)).toFixed(1)}`;
        band += ' Z';
        if (run.length === 1) {
          if (!probe) mk('line', { x1: x(f.t), x2: x(f.t), y1: y(f.min), y2: y(f.max), 'vector-effect': 'non-scaling-stroke' }, 'band-stem');
          mk('rect', { x: x(f.t) - 2, y: y(f.avg) - 2, width: 4, height: 4 }, 'dot');
        }
        run = [];
      };
      if (cols) {
        let prevT = null;
        cols.forEach(k => {
          if (!k) return;
          if (k.loss > 0) bar(k.t, k.loss);
          if (!k.ok || k.cut || (prevT != null && k.t - prevT > ax.gap)) flush();
          if (k.ok) { run.push({ t: k.t, avg: k.s / k.n, min: k.mn, max: k.mx }); prevT = k.t; } else prevT = null;
        });
      } else {
        for (const i of vis) {
          const p = pts[i];
          if (p.lossPct > 0) bar(ts[i], p.lossPct);
          if (!p.ok || brk[i]) flush();
          if (p.ok) run.push({ t: ts[i], avg: p.avg, min: probe ? p.avg : p.min, max: probe ? p.avg : p.max });
        }
      }
      flush();
      if (band) mk('path', { d: band }, 'a');
      if (line) mk('path', { d: line, 'vector-effect': 'non-scaling-stroke' }, 'l');
    });
    const yl = H('div', { class: 'chart-y' }, ...[1, 0.5, 0].map(f => H('span', { text: axisMs(top * f) })));
    const area = H('div', { class: 'chart-area' }, svg);
    plot.append(yl, area);
    if (!vis.length) plot.append(H('div', { class: 'chart-empty', text: T('chart.empty') }));
    // crosshair: snaps to the nearest real point by time; nothing in a gap
    const vx = H('div', { class: 'xh-v', hidden: true }), hy = H('div', { class: 'xh-h', hidden: true });
    const ylab = H('div', { class: 'xh-lab y', hidden: true }), xlab = H('div', { class: 'xh-lab x', hidden: true });
    const tip = H('div', { class: 'xh-tip', hidden: true }), ring = H('div', { class: 'xh-pt', hidden: true });
    area.append(vx, hy, ring, ylab, xlab, tip);
    const xticks = () => wrap.querySelectorAll('.chart-x span');
    const hide = () => { [vx, hy, ring, ylab, xlab, tip].forEach(e => { e.hidden = true; }); xticks().forEach(s => { s.style.visibility = ''; }); };
    area.addEventListener('mouseleave', hide);
    area.addEventListener('mousemove', e => {
      if (!vis.length || o.dragging && o.dragging()) { hide(); return; }
      const rc = area.getBoundingClientRect(), mx = e.clientX - rc.left, my = e.clientY - rc.top;
      if (mx < 0 || my < 0 || mx > rc.width || my > rc.height || t1 <= t0) { hide(); return; }
      const ppm = rc.width / (t1 - t0), t = t0 + mx / ppm;
      const i = chartSnap(ts, brk, t, Math.max(10, barPx / 2 + 4) / ppm);
      if (i < 0 || ts[i] < t0 || ts[i] > t1) { hide(); return; }
      const xi = (ts[i] - t0) * ppm;
      vx.style.left = xi + 'px'; hy.style.top = my + 'px';
      ylab.textContent = axisMs(top * (1 - my / rc.height)); ylab.style.top = (my - 8) + 'px';
      const p = pts[i];
      xlab.hidden = !p.at;
      if (p.at) {
        xlab.textContent = fmtRangeTime(p.at, range);
        const lw = xlab.offsetWidth || 60;
        xlab.style.left = Math.max(lw / 2, Math.min(rc.width - lw / 2, xi)) + 'px';
      }
      ring.hidden = !p.ok;
      if (p.ok) { ring.style.left = xi + 'px'; ring.style.top = (rc.height - (p.avg / top) * rc.height) + 'px'; }
      tip.replaceChildren(...tipLines(pts, i, range).map(([tt, k]) => k ? H('div', { class: 'row k-' + k }, H('i'), H('span', { text: tt })) : H('div', { class: 'time', text: tt })));
      vx.hidden = hy.hidden = ylab.hidden = tip.hidden = false;
      const xr = xlab.hidden ? null : xlab.getBoundingClientRect();
      xticks().forEach(s => {
        const b = s.getBoundingClientRect();
        s.style.visibility = xr && b.right + 4 > xr.left && b.left - 4 < xr.right ? 'hidden' : '';
      });
      const tw = tip.offsetWidth, th = tip.offsetHeight;
      tip.style.left = Math.max(0, xi + 14 + tw > rc.width ? xi - 14 - tw : xi + 14) + 'px';
      const above = my - th - 10;
      tip.style.top = Math.max(0, Math.min(rc.height - th, above >= 0 ? above : my + 10)) + 'px';
    });
    // x axis: start, end and a few ticks between
    const xa = H('div', { class: 'chart-x' });
    if (!ax.timed) {
      if (vis.length) xa.append(H('span', { text: T('chart.recent', vis.length) }), H('span', { text: T('chart.now') }));
    } else if (vis.length || o.from != null) {
      const k = Math.max(2, Math.min(6, Math.floor(areaPx / 150) + 1));
      const step = (t1 - t0) / (k - 1), fr = range === 'month' && step >= 28 * 86400000 ? 'month' : step >= 86400000 ? 'day' : step >= 60000 ? 'minute' : 'probe';
      for (let j = 0; j < k; j++) xa.append(H('span', { text: fmtRangeTime(t0 + (t1 - t0) * j / (k - 1), fr) }));
    }
    wrap.append(head, plot, xa);
    wrap._ax = ax;
    return wrap;
  }
  // RB holds each page's time filter; a chart's axis spans it.
  const RB = {};
  const refFilterKind = ref => ref.kind === 'hop' ? 'trace' : ref.kind === 'http' ? 'http' : 'ping';
  function refFilter(ref) {
    const rb = RB[refFilterKind(ref)];
    if (!rb || !rb.el.isConnected || !rb.active()) return [null, null];
    const b = rb.bounds() || [null, null];
    return [b[0] ? b[0].getTime() : null, b[1] ? b[1].getTime() : null];
  }
  // chartWidget: range selector + chart for a ping target, hop or HTTP
  // target; refresh(row) is called on every state update. Keys (with
  // focus or hover): ←/→ pan 10% (Shift: a page), ↑/↓ zoom, Home/End;
  // the wheel zooms at the pointer, dragging pans.
  function chartWidget(ref, opt) {
    opt = opt || {};
    let range = opt.range || 'probe', pts = null, ptsFor = '', loading = false, older = false, more = false, at = 0, lastRow = null;
    let view = newView(), viewFor = '', cur = null, hover = false, drag = null;
    const segs = H('div', { class: 'seg' });
    const host = H('div', { class: 'chart-nav', tabindex: '0', 'aria-label': T('chart.title') });
    RANGES.forEach(r => {
      const b = H('button', { class: 'seg-btn', type: 'button', text: T('chart.range.' + r), onclick: () => { range = r; pts = null; ptsFor = ''; refresh(lastRow, true); } });
      b.dataset.r = r; segs.append(b);
    });
    const wantKey = () => { const [f, t] = refFilter(ref); return range + '|' + (f || '') + '|' + (t || ''); };
    const areaPx = () => { const a = host.querySelector('.chart-area'); return (a && a.clientWidth) || Math.max(200, host.clientWidth - 62) || 600; };
    const ax = () => cur && cur._ax;
    function draw() {
      const want = wantKey();
      if (viewFor !== want) { view = newView(); viewFor = want; }
      const [f, t] = refFilter(ref);
      const o = { view, from: f, to: t, more, areaPx: areaPx(), onPop: opt.onPop ? () => opt.onPop(range) : null, latest: () => { view.panned = false; draw(); }, big: opt.big, dragging: () => !!drag };
      let el;
      if (ptsFor === want && pts) el = rangeChart(pts, range, o);
      else if (range === 'probe' && lastRow && lastRow.hist) el = rangeChart(histToPts(lastRow.hist), range, o);
      else el = H('div', { class: 'chart-empty static', text: T('chart.loading') });
      cur = el._ax ? el : null;
      host.replaceChildren(el);
      if (view.wantOlder) { view.wantOlder = false; loadOlder(); }
    }
    function moved() { draw(); }
    host.addEventListener('wheel', e => {
      const a = ax(), area = host.querySelector('.chart-area');
      if (!a || !area) return;
      e.preventDefault();
      const rc = area.getBoundingClientRect();
      if (Math.abs(e.deltaX) > Math.abs(e.deltaY)) vPan(view, a, e.deltaX / rc.width, more);
      else vZoom(view, a, Math.max(0.5, Math.min(2, Math.pow(1.002, e.deltaY))), (e.clientX - rc.left) / rc.width, more);
      moved();
    }, { passive: false });
    host.addEventListener('mousedown', e => {
      if (e.button !== 0 || !ax() || e.target.closest('button')) return;
      drag = { x: e.clientX }; host.focus({ preventScroll: true }); e.preventDefault();
    });
    window.addEventListener('mousemove', e => {
      if (!drag || !host.isConnected) return;
      const a = ax(), area = host.querySelector('.chart-area');
      if (!a || !area) return;
      vPan(view, a, -(e.clientX - drag.x) / area.clientWidth, more); drag.x = e.clientX; moved();
    });
    window.addEventListener('mouseup', () => { drag = null; });
    host.addEventListener('mouseenter', () => { hover = true; });
    host.addEventListener('mouseleave', () => { hover = false; });
    const onKey = e => {
      if (!host.isConnected) { document.removeEventListener('keydown', onKey); return; }
      const ae = document.activeElement;
      // only with focus, or hover while nothing else (an input, a table) has it
      if (!(ae === host || (hover && (!ae || ae === document.body)))) return;
      if (e.ctrlKey || e.metaKey || e.altKey) return;
      const a = ax();
      if (!a) return;
      const step = e.shiftKey ? 1 : 0.1;
      let anchor = 0.5;
      switch (e.key) {
        case 'ArrowLeft': vPan(view, a, -step, more); break;
        case 'ArrowRight': vPan(view, a, step, more); break;
        case 'ArrowUp': vZoom(view, a, 0.8, anchor, more); break;
        case 'ArrowDown': vZoom(view, a, 1.25, anchor, more); break;
        case 'Home': vSetEnd(view, a, -Infinity, more); break;
        case 'End': view.panned = false; break;
        default: return;
      }
      e.preventDefault(); e.stopPropagation(); moved();
    };
    document.addEventListener('keydown', onKey);
    function loadOlder() {
      if (older || !more || !pts || !pts.length || ptsFor !== wantKey()) return;
      older = true;
      const want = ptsFor, [f] = refFilter(ref);
      histQuery(ref, range, f, new Date(pts[0].at).getTime(), CHART_FETCH).then(j => {
        older = false;
        if (ptsFor !== want) return;
        const got = ((j.data && j.data.points) || []).filter(p => p.at < pts[0].at);
        more = got.length >= CHART_FETCH - 1;
        if (got.length) { pts = got.concat(pts); draw(); }
      }).catch(() => { older = false; });
    }
    function refresh(row, force) {
      if (row) lastRow = row;
      segs.querySelectorAll('.seg-btn').forEach(b => b.classList.toggle('on', b.dataset.r === range));
      const want = wantKey();
      const busy = !!host.querySelector('.xh-tip:not([hidden])') || drag;
      if (!pts || force || ptsFor !== want || (!view.panned && !busy)) draw();
      if (loading || (!force && ptsFor === want && Date.now() - at < (range === 'probe' ? 1000 : 5000))) return;
      loading = true; at = Date.now();
      const [f, t] = refFilter(ref);
      histQuery(ref, range, f, t, CHART_FETCH).then(j => {
        loading = false;
        if (wantKey() !== want) return;
        const fresh = (j.data && j.data.points) || [];
        if (ptsFor === want && pts && pts.length && fresh.length) {
          // keep the older pages, but none from before a stats reset
          const ra = (j.data && j.data.resetAt) || 0;
          pts = pts.filter(p => p.at < fresh[0].at && new Date(p.at).getTime() >= ra).concat(fresh);
        } else { pts = fresh; more = fresh.length >= CHART_FETCH; }
        ptsFor = want;
        if (!host.querySelector('.xh-tip:not([hidden])') && !drag) draw();
      }).catch(() => { loading = false; });
    }
    const right = H('div', { class: 'chart-tools' }, segs);
    return { el: H('div', null, right, host, opt.hint ? H('div', { class: 'chart-hint', text: T('chart.nav.hint') }) : null), refresh, range: () => range, focus: () => host.focus({ preventScroll: true }) };
  }
  function histQuery(ref, range, from, to, limit) {
    const q = new URLSearchParams({ range });
    if (ref.kind === 'hop') { q.set('tab', ref.tab); q.set('ttl', ref.ttl); }
    else if (ref.kind === 'http') q.set('http', ref.id);
    else q.set('id', ref.id);
    if (from != null) q.set('from', String(Math.floor(from)));
    if (to != null) q.set('to', String(Math.ceil(to)));
    if (limit) q.set('limit', String(limit));
    return fetch('/api/history?' + q, { credentials: 'same-origin', cache: 'no-store' }).then(r => r.json());
  }
  // openTrendView: the chart large, over the whole page (read-only: it
  // only reads history). Escape or ✕ closes it. rowOf(S) finds the row.
  let trendView = null;
  function closeTrendView() { if (trendView) { trendView.bg.remove(); trendView = null; } }
  function openTrendView(ref, title, range, rowOf) {
    closeTrendView();
    const chart = chartWidget(ref, { range, big: true, hint: true });
    const close = H('button', { class: 'win-x', type: 'button', onclick: closeTrendView, title: T('detail.close'), 'aria-label': T('detail.close') }, '✕');
    const box = H('div', { class: 'chart-full-box glass-pop' }, H('div', { class: 'gpane gpane-title' }, H('div', { class: 'chart-full-head' }, H('h2', { text: T('chart.trend_title', title) }), close)), H('div', { class: 'gpane grow' }, chart.el));
    const bg = H('div', { class: 'chart-full' }, box);
    document.body.append(bg);
    trendView = { bg, update: st => { const r = rowOf(st); if (!r) { closeTrendView(); return; } chart.refresh(r); } };
    if (S) trendView.update(S);
    setTimeout(() => chart.focus(), 30);
  }
  function histURL(ref, range, fmt) {
    const q = new URLSearchParams({ range });
    if (ref.kind === 'hop') { q.set('tab', ref.tab); q.set('ttl', ref.ttl); }
    else if (ref.kind === 'http') q.set('http', ref.id);
    else q.set('id', ref.id);
    return '/api/history.' + (fmt === 'txt' || fmt === 'md' ? fmt : 'csv') + '?' + q;
  }
  function histCSV(ref, range, fmt) { download(histURL(ref, range, fmt)); }
  // histExportButton: a detail window's 「导出」 menu (files, then copies)
  function histExportButton(ref, range) {
    const items = () => [
      { label: T('history.export.button'), run: () => histCSV(ref, range(), 'csv') },
      { label: T('history.export.txt'), run: () => histCSV(ref, range(), 'txt') },
      { label: T('history.export.md'), run: () => histCSV(ref, range(), 'md') }, '-',
      { label: T('copy.txt'), run: () => copyFrom(histURL(ref, range(), 'txt')) },
      { label: T('copy.md'), run: () => copyFrom(histURL(ref, range(), 'md')) },
    ];
    return H('button', { class: 'btn', onclick: e => { const r = e.currentTarget.getBoundingClientRect(); openMenu(r.left, r.bottom + 4, items()); } }, T('detail.export') + ' ▾');
  }
  // copyFrom puts the text at href (the same TXT as the export) on the
  // clipboard. Over plain http (no secure context) it falls back to a
  // hidden textarea + execCommand. Texts over 5 MB are refused.
  const COPY_MAX = 5 * 1024 * 1024;
  function copyFrom(href) {
    const done = () => toast(T('web.copied'));
    const fail = e => toast(e && e.ours ? e.message : T('copy.failed', String((e && e.message) || e)));
    const get = () => fetch(href, { credentials: 'same-origin', cache: 'no-store' }).then(async r => {
      const t = await r.text();
      if (!r.ok) { let m = t; try { m = JSON.parse(t).message || t; } catch (_) { /* text */ } const e = new Error(T('copy.failed', m)); e.ours = true; throw e; }
      const n = new Blob([t]).size;
      if (n > COPY_MAX) { const e = new Error(T('copy.too_large', (n / 1048576).toFixed(1) + ' MB')); e.ours = true; throw e; }
      return t;
    });
    const legacy = t => {
      const ta = H('textarea', { style: 'position:fixed;top:0;left:0;opacity:0' }); ta.value = t;
      document.body.append(ta); ta.focus(); ta.select();
      let ok = false;
      try { ok = document.execCommand('copy'); } catch (_) { ok = false; }
      ta.remove();
      if (ok) done(); else toast(T('copy.failed', 'clipboard'));
    };
    if (window.isSecureContext && navigator.clipboard) {
      // ClipboardItem with a promise keeps the click's permission (Safari)
      if (window.ClipboardItem && navigator.clipboard.write) {
        let fetched = null;
        const p = get().then(t => { fetched = t; return new Blob([t], { type: 'text/plain' }); });
        navigator.clipboard.write([new ClipboardItem({ 'text/plain': p })]).then(done, e => {
          if (e && e.ours) { fail(e); return; }
          p.then(() => navigator.clipboard.writeText(fetched).then(done, () => legacy(fetched)), fail);
        });
        return;
      }
      get().then(t => navigator.clipboard.writeText(t).then(done, () => legacy(t)), fail);
      return;
    }
    get().then(legacy, fail);
  }
  function download(href) {
    const a = H('a', { href, download: '' });
    document.body.append(a); a.click(); a.remove();
  }
  // tableExportButton: a Ping / HTTP / IP lookup table's 「导出 ▾」 menu.
  // The server builds the file from the table as shown here: columns in
  // table order, rows in table order (all, or only the selected ones),
  // the time filter's range; extra() adds query fields (a guest's lookup).
  function tableExportButton(kind, tbl, rb, extra) {
    const href = (fmt, sel) => {
      const rows = sel ? tbl.selected() : tbl.display();
      const q = new URLSearchParams({ format: fmt, lang: LANG || DEFAULT_LANG || '', cols: tbl.cols.map(c => c.id).join(','), keys: rows.map(r => r.id).join(',') });
      if (sel) q.set('selected', '1');
      if (tbl.sort && tbl.sort.col) { q.set('sort', tbl.sort.col); if (tbl.sort.desc) q.set('desc', '1'); }
      if (rb) for (const [k, v] of new URLSearchParams(rb.exportQuery())) q.set(k, v);
      if (extra) for (const [k, v] of Object.entries(extra() || {})) if (v) q.set(k, v);
      return '/api/' + kind + '/export?' + q;
    };
    const items = sel => [
      { label: T('history.export.button'), run: () => download(href('csv', sel) + '&download=1') },
      { label: T('history.export.txt'), run: () => download(href('txt', sel) + '&download=1') },
      { label: T('history.export.md'), run: () => download(href('md', sel) + '&download=1') }, '-',
      { label: T('copy.txt'), run: () => copyFrom(href('txt', sel)) },
      { label: T('copy.md'), run: () => copyFrom(href('md', sel)) },
    ];
    return H('button', { class: 'btn sm tbl-export', onclick: e => {
      const r = e.currentTarget.getBoundingClientRect(), total = tbl.display().length, n = tbl.selected().length;
      if (!total) { toast(T('chart.empty')); return; }
      openMenu(r.left, r.bottom + 4, n ? [{ label: T('table.export.all', total), sub: items(false) }, { label: T('table.export.selected', n), sub: items(true) }] : items(false));
    } }, T('detail.export') + ' ▾');
  }
  // trace export: one tab (id) or all tabs (id 0, a zip); txt, csv or md
  function traceExport(id, fmt) {
    const q = new URLSearchParams({ format: fmt, lang: LANG || DEFAULT_LANG || '' });
    if (id) q.set('tab', id); else q.set('all', '1');
    download('/api/trace/export?' + q);
  }
  // traceTextURL: the TXT or Markdown of one tab, or of all tabs in one text
  function traceTextURL(id, fmt) {
    const q = new URLSearchParams({ format: fmt || 'txt', lang: LANG || DEFAULT_LANG || '' });
    if (id) q.set('tab', id); else { q.set('all', '1'); q.set('join', '1'); }
    return '/api/trace/export?' + q;
  }
  function traceExportItems(id) {
    return [
      { label: T('trace.export.tab_txt'), disabled: !id, run: () => traceExport(id, 'txt') },
      { label: T('trace.export.tab_csv'), disabled: !id, run: () => traceExport(id, 'csv') },
      { label: T('trace.export.tab_md'), disabled: !id, run: () => traceExport(id, 'md') },
      { label: T('trace.copy.tab'), disabled: !id, run: () => copyFrom(traceTextURL(id, 'txt')) },
      { label: T('trace.copy.tab_md'), disabled: !id, run: () => copyFrom(traceTextURL(id, 'md')) }, '-',
      { label: T('trace.export.all_txt'), run: () => traceExport(0, 'txt') },
      { label: T('trace.export.all_csv'), run: () => traceExport(0, 'csv') },
      { label: T('trace.export.all_md'), run: () => traceExport(0, 'md') },
      { label: T('trace.copy.all'), run: () => copyFrom(traceTextURL(0, 'txt')) },
      { label: T('trace.copy.all_md'), run: () => copyFrom(traceTextURL(0, 'md')) },
    ];
  }

  // liveCtr / foldLive: same as the desktop's (internal/app/range_fold.go).
  const liveCtr = r => ({ s: r.success || 0, f: r.failure || 0, sum: (r.avg || 0) * (r.success || 0), last: r.last || 0, lastOk: r.lastOk || 0, lastFail: r.lastFail || 0 });
  function foldLive(v, b, c) {
    const dS = c.s - b.s, dF = c.f - b.f;
    if (dS < 0 || dF < 0 || dS + dF === 0) return v;
    v = Object.assign({}, v);
    if (dS > 0) {
      const sum = v.avg * v.success + (c.sum - b.sum);
      if (!v.success) { v.min = v.max = v.median = c.last; } else { v.min = Math.min(v.min, c.last); v.max = Math.max(v.max, c.last); }
      v.success += dS; v.avg = Math.max(0, sum / v.success); v.last = c.last; v.lastOk = Math.max(v.lastOk || 0, c.lastOk);
    }
    v.failure += dF;
    if (dF > 0) v.lastFail = Math.max(v.lastFail || 0, c.lastFail);
    const n = v.success + v.failure;
    v.successRate = v.success * 100 / n; v.lossRate = v.failure * 100 / n;
    return v;
  }
  // ---------- time range filter (per browser tab, never saved on the server) ----------
  const RANGE_PRESETS = ['all', 'today', 'yesterday', 'thisweek', 'lastweek', 'thismonth', 'lastmonth', 'thisyear', 'lastyear', 'custom'];
  // rangeBounds: [from, to) in this browser's time zone; weeks start on Monday
  function rangeBounds(p, now) {
    const y = now.getFullYear(), m = now.getMonth(), d = now.getDate();
    const back = (now.getDay() + 6) % 7;
    switch (p) {
      case 'today': return [new Date(y, m, d), new Date(y, m, d + 1)];
      case 'yesterday': return [new Date(y, m, d - 1), new Date(y, m, d)];
      case 'thisweek': return [new Date(y, m, d - back), new Date(y, m, d - back + 7)];
      case 'lastweek': return [new Date(y, m, d - back - 7), new Date(y, m, d - back)];
      case 'thismonth': return [new Date(y, m, 1), new Date(y, m + 1, 1)];
      case 'lastmonth': return [new Date(y, m - 1, 1), new Date(y, m, 1)];
      case 'thisyear': return [new Date(y, 0, 1), new Date(y + 1, 0, 1)];
      case 'lastyear': return [new Date(y - 1, 0, 1), new Date(y, 0, 1)];
    }
    return null;
  }
  const pad2 = n => String(n).padStart(2, '0');
  const localInput = t => t.getFullYear() + '-' + pad2(t.getMonth() + 1) + '-' + pad2(t.getDate()) + 'T' + pad2(t.getHours()) + ':' + pad2(t.getMinutes());
  const fmtMinute = t => localInput(t).replace('T', ' ');
  // rangeBar: quick picks + custom range; onChange() re-renders the table.
  // kind: ping | http | trace; tabId() gives the trace tab.
  function rangeBar(kind, onChange, tabId) {
    const key = 'ht.range.' + kind;
    let st = { p: 'all' };
    try { st = JSON.parse(sessionStorage.getItem(key) || 'null') || st; } catch (_) { /* default */ }
    let res = null, resFor = '', busy = false, at = 0, err = '';
    const sel = H('select', { 'aria-label': T('range.label') }, RANGE_PRESETS.map(p => H('option', { value: p, text: T('range.' + p) })));
    const fromI = H('input', { type: 'datetime-local', step: '60' }), toI = H('input', { type: 'datetime-local', step: '60' });
    const apply = H('button', { class: 'btn sm primary' }, T('range.apply'));
    const cancel = H('button', { class: 'btn sm' }, T('range.cancel'));
    const editor = H('div', { class: 'row range-edit' }, H('span', { class: 'desc', text: T('range.from') }), fromI, H('span', { class: 'desc', text: T('range.to') }), toI, apply, cancel);
    const badge = H('span', { class: 'badge' }), clear = H('button', { class: 'btn sm' }, T('range.clear'));
    const note = H('div', { class: 'status' });
    const el = H('div', { class: 'range-bar' }, H('div', { class: 'row' }, H('span', { class: 'desc', text: T('range.label') }), sel, badge, clear), editor, note);
    editor.hidden = true;
    const save = () => sessionStorage.setItem(key, JSON.stringify(st));
    const bounds = () => {
      if (st.p === 'custom') return [st.from ? new Date(st.from) : null, st.to ? new Date(st.to) : null];
      return rangeBounds(st.p, new Date());
    };
    const active = () => st.p !== 'all' && !!bounds();
    const historyOff = () => kind !== 'trace' && S && !S.settings.historySave;
    function openEditor() {
      const b = bounds() || rangeBounds('today', new Date());
      fromI.value = localInput(b[0] || rangeBounds('today', new Date())[0]);
      toI.value = localInput(b[1] || new Date());
      editor.hidden = false;
    }
    sel.onchange = () => {
      if (sel.value === 'custom') { openEditor(); return; }
      st = { p: sel.value }; editor.hidden = true; save(); refresh(true);
    };
    apply.onclick = () => {
      const f = new Date(fromI.value), t = new Date(toI.value);
      if (!(t > f)) { toast(T('range.err.order')); return; }
      st = { p: 'custom', from: f.toISOString(), to: t.toISOString() }; editor.hidden = true; save(); refresh(true);
    };
    cancel.onclick = () => { editor.hidden = true; paint(); };
    clear.onclick = () => { st = { p: 'all' }; editor.hidden = true; save(); refresh(true); };
    function query() {
      const b = bounds(), q = new URLSearchParams({ kind });
      if (b[0]) q.set('from', b[0].toISOString());
      if (b[1]) q.set('to', b[1].toISOString());
      if (kind === 'trace' && tabId) q.set('tab', tabId() || 0);
      return q.toString();
    }
    function paint() {
      const off = historyOff();
      sel.disabled = off;
      sel.value = editor.hidden ? (off ? 'all' : st.p) : 'custom';
      const on = active() && !off;
      badge.hidden = clear.hidden = !on;
      if (on) { const b = bounds(); badge.textContent = T('range.active', (b[0] ? fmtMinute(b[0]) : T('range.open_start')) + ' – ' + (b[1] ? fmtMinute(b[1]) : T('range.now'))); }
      let n = '', cls = 'status';
      if (off) n = T('range.need_history');
      else if (on && err) { n = T('range.failed', err); cls = 'status err'; }
      else if (on && kind === 'trace') n = T('range.trace_note');
      else if (on && res && ['ping', 'http'].some(k => Object.values(res[k] || {}).some(x => x.approx))) n = T('range.approx');
      else if (on) n = T('range.note');
      note.textContent = n; note.className = cls;
    }
    async function refresh(force) {
      paint();
      if (!active() || historyOff()) { if (res) { res = null; onChange(); } return; }
      const q = query();
      const b = bounds(), live = !b[1] || b[1] > new Date();
      if (busy || !force && q === resFor && (!live || Date.now() - at < 5000)) return;
      busy = true; at = Date.now();
      try {
        const r = await fetch('/api/stats/range?' + q, { credentials: 'same-origin', cache: 'no-store' }).then(x => x.json());
        if (!r.ok) throw new Error(r.message || 'error');
        res = r.data; resFor = q; err = ''; base = new Map();
      } catch (e) { err = String(e.message || e); res = null; resFor = q; }
      busy = false;
      paint(); onChange();
    }
    const timer = setInterval(() => { if (el.isConnected) refresh(false); else clearInterval(timer); }, 5000);
    // overlay: copy rows with the range's stats (same field names)
    // While the range reaches now, the probes made since a row first met
    // this result are folded in (foldLive), so the table moves with every
    // probe; the next query (5 s) brings exact values.
    let base = new Map();
    function overlay(rows, pick) {
      if (!res || !res.active || !active() || historyOff() || resFor !== query()) return rows;
      const b = bounds(), live = !b || !b[1] || b[1] > new Date();
      return rows.map(r => {
        let v = pick(res, r);
        if (!v) return r;
        const key = r.ttl != null ? (tabId ? tabId() : 0) + ':' + r.ttl : String(r.id), cur = liveCtr(r);
        if (!base.has(key)) base.set(key, cur);
        else if (live) {
          v = foldLive(v, base.get(key), cur);
          if ('checks' in v) v.checks = v.success + v.failure;
          if ('rtt' in v) v.rtt = ms(v.last, v.success > 0);
        }
        return Object.assign({}, r, v);
      });
    }
    paint();
    // exportQuery: the from/to of the filter while it applies, else ''
    const exportQuery = () => {
      if (!active() || historyOff()) return '';
      const q = new URLSearchParams(query()); q.delete('kind'); q.delete('tab');
      return q.toString();
    };
    const rb = { el, refresh, overlay, active, exportQuery, bounds };
    RB[kind] = rb;
    return rb;
  }

  // ---------- app state ----------
  let S = null;
  let page = (location.hash || '').replace('#', '') || localStorage.getItem('ht.page') || 'ping';
  let cur = null; // current page controller
  const NAV = [
    ['ping', 'nav.ping', 'M3 12h4l3-8 4 16 3-8h4'],
    ['http', 'nav.http', 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM2 12h20M12 2a15 15 0 0 1 0 20M12 2a15 15 0 0 0 0 20'],
    ['trace', 'nav.trace', 'M5 19a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM19 9a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM7 16l10-8'],
    ['iplookup', 'nav.iplookup', 'M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16zM21 21l-4.3-4.3'],
    ['settings', 'nav.settings', 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z'],
    ['about', 'nav.about', 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM12 16v-4M12 8h.01'],
  ];
  const TOP = {
    ping: ['top.ping.title', 'top.ping.sub'], http: ['top.http.title', 'top.http.sub'], trace: ['top.trace.title', 'top.trace.sub'],
    iplookup: ['top.lookup.title', 'top.lookup.sub'], settings: ['top.settings.title', 'top.settings.sub'],
    about: ['top.about.title', 'top.about.sub'],
  };
  if (!TOP[page]) page = 'ping';

  function buildNav() {
    const nav = $('#nav'); nav.replaceChildren();
    const foot = $('#footIcons'); if (foot) foot.replaceChildren();
    NAV.forEach(([id, key, d]) => {
      const svg = `<svg viewBox="0 0 24 24"><path d="${d}"/></svg>`;
      if (foot && (id === 'settings' || id === 'about')) { // bottom-left icon buttons
        const b = H('button', { type: 'button', class: 'foot-icon' + (id === page ? ' on' : ''), title: T(key), 'aria-label': T(key), onclick: () => go(id) });
        b.innerHTML = svg; foot.append(b); return;
      }
      const a = H('a', { class: id === page ? 'on' : '', onclick: () => go(id) });
      a.innerHTML = svg; a.append(T(key));
      nav.append(a);
    });
  }
  function go(id) {
    page = id; localStorage.setItem('ht.page', id);
    history.replaceState(null, '', '#' + id);
    $('#side').classList.remove('open');
    mount();
  }
  function mount() {
    buildNav();
    closeMenu();
    const [t, s] = TOP[page];
    $('#topTitle').textContent = T(t); $('#topSub').textContent = T(s);
    document.title = T(t) + ' · HinaTracer';
    const c = $('#content'); c.replaceChildren(); c.scrollTop = 0;
    cur = PAGES[page](c);
    if (S) update();
  }
  function update() {
    if (!S) return;
    const root = document.documentElement;
    applyLook(); void root;
    $('#ver').textContent = 'v' + S.version;
    const b = $('#topBadges'); b.replaceChildren();
    if (GUEST) b.append(H('span', { class: 'badge guest', title: T('web.guest.tip') }, T('web.badge.guest')));
    if (S.web.headless) b.append(H('span', { class: 'badge muted', text: T('web.badge.headless') }));
    if (page === 'http' && S.http.running) b.append(H('span', { class: 'badge ok' }, H('span', { class: 'dot' }), T('badge.monitoring')));
    if (page === 'ping' && S.ping.running) b.append(H('span', { class: 'badge ok' }, H('span', { class: 'dot' }), T('badge.monitoring')));
    if (page === 'trace' && S.trace.tabs.some(t => t.running)) b.append(H('span', { class: 'badge' }, H('span', { class: 'dot' }), T('badge.tracing')));
    updBanner();
    if (cur && cur.update) cur.update(S);
    if (modal && modal.update) modal.update(S);
    if (trendView) trendView.update(S);
  }

  // ---------- update banner ----------
  let bannerDismissed = '';
  function updBanner() {
    const el = $('#updBanner'), u = S.update;
    if (u.phase !== 'available' && u.phase !== 'downloading' && u.phase !== 'ready' || bannerDismissed === u.latest) { el.hidden = true; return; }
    const sig = u.phase + u.latest + u.text;
    if (el._sig === sig && !el.hidden) return;
    el._sig = sig; el.hidden = false;
    el.replaceChildren(H('b', { text: T('update.title', u.latest) }), u.pre ? H('span', { class: 'badge', text: T('update.prerelease') }) : null,
      u.phase !== 'available' ? H('span', { class: 'status', text: u.text }) : null, H('span', { class: 'sp' }),
      u.url ? H('a', { class: 'btn sm', href: u.url, target: '_blank', rel: 'noopener' }, T('update.open_page')) : null,
      u.phase === 'available' && u.canInstall ? H('button', { class: 'btn sm primary', onclick: () => api('update/install') }, T('update.now')) : null,
      u.phase === 'available' ? H('button', { class: 'btn sm', onclick: () => api('update/skip') }, T('update.skip')) : null,
      H('button', { class: 'btn sm', onclick: () => { bannerDismissed = u.latest; el.hidden = true; } }, T('update.later')));
  }

  // ---------- detail modal (ping target or hop) ----------
  function openDetail(ref) {
    openModal(box => {
      const title = H('h2'), stats = H('div', { class: 'stats' }), chartBox = H('div'), kv = H('div', { class: 'kv' });
      const actions = H('div', { class: 'actions' });
      const rowOf = st => {
        if (ref.kind !== 'hop') return st.ping.rows.find(x => x.id === ref.id);
        const tb = st.trace.tabs.find(t => t.id === ref.tab);
        return tb && tb.hops.find(h => h.ttl === ref.ttl);
      };
      const chart = chartWidget(ref, { onPop: rng => {
        const r = rowOf(S);
        if (r) openTrendView(ref, ref.kind === 'hop' ? 'TTL ' + r.ttl + ' · ' + r.addr : (r.alias || r.host), rng, rowOf);
      } });
      chartBox.append(chart.el);
      box.classList.add('glass-pop');
      const xBtn = H('button', { class: 'win-x', type: 'button', title: T('detail.close'), 'aria-label': T('detail.close'), onclick: closeModal }, '✕');
      const pane = (...k) => H('div', { class: 'gpane' }, ...k);
      box.append(pane(H('div', { class: 'gpane-head' }, title, H('span', { class: 'sp' }), xBtn), stats), pane(chartBox), pane(kv), pane(actions));
      let text = '', host = '', lastSig = '';
      actions.append(
        histExportButton(ref, () => chart.range()),
        H('button', { class: 'btn', onclick: () => copy(text) }, T('detail.copy_all')),
        H('button', { class: 'btn', onclick: () => { if (host) { api('ping/trace', { hosts: [host] }); closeModal(); go('trace'); } } }, T('detail.traceroute')),
        H('button', { class: 'btn primary', onclick: closeModal }, T('detail.close')));
      return st => {
        let r, isHop = ref.kind === 'hop', tab;
        if (isHop) { tab = st.trace.tabs.find(t => t.id === ref.tab); r = tab && tab.hops.find(h => h.ttl === ref.ttl); }
        else r = st.ping.rows.find(x => x.id === ref.id);
        if (!r) return;
        chart.refresh(r);
        const sig = JSON.stringify(r);
        if (sig === lastSig) return;
        lastSig = sig;
        title.textContent = isHop ? T('detail.hop_title') : T('detail.title');
        host = isHop ? (r.addr !== '*' ? r.addr : '') : r.host;
        const ok = r.success > 0;
        const items = [
          ['detail.last', ms(r.last, ok)], ['detail.avg', ms(r.avg, ok)], ['detail.min', ms(r.min, ok)], ['detail.max', ms(r.max, ok)],
          ['detail.median', ms(r.median, ok)], ['detail.loss', r.success + r.failure ? pct(r.lossRate) : '—'],
          ['detail.success', r.success], ['detail.failure', r.failure],
        ];
        stats.replaceChildren(...items.map(([k, v]) => H('div', { class: 'stat' }, H('div', { class: 'k', text: T(k) }), H('div', { class: 'v', text: v }))));
        const pairs = isHop ? [
          ['detail.ttl', r.ttl], ['detail.ip', r.addr], ['detail.rdns', r.rdns], ['detail.location', r.loc],
          ['detail.region', r.region], ['detail.asn', r.asn], ['detail.asn_region', r.asnRegion],
          ['detail.last_ok', fmtTime(r.lastOk)], ['detail.last_fail', fmtTime(r.lastFail)], ['detail.samples', r.samples],
        ] : [
          ['detail.alias', r.alias], ['detail.host', r.host], ['detail.ip', r.ip], ['detail.proto', r.proto],
          ['detail.rdns', r.rdns], ['detail.location', r.loc], ['detail.region', r.region], ['detail.asn', r.asn],
          ['detail.asn_region', r.asnRegion], ['detail.rate', r.success + r.failure ? pct(r.successRate) : '—'],
          ['detail.last_ok', fmtTime(r.lastOk)], ['detail.last_fail', fmtTime(r.lastFail)], ['detail.samples', r.samples],
        ];
        kv.replaceChildren(...pairs.flatMap(([k, v]) => {
          const val = H('div');
          if (k === 'detail.region' || k === 'detail.asn_region') val.append(flagCell(k === 'detail.region' ? r.iso : r.asnIso, v)[1]());
          else val.textContent = dash(v);
          return [H('div', { text: T(k) }), val];
        }));
        if (!isHop && !r.enabled) title.append(' ', H('span', { class: 'badge muted', text: T('badge.disabled') }));
        text = [...items, ...pairs].map(([k, v]) => T(k) + ': ' + dash(v)).join('\n');
      };
    }, true);
    modal.update(S);
  }

  // ---------- HTTP page ----------
  const httpSort = JSON.parse(localStorage.getItem('ht.httpSort') || 'null') || { col: '#', desc: false };
  function httpState(r) {
    if (!r.enabled) return ['muted', T('badge.disabled')];
    if (!r.checks) return ['muted', T('http.state.pending')];
    return r.ok ? ['ok', T('http.state.up')] : ['bad', T('http.state.down')];
  }
  function httpDel(ids) {
    if (!ids.length) return;
    confirmBox(T('ping.delete_sel'), T('web.confirm_delete', ids.length), T('http.ctx.delete'), () => api('http/delete', { ids }), true);
  }
  function httpEdit(r) {
    const s = r.spec || {};
    openModal(box => {
      const f = {};
      const inp = (k, v, ph) => (f[k] = H('input', { type: 'text', class: 'w-full', value: v == null ? '' : String(v), placeholder: ph || '' }));
      const num = (k, v, min, max, ph) => (f[k] = H('input', { type: 'number', class: 'w-num', value: String(v), min, max, placeholder: ph }));
      const chk = (k, v, label) => H('label', { class: 'chk' }, (f[k] = H('input', { type: 'checkbox', checked: v })), ' ', label);
      const METHODS = ['GET', 'HEAD', 'POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS'];
      const method = H('select', null, ...METHODS.map(m => H('option', { value: m, text: m })), H('option', { value: '', text: T('http.method_custom') }));
      const cur = (s.method || 'GET').toUpperCase();
      method.value = METHODS.includes(cur) ? cur : '';
      const custom = H('input', { type: 'text', class: 'w-alias', placeholder: T('ph.http.custom_method'), value: METHODS.includes(cur) ? '' : cur });
      const methodName = () => (method.value || custom.value.trim().toUpperCase());
      const hasBody = () => !['GET', 'HEAD', 'OPTIONS', ''].includes(methodName());
      const body = H('textarea', { rows: '3', class: 'w-full', placeholder: T('ph.http.body') }); body.value = s.body || '';
      const ctype = H('input', { type: 'text', class: 'w-full', placeholder: T('ph.http.content_type'), value: s.contentType || '' });
      const headers = H('textarea', { rows: '3', class: 'w-full', placeholder: T('ph.http.headers') }); headers.value = s.headers || '';
      const bodyField = H('div', null, H('div', { class: 'field' }, H('label', { text: T('http.body') }), body),
        H('div', { class: 'field' }, H('label', { text: T('http.content_type') }), ctype), H('div', { class: 'desc', text: T('http.body_hint') }));
      const syncBody = () => { custom.hidden = method.value !== ''; bodyField.hidden = !hasBody(); };
      method.addEventListener('change', syncBody); custom.addEventListener('input', syncBody); syncBody();
      const field = (k, el) => H('div', { class: 'field' }, H('label', { text: T(k) }), el);
      box.append(H('h2', { text: T('http.edit.title') }),
        field('ping.alias', inp('alias', r.alias, T('ph.http.alias'))), field('http.url', inp('url', s.url, T('ph.http.url'))),
        H('div', { class: 'row' }, field('http.method', H('div', { class: 'row' }, method, custom)),
          field('http.interval', num('interval', s.interval || 30, 1, 86400, T('ph.http.interval'))), field('http.timeout', num('timeout', s.timeout || 10, 1, 120, T('ph.http.timeout')))),
        bodyField,
        field('http.expect', inp('expect', s.expect, T('ph.http.expect'))),
        field('http.match', inp('match', s.match, T('ph.http.match'))), H('div', { class: 'desc', text: T('http.match_hint') }),
        H('div', { class: 'row' }, chk('regex', s.matchRegex, T('http.match_regex')), chk('invert', s.matchInvert, T('http.match_invert'))),
        field('http.headers', headers),
        H('div', { class: 'row' }, chk('follow', !s.noFollow, T('http.follow')), chk('insecure', s.insecure, T('http.insecure'))),
        H('div', { class: 'actions' }, H('button', { class: 'btn', onclick: closeModal }, T('alias.cancel')),
          H('button', { class: 'btn primary', onclick: async () => {
            const hb = hasBody();
            const spec = { url: f.url.value.trim(), method: methodName(), body: hb ? body.value : '', contentType: hb ? ctype.value.trim() : '', interval: parseFloat(f.interval.value) || 30,
              timeout: parseFloat(f.timeout.value) || 10, expect: f.expect.value.trim(), match: f.match.value,
              matchRegex: f.regex.checked, matchInvert: f.invert.checked, headers: headers.value, noFollow: !f.follow.checked, insecure: f.insecure.checked };
            const res = await api('http/update', { id: r.id, alias: f.alias.value, spec });
            if (res.ok) closeModal(); else if (res.message) toast(res.message);
          } }, T('alias.ok'))));
    }, true);
  }
  function httpBatch() {
    openModal(box => {
      const ta = H('textarea', { rows: '10', class: 'w-full', placeholder: T('ph.http.batch') });
      box.append(H('h2', { text: T('http.batch.title') }), H('div', { class: 'desc', text: T('http.batch.desc') }), ta,
        H('div', { class: 'actions' }, H('button', { class: 'btn', onclick: closeModal }, T('import.cancel')),
          H('button', { class: 'btn primary', onclick: async () => { const r = await api('http/addmany', { text: ta.value }); closeModal(); if (r.message) toast(r.message); } }, T('import.submit'))));
    });
  }
  function openHTTPDetail(id) {
    const ref = { kind: 'http', id };
    openModal(box => {
      const title = H('h2'), url = H('div', { class: 'desc mono' }), stats = H('div', { class: 'stats' }), kv = H('div', { class: 'kv' }), err = H('div', { class: 'status bad' });
      const rowOf = st => st.http.rows.find(x => x.id === id);
      const chart = chartWidget(ref, { onPop: rng => { const r = rowOf(S); if (r) openTrendView(ref, r.alias || r.spec.url, rng, rowOf); } });
      box.append(title, url, stats, chart.el, kv, err, H('div', { class: 'actions' },
        histExportButton(ref, () => chart.range()),
        H('button', { class: 'btn primary', onclick: closeModal }, T('detail.close'))));
      let lastSig = '';
      return st => {
        const r = st.http.rows.find(x => x.id === id);
        if (!r) return;
        chart.refresh(r);
        const sig = JSON.stringify(r);
        if (sig === lastSig) return;
        lastSig = sig;
        const [cls, label] = httpState(r);
        title.replaceChildren(r.alias || r.spec.url, ' ', H('span', { class: 'badge ' + cls, text: label }));
        url.textContent = r.spec.url;
        const has = r.success > 0, chk = r.checks > 0;
        stats.replaceChildren(...[
          ['http.col.code', r.code || '—'], ['ping.col.last', ms(r.last, chk && r.ok)], ['ping.col.avg', ms(r.avg, has)],
          ['ping.col.min', ms(r.min, has)], ['ping.col.max', ms(r.max, has)], ['ping.col.median', ms(r.median, has)],
          ['ping.col.rate', chk ? pct(r.successRate) : '—'], ['ping.col.loss', chk ? pct(r.lossRate) : '—'], ['ping.col.failure', r.failure],
        ].map(([k, v]) => H('div', { class: 'stat' }, H('div', { class: 'k', text: T(k) }), H('div', { class: 'v', text: String(v) }))));
        const https = /^https:/i.test(r.spec.url);
        const pairs = [
          ['DNS', ms(r.dnsMs, chk)], [T('http.t.connect'), ms(r.connectMs, chk)], ['TLS', ms(r.tlsMs, r.tlsMs > 0)], ['TTFB', ms(r.ttfbMs, r.ttfbMs > 0)],
          [T('http.cert'), https && chk && r.certDays >= 0 ? T('http.cert_days', r.certDays) : '—'],
          [T('ping.col.ip'), dash(r.ip)], [T('ping.col.location'), dash(r.loc)], [T('ping.col.region'), null],
          [T('ping.col.asn'), dash(r.asn)], [T('ping.col.asn_region'), dash(r.asnRegion)], [T('http.col.checked'), fmtTime(r.lastCheck)],
        ];
        kv.replaceChildren(...pairs.flatMap(([k, v]) => [H('div', { text: k }), v === null ? flagCell(r.iso, r.region)[1]() : H('div', { text: v })]));
        err.textContent = r.err || ''; err.hidden = !r.err;
      };
    }, true);
    modal.update(S);
  }
  function httpPage(root) {
    const url = H('input', { type: 'text', class: 'w-host', placeholder: T('ph.http.url') });
    const alias = H('input', { type: 'text', class: 'w-alias', placeholder: T('ph.http.alias') });
    const add = async () => {
      const r = await api('http/add', { url: url.value, alias: alias.value });
      if (r.ok) { url.value = ''; alias.value = ''; url.focus(); }
      if (r.message) toast(r.message);
    };
    [url, alias].forEach(i => i.addEventListener('keydown', e => { if (e.key === 'Enter') add(); }));
    const startBtn = H('button', { class: 'btn primary', onclick: () => api(S.http.running ? 'http/stop' : 'http/start') });
    const delBtn = H('button', { class: 'btn danger', onclick: () => httpDel(tbl.selected().map(r => r.id)) }, T('ping.delete_sel'));
    const status = H('div', { class: 'status' });
    root.append(H('section', { class: 'card' },
      W(H('div', { class: 'row end' },
        H('div', { class: 'field' }, H('label', { text: T('http.url') }), url),
        H('div', { class: 'field' }, H('label', { text: T('ping.alias') }), alias),
        H('button', { class: 'btn primary', onclick: add }, T('http.add')),
        H('button', { class: 'btn', onclick: httpBatch }, T('http.batch')))),
      W(H('div', { class: 'row' }, startBtn, H('button', { class: 'btn', onclick: () => resetStats('http', [], S.http.rows.length) }, T('ping.reset')), delBtn)),
      status));
    const C = (id, key, cell, sort, extra) => Object.assign({ id, label: key.includes('.') ? T(key) : key, cell, sort }, extra || {});
    const has = r => r.success > 0;
    const rb = rangeBar('http', () => update());
    const tbl = Table({ id: 'http',
      key: r => r.id, check: true, defaultSort: '#', sortState: httpSort, empty: T('http.empty'),
      onSort: s => localStorage.setItem('ht.httpSort', JSON.stringify(s)),
      rowClass: r => (!r.enabled ? 'off' : r.failing ? 'bad' : ''),
      onOpen: r => openHTTPDetail(r.id),
      onMenu: (e, r, sel) => openMenu(e.clientX, e.clientY, sel.length > 1 ? multi(sel) : one(r)),
      onSelect: () => { delBtn.disabled = tbl.selected().length === 0; },
      cols: [
        C('#', 'ping.col.n', r => String(S.http.rows.findIndex(x => x.id === r.id) + 1), null, { num: true }),
        C('alias', 'ping.col.alias', r => dash(r.alias), r => r.alias.toLowerCase()),
        C('url', 'URL', r => r.spec.url, r => r.spec.url.toLowerCase(), { mono: true }),
        C('status', 'http.col.status', r => { const [c, l] = httpState(r); return [c + l, () => H('span', { class: 'badge ' + c, text: l })]; }, r => (r.failing ? 2 : 0) + (r.checks ? 0 : 1), { copy: r => httpState(r)[1] }),
        C('code', 'http.col.code', r => (r.code ? String(r.code) : '—'), r => r.code || null, { num: true }),
        C('ip', 'ping.col.ip', r => dash(r.ip), r => ipKey(r.ip), { mono: true }),
        C('location', 'ping.col.location', r => dash(r.loc), r => (placeholder(r.loc) ? '' : r.loc)),
        C('region', 'ping.col.region', r => flagCell(r.iso, r.region), r => (placeholder(r.region) ? '' : r.region)),
        C('asn', 'ping.col.asn', r => dash(r.asn), r => (placeholder(r.asn) ? '' : r.asn)),
        C('asn_region', 'ping.col.asn_region', r => flagCell(r.asnIso, r.asnRegion), r => (placeholder(r.asnRegion) ? '' : r.asnRegion)),
        C('success', 'ping.col.success', r => String(r.success), r => r.success, { num: true }),
        C('failure', 'ping.col.failure', r => String(r.failure), r => r.failure, { num: true }),
        C('success_rate', 'ping.col.rate', r => (r.checks ? pct(r.successRate) : '—'), r => r.successRate, { num: true }),
        C('loss_rate', 'ping.col.loss', r => lossCell(r.lossRate, r.checks > 0), r => r.lossRate, { num: true, copy: r => (r.checks > 0 ? pct(r.lossRate) : '') }),
        C('last', 'ping.col.last', r => ms(r.last, r.ranged ? has(r) : r.checks > 0 && r.ok), r => (r.checks ? r.last : null), { num: true }),
        C('avg', 'ping.col.avg', r => ms(r.avg, has(r)), r => (has(r) ? r.avg : null), { num: true }),
        C('min', 'ping.col.min', r => ms(r.min, has(r)), r => (has(r) ? r.min : null), { num: true }),
        C('max', 'ping.col.max', r => ms(r.max, has(r)), r => (has(r) ? r.max : null), { num: true }),
        C('median', 'ping.col.median', r => ms(r.median, has(r)), r => (has(r) ? r.median : null), { num: true }),
        C('checked', 'http.col.checked', r => fmtTime(r.lastCheck), r => r.lastCheck || null),
        C('last_ok', 'ping.col.last_ok', r => fmtTime(r.lastOk), r => r.lastOk || null),
        C('last_fail', 'ping.col.last_fail', r => fmtTime(r.lastFail), r => r.lastFail || null),
      ],
    });
    root.append(H('section', { class: 'card grow' },
      H('div', { class: 'row' }, H('h3', { text: T('http.list_label') }), H('span', { class: 'desc', text: T('web.table_hint') }), H('span', { class: 'sp' }), tableExportButton('http', tbl, rb)), rb.el, tbl.el));
    function one(r) {
      const byOrder = httpSort.col === '#' && !httpSort.desc;
      const i = S.http.rows.findIndex(x => x.id === r.id);
      return [
        { label: T('http.ctx.detail'), run: () => openHTTPDetail(r.id) },
        { label: T('http.ctx.edit'), run: () => httpEdit(r) }, '-',
        { label: T('http.ctx.copy_url'), run: () => copy(r.spec.url) },
        { label: T('ping.ctx.copy_alias'), run: () => copy(r.alias), disabled: !r.alias },
        { label: T('ping.ctx.copy_ip'), run: () => copy(r.ip), disabled: !r.ip },
        { label: T('ping.ctx.copy_location'), run: () => copy(r.loc), disabled: placeholder(r.loc) },
        { label: T('ping.ctx.copy_region'), run: () => copy(r.region), disabled: placeholder(r.region) },
        { label: T('ping.ctx.copy_asn'), run: () => copy(r.asn), disabled: placeholder(r.asn) },
        { label: T('ping.ctx.copy_asn_region'), run: () => copy(r.asnRegion), disabled: placeholder(r.asnRegion) },
        { label: T('ping.ctx.copy_rate'), run: () => copy(pct(r.successRate)), disabled: !r.checks },
        { label: T('ping.ctx.copy_loss'), run: () => copy(pct(r.lossRate)), disabled: !r.checks },
        { label: T('ping.ctx.copy_last'), run: () => copy(ms(r.last, r.checks > 0 && r.ok)), disabled: !r.checks }, '-',
        { label: T('ping.ctx.traceroute'), run: () => httpTrace([r]), disabled: !hostOf(r) },
        { label: T('ping.ctx.move_up'), run: () => api('http/move', { id: r.id, up: true }), disabled: !byOrder || i <= 0 },
        { label: T('ping.ctx.move_down'), run: () => api('http/move', { id: r.id, up: false }), disabled: !byOrder || i >= S.http.rows.length - 1 },
        { label: T(r.enabled ? 'ping.ctx.disable' : 'ping.ctx.enable'), run: () => api('http/enable', { ids: [r.id], enabled: !r.enabled }) },
        { label: T('http.ctx.reset'), run: () => resetStats('http', [r.id], 1) }, '-',
        { label: T('http.ctx.delete'), danger: true, run: () => httpDel([r.id]) },
      ];
    }
    function multi(sel) {
      const ids = sel.map(r => r.id), n = ids.length;
      return [
        { label: T('ping.ctx.multi.enable', n), run: () => api('http/enable', { ids, enabled: true }) },
        { label: T('ping.ctx.multi.disable', n), run: () => api('http/enable', { ids, enabled: false }) },
        { label: T('http.ctx.reset'), run: () => resetStats('http', ids, n) },
        { label: T('ping.ctx.multi.traceroute', n), run: () => httpTrace(sel) }, '-',
        { label: T('ping.ctx.multi.delete', n), danger: true, run: () => httpDel(ids) },
      ];
    }
    function hostOf(r) { try { return new URL(r.spec.url).hostname.replace(/^\[|\]$/g, ''); } catch (e) { return ''; } }
    async function httpTrace(sel) {
      const hosts = sel.map(hostOf).filter(Boolean);
      if (!hosts.length) return;
      const r = await api('ping/trace', { hosts });
      if (r.message) toast(r.message);
      if (r.ok) go('trace');
    }
    return {
      update(st) {
        startBtn.textContent = T(st.http.running ? 'http.stop_all' : 'http.start_all');
        startBtn.className = 'btn ' + (st.http.running ? '' : 'primary');
        status.textContent = st.http.status;
        rb.refresh(false);
        tbl.update(rb.overlay(st.http.rows, (res, r) => {
          const v = res.http && res.http[r.id];
          return v && Object.assign({}, v, { checks: v.success + v.failure, ranged: true });
        }));
        delBtn.disabled = tbl.selected().length === 0;
      },
    };
  }

  // ---------- Ping page ----------
  const pingSort = JSON.parse(localStorage.getItem('ht.pingSort') || 'null') || { col: '#', desc: false };
  function pingPage(root) {
    const host = H('input', { type: 'text', class: 'w-host', placeholder: T('ping.host_ph') });
    const alias = H('input', { type: 'text', class: 'w-alias', placeholder: T('ping.alias_ph') });
    const add = async (replace) => {
      const r = await api('ping/add', { host: host.value, alias: alias.value, replace });
      if (r.status === 'conflict') {
        confirmBox(T('dup.title'), r.message, T('dup.replace'), () => add(true));
        return;
      }
      if (r.ok) { host.value = ''; alias.value = ''; host.focus(); }
      if (r.message && r.status !== 'added') toast(r.message);
    };
    [host, alias].forEach(i => i.addEventListener('keydown', e => { if (e.key === 'Enter') add(false); }));
    const interval = H('input', { type: 'number', class: 'w-num', min: '0.2', max: '3600', step: '0.5' });
    interval.addEventListener('change', () => api('ping/interval', { seconds: parseFloat(interval.value) || 1 }));
    const startBtn = H('button', { class: 'btn primary', onclick: () => api(S.ping.running ? 'ping/stop' : 'ping/start') });
    const resetBtn = H('button', { class: 'btn', onclick: () => resetStats('ping', [], S.ping.rows.length) }, T('ping.reset'));
    const delBtn = H('button', { class: 'btn danger', onclick: () => delIDs(tbl.selected().map(r => r.id)) }, T('ping.delete_sel'));
    const status = H('div', { class: 'status' }), db = H('div', { class: 'status' }), imp = H('div', { class: 'status accent' });

    root.append(H('section', { class: 'card' },
      W(H('h3', { text: T('ping.add_title') })),
      W(H('div', { class: 'row end' },
        H('div', { class: 'field' }, H('label', { text: T('ping.host') }), host),
        H('div', { class: 'field' }, H('label', { text: T('ping.alias') }), alias),
        H('button', { class: 'btn primary', onclick: () => add(false) }, T('ping.add')),
        H('button', { class: 'btn', onclick: importDlg }, T('ping.import')))),
      W(H('h3', { text: T('ping.control') })),
      W(H('div', { class: 'row' },
        H('label', { class: 'pill' }, H('span', { text: T('ping.interval') }), interval, H('span', { text: T('ping.seconds') })),
        startBtn, resetBtn, delBtn)),
      status, db, imp));

    const C = (id, key, cell, sort, extra) => Object.assign({ id, label: T(key), cell, sort }, extra || {});
    const has = r => r.success > 0;
    const rb = rangeBar('ping', () => update());
    const tbl = Table({ id: 'ping',
      key: r => r.id, check: true, defaultSort: '#', sortState: pingSort, empty: T('ping.empty'),
      onSort: s => localStorage.setItem('ht.pingSort', JSON.stringify(s)),
      rowClass: r => (!r.enabled ? 'off' : r.failing ? 'bad' : ''),
      onOpen: r => openDetail({ kind: 'ping', id: r.id }),
      onMenu: (e, r, sel) => openMenu(e.clientX, e.clientY, sel.length > 1 ? multiMenu(sel) : oneMenu(r)),
      onSelect: () => { delBtn.disabled = tbl.selected().length === 0; },
      cols: [
        C('#', 'ping.col.n', r => String(S.ping.rows.findIndex(x => x.id === r.id) + 1), null, { num: true }),
        C('alias', 'ping.col.alias', r => dash(r.alias), r => r.alias.toLowerCase()),
        C('host', 'ping.col.host', r => r.host, r => r.host.toLowerCase(), { mono: true }),
        C('ip', 'ping.col.ip', r => dash(r.ip), r => ipKey(r.ip), { mono: true }),
        C('rdns', 'ping.col.rdns', r => dash(r.rdns), r => (placeholder(r.rdns) ? '' : r.rdns.toLowerCase())),
        C('location', 'ping.col.location', r => dash(r.loc), r => (placeholder(r.loc) ? '' : r.loc)),
        C('region', 'ping.col.region', r => flagCell(r.iso, r.region), r => (placeholder(r.region) ? '' : r.region)),
        C('asn', 'ping.col.asn', r => dash(r.asn), r => (placeholder(r.asn) ? '' : r.asn)),
        C('asn_region', 'ping.col.asn_region', r => flagCell(r.asnIso, r.asnRegion), r => (placeholder(r.asnRegion) ? '' : r.asnRegion)),
        C('success', 'ping.col.success', r => String(r.success), r => r.success, { num: true }),
        C('failure', 'ping.col.failure', r => String(r.failure), r => r.failure, { num: true }),
        C('success_rate', 'ping.col.rate', r => (r.success + r.failure ? pct(r.successRate) : '—'), r => r.successRate, { num: true }),
        C('loss_rate', 'ping.col.loss', r => lossCell(r.lossRate, r.success + r.failure > 0), r => r.lossRate, { num: true, copy: r => (r.success + r.failure > 0 ? pct(r.lossRate) : '') }),
        C('last', 'ping.col.last', r => ms(r.last, has(r)), r => (has(r) ? r.last : null), { num: true }),
        C('avg', 'ping.col.avg', r => ms(r.avg, has(r)), r => (has(r) ? r.avg : null), { num: true }),
        C('min', 'ping.col.min', r => ms(r.min, has(r)), r => (has(r) ? r.min : null), { num: true }),
        C('max', 'ping.col.max', r => ms(r.max, has(r)), r => (has(r) ? r.max : null), { num: true }),
        C('median', 'ping.col.median', r => ms(r.median, has(r)), r => (has(r) ? r.median : null), { num: true }),
        C('last_ok', 'ping.col.last_ok', r => fmtTime(r.lastOk), r => r.lastOk || null),
        C('last_fail', 'ping.col.last_fail', r => fmtTime(r.lastFail), r => r.lastFail || null),
      ],
    });
    root.append(H('section', { class: 'card grow' },
      H('div', { class: 'row' }, H('h3', { text: T('ping.list_label') }), H('span', { class: 'desc', text: T('web.table_hint') }), H('span', { class: 'sp' }), tableExportButton('ping', tbl, rb)), rb.el, tbl.el));

    function delIDs(ids) {
      if (!ids.length) return;
      confirmBox(T('ping.delete_sel'), T('web.confirm_delete', ids.length), T('ping.ctx.delete'), () => api('ping/delete', { ids }), true);
    }
    function editAlias(r) {
      openModal(box => {
        const inp = H('input', { type: 'text', class: 'w-full', value: r.alias, placeholder: T('ping.alias_ph') });
        const ok = () => { api('ping/alias', { id: r.id, alias: inp.value }); closeModal(); };
        inp.addEventListener('keydown', e => { if (e.key === 'Enter') ok(); });
        box.append(H('h2', { text: T('alias.title') }), H('div', { class: 'desc', text: T('alias.desc') + ' ' + r.host }), inp,
          H('div', { class: 'actions' }, H('button', { class: 'btn', onclick: closeModal }, T('alias.cancel')), H('button', { class: 'btn primary', onclick: ok }, T('alias.ok'))));
      });
    }
    function oneMenu(r) {
      const byOrder = pingSort.col === '#' && !pingSort.desc;
      const i = S.ping.rows.findIndex(x => x.id === r.id);
      return [
        { label: T('ping.ctx.detail'), run: () => openDetail({ kind: 'ping', id: r.id }) },
        { label: T('ping.ctx.edit_alias'), run: () => editAlias(r) }, '-',
        { label: T('ping.ctx.copy_alias'), run: () => copy(r.alias), disabled: !r.alias },
        { label: T('ping.ctx.copy_host'), run: () => copy(r.host) },
        { label: T('ping.ctx.copy_ip'), run: () => copy(r.ip), disabled: placeholder(r.ip) },
        { label: T('ping.ctx.copy_rdns'), run: () => copy(r.rdns), disabled: placeholder(r.rdns) },
        { label: T('ping.ctx.copy_location'), run: () => copy(r.loc), disabled: placeholder(r.loc) },
        { label: T('ping.ctx.copy_region'), run: () => copy(r.region), disabled: placeholder(r.region) },
        { label: T('ping.ctx.copy_asn'), run: () => copy(r.asn), disabled: placeholder(r.asn) },
        { label: T('ping.ctx.copy_loss'), run: () => copy(pct(r.lossRate)) },
        { label: T('ping.ctx.copy_last'), run: () => copy(ms(r.last, has(r))), disabled: !has(r) }, '-',
        { label: T('ping.ctx.move_up'), run: () => api('ping/move', { id: r.id, up: true }), disabled: !byOrder || i <= 0 },
        { label: T('ping.ctx.move_down'), run: () => api('ping/move', { id: r.id, up: false }), disabled: !byOrder || i >= S.ping.rows.length - 1 },
        { label: T(r.enabled ? 'ping.ctx.disable' : 'ping.ctx.enable'), run: () => api('ping/enable', { ids: [r.id], enabled: !r.enabled }) },
        { label: T('ping.ctx.traceroute'), run: () => { api('ping/trace', { hosts: [r.host] }); go('trace'); } },
        { label: T('ping.ctx.reset'), run: () => resetStats('ping', [r.id], 1) }, '-',
        { label: T('ping.ctx.delete'), danger: true, run: () => delIDs([r.id]) },
      ];
    }
    function multiMenu(sel) {
      const ids = sel.map(r => r.id), n = ids.length;
      return [
        { label: T('ping.ctx.multi.enable', n), run: () => api('ping/enable', { ids, enabled: true }) },
        { label: T('ping.ctx.multi.disable', n), run: () => api('ping/enable', { ids, enabled: false }) },
        { label: T('ping.ctx.multi.traceroute', n), run: () => { api('ping/trace', { hosts: sel.map(r => r.host) }); go('trace'); } },
        { label: T('web.copy_hosts', n), run: () => copy(sel.map(r => r.host).join('\n')) },
        { label: T('ping.ctx.reset'), run: () => resetStats('ping', ids, n) }, '-',
        { label: T('ping.ctx.multi.delete', n), danger: true, run: () => delIDs(ids) },
      ];
    }
    function importDlg() {
      openModal(box => {
        const ta = H('textarea', { rows: '10', class: 'w-full', placeholder: T('import.placeholder') });
        const send = async mode => {
          const r = await api('ping/import', { text: ta.value, mode });
          if (r.status === 'conflict') {
            box.replaceChildren(H('h2', { text: T('import.conflict.title') }), H('div', { class: 'desc', text: T('import.conflict.desc') }),
              H('div', { class: 'kv', style: 'grid-template-columns:1fr' }, ...(r.data || []).map(s => H('div', { text: s }))),
              H('div', { class: 'actions' }, H('button', { class: 'btn', onclick: closeModal }, T('import.conflict.cancel')),
                H('button', { class: 'btn', onclick: () => send('skip') }, T('import.conflict.skip_all')),
                H('button', { class: 'btn primary', onclick: () => send('replace') }, T('import.conflict.replace_all'))));
            return;
          }
          closeModal();
          if (r.message) toast(r.message);
        };
        box.append(H('h2', { text: T('import.title') }), H('div', { class: 'desc', text: T('import.desc') }), ta,
          H('div', { class: 'actions' }, H('button', { class: 'btn', onclick: closeModal }, T('import.cancel')), H('button', { class: 'btn primary', onclick: () => send('') }, T('import.submit'))));
      });
    }
    let lastInterval = null;
    return {
      update(st) {
        startBtn.textContent = T(st.ping.running ? 'ping.stop' : 'ping.start');
        startBtn.className = 'btn ' + (st.ping.running ? '' : 'primary');
        if (document.activeElement !== interval && lastInterval !== st.ping.interval) { interval.value = st.ping.interval; lastInterval = st.ping.interval; }
        status.textContent = st.ping.status; status.className = 'status' + (st.ping.running ? ' ok' : '');
        db.textContent = st.settings.dbLoading ? T('db.loading') : st.settings.dbStatus;
        rb.refresh(false);
        tbl.update(rb.overlay(st.ping.rows, (res, r) => res.ping && res.ping[r.id]));
        delBtn.disabled = tbl.selected().length === 0;
      },
    };
  }
  function ipKey(ip) {
    if (!ip || ip === '—' || ip === '…') return null;
    if (/^\d+\.\d+\.\d+\.\d+$/.test(ip)) return '0' + ip.split('.').map(n => n.padStart(3, '0')).join('.');
    if (ip.includes(':')) {
      let parts = ip.split('::'), head = parts[0] ? parts[0].split(':') : [], tail = parts[1] ? parts[1].split(':') : [];
      if (parts.length === 2) head = head.concat(Array(8 - head.length - tail.length).fill('0'), tail);
      return '1' + head.map(h => h.padStart(4, '0')).join(':');
    }
    return '2' + ip;
  }

  // ---------- Trace page ----------
  function tracePage(root) {
    const list = H('div', { class: 'tablist' });
    const startAll = H('button', { class: 'btn sm', onclick: () => api('trace/startall') }, T('trace.tab.start_all'));
    const stopAll = H('button', { class: 'btn sm', onclick: () => api('trace/stopall') }, T('trace.tab.stop_all'));
    const side = H('div', { class: 'card tabs' },
      H('button', { class: 'btn primary', onclick: () => api('trace/new').then(r => { if (r.ok) api('trace/active', { id: r.data }); }) }, T('trace.tab.add')),
      W(H('div', { class: 'tab-tools' }, startAll, stopAll)), list);
    const host = H('input', { type: 'text', class: 'w-host', placeholder: T('trace.placeholder') });
    const alias = H('input', { type: 'text', class: 'w-alias', placeholder: T('trace.alias_ph') });
    const mtr = H('input', { type: 'checkbox' });
    const interval = H('input', { type: 'number', class: 'w-num', min: '0.2', max: '3600', step: '0.5' });
    const runBtn = H('button', { class: 'btn primary' });
    const resetBtn = H('button', { class: 'btn' }, T('trace.reset'));
    const status = H('div', { class: 'status' });
    let tab = null;
    const save = (extra) => tab && api('trace/update', Object.assign({ id: tab.id, host: host.value, alias: alias.value, mtr: mtr.checked, interval: parseFloat(interval.value) || 1 }, extra || {}));
    host.addEventListener('change', () => save()); alias.addEventListener('change', () => save());
    mtr.addEventListener('change', () => save()); interval.addEventListener('change', () => save());
    host.addEventListener('keydown', e => { if (e.key === 'Enter') { save().then(() => api('trace/start', { id: tab.id })); } });
    runBtn.onclick = async () => {
      if (!tab) return;
      if (tab.running) { api('trace/stop', { id: tab.id }); return; }
      await save(); api('trace/start', { id: tab.id });
    };
    resetBtn.onclick = () => tab && api('trace/reset', { id: tab.id });
    const has = r => r.success > 0;
    const C = (id, key, cell, sort, extra) => Object.assign({ id, label: T(key), cell, sort }, extra || {});
    const tblSort = { col: 'ttl', desc: false };
    const rb = rangeBar('trace', () => update(), () => tab && tab.id);
    const exportBtn = H('button', { class: 'btn sm', onclick: e => { const r = e.currentTarget.getBoundingClientRect(); openMenu(r.left, r.bottom + 4, traceExportItems(tab ? tab.id : 0)); } }, T('trace.export'));
    const tbl = Table({ id: 'trace',
      key: r => r.ttl, check: true, defaultSort: 'ttl', sortState: tblSort, empty: T('trace.empty'),
      rowClass: r => (r.failing ? 'bad' : ''),
      onOpen: r => openDetail({ kind: 'hop', tab: tab.id, ttl: r.ttl }),
      onMenu: (e, r, sel) => {
        const ips = sel.map(h => h.addr).filter(a => a && a !== '*');
        openMenu(e.clientX, e.clientY, sel.length > 1 ? [
          { label: T('trace.ctx.multi.add_ping', ips.length), disabled: !ips.length, run: () => api('ping/addmany', { hosts: ips }).then(x => toast(x.message)) },
          { label: T('web.copy_hosts', ips.length), disabled: !ips.length, run: () => copy(ips.join('\n')) },
        ] : [
          { label: T('trace.ctx.detail'), run: () => openDetail({ kind: 'hop', tab: tab.id, ttl: r.ttl }) },
          { label: T('trace.ctx.add_ping'), disabled: r.addr === '*', run: () => api('ping/add', { host: r.addr }).then(x => toast(x.message)) }, '-',
          { label: T('trace.ctx.copy_ip'), disabled: r.addr === '*', run: () => copy(r.addr) },
          { label: T('trace.ctx.copy_rdns'), disabled: placeholder(r.rdns), run: () => copy(r.rdns) },
          { label: T('trace.ctx.copy_rtt'), run: () => copy(r.rtt) },
          { label: T('trace.ctx.copy_location'), disabled: placeholder(r.loc), run: () => copy(r.loc) },
          { label: T('trace.ctx.copy_region'), disabled: placeholder(r.region), run: () => copy(r.region) },
          { label: T('trace.ctx.copy_asn'), disabled: placeholder(r.asn), run: () => copy(r.asn) },
          { label: T('trace.ctx.copy_asn_region'), disabled: placeholder(r.asnRegion), run: () => copy(r.asnRegion) },
        ]);
      },
      cols: [
        C('ttl', 'trace.col.ttl', r => String(r.ttl), r => r.ttl, { num: true }),
        C('ip', 'trace.col.ip', r => r.addr, r => ipKey(r.addr), { mono: true }),
        C('rdns', 'trace.col.rdns', r => dash(r.rdns), r => (placeholder(r.rdns) ? '' : r.rdns)),
        C('rtt', 'trace.col.rtt', r => r.rtt, r => (has(r) ? r.last : null), { num: true }),
        C('loss', 'ping.col.loss', r => lossCell(r.lossRate, r.success + r.failure > 0), r => r.lossRate, { num: true, copy: r => (r.success + r.failure > 0 ? pct(r.lossRate) : '') }),
        C('avg', 'ping.col.avg', r => ms(r.avg, has(r)), r => (has(r) ? r.avg : null), { num: true }),
        C('min', 'ping.col.min', r => ms(r.min, has(r)), r => (has(r) ? r.min : null), { num: true }),
        C('max', 'ping.col.max', r => ms(r.max, has(r)), r => (has(r) ? r.max : null), { num: true }),
        C('location', 'trace.col.location', r => dash(r.loc), r => (placeholder(r.loc) ? '' : r.loc)),
        C('region', 'trace.col.region', r => flagCell(r.iso, r.region), r => (placeholder(r.region) ? '' : r.region)),
        C('asn', 'trace.col.asn', r => dash(r.asn), r => (placeholder(r.asn) ? '' : r.asn)),
        C('asn_region', 'trace.col.asn_region', r => flagCell(r.asnIso, r.asnRegion), r => (placeholder(r.asnRegion) ? '' : r.asnRegion)),
      ],
    });
    const main = H('div', { class: 'trace-main' },
      H('section', { class: 'card' }, H('h3', { text: T('trace.target') }), H('div', { class: 'desc', text: T('trace.desc') }),
        W(H('div', { class: 'row end' },
          H('div', { class: 'field' }, H('label', { text: T('trace.label') }), host),
          H('div', { class: 'field' }, H('label', { text: T('trace.alias') }), alias),
          H('label', { class: 'pill' }, mtr, H('span', { text: T('trace.mtr') })),
          H('label', { class: 'pill' }, H('span', { text: T('trace.interval') }), interval, H('span', { text: T('ping.seconds') })),
          runBtn, resetBtn)), status),
      H('section', { class: 'card grow' }, H('div', { class: 'row' }, H('h3', { text: T('trace.hops') }), H('span', { class: 'sp' }), exportBtn), rb.el, tbl.el));
    root.append(H('div', { class: 'trace' }, side, main));
    const tabEls = new Map();
    let shownTab = null, guestTab = null;
    return {
      update(st) {
        const tabs = st.trace.tabs;
        tab = tabs.find(t => t.id === (GUEST && guestTab != null ? guestTab : st.trace.active)) || tabs[0] || null;
        // tab list (keyed)
        const live = new Set(tabs.map(t => t.id));
        for (const [k, el] of tabEls) if (!live.has(k)) { el.remove(); tabEls.delete(k); }
        let prev = null;
        tabs.forEach((t, i) => {
          let el = tabEls.get(t.id);
          if (!el) {
            el = H('div', { class: 'tab' }, H('span', { class: 'dot-run' }), H('span', { class: 't' }),
              H('button', { class: 'x', title: T('trace.tab.ctx.close'), onclick: e => { e.stopPropagation(); api('trace/close', { id: t.id }); } }, '×'));
            el.onclick = () => { if (GUEST) { guestTab = t.id; update(); } else api('trace/active', { id: t.id }); };
            el.oncontextmenu = e => {
              e.preventDefault();
              const cur = S.trace.tabs.find(x => x.id === t.id) || t, idx = S.trace.tabs.indexOf(cur);
              openMenu(e.clientX, e.clientY, GUEST ? traceExportItems(t.id) : [
                ...traceExportItems(t.id), '-',
                cur.running ? { label: T('trace.tab.ctx.stop'), run: () => api('trace/stop', { id: t.id }) }
                  : { label: T('trace.tab.ctx.start'), disabled: !cur.host, run: () => api('trace/start', { id: t.id }) },
                { label: T('trace.tab.ctx.duplicate'), run: () => api('trace/duplicate', { id: t.id }) },
                { label: T('ping.ctx.move_up'), disabled: idx <= 0, run: () => api('trace/move', { id: t.id, to: idx - 1 }) },
                { label: T('ping.ctx.move_down'), disabled: idx >= S.trace.tabs.length - 1, run: () => api('trace/move', { id: t.id, to: idx + 1 }) }, '-',
                { label: T('trace.tab.ctx.close_others'), disabled: S.trace.tabs.length < 2, run: () => api('trace/closeothers', { id: t.id }) },
                { label: T('trace.tab.ctx.close'), danger: true, run: () => api('trace/close', { id: t.id }) },
              ]);
            };
            tabEls.set(t.id, el);
          }
          const want = prev ? prev.nextSibling : list.firstChild;
          if (want !== el) list.insertBefore(el, want);
          prev = el;
          el.classList.toggle('on', tab && t.id === tab.id);
          el.children[0].classList.toggle('on', t.running);
          if (el.children[1].textContent !== t.title) el.children[1].textContent = t.title;
          el.title = t.host;
        });
        startAll.disabled = !tabs.some(t => !t.running && t.host);
        stopAll.disabled = !tabs.some(t => t.running);
        if (!tab) return;
        const switched = shownTab !== tab.id;
        if (switched) { shownTab = tab.id; tbl.clear(); }
        const sync = (inp, v) => { if (switched || document.activeElement !== inp && inp._v !== v) { inp.value = v; inp._v = v; } };
        sync(host, tab.host); sync(alias, tab.alias); sync(interval, tab.interval);
        if (switched || document.activeElement !== mtr) mtr.checked = tab.mtr;
        runBtn.textContent = T(tab.running ? 'trace.stop' : 'trace.start');
        runBtn.className = 'btn ' + (tab.running ? '' : 'primary');
        resetBtn.disabled = tab.running;
        status.textContent = tab.status || T('trace.status.stopped');
        rb.refresh(false);
        tbl.update(rb.overlay(tab.hops, (res, r) => {
          const v = res.trace && res.trace[tab.id] && res.trace[tab.id][r.ttl];
          return v && Object.assign({}, v, { rtt: ms(v.last, v.success > 0) });
        }));
      },
    };
  }

  // ---------- IP lookup page ----------
  function lookupPage(root) {
    const ta = H('textarea', { rows: '5', class: 'w-full', placeholder: T('lookup.placeholder') });
    let guestRows = null, guestStatus = '', guestQuery = '';
    const run = async () => {
      if (!GUEST) { api('lookup', { text: ta.value }); return; }
      guestStatus = T('lookup.status.running', ta.value.split(/[\s,;]+/).filter(Boolean).length); status.textContent = guestStatus;
      guestQuery = ta.value;
      try {
        const r = await fetch('/api/lookup/query?q=' + encodeURIComponent(ta.value), { cache: 'no-store' });
        const j = await r.json();
        guestRows = (j.data || []).map((x, i) => ({ id: i + 1, query: x.query, ip: x.ip || '', rdns: x.rdns || '', loc: x.loc || '', region: x.region || '',
          iso: x.iso || '', asn: x.asn || '', asnRegion: x.asnRegion || '', asnIso: x.asnIso || '', err: x.err || '', status: x.err ? 'error' : 'ready',
          ping: '—', pingState: '', pingTarget: x.ip || '', domainTarget: '' }));
        guestStatus = T('lookup.status.done', guestRows.filter(x => x.ip).length);
        update();
      } catch (e) { guestStatus = ''; }
    };
    ta.addEventListener('keydown', e => { if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) run(); });
    const status = H('div', { class: 'status' });
    root.append(H('section', { class: 'card' }, H('h3', { text: T('lookup.title') }), H('div', { class: 'desc', text: T('lookup.desc') }), ta,
      H('div', { class: 'row' }, H('button', { class: 'btn primary', onclick: run }, T('lookup.query')), H('button', { class: 'btn', onclick: () => api('lookup/clear') }, T('lookup.clear')), status)));
    if (GUEST) ta.value = '';
    const C = (id, key, cell, sort, extra) => Object.assign({ id, label: T(key), cell, sort }, extra || {});
    const rowText = r => [r.query, r.ip, r.ping, r.rdns, r.loc, r.region, r.asn, r.asnRegion, r.err].map(dash).join('\t');
    const tbl = Table({ id: 'lookup',
      key: r => r.id, check: true, defaultSort: 'ip', empty: T('lookup.empty'),
      rowClass: r => (r.status === 'error' ? 'bad' : ''),
      onMenu: (e, r, sel) => {
        const ips = sel.filter(x => x.ip).map(x => x.pingTarget);
        const doms = [...new Set(sel.map(x => x.domainTarget).filter(Boolean))];
        const tr = [...new Set(sel.filter(x => x.ip).map(x => x.ip))];
        const n = sel.length > 1;
        openMenu(e.clientX, e.clientY, [
          { label: T('lookup.ctx.copy_ip'), disabled: !tr.length, run: () => copy(tr.join('\n')) },
          { label: T('lookup.ctx.copy_row'), run: () => copy(sel.map(rowText).join('\n')) },
          { label: T('lookup.ctx.copy_all'), run: () => copy(tbl.display().map(rowText).join('\n')) }, '-',
          { label: n ? T('lookup.ctx.multi.add_ping_ip', ips.length) : T('lookup.ctx.add_ping_ip'), disabled: !ips.length, run: () => api('ping/addmany', { hosts: ips }).then(x => toast(x.message)) },
          { label: n ? T('lookup.ctx.multi.add_ping_domain', doms.length) : T('lookup.ctx.add_ping_domain'), disabled: !doms.length, run: () => api('ping/addmany', { hosts: doms }).then(x => toast(x.message)) },
          { label: n ? T('lookup.ctx.multi.add_trace', tr.length) : T('lookup.ctx.add_trace'), disabled: !tr.length, run: () => api('ping/trace', { hosts: tr }).then(x => toast(x.message)) },
        ]);
      },
      cols: [
        C('query', 'lookup.col.query', r => r.query, r => r.query.toLowerCase()),
        C('ip', 'lookup.col.ip', r => dash(r.ip), r => ipKey(r.ip), { mono: true }),
        C('ping', 'lookup.col.ping', r => r.ping, r => (r.pingState === 'ok' ? r.pingMs : null), { num: true }),
        C('rdns', 'lookup.col.rdns', r => dash(r.rdns), r => (placeholder(r.rdns) ? '' : r.rdns)),
        C('location', 'lookup.col.location', r => dash(r.loc), r => (placeholder(r.loc) ? '' : r.loc)),
        C('region', 'lookup.col.region', r => flagCell(r.iso, r.region), r => (placeholder(r.region) ? '' : r.region)),
        C('asn', 'lookup.col.asn', r => dash(r.asn), r => (placeholder(r.asn) ? '' : r.asn)),
        C('asn_region', 'lookup.col.asn_region', r => flagCell(r.asnIso, r.asnRegion), r => (placeholder(r.asnRegion) ? '' : r.asnRegion)),
        C('error', 'lookup.col.error', r => dash(r.err), r => r.err),
      ],
    });
    root.append(H('section', { class: 'card grow' }, H('div', { class: 'row' }, H('h3', { text: T('lookup.results') }), H('span', { class: 'sp' }),
      tableExportButton('lookup', tbl, null, () => (GUEST && guestRows ? { q: guestQuery } : null))), tbl.el));
    let first = true;
    return {
      update(st) {
        if (GUEST) { status.textContent = guestStatus; tbl.update(guestRows || []); return; }
        if (first) { ta.value = st.lookup.input || ''; first = false; }
        status.textContent = st.lookup.status;
        tbl.update(st.lookup.rows);
      },
    };
  }

  // ---------- Settings page ----------
  function settingsPage(root) {
    // 「我的语言」 (this browser, localStorage) and 「默认语言」 (config,
    // admin only). Effective language: the browser's own, else the default.
    let defLang = null;
    // color theme chips; value '' = follow the default (shown in its colors)
    function swatches(label, get, set, follow) {
      const chips = H('div', { class: 'palette', role: 'radiogroup', 'aria-label': label });
      const ids = follow ? [''].concat(PALETTES) : PALETTES;
      const name = id => T('settings.palette.' + id);
      const btns = ids.map(v => {
        const b = H('button', { type: 'button', class: 'swatch', role: 'radio', 'data-v': v, 'data-p': v || defPalette() }, H('i', null, H('b')), H('span'));
        b.addEventListener('click', async () => { await set(v); paint(); });
        chips.append(b); return b;
      });
      function paint() {
        const cur = get();
        btns.forEach(b => {
          const v = b.dataset.v, on = v === cur;
          if (!v) b.dataset.p = defPalette();
          const txt = v ? name(v) : T('settings.palette_follow', name(defPalette()));
          b.querySelector('span').textContent = txt; b.title = txt;
          b.classList.toggle('on', on); b.setAttribute('aria-checked', on);
        });
      }
      paint();
      return { el: chips, paint };
    }
    let myPal = null, defPal = null, myModeSel = null;
    function lookFields() {
      myPal = swatches(T('settings.palette_mine'), myPalette, v => {
        if (v) localStorage.setItem('ht.palette', v); else localStorage.removeItem('ht.palette');
        applyLook();
      }, true);
      myModeSel = H('select');
      const fillMode = () => {
        myModeSel.replaceChildren(H('option', { value: '', text: T('settings.theme_follow', T('settings.theme.' + defMode())) }),
          ...['light', 'dark', 'system'].map(v => H('option', { value: v, text: T('settings.theme.' + v) })));
        myModeSel.value = myMode();
      };
      fillMode(); myModeSel.fill = fillMode;
      myModeSel.onchange = () => {
        if (myModeSel.value) localStorage.setItem('ht.mode', myModeSel.value); else localStorage.removeItem('ht.mode');
        applyLook();
      };
      return [H('label', { text: T('settings.palette_mine') }), H('div', { class: 'field' }, myPal.el, H('div', { class: 'hint', text: T('settings.palette_mine_desc') })),
        H('label', { text: T('settings.theme_mine') }), H('div', { class: 'field' }, myModeSel, H('div', { class: 'hint', text: T('settings.palette_mine_desc') }))];
    }
    function defaultPaletteField() {
      defPal = swatches(T('settings.palette_default'), defPalette, async v => {
        const r = await api('settings/palette', { palette: v });
        if (r && r.ok && S) { S.settings.palette = v; applyLook(); if (myPal) myPal.paint(); }
      }, false);
      return [H('label', { text: T('settings.palette_default') }), H('div', { class: 'field' }, defPal.el, H('div', { class: 'hint', text: T('settings.palette_default_desc') }))];
    }
    function langCard() {
      const langName = c => (LANGS.find(l => l.code === c) || { name: c || '' }).name;
      const mine = H('select');
      const fill = () => {
        mine.replaceChildren(H('option', { value: '', text: T('settings.language_follow', langName(DEFAULT_LANG)) }),
          ...LANGS.map(l => H('option', { value: l.code, text: l.name })));
        mine.value = localStorage.getItem('ht.lang') || '';
      };
      fill();
      mine.onchange = async () => {
        if (mine.value) localStorage.setItem('ht.lang', mine.value); else localStorage.removeItem('ht.lang');
        await loadI18n(); mount();
      };
      const kids = [H('label', { text: T('settings.language_mine') }),
        H('div', { class: 'field' }, mine, H('div', { class: 'hint', text: T('settings.language_browser') }))];
      if (!GUEST) {
        defLang = H('select', null, ...LANGS.map(l => H('option', { value: l.code, text: l.name })));
        defLang.value = DEFAULT_LANG;
        defLang.onchange = async () => {
          await api('settings/language', { code: defLang.value });
          DEFAULT_LANG = defLang.value;
          await loadI18n(); mount();
        };
        kids.push(H('label', { text: T('settings.language_default') }),
          H('div', { class: 'field' }, defLang, H('div', { class: 'hint', text: T('settings.language_default_desc') })));
      }
      kids.push(...lookFields());
      return H('section', { class: 'card' }, H('h3', { text: T('settings.appearance') }), H('div', { class: 'form' }, ...kids));
    }
    if (GUEST) {
      const dbs = H('div', { class: 'status' });
      root.append(langCard(), H('section', { class: 'card' }, H('h3', { text: T('web.badge.guest') }), H('div', { class: 'desc', text: T('web.guest.tip') }), dbs));
      return { update(st) { dbs.textContent = st.settings.dbLoading ? T('db.loading') : st.settings.dbStatus; } };
    }
    const theme = H('select', null, ...['light', 'dark', 'system'].map(v => H('option', { value: v, text: T('settings.theme.' + v) })));
    theme.onchange = () => api('settings/theme', { theme: theme.value });
    const appear = langCard();
    appear.querySelector('.form').append(
      H('label', { text: T('settings.theme') }), H('div', { class: 'field' }, theme, H('div', { class: 'hint', text: T('web.theme_desc') })), ...defaultPaletteField());
    root.append(appear);

    const ipdb = H('input', { type: 'text', class: 'w-full', placeholder: T('settings.ipdb_ph') });
    const geo = H('input', { type: 'text', class: 'w-full', placeholder: T('settings.geoip_ph') });
    const asn = H('input', { type: 'text', class: 'w-full', placeholder: T('settings.asn_ph') });
    const dbs = H('div', { class: 'status' });
    root.append(H('section', { class: 'card' }, H('h3', { text: T('settings.data') }), H('div', { class: 'desc', text: T('settings.data_desc') }),
      H('div', { class: 'form' },
        H('label', { text: T('settings.ipdb') }), H('div', { class: 'field' }, ipdb, H('div', { class: 'hint', text: T('settings.ipdb_desc') })),
        H('label', { text: T('settings.geoip') }), H('div', { class: 'field' }, geo, H('div', { class: 'hint', text: T('settings.geoip_desc') })),
        H('label', { text: T('settings.asn') }), H('div', { class: 'field' }, asn, H('div', { class: 'hint', text: T('settings.asn_desc') }))),
      H('div', { class: 'row' },
        H('button', { class: 'btn primary', onclick: () => api('settings/data', { ipdb: ipdb.value, geoip: geo.value, asn: asn.value }).then(() => toast(T('web.saved'))) }, T('settings.save_load')),
        H('button', { class: 'btn', onclick: () => api('settings/reloaddb') }, T('settings.reload'))), dbs));

    const files = H('div', { class: 'col', style: 'display:flex;flex-direction:column;gap:12px' });
    const dlStart = H('button', { class: 'btn primary', onclick: () => api('datadl/start') }, T('settings.dl_all.start'));
    const dlCancel = H('button', { class: 'btn', onclick: () => api('datadl/cancel') }, T('settings.dl_all.cancel'));
    const dlStatus = H('div', { class: 'status' });
    root.append(H('section', { class: 'card' }, H('h3', { text: T('settings.dl_all.title') }), H('div', { class: 'desc', text: T('settings.dl_all.desc') }),
      H('div', { class: 'row' }, dlStart, dlCancel), files, dlStatus));

    const asp = H('input', { type: 'checkbox' }), ast = H('input', { type: 'checkbox' }), ash = H('input', { type: 'checkbox' });
    const saveStartup = () => api('settings/startup', { autoStartPing: asp.checked, autoStartHttp: ash.checked, autoStartTrace: ast.checked });
    asp.onchange = saveStartup; ast.onchange = saveStartup; ash.onchange = saveStartup;
    root.append(H('section', { class: 'card' }, H('h3', { text: T('settings.startup') }), H('div', { class: 'desc', text: T('settings.startup_desc') }),
      H('label', { class: 'row' }, asp, T('settings.autostart')), H('label', { class: 'row' }, ash, T('settings.autostart_http')), H('label', { class: 'row' }, ast, T('settings.autostart_trace'))));

    const histSave = H('input', { type: 'checkbox' });
    const histDays = H('input', { type: 'number', min: '1', max: '3650', value: '90', placeholder: T('ph.history.days'), style: 'width:100px' });
    const histPath = H('div', { class: 'hint' });
    const saveHist = () => api('settings/history', { save: histSave.checked, days: +histDays.value }).then(r => { if (r.ok) toast(T('settings.history.applied')); else if (r.message) toast(r.message); });
    root.append(H('section', { class: 'card' }, H('h3', { text: T('settings.history.title') }),
      histPath,
      H('label', { class: 'row' }, histSave, T('settings.history.save')),
      H('div', { class: 'row' }, H('span', { text: T('settings.history.days') }), histDays,
        H('button', { class: 'btn', onclick: saveHist }, T('settings.history.apply')))));

    const onRun = H('input', { type: 'checkbox' });
    const chan = H('select', null, H('option', { value: 'stable', text: T('update.channel.stable') }), H('option', { value: 'prerelease', text: T('update.channel.prerelease') }));
    const saveUpd = extra => api('settings/update', Object.assign({ channel: chan.value, checkOnRun: onRun.checked }, extra || {}));
    onRun.onchange = () => saveUpd(); chan.onchange = () => saveUpd();
    const skipRow = H('div', { class: 'row' });
    root.append(H('section', { class: 'card' }, H('h3', { text: T('update.settings.title') }), H('div', { class: 'desc', text: T('update.settings.desc') }),
      H('label', { class: 'row' }, onRun, T('update.settings.on_start')),
      H('div', { class: 'row' }, H('span', { text: T('update.settings.channel') }), chan), skipRow));

    const webInfo = H('div', { class: 'kv' });
    root.append(H('section', { class: 'card' }, H('h3', { text: T('web.settings.title') }), H('div', { class: 'desc', text: T('web.settings.desc') }), webInfo));

    // guest mode + API tokens (admin only; the server refuses guests anyway)
    const guestBox = H('input', { type: 'checkbox' });
    const guestHint = H('div', { class: 'hint' });
    guestBox.addEventListener('change', async () => {
      const r = await api('web/guest', { enabled: guestBox.checked });
      if (r.ok) toast(T(guestBox.checked ? 'web.guest.on' : 'web.guest.off')); else guestBox.checked = !guestBox.checked;
    });
    root.append(H('section', { class: 'card' }, H('h3', { text: T('web.guest.allow') }),
      H('label', { class: 'row' }, guestBox, T('web.guest.allow')), guestHint));
    const tokName = H('input', { type: 'text', class: 'w-alias', placeholder: T('web.token.name_ph') });
    const tokRole = H('select', null, H('option', { value: 'guest', text: T('web.token.role.guest') }), H('option', { value: 'admin', text: T('web.token.role.admin') }));
    const tokNew = H('div', { class: 'token-new', hidden: true });
    const tokList = H('div', { class: 'kv tokens' });
    const mcpLine = H('code', { class: 'status' });
    async function loadTokens() {
      const r = await fetch('/api/web/tokens', { cache: 'no-store' });
      if (!r.ok) return;
      const j = await r.json();
      const list = j.data || [];
      tokList.replaceChildren(...(list.length ? list.flatMap(t => [
        H('div', null, H('b', { text: t.name }), ' ', H('span', { class: 'badge' + (t.role === 'admin' ? '' : ' muted'), text: T('web.token.role.' + t.role) })),
        H('div', { class: 'row' }, H('span', { class: 'status', text: T('web.token.created_at', new Date(t.created * 1000).toLocaleString()) }),
          H('button', { class: 'btn sm danger', onclick: () => api('web/tokens/revoke', { id: t.id }).then(x => { if (x.ok) { toast(T('web.token.revoked')); loadTokens(); } }) }, T('web.token.revoke')))]) :
        [H('div', { class: 'status', text: T('web.token.none') })]));
    }
    const createTok = async () => {
      const r = await api('web/tokens/create', { name: tokName.value, role: tokRole.value });
      if (!r.ok) { if (r.message) toast(r.message); return; }
      tokName.value = '';
      const code = H('code', { class: 'secret', text: r.data.token });
      tokNew.replaceChildren(H('div', { text: T('web.token.created', r.data.name) }), code,
        H('div', { class: 'row' }, H('button', { class: 'btn primary sm', onclick: () => copy(r.data.token) }, T('web.token.copy')),
          H('button', { class: 'btn sm', onclick: () => { tokNew.hidden = true; tokNew.replaceChildren(); } }, T('web.token.dismiss'))));
      tokNew.hidden = false;
      loadTokens();
    };
    root.append(H('section', { class: 'card' }, H('h3', { text: T('web.mcp.title') }), H('div', { class: 'desc', text: T('web.mcp.desc') }),
      H('div', { class: 'row' }, H('span', { text: T('web.mcp.url') }), mcpLine)));
    root.append(H('section', { class: 'card' }, H('h3', { text: T('web.token.title') }), H('div', { class: 'desc', text: T('web.token.desc') }),
      tokNew,
      H('div', { class: 'row end' }, H('div', { class: 'field' }, H('label', { text: T('web.token.name') }), tokName),
        H('div', { class: 'field' }, H('label', { text: T('web.token.role') }), tokRole),
        H('button', { class: 'btn primary', onclick: () => createTok() }, T('web.token.create'))),
      tokList));
    mcpLine.textContent = location.origin + '/mcp';
    loadTokens();

    const links = [
      ['settings.dl.ipdb', 'https://github.com/nmgliangwei/qqwry.ipdb'],
      ['settings.dl.geoip', 'https://github.com/Loyalsoldier/geoip/releases'],
      ['settings.dl.asn', 'https://iptoasn.com/'],
    ];
    root.append(H('section', { class: 'card' }, H('h3', { text: T('settings.downloads') }),
      ...links.map(([k, u]) => H('div', { class: 'field' }, H('span', { text: T(k) }), H('a', { class: 'link', href: u, target: '_blank', rel: 'noopener', text: u })))));
    const cfg = H('div', { class: 'status' });
    root.append(H('section', { class: 'card' }, H('h3', { text: T('settings.config') }), H('div', { class: 'desc', text: T('settings.config_desc') }), H('code', null, cfg)));

    const prog = new Map();
    let firstPaths = true, lastSig = {};
    const sync = (el, v, k) => { if (document.activeElement !== el && lastSig[k] !== v) { if (el.type === 'checkbox') el.checked = v; else el.value = v; lastSig[k] = v; } };
    return {
      update(st) {
        const s = st.settings;
        sync(theme, s.theme, 'theme');
        if (lastSig.look !== s.theme + '/' + s.palette) { lastSig.look = s.theme + '/' + s.palette; if (myPal) myPal.paint(); if (defPal) defPal.paint(); if (myModeSel) myModeSel.fill(); }
        if (defLang && st.lang) sync(defLang, st.lang, 'deflang');
        sync(guestBox, !!st.web.guest, 'guest');
        guestBox.disabled = !st.web.hasPassword;
        guestHint.textContent = st.web.hasPassword ? T('web.guest.desc') : T('web.guest.need_password');
        guestHint.classList.toggle('bad', !st.web.hasPassword);
        if (firstPaths) { firstPaths = false; }
        sync(ipdb, s.ipdb, 'ipdb'); sync(geo, s.geoip, 'geo'); sync(asn, s.asn, 'asn');
        dbs.textContent = s.dbLoading ? T('db.loading') : s.dbStatus;
        sync(asp, s.autoStartPing, 'asp'); sync(ash, s.autoStartHttp, 'ash'); sync(ast, s.autoStartTrace, 'ast');
        sync(histSave, !!s.historySave, 'histSave');
        if (document.activeElement !== histDays && histDays._v !== s.historyDays) { histDays.value = s.historyDays; histDays._v = s.historyDays; }
        if (histPath._v !== s.historyPath) { histPath.textContent = T('settings.history.desc', s.historyPath || ''); histPath._v = s.historyPath; }
        sync(onRun, s.updateCheckOnStart, 'onRun'); sync(chan, s.updateChannel, 'chan');
        if (skipRow._v !== s.updateSkip) {
          skipRow._v = s.updateSkip;
          skipRow.replaceChildren(...(s.updateSkip ? [H('span', { class: 'status', text: T('update.settings.skipped', s.updateSkip) }),
            H('button', { class: 'btn sm', onclick: () => saveUpd({ unskip: true }) }, T('update.settings.unskip'))] : []));
        }
        const d = st.datadl;
        dlStart.disabled = d.running; dlCancel.disabled = !d.running;
        dlStatus.textContent = d.status || '';
        (d.files || []).forEach(f => {
          let p = prog.get(f.kind);
          if (!p) {
            p = H('div', { class: 'prog' }, H('div', { class: 'lbl' }, H('span'), H('span')), H('div', { class: 'bar' }, H('i')));
            prog.set(f.kind, p); files.append(p); // first appearance follows the fixed file order
          }
          p.className = 'prog ' + f.state;
          p.children[0].children[0].textContent = f.filename;
          p.children[0].children[1].textContent = f.text.slice(f.filename.length + 2);
          const w = f.state === 'ok' ? 100 : f.total > 0 ? f.percent : f.state === 'running' ? 15 : 0;
          p.children[1].firstChild.style.width = w + '%';
        });
        const w = st.web;
        // the running listener first (a startup option may override the
        // saved address), the saved one only when it differs
        const listenInfo = w => {
          const saved = (w.host.includes(':') ? '[' + w.host + ']' : w.host) + ':' + w.port;
          const cur = w.listen || saved;
          return H('div', null, H('div', { text: cur }), cur !== saved ? H('div', { class: 'hint', text: T('web.listen.saved_differs', saved) }) : null);
        };
        const sig = JSON.stringify(w);
        if (webInfo._v !== sig) {
          webInfo._v = sig;
          webInfo.replaceChildren(H('div', { text: T('web.listen.title') }), listenInfo(w),
            H('div', { text: T('web.pw.title') }), H('div', { text: T(w.hasPassword ? 'web.pw.set' : 'web.pw.none') }),
            H('div', { text: T('web.mode') }), H('div', { text: T(w.headless ? 'web.mode.headless' : 'web.mode.desktop') }));
        }
        cfg.textContent = s.configPath;
      },
    };
  }

  // ---------- About page ----------
  function aboutPage(root) {
    const st = H('span', { class: 'status' });
    const btn = H('button', { class: 'btn', onclick: () => api('update/check') }, T('update.check'));
    const url = 'https://github.com/hinasatou/hinatracer';
    root.append(H('section', { class: 'card' },
      H('div', { class: 'row' }, H('img', { src: '/icon.png', alt: '', style: 'width:48px;height:48px;border-radius:10px' }),
        H('div', null, H('div', { style: 'font-size:20px;font-weight:700', text: T('about.name') }), H('div', { class: 'status', id: 'aboutVer' }))),
      H('div', { class: 'row' }, btn, st),
      H('hr', { style: 'border:0;border-top:1px solid var(--border);width:100%;margin:0' }),
      H('div', { text: T('about.desc') }),
      H('div', { class: 'status', text: T('about.license') }),
      H('div', { class: 'row' }, H('span', { text: T('about.homepage') }), H('a', { class: 'link', href: url, target: '_blank', rel: 'noopener', text: url }))));
    return {
      update(s) {
        $('#aboutVer').textContent = T('about.version', s.version);
        const u = s.update;
        btn.disabled = u.phase === 'checking' || u.phase === 'downloading';
        st.textContent = u.text || '';
        st.className = 'status' + (u.phase === 'error' ? ' err' : u.phase === 'uptodate' ? ' ok' : u.phase === 'available' ? ' accent' : '');
      },
    };
  }

  const PAGES = { ping: pingPage, http: httpPage, trace: tracePage, iplookup: lookupPage, settings: settingsPage, about: aboutPage };

  // ---------- live updates ----------
  let es = null, retry = null;
  function connect() {
    if (es) es.close();
    es = new EventSource('/api/events');
    es.addEventListener('state', e => {
      $('#conn').hidden = true;
      const prevLang = S && S.lang;
      S = JSON.parse(e.data);
      if (prevLang && prevLang !== S.lang) { loadI18n().then(mount); return; }
      update();
    });
    es.onerror = () => {
      $('#conn').textContent = T('web.reconnecting'); $('#conn').hidden = false;
      if (es.readyState === EventSource.CLOSED) {
        clearTimeout(retry);
        retry = setTimeout(async () => { if (await checkSession()) connect(); }, 3000);
      }
    };
  }

  // ---------- login ----------
  function showLogin() {
    if (es) { es.close(); es = null; }
    $('#login').hidden = false;
    $('#loginDesc').textContent = T('web.login.desc');
    $('#loginPw').placeholder = T('web.login.password');
    $('#loginBtn').textContent = T('web.login.submit');
    const gb = $('#loginGuest');
    gb.hidden = !guestAllowed; gb.textContent = T('web.login.guest');
    setTimeout(() => $('#loginPw').focus(), 30);
  }
  $('#loginForm').addEventListener('submit', async e => {
    e.preventDefault();
    $('#loginErr').textContent = '';
    const r = await fetch('/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-HinaTracer': '1' }, body: JSON.stringify({ password: $('#loginPw').value }) });
    if (r.ok) { $('#login').hidden = true; $('#loginPw').value = ''; start(); return; }
    let j = {}; try { j = await r.json(); } catch (x) { /* empty */ }
    $('#loginErr').textContent = j.message || T('web.login.failed');
  });
  $('#loginGuest').addEventListener('click', async () => {
    $('#loginErr').textContent = '';
    const r = await fetch('/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-HinaTracer': '1' }, body: JSON.stringify({ guest: true }) });
    if (r.ok) { $('#login').hidden = true; start(); return; }
    let j = {}; try { j = await r.json(); } catch (x) { /* empty */ }
    $('#loginErr').textContent = j.message || T('web.login.guest_off');
  });
  let authRequired = false, guestAllowed = false;
  async function checkSession() {
    try {
      const j = await (await fetch('/api/session', { cache: 'no-store' })).json();
      authRequired = j.authRequired; guestAllowed = !!j.guest;
      GUEST = j.role === 'guest';
      document.body.classList.toggle('guest', GUEST);
      if (j.authRequired && !j.loggedIn) { showLogin(); return false; }
      return true;
    } catch (e) { return false; }
  }
  $('#logout').onclick = async () => { await api('logout'); showLogin(); };
  window.addEventListener('hashchange', () => {
    const p = location.hash.replace('#', '');
    if (TOP[p] && p !== page) go(p);
  });
  $('#menuBtn').onclick = () => $('#side').classList.toggle('open');

  async function start() {
    if (!await checkSession()) return;
    const lo = $('#logout');
    lo.hidden = !authRequired; lo.textContent = T('web.logout');
    mount();
    try {
      const r = await fetch('/api/state', { cache: 'no-store' });
      if (r.ok) { S = await r.json(); update(); }
    } catch (e) { /* the event stream fills in */ }
    connect();
  }
  (async () => {
    await loadI18n();
    if (await checkSession()) start();
  })();
})();
