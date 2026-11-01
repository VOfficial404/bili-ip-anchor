"use strict";

const $ = (id) => document.getElementById(id);

class Unauthorized extends Error {}

async function api(method, path, body) {
  const headers = { "X-Requested-With": "bia" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const res = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    credentials: "same-origin",
  });
  if (res.status === 401 && path !== "/api/session") {
    showGate();
    throw new Unauthorized();
  }
  if (!res.ok) {
    let msg = `请求失败（HTTP ${res.status}）`;
    try { msg = (await res.json()).error || msg; } catch {}
    throw new Error(msg);
  }
  return res;
}

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text != null) e.textContent = text;
  return e;
}

/* 时间显示 */

const pad = (n) => String(n).padStart(2, "0");

function fmtTime(iso) {
  const d = new Date(iso);
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  const today = new Date();
  const tomorrow = new Date(today.getTime() + 86400000);
  if (d.toDateString() === today.toDateString()) return `今天 ${hm}`;
  if (d.toDateString() === tomorrow.toDateString()) return `明天 ${hm}`;
  return `${d.getMonth() + 1}月${d.getDate()}日 ${hm}`;
}

function fmtRelative(iso) {
  const sec = Math.round((new Date(iso) - Date.now()) / 1000);
  const abs = Math.abs(sec);
  let text;
  if (abs < 60) text = "不到 1 分钟";
  else if (abs < 3600) text = `${Math.round(abs / 60)} 分钟`;
  else {
    const h = Math.floor(abs / 3600);
    const m = Math.round((abs % 3600) / 60);
    text = m ? `${h} 小时 ${m} 分钟` : `${h} 小时`;
  }
  return sec >= 0 ? `${text}后` : `${text}前`;
}

/* 页面切换 */

let timer = null;

function showGate() {
  clearInterval(timer);
  timer = null;
  $("panel").hidden = true;
  $("signout").hidden = true;
  $("gate").hidden = false;
  $("password").focus();
}

function showPanel() {
  $("gate").hidden = true;
  $("panel").hidden = false;
  $("signout").hidden = false;
  refresh();
  if (!timer) timer = setInterval(refresh, 5000);
}

$("gate-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  $("gate-error").textContent = "";
  try {
    await api("POST", "/api/session", { password: $("password").value });
    $("password").value = "";
    showPanel();
  } catch (err) {
    $("gate-error").textContent = err.message;
  }
});

$("signout").addEventListener("click", async () => {
  try { await api("DELETE", "/api/session"); } catch {}
  showGate();
});

/* 主视觉：位置和邮戳 */

const SVG_NS = "http://www.w3.org/2000/svg";
let stampWord = null;

function svg(tag, attrs, text) {
  const e = document.createElementNS(SVG_NS, tag);
  for (const [k, v] of Object.entries(attrs || {})) e.setAttribute(k, v);
  if (text != null) e.textContent = text;
  return e;
}

function renderStamp(word) {
  if (word === stampWord) return;
  const wasPending = !stampWord;
  stampWord = word;

  const chars = Array.from(word || "?");
  const size = chars.length <= 2 ? 46 : chars.length === 3 ? 38 : chars.length <= 5 ? 27 : 20;
  const d = new Date();
  const date = `${d.getFullYear()}.${pad(d.getMonth() + 1)}.${pad(d.getDate())}`;

  const root = svg("svg", { viewBox: "0 0 220 220" });
  const filter = svg("filter", { id: "ink", x: "-5%", y: "-5%", width: "110%", height: "110%" });
  filter.append(
    svg("feTurbulence", { type: "fractalNoise", baseFrequency: "0.85", numOctaves: "2", seed: "7", result: "noise" }),
    svg("feDisplacementMap", { in: "SourceGraphic", in2: "noise", scale: "2.2" }),
  );
  const defs = svg("defs");
  defs.append(filter, svg("path", { id: "ring", d: "M 110 110 m -77 0 a 77 77 0 1 1 154 0 a 77 77 0 1 1 -154 0" }));

  const g = svg("g", { filter: "url(#ink)", fill: "none", stroke: "currentColor" });
  g.append(
    svg("circle", { cx: 110, cy: 110, r: 103, "stroke-width": 4 }),
    svg("circle", { cx: 110, cy: 110, r: 94, "stroke-width": 1.4 }),
    svg("circle", { cx: 110, cy: 110, r: 62, "stroke-width": 1.4 }),
  );
  const ring = svg("text", { class: "ring-text", fill: "currentColor", stroke: "none" });
  ring.append(svg("textPath", { href: "#ring", textLength: 466, lengthAdjust: "spacing" }, `BILIBILI IP ANCHOR ★ ${date} ★`));
  g.append(
    ring,
    svg("text", { class: "word", x: 110, y: 106, "font-size": size, fill: "currentColor", stroke: "none", "text-anchor": "middle", "dominant-baseline": "central" }, word || "?"),
    svg("text", { class: "sub", x: 112, y: 148, fill: "currentColor", stroke: "none", "text-anchor": "middle" }, "IP 属地"),
  );
  root.append(defs, g);

  const box = $("stamp");
  box.replaceChildren(root);
  box.classList.toggle("is-pending", !word);
  box.classList.toggle("is-new", Boolean(word) && wasPending);
}

