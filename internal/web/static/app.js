'use strict';

// Splash Bench front end: picks models and tasks, starts the comparison and
// renders the live view the server streams via Server-Sent Events.

const PROMPTS = [
  { name: 'code', label: 'Code' },
  { name: 'prose', label: 'Fließtext' },
  { name: 'translate', label: 'Übersetzung' }
];
const ENGINES = [
  { key: 'omlx', field: 'omlx', engine: '', lane: 'lane-omlx' },
  { key: 'splash', field: 'splash', engine: 'splash', lane: 'lane-splash' }
];
const WORTHWHILE = 1.15;
const MAX_REPEATS = 10;

const state = {
  status: null,
  models: null, // Set of selected model ids; null until the first status
  prompts: new Set(PROMPTS.map(p => p.name)),
  repeats: 3,
  metric: 'tps',
  view: null,
  chartKey: ''
};

const $ = id => document.getElementById(id);

// ---------- formatting ----------

function decimal(v, digits) {
  return v.toLocaleString('de-DE', { minimumFractionDigits: digits, maximumFractionDigits: digits });
}

function factorText(f) {
  return f > 0 ? decimal(f, 2) + '×' : '–';
}

function factorClass(f) {
  if (!(f > 0)) return 'neutral';
  if (f >= WORTHWHILE) return 'good';
  if (f < 1) return 'bad';
  return 'neutral';
}

function promptLabel(name) {
  const p = PROMPTS.find(p => p.name === name);
  return p ? p.label : name;
}

function modelName(id) {
  const m = state.status && state.status.models.find(m => m.id === id);
  return m ? m.name : id;
}

function formatDate(iso) {
  return new Date(iso).toLocaleDateString('de-DE', { day: '2-digit', month: '2-digit', year: 'numeric' });
}

function systemText(sys) {
  if (!sys) return '';
  const parts = [];
  if (sys.chip) parts.push(sys.chip);
  if (sys.memory_gb) parts.push(decimal(sys.memory_gb, 0) + ' GB');
  return parts.join(' · ');
}

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

// ---------- server ----------

async function post(path, body) {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body)
  });
  if (!res.ok) {
    let message = 'Fehler ' + res.status;
    try { message = (await res.json()).error || message; } catch (e) { /* not JSON */ }
    throw new Error(message);
  }
  return res.status === 202 || res.status === 200 ? res.json().catch(() => null) : null;
}

async function loadStatus() {
  $('status-text').textContent = 'Prüfe Voraussetzungen …';
  try {
    const res = await fetch('api/status');
    state.status = await res.json();
  } catch (e) {
    state.status = {
      ready: false, connected: false, system: {}, models: [],
      checks: [{ title: 'Splash Bench', level: 'fail', detail: 'Das Programm antwortet nicht.', hint: 'Splash Bench neu starten.' }]
    };
  }
  if (state.models === null && state.status.models.length > 0) {
    const usable = state.status.models.filter(m => m.ready && m.fits);
    const recommended = usable.filter(m => m.recommended);
    state.models = new Set((recommended.length ? recommended : usable).map(m => m.id));
  }
  renderStatus();
  renderControls();
  if (state.view) renderView();
}

function connect() {
  const events = new EventSource('api/events');
  state.events = events;
  events.onmessage = e => {
    const first = state.view === null;
    state.view = JSON.parse(e.data);
    if (first) renderControls();
    renderView();
  };
}

let toastTimer = null;
function toast(message) {
  const t = $('toast');
  t.textContent = message;
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { t.hidden = true; }, 6000);
}

async function act(fn) {
  try { await fn(); } catch (e) { toast(e.message); }
}

// ---------- controls ----------

function running() {
  return state.view !== null && state.view.phase === 'running';
}

const CHECK_ICONS = { ok: '✓', warn: '!', fail: '×', skip: '–' };

function renderStatus() {
  const s = state.status;
  const warned = s.checks.some(c => c.level === 'warn');
  const level = !s.ready ? 'bad' : warned ? 'warn' : 'ok';
  const box = $('status');
  for (const l of ['ok', 'warn', 'bad']) box.classList.toggle(l, l === level);

  const parts = [{ ok: 'Bereit', warn: 'Bereit mit Hinweisen', bad: 'Voraussetzungen fehlen' }[level]];
  if (s.system.splash) parts.push('Splash ' + s.system.splash);
  if (s.system.omlx) parts.push('oMLX ' + s.system.omlx);
  const sys = systemText(s.system);
  if (sys) parts.push(sys);
  $('status-text').textContent = parts.join(' · ');

  const list = $('checks');
  list.replaceChildren();
  for (const c of s.checks) list.append(checkItem(c));
  renderCatalog();
  if (!s.ready) showRequirements(true);
}

function checkItem(c) {
  const li = el('li', 'check ' + c.level);
  const icon = el('span', 'check-icon', CHECK_ICONS[c.level] || '?');
  icon.setAttribute('aria-label', { ok: 'erfüllt', warn: 'Hinweis', fail: 'fehlt', skip: 'nicht geprüft' }[c.level] || '');
  li.append(icon, el('span', 'check-title', c.title), el('span', 'check-detail', c.detail));
  const more = el('div');
  if (c.hint) more.append(el('div', 'check-hint', c.hint));
  if (c.command) more.append(commandRow(c.command));
  li.append(more);
  return li;
}

// copyDiagnosis puts what a bug report needs on the clipboard.
async function copyDiagnosis() {
  if (!state.status) throw new Error('Die Prüfung läuft noch – gleich noch einmal versuchen.');
  await navigator.clipboard.writeText(state.status.diagnosis);
  toast('Diagnose kopiert – füge sie in die Fehlermeldung ein.');
}

function commandRow(command) {
  const row = el('div', 'check-command');
  const copy = el('button', '', 'Kopieren');
  copy.type = 'button';
  copy.addEventListener('click', () => act(async () => {
    await navigator.clipboard.writeText(command);
    copy.textContent = 'Kopiert';
  }));
  row.append(el('code', '', command), copy);
  return row;
}

// catalogItem shows a catalog model like a check: ready, missing, or too
// big for this Mac.
function catalogItem(e) {
  if (!e.in_omlx) return standaloneItem(e);
  const level = e.ready ? 'ok' : e.fits ? 'skip' : 'fail';
  const li = el('li', 'check ' + (level === 'skip' ? 'missing' : level));
  const icon = el('span', 'check-icon', { ok: '✓', skip: '↓', fail: '×' }[level]);
  icon.setAttribute('aria-label', { ok: 'bereit', skip: 'nicht geladen', fail: 'zu groß' }[level]);
  let detail = 'ca. ' + decimal(e.need_gb, 0) + ' GB mit Splash · ';
  detail += e.ready ? 'bereit' : e.fits ? 'noch nicht geladen' : 'zu groß für diesen Mac';
  li.append(icon, el('span', 'check-title', e.name), el('span', 'check-detail', detail));
  const more = el('div');
  more.append(el('div', 'check-hint', e.advice));
  if (e.command && e.fits) more.append(commandRow(e.command));
  li.append(more);
  return li;
}

// standaloneItem shows a family Splash serves only without oMLX, so there
// is nothing to download for this app.
function standaloneItem(e) {
  const li = el('li', 'check missing');
  const icon = el('span', 'check-icon', 'i');
  icon.setAttribute('aria-label', 'nur ohne oMLX');
  const detail = 'ca. ' + decimal(e.need_gb, 0) + ' GB mit Splash · nicht in oMLX';
  li.append(icon, el('span', 'check-title', e.name), el('span', 'check-detail', detail));
  const more = el('div');
  more.append(el('div', 'check-hint', e.advice));
  li.append(more);
  return li;
}

function renderCatalog() {
  const list = $('catalog-list');
  list.replaceChildren();
  for (const e of (state.status && state.status.catalog) || []) list.append(catalogItem(e));
  $('catalog-asof').textContent = 'Katalog: ' + ((state.status && state.status.catalog_as_of) || '–');
}