function renderZone(s) {
  const place = $("place");
  const meta = $("place-meta");
  meta.replaceChildren();
  const z = s.zone;
  if (!z) {
    place.textContent = s.zone_error ? "暂时查不到" : "查询中";
    place.classList.add("is-pending");
    if (s.zone_error) meta.append(el("span", "error small", `查询失败：${s.zone_error}`));
    renderStamp("");
    return;
  }
  place.classList.remove("is-pending");
  const domestic = z.country === "中国";
  const parts = domestic ? [z.province, z.city] : [z.country, z.province];
  place.textContent = [...new Set(parts.filter(Boolean))].join(" ") || "未知地区";
  if (z.isp) meta.append(el("span", "tag", z.isp));
  if (z.addr) {
    const tag = el("span", "tag");
    tag.append(el("code", null, z.addr));
    meta.append(tag);
  }
  renderStamp((domestic ? z.province : z.country) || "");
}

/* 账号列表 */

const rows = new Map();

function faceURL(face) {
  if (!/^(https?:)?\/\/[a-z0-9.-]+\.hdslb\.com\//i.test(face)) return "";
  return face.replace(/^(https?:)?\/\//, "https://") + "@96w_96h_1c.webp";
}

function accountState(a) {
  if (a.running) return ["running", "运行中"];
  if (!a.logged_in) return ["bad", "登录失效"];
  if (!a.last) return ["idle", "等待首次运行"];
  return a.last.ok ? ["ok", "正常"] : ["warn", "上次失败"];
}

function buildRow(mid) {
  const img = el("img");
  img.alt = "";
  img.hidden = true;
  img.referrerPolicy = "no-referrer";
  const initial = el("span", "initial");
  img.addEventListener("load", () => { img.hidden = false; initial.hidden = true; });
  img.addEventListener("error", () => { img.hidden = true; initial.hidden = false; });
  const avatar = el("span", "avatar");
  avatar.append(img, initial);

  const name = el("span", "acc-name");
  const pill = el("span", "pill");
  const nameLine = el("div", "acc-name-line");
  nameLine.append(name, pill);
  const who = el("div", "acc-who");
  const uid = el("div", "acc-uid", `UID ${mid}`);
  who.append(nameLine, uid);
  const id = el("div", "acc-id");
  id.append(avatar, who);

  const last = el("dd");
  const next = el("dd");
  const times = el("dl", "acc-times");
  times.append(el("dt", null, "上次运行"), last, el("dt", null, "下次运行"), next);

  const relogin = el("button", "primary", "重新扫码");
  const run = el("button", "secondary", "立即运行");
  const remove = el("button", "link danger", "退出");
  for (const b of [relogin, run, remove]) b.type = "button";
  const actions = el("div", "acc-actions");
  actions.append(relogin, run, remove);

  const err = el("p", "acc-error");
  const li = el("li", "acc");
  li.append(id, times, actions, err);

  const row = { el: li, img, initial, name, uid, pill, last, next, relogin, run, remove, err, face: null, label: "" };
  relogin.addEventListener("click", openQR);
  run.addEventListener("click", () => runAccount(mid, row));
  remove.addEventListener("click", () => removeAccount(mid, row));
  return row;
}

function updateRow(row, a) {
  row.label = a.uname || `UID ${a.mid}`;
  row.name.textContent = row.label;
  row.name.title = row.label;
  row.uid.hidden = !a.uname;
  row.initial.textContent = a.uname ? Array.from(a.uname)[0] : "B";

  const face = faceURL(a.face || "");
  if (face !== row.face) {
    row.face = face;
    row.img.hidden = true;
    row.initial.hidden = false;
    if (face) row.img.src = face;
    else row.img.removeAttribute("src");
  }

  const [state, text] = accountState(a);
  row.pill.dataset.state = state;
  row.pill.textContent = text;

  row.last.replaceChildren();
  if (a.last) row.last.append(`${fmtTime(a.last.at)} `, el("span", a.last.ok ? "ok" : "bad", a.last.ok ? "成功" : "失败"));
  else row.last.textContent = "还没有";
  row.next.textContent = a.running ? "正在运行" : a.next ? `${fmtTime(a.next)}（${fmtRelative(a.next)}）` : "—";

  if (!a.logged_in) row.err.textContent = "这个账号的会话已失效，点「重新扫码」用同一个账号登录即可恢复。";
  else row.err.textContent = a.last && !a.last.ok ? a.last.error || "" : "";

  row.relogin.hidden = a.logged_in;
  row.run.hidden = !a.logged_in;
  row.run.disabled = a.running;
  row.run.textContent = a.running ? "运行中" : "立即运行";
}

function renderAccounts(list) {
  const ul = $("accounts");
  const seen = new Set();
  list.forEach((a, i) => {
    seen.add(a.mid);
    let row = rows.get(a.mid);
    if (!row) {
      row = buildRow(a.mid);
      rows.set(a.mid, row);
    }
    if (ul.children[i] !== row.el) ul.insertBefore(row.el, ul.children[i] || null);
    updateRow(row, a);
  });
  for (const [mid, row] of rows) {
    if (!seen.has(mid)) {
      row.el.remove();
      rows.delete(mid);
    }
  }

  const empty = list.length === 0;
  ul.hidden = empty;
  $("accounts-empty").hidden = !empty;
  $("run-all").hidden = empty;
  $("run-all").disabled = list.every((a) => a.running || !a.logged_in);

  const bad = list.filter((a) => !a.logged_in).length;
  $("acc-summary").textContent = empty
    ? "可以添加多个账号，每个账号各自排程。"
    : `${list.length} 个账号，各自排程、同时进行${bad ? `；${bad} 个需要重新扫码` : ""}`;
}

async function runAccount(mid, row) {
  row.run.disabled = true;
  try {
    await api("POST", `/api/accounts/${mid}/run`);
    setTimeout(refresh, 600);
  } catch (err) {
    if (err instanceof Unauthorized) return;
    row.err.textContent = err.message;
    row.run.disabled = false;
  }
}

async function removeAccount(mid, row) {
  if (!confirm(`退出「${row.label}」？\n服务器上的这份会话会在 B 站注销并删除，之后不再为这个账号运行。`)) return;
  row.remove.disabled = true;
  try {
    await api("DELETE", `/api/accounts/${mid}`);
    refresh();
  } catch (err) {
    if (err instanceof Unauthorized) return;
    $("acc-error").textContent = err.message;
    row.remove.disabled = false;
  }
}

$("run-all").addEventListener("click", async () => {
  $("run-all").disabled = true;
  $("acc-error").textContent = "";
  try {
    await api("POST", "/api/run");
    setTimeout(refresh, 600);
  } catch (err) {
    if (!(err instanceof Unauthorized)) $("acc-error").textContent = err.message;
  }
});

/* 设置 */

let settingsDirty = false;

function renderSettings(st) {
  if (settingsDirty) return;
  $("interval").value = st.interval;
  $("jitter").value = st.jitter;
  $("bvid").value = st.bvid;
  $("dynamic").checked = st.do_dynamic;
}

$("settings").addEventListener("input", () => {
  settingsDirty = true;
  $("saved").textContent = "";
});

$("settings").addEventListener("submit", async (e) => {
  e.preventDefault();
  const saved = $("saved");
  saved.classList.remove("is-error");
  try {
    await api("PUT", "/api/settings", {
      interval: $("interval").value.trim(),
      jitter: $("jitter").value.trim(),
      bvid: $("bvid").value.trim(),
      do_dynamic: $("dynamic").checked,
    });
    settingsDirty = false;
    saved.textContent = "已保存";
    refresh();
  } catch (err) {
    if (err instanceof Unauthorized) return;
    saved.classList.add("is-error");
    saved.textContent = err.message;
  }
});

/* 日志 */

let logSig = "";

function renderLogs(lines) {
  const sig = `${lines.length}|${lines[lines.length - 1] || ""}`;
  if (sig === logSig) return;
  logSig = sig;
  const box = $("log");
  const atBottom = box.scrollHeight - box.scrollTop - box.clientHeight < 24;
  const frag = document.createDocumentFragment();
  for (const line of lines) {
    const row = el("span", /失败|失效|错误/.test(line) ? "is-bad" : null);
    const m = /^\d{4}\/(\d\d\/\d\d \d\d:\d\d:\d\d) (?:(\[[^\]]*\]) )?(.*)$/.exec(line);
    if (m) {
      row.append(el("span", "t", `${m[1]} `));
      if (m[2]) row.append(el("span", "who", `${m[2]} `));
      row.append(m[3]);
    } else {
      row.append(line);
    }
    frag.append(row, "\n");
  }
  box.replaceChildren(frag);
  if (atBottom) box.scrollTop = box.scrollHeight;
}

/* 轮询状态 */

async function refresh() {
  const accErr = $("acc-error");
  try {
    const s = await (await api("GET", "/api/status")).json();
    renderZone(s);
    renderAccounts(s.accounts || []);
    renderSettings(s.settings);
    renderLogs(s.logs || []);
    $("version").textContent = s.version;
    if (accErr.dataset.conn) {
      accErr.textContent = "";
      delete accErr.dataset.conn;
    }
  } catch (err) {
    if (err instanceof Unauthorized) return;
    accErr.textContent = `连不上服务：${err.message}`;
    accErr.dataset.conn = "1";
  }
}

/* 扫码添加账号 */

const QR_TEXT = {
  waiting: "用 B 站 App 扫一扫",
  scanned: "已扫码，请在手机上确认登录",
  expired: "二维码已过期",
  success: "登录成功，已添加这个账号",
};

let qrPoll = null;
let qrURL = null;

function stopQR() {
  clearInterval(qrPoll);
  qrPoll = null;
}

function setQRState(text, ok) {
  $("qr-state").textContent = text;
  $("qr-state").classList.toggle("is-ok", Boolean(ok));
}

async function startQR() {
  stopQR();
  const frame = document.querySelector(".qr-frame");
  frame.classList.add("is-dim");
  $("qr-renew").hidden = true;
  setQRState("正在获取二维码");
  try {
    const blob = await (await api("POST", "/api/qr")).blob();
    if (qrURL) URL.revokeObjectURL(qrURL);
    qrURL = URL.createObjectURL(blob);
    $("qr-img").src = qrURL;
    frame.classList.remove("is-dim");
    setQRState(QR_TEXT.waiting);
    qrPoll = setInterval(pollQR, 2000);
  } catch (err) {
    if (err instanceof Unauthorized) { $("qr").close(); return; }
    setQRState(err.message);
    $("qr-renew").hidden = false;
  }
}

async function pollQR() {
  try {
    const { state } = await (await api("GET", "/api/qr")).json();
    if (QR_TEXT[state]) setQRState(QR_TEXT[state], state === "success");
    if (state === "expired" || state === "none") {
      stopQR();
      document.querySelector(".qr-frame").classList.add("is-dim");
      $("qr-renew").hidden = false;
    }
    if (state === "success") {
      stopQR();
      refresh();
      setTimeout(() => $("qr").close(), 1200);
    }
  } catch (err) {
    stopQR();
    if (err instanceof Unauthorized) { $("qr").close(); return; }
    setQRState(err.message);
    $("qr-renew").hidden = false;
  }
}

function openQR() {
  $("qr").showModal();
  startQR();
}

$("qr-open").addEventListener("click", openQR);
$("qr-open-empty").addEventListener("click", openQR);
$("qr-renew").addEventListener("click", startQR);
$("qr-close").addEventListener("click", () => $("qr").close());
$("qr").addEventListener("close", stopQR);

/* 启动：有有效会话就直接进面板 */

(async () => {
  try {
    await api("GET", "/api/status");
    showPanel();
  } catch (err) {
    if (!(err instanceof Unauthorized)) showGate();
  }
})();