function showCatalog(open) {
  $('catalog').hidden = !open;
  $('catalog-toggle').setAttribute('aria-expanded', String(open));
  if (!open) return;
  $('catalog').scrollIntoView({ behavior: 'smooth', block: 'start' });
  $('catalog-title').focus({ preventScroll: true });
}

function showRequirements(open) {
  $('requirements').hidden = !open;
  $('status').setAttribute('aria-expanded', String(open));
}

// revealRequirements opens the requirements and brings them into view, so a
// hint elsewhere on the page leads straight to what is missing.
function revealRequirements() {
  showRequirements(true);
  $('requirements').scrollIntoView({ behavior: 'smooth', block: 'start' });
  $('requirements-title').focus({ preventScroll: true });
}

function chip(label, pressed, disabled, onClick, title) {
  const b = el('button', 'chip', label);
  b.type = 'button';
  b.setAttribute('aria-pressed', String(pressed));
  b.disabled = disabled;
  if (title) b.title = title;
  b.addEventListener('click', onClick);
  return b;
}

function toggle(set, key) {
  if (set.has(key)) set.delete(key); else set.add(key);
  renderControls();
}

function renderControls() {
  const locked = running();
  const models = $('models');
  models.replaceChildren();
  const list = state.status ? state.status.models : [];
  if (list.length === 0 && !state.status) models.append(el('span', 'empty', '…'));
  if (list.length === 0 && state.status) {
    const why = el('button', 'link', 'keine Splash-fähigen Modelle – warum?');
    why.type = 'button';
    why.addEventListener('click', revealRequirements);
    models.append(why);
  }
  for (const m of list) {
    const pressed = m.ready && state.models !== null && state.models.has(m.id);
    let title = m.ready ? m.id : m.reason;
    if (m.ready && !m.recommended) title = m.id + ' – von oMLX als Splash-fähig gemeldet, nicht getestet';
    if (m.ready && !m.fits) title = 'braucht ca. ' + decimal(m.need_gb, 0) + ' GB – mehr, als für Modelle nutzbar ist';
    const c = chip(m.name, pressed, locked || !m.ready, () => toggle(state.models, m.id), title);
    if (m.ready && !m.fits) c.append(el('span', 'tight', 'RAM knapp'));
    else if (m.ready && !m.recommended) c.append(el('span', 'untested', 'ungetestet'));
    models.append(c);
  }

  const prompts = $('prompts');
  prompts.replaceChildren();
  for (const p of PROMPTS) {
    prompts.append(chip(p.label, state.prompts.has(p.name), locked, () => toggle(state.prompts, p.name)));
  }

  $('repeats').textContent = state.repeats;
  $('dec').disabled = locked || state.repeats <= 1;
  $('inc').disabled = locked || state.repeats >= MAX_REPEATS;

  const ready = state.status && state.status.ready && selectedModels().length > 0 && state.prompts.size > 0;
  $('start').disabled = !ready;
  $('start').hidden = locked;
  $('cancel').hidden = !locked;
}

function selectedModels() {
  if (!state.status || !state.models) return [];
  return state.status.models.filter(m => m.ready && state.models.has(m.id)).map(m => m.id);
}

function startComparison() {
  return post('api/start', {
    models: selectedModels(),
    prompts: PROMPTS.map(p => p.name).filter(n => state.prompts.has(n)),
    repeats: state.repeats
  });
}

// ---------- view ----------

let lastLocked = null;

function renderView() {
  const v = state.view;
  const locked = v.phase === 'running';
  if (locked !== lastLocked) {
    lastLocked = locked;
    renderControls();
  }

  $('stage-idle').hidden = v.phase !== 'idle';
  $('stage-running').hidden = v.phase !== 'running';
  $('stage-done').hidden = v.phase !== 'done';
  $('results').hidden = v.phase === 'idle';

  $('blocked').hidden = !(v.phase === 'idle' && state.status && !state.status.ready);
  const previous = $('previous');
  previous.hidden = !(v.phase === 'idle' && v.previous_at);
  if (v.previous_at) previous.textContent = 'Letzte Messung vom ' + formatDate(v.previous_at) + ' ansehen';

  if (v.phase === 'running') renderLanes(v);
  if (v.phase === 'done') renderSummary(v);
  if (v.phase !== 'idle') {
    renderCharts(v);
    renderProblems(v);
    renderFooter(v);
  }
}

function comparisonOf(v, model) {
  return v.comparison.find(c => c.model === model);
}

function promptOf(cmp, prompt) {
  return cmp ? cmp.prompts.find(p => p.prompt === prompt) : undefined;
}

// highestRate is the scale for a model's tok/s bars.
function highestRate(cmp, extra) {
  let most = extra || 0;
  for (const p of cmp ? cmp.prompts : []) {
    for (const e of ENGINES) if (p[e.field]) most = Math.max(most, p[e.field].tokens_per_second);
  }
  return most;
}

function stepText(step) {
  switch (step.kind) {
    case 'check': return 'Prüfe, ob Splash das Modell laden kann …';
    case 'switch': return (step.engine === 'splash' ? 'Splash' : 'oMLX') + ' wird eingeschaltet …';
    case 'warmup': return 'Modell wird geladen …';
    case 'measure': return promptLabel(step.prompt) + ' · Durchlauf ' + step.repeat + '/' + step.repeats;
    case 'restore': return 'Ausgangszustand wird wiederhergestellt …';
    default: return '';
  }
}

function renderLanes(v) {
  const step = v.step;
  const progress = v.total ? Math.round(v.done / v.total * 100) : 0;
  $('progress-text').textContent = progress + ' %';
  $('progress').setAttribute('aria-valuenow', String(progress));
  $('progress').firstElementChild.style.width = progress + '%';
  if (!step) {
    $('live-model').textContent = '';
    $('live-task').textContent = 'wird vorbereitet …';
    return;
  }
  $('live-model').textContent = modelName(step.model);
  $('live-task').textContent = stepText(step);

  const cmp = comparisonOf(v, step.model);
  const liveRate = v.live ? v.live.tokens_per_second : 0;
  const scale = Math.max(highestRate(cmp, liveRate) * 1.15, 1);
  for (const e of ENGINES) {
    const active = (step.engine || '') === e.engine && step.kind !== 'restore';
    let rate = 0;
    if (active && step.kind === 'measure' && liveRate > 0) {
      rate = liveRate;
    } else if (step.prompt) {
      const p = promptOf(cmp, step.prompt);
      rate = p && p[e.field] ? p[e.field].tokens_per_second : 0;
    }
    let hint = rate > 0 ? 'Median' : 'wartet';
    if (active) hint = step.kind === 'measure' ? 'misst' : 'lädt';

    const lane = $(e.lane);
    lane.classList.toggle('active', active);
    lane.querySelector('.lane-hint').textContent = hint;
    lane.querySelector('.lane-value').textContent = rate > 0 ? decimal(rate, 1) : '–';
    lane.querySelector('.fill').style.width = Math.min(100, rate / scale * 100) + '%';
  }
}

function failureOf(v, model) {
  return (v.failures || []).find(f => f.model === model);
}

// failedTile replaces the speed-up for a model one engine could not run.
function failedTile(model, f) {
  const tile = el('div', 'tile');
  tile.append(el('span', 'tile-name', modelName(model)));
  tile.append(el('span', 'tile-factor bad', '–'));
  tile.append(el('span', 'tile-caption', f.title));
  if (f.detail) tile.append(el('span', 'tile-detail', f.detail));
  tile.append(el('span', 'badge bad', 'Nicht vergleichbar'));
  return tile;
}

function renderSummary(v) {
  const box = $('stage-done');
  box.replaceChildren();
  for (const model of v.models || []) {
    const f = failureOf(v, model);
    if (f) {
      box.append(failedTile(model, f));
      continue;
    }
    const cmp = comparisonOf(v, model);
    if (!cmp) continue;
    const tile = el('div', 'tile');
    tile.append(el('span', 'tile-name', modelName(cmp.model)));
    tile.append(el('span', 'tile-factor ' + factorClass(cmp.mean_speedup), factorText(cmp.mean_speedup)));
    let caption = 'noch nicht mit beiden Engines gemessen';
    if (cmp.mean_speedup > 0) caption = cmp.mean_speedup >= 1 ? 'im Schnitt schneller mit Splash' : 'im Schnitt langsamer mit Splash';
    tile.append(el('span', 'tile-caption', caption));
    const verdict = { splash: 'Splash lohnt sich', omlx: 'oMLX reicht', open: 'Noch offen' }[cmp.verdict];
    tile.append(el('span', 'badge' + (cmp.verdict === 'splash' ? ' good' : ''), verdict));
    box.append(tile);
  }
  if (box.children.length === 0) box.append(el('p', 'lead', 'Keine Messwerte – der Vergleich wurde vor dem ersten Durchlauf beendet.'));
}

// renderCharts builds the bars once per model and prompt selection and then
// only updates widths and numbers, so bars grow smoothly while measuring.
function renderCharts(v) {
  const key = (v.models || []).join('|') + '#' + (v.prompts || []).join('|');
  const charts = $('charts');
  if (key !== state.chartKey) {
    state.chartKey = key;
    charts.replaceChildren();
    for (const model of v.models || []) {
      const chart = el('div', 'chart');
      chart.dataset.model = model;
      chart.append(el('h2', '', modelName(model)));
      chart.append(el('p', 'chart-problem'));
      for (const prompt of v.prompts || []) chart.append(buildRow(prompt));
      charts.append(chart);
    }
  }
  const tps = state.metric === 'tps';
  $('metric-note').textContent = tps ? 'tok/s · mehr ist besser' : 'bis zum ersten Token · weniger ist besser';
  for (const chart of charts.children) {
    const cmp = comparisonOf(v, chart.dataset.model);
    const failure = failureOf(v, chart.dataset.model);
    const problem = chart.querySelector('.chart-problem');
    problem.textContent = failure ? failure.title : '';
    problem.hidden = !failure;
    const value = (p, e) => {
      const r = p && p[e.field];
      if (!r) return 0;
      return tps ? r.tokens_per_second : r.time_to_first_token_ns / 1e6;
    };
    let scale = 0;
    for (const prompt of v.prompts || []) {
      for (const e of ENGINES) scale = Math.max(scale, value(promptOf(cmp, prompt), e));
    }
    scale = Math.max(scale * (tps ? 1.08 : 1.15), 1e-9);
    for (const row of chart.querySelectorAll('.row')) {
      const p = promptOf(cmp, row.dataset.prompt);
      const factor = row.querySelector('.row-factor');
      factor.textContent = p && p.speedup > 0 ? factorText(p.speedup) : '';
      factor.className = 'row-factor ' + factorClass(p ? p.speedup : 0);
      for (const e of ENGINES) {
        const x = value(p, e);
        const bar = row.querySelector('.bar.' + e.key);
        bar.querySelector('.fill').style.width = (x / scale * 100) + '%';
        bar.querySelector('.bar-value').textContent = x > 0 ? (tps ? decimal(x, 1) : decimal(x, 0) + ' ms') : '–';
      }
    }
  }
}

function buildRow(prompt) {
  const row = el('div', 'row');
  row.dataset.prompt = prompt;
  const head = el('div', 'row-head');
  head.append(el('span', 'row-label', promptLabel(prompt)), el('span', 'row-factor'));
  row.append(head);
  for (const e of ENGINES) {
    const bar = el('div', 'bar ' + e.key);
    const track = el('div', 'track');
    track.append(el('div', 'fill ' + e.key));
    bar.append(track, el('span', 'bar-value', '–'));
    row.append(bar);
  }
  return row;
}

// renderProblems explains each model that could not be measured: what
// went wrong, the likely cause, and the log and raw message for digging.
function renderProblems(v) {
  const box = $('problems');
  const failures = v.failures || [];
  box.hidden = failures.length === 0;
  box.replaceChildren();
  for (const f of failures) {
    const item = el('div', 'problem');
    item.append(el('strong', '', modelName(f.model) + ': ' + f.title));
    if (f.detail) item.append(el('p', '', f.detail));
    const skipped = f.stage === 'check' ? 'Das Modell wurde vor der Messung aussortiert. ' : '';
    item.append(el('p', '', skipped + 'Die übrigen Modelle wurden trotzdem gemessen; die Engine-Einstellung ist zurückgesetzt.'));
    const more = el('details');
    more.append(el('summary', '', 'Technische Details'));
    if (f.log) more.append(commandRow(f.log));
    more.append(el('pre', '', f.raw));
    item.append(more);
    box.append(item);
  }
}

function renderFooter(v) {
  const done = v.phase === 'done';
  $('results-footer').hidden = !done;
  const notice = $('notice');
  notice.hidden = !(done && (v.cancelled || v.error));
  notice.classList.toggle('error', Boolean(v.error));
  notice.textContent = v.error
    ? 'Fehler: ' + v.error
    : 'Abgebrochen. Die Engine-Einstellung ist zurückgesetzt, das Ergebnis ist unvollständig.';
  if (!done) return;

  const parts = ['Ab 1,15× lohnt sich Splash'];
  if (v.repeats) parts.push('Median aus ' + v.repeats + (v.repeats === 1 ? ' Durchlauf' : ' Durchläufen'));
  if (v.source === 'archive' && v.measured_at) parts.push('Messung vom ' + formatDate(v.measured_at));
  const sys = systemText(v.system);
  if (sys) parts.push(sys);
  $('footnote').textContent = parts.join(' · ');
  $('download').hidden = v.comparison.length === 0;
}

// ---------- theme ----------

const MOON = '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/></svg>';
const SUN = '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>';
const systemDark = window.matchMedia('(prefers-color-scheme: dark)');

function currentTheme() {
  return document.documentElement.dataset.theme || (systemDark.matches ? 'dark' : 'light');
}

function renderThemeButton() {
  const dark = currentTheme() === 'dark';
  const button = $('theme');
  button.innerHTML = dark ? SUN : MOON;
  button.setAttribute('aria-label', dark ? 'Helle Darstellung' : 'Dunkle Darstellung');
}

function switchTheme() {
  const next = currentTheme() === 'dark' ? 'light' : 'dark';
  document.documentElement.dataset.theme = next;
  try { localStorage.setItem('theme', next); } catch (e) { /* not stored, still switched */ }
  renderThemeButton();
}

async function quitApp() {
  if (running() && !window.confirm('Der laufende Vergleich wird abgebrochen und die Engine zurückgesetzt. Beenden?')) return;
  await post('api/quit');
  if (state.events) state.events.close();
  $('page').hidden = true;
  $('ended').hidden = false;
}

// ---------- wiring ----------

function init() {
  $('dec').addEventListener('click', () => { state.repeats = Math.max(1, state.repeats - 1); renderControls(); });
  $('inc').addEventListener('click', () => { state.repeats = Math.min(MAX_REPEATS, state.repeats + 1); renderControls(); });
  $('start').addEventListener('click', () => act(startComparison));
  $('cancel').addEventListener('click', () => act(() => post('api/cancel')));
  $('reset').addEventListener('click', () => act(() => post('api/reset')));
  $('previous').addEventListener('click', () => act(() => post('api/previous')));
  $('recheck').addEventListener('click', loadStatus);
  $('copy-diagnosis').addEventListener('click', () => act(copyDiagnosis));
  $('blocked').addEventListener('click', revealRequirements);
  $('catalog-toggle').addEventListener('click', () => showCatalog($('catalog').hidden));
  $('catalog-close').addEventListener('click', () => showCatalog(false));
  $('status').addEventListener('click', () => showRequirements($('requirements').hidden));
  $('theme').addEventListener('click', switchTheme);
  $('quit').addEventListener('click', () => act(quitApp));
  systemDark.addEventListener('change', renderThemeButton);
  for (const b of document.querySelectorAll('.segmented button')) {
    b.addEventListener('click', () => {
      state.metric = b.dataset.metric;
      for (const o of document.querySelectorAll('.segmented button')) o.setAttribute('aria-pressed', String(o === b));
      if (state.view) renderCharts(state.view);
    });
  }
  renderThemeButton();
  renderControls();
  loadStatus();
  connect();
}

init();
