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

function icon(name) {
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("class", "icon");
  const use = document.createElementNS("http://www.w3.org/2000/svg", "use");
  use.setAttribute("href", `#i-${name}`);
  svg.append(use);
  return svg;
}

function toast(msg) {
  const t = el("div", "toast", msg);
  t.setAttribute("role", "status");
  $("toasts").append(t);
  while ($("toasts").children.length > 3) $("toasts").firstChild.remove();
  setTimeout(() => t.remove(), 4000);
}

/* 时间显示 */

const pad = (n) => String(n).padStart(2, "0");

function fmtTime(iso) {
  const d = new Date(iso);
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const day = (d - today) / 86400000;
  if (day >= 0 && day < 1) return `今天 ${hm}`;
  if (day >= 1 && day < 2) return `明天 ${hm}`;
  if (day < 0 && day >= -1) return `昨天 ${hm}`;
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

const PAGES = { overview: "概览", accounts: "账号", settings: "设置", logs: "日志" };
let timer = null;

function currentPage() {
  const p = location.hash.slice(1);
  return PAGES[p] ? p : "overview";
}

function showPage() {
  const page = currentPage();
  $("page-title").textContent = PAGES[page];
  document.title = `${PAGES[page]} · Bili IP Anchor`;
  for (const a of document.querySelectorAll(".nav a")) {
    if (a.dataset.page === page) a.setAttribute("aria-current", "page");
    else a.removeAttribute("aria-current");
  }
  for (const s of document.querySelectorAll(".page")) s.classList.toggle("is-active", s.dataset.page === page);
  $("page-actions").hidden = page !== "overview" && page !== "accounts";
  if (page === "logs") scrollLogToEnd();
}

window.addEventListener("hashchange", showPage);

function showGate() {
  clearInterval(timer);
  timer = null;
  $("panel").hidden = true;
  $("gate").hidden = false;
  $("password").focus();
}

function showPanel() {
  $("gate").hidden = true;
  $("panel").hidden = false;
  showPage();
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
    $("gate-error").textContent = err.message === "密码不对" ? "面板密码不对" : err.message;
  }
});

$("signout").addEventListener("click", async () => {
  try { await api("DELETE", "/api/session"); } catch {}
  showGate();
});

/* 概览 */

function zonePlace(z) {
  const parts = z.country === "中国" ? [z.province, z.city] : [z.country, z.province];
  return [...new Set(parts.filter(Boolean))].join(" ") || "未知地区";
}

function renderZone(s) {
  const place = $("place");
  const meta = $("place-meta");
  meta.replaceChildren();
  if (!s.zone) {
    place.textContent = s.zone_error ? "暂时查不到" : "查询中";
    place.classList.add("is-pending");
    if (s.zone_error) meta.append(el("span", "error small", `查询失败：${s.zone_error}`));
    return;
  }
  place.classList.remove("is-pending");
  place.textContent = zonePlace(s.zone);
  if (s.zone.isp) meta.append(el("span", "badge", s.zone.isp));
  if (s.zone.addr) meta.append(el("span", "badge mono", s.zone.addr));
}

function renderStats(list) {
  let ok = 0, bad = 0, failed = 0, next = null;
  for (const a of list) {
    if (!a.logged_in) bad++;
    else if (a.last && !a.last.ok) failed++;
    else if (a.last) ok++;
    if (a.next && (!next || a.next < next)) next = a.next;
  }
  $("stat-count").textContent = list.length;
  $("stat-ok").textContent = ok;
  $("stat-failed").textContent = `${failed} 个上次失败`;
  $("stat-bad").textContent = bad;
  $("stat-bad").classList.toggle("bad", bad > 0);
  $("stat-next").textContent = next ? fmtTime(next) : "—";
  $("stat-next-sub").textContent = next ? fmtRelative(next) : "没有排程";

  const badge = $("nav-badge");
  badge.hidden = list.length === 0;
  badge.textContent = bad || list.length;
  badge.classList.toggle("is-bad", bad > 0);
}

/* 账号列表 */

const rows = new Map();
const AVATAR_COLORS = ["#e11d48", "#db2777", "#c026d3", "#9333ea", "#4f46e5", "#0284c7", "#0891b2", "#0d9488", "#059669", "#65a30d", "#ca8a04", "#ea580c"];

function initials(name) {
  const words = name.trim().split(/\s+/);
  if (words.length > 1 && /^[\x00-\x7f]+$/.test(name)) return (words[0][0] + words[1][0]).toUpperCase();
  return Array.from(name.trim())[0] || "?";
}

function avatarColor(name) {
  let h = 0;
  for (const ch of name) h = (h * 31 + ch.codePointAt(0)) >>> 0;
  return AVATAR_COLORS[h % AVATAR_COLORS.length];
}

function faceURL(face) {
  if (!/^(https?:)?\/\/[a-z0-9.-]+\.hdslb\.com\//i.test(face)) return "";
  return face.replace(/^(https?:)?\/\//, "https://") + "@96w_96h_1c.webp";
}

function accountState(a) {
  if (a.running) return ["运行中", "var(--accent)"];
  if (!a.logged_in) return ["登录失效", "var(--danger)"];
  if (!a.last) return ["等待首次运行", "var(--muted)"];
  return a.last.ok ? ["正常", "var(--success)"] : ["上次失败", "var(--warning)"];
}

function buildRow(mid) {
  const img = el("img");
  img.alt = "";
  img.hidden = true;
  img.referrerPolicy = "no-referrer";
  const initial = el("span");
  img.addEventListener("load", () => { img.hidden = false; initial.hidden = true; });
  img.addEventListener("error", () => { img.hidden = true; initial.hidden = false; });
  const avatar = el("span", "avatar");
  avatar.append(img, initial);

  const name = el("span", "acc-name");
  const state = el("span", "badge state");
  const nameLine = el("div", "acc-name-line");
  nameLine.append(name, state);
  const uid = el("span", "muted tiny", `UID ${mid}`);
  const who = el("div", "acc-who");
  who.append(nameLine, uid);
  const id = el("div", "acc-id");
  id.append(avatar, who);

  const last = el("dd");
  const next = el("dd");
  const times = el("dl", "acc-times");
  times.append(el("dt", null, "上次运行"), last, el("dt", null, "下次运行"), next);

  const relogin = el("button", "primary", "重新扫码");
  const run = el("button", null, "立即运行");
  const remove = el("button", "danger", "退出");
  for (const b of [relogin, run, remove]) b.type = "button";
  const actions = el("div", "acc-actions");
  actions.append(relogin, run, remove);

  const main = el("div", "acc-main");
  main.append(id, times, actions);
  const err = el("p", "acc-error");
  const li = el("li", "acc");
  li.append(main, err);

  const row = { el: li, avatar, img, initial, name, uid, state, last, next, relogin, run, remove, err, face: null, label: "" };
  relogin.addEventListener("click", openQR);
  run.addEventListener("click", () => runAccount(mid, row));
  remove.addEventListener("click", () => askRemove(mid, row));
  return row;
}

function updateRow(row, a) {
  row.label = a.uname || `UID ${a.mid}`;
  row.name.textContent = row.label;
  row.name.title = row.label;
  row.uid.hidden = !a.uname;
  row.initial.textContent = initials(row.label);
  row.avatar.style.background = avatarColor(row.label);

  const face = faceURL(a.face || "");
  if (face !== row.face) {
    row.face = face;
    row.img.hidden = true;
    row.initial.hidden = false;
    if (face) row.img.src = face;
    else row.img.removeAttribute("src");
  }

  const [text, color] = accountState(a);
  row.state.textContent = text;
  row.state.style.setProperty("--c", color);

  row.last.replaceChildren();
  if (a.last) row.last.append(`${fmtTime(a.last.at)} `, el("span", a.last.ok ? "ok" : "bad", a.last.ok ? "成功" : "失败"));
  else row.last.textContent = "还没有";
  row.next.textContent = a.running ? "正在运行" : a.next ? `${fmtTime(a.next)}（${fmtRelative(a.next)}）` : "—";

  if (!a.logged_in) row.err.textContent = "这个账号的会话已失效，点「重新扫码」用同一个账号登录即可恢复。";
  else row.err.textContent = a.last && !a.last.ok ? a.last.error || "" : "";

  row.relogin.hidden = a.logged_in;
  row.run.hidden = !a.logged_in;
  row.run.disabled = a.running || row.busy;
  row.run.textContent = a.running ? "运行中" : "立即运行";
  row.remove.disabled = Boolean(row.busy);
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
  for (const box of document.querySelectorAll("[data-empty]")) {
    if (box.hidden !== !empty) box.hidden = !empty;
    if (empty && !box.firstChild) {
      box.append($("empty-tpl").content.cloneNode(true));
      box.querySelector("[data-qr]").addEventListener("click", openQR);
    }
  }
  $("run-all").hidden = empty;
  $("run-all").disabled = !list.some((a) => a.logged_in && !a.running);
}

async function runAccount(mid, row) {
  row.busy = true;
  row.run.disabled = true;
  try {
    await api("POST", `/api/accounts/${mid}/run`);
  } catch (err) {
    if (!(err instanceof Unauthorized)) toast(err.message);
  }
  row.busy = false;
  refresh();
}

let removing = null;

function askRemove(mid, row) {
  removing = { mid, row };
  $("confirm-title").textContent = `退出「${row.label}」？`;
  $("confirm").showModal();
  $("confirm-ok").focus();
}

$("confirm-cancel").addEventListener("click", () => $("confirm").close());
$("confirm-ok").addEventListener("click", async () => {
  $("confirm").close();
  const { mid, row } = removing;
  row.busy = true;
  row.remove.disabled = true;
  try {
    await api("DELETE", `/api/accounts/${mid}`);
    toast(`已退出 ${row.label}`);
  } catch (err) {
    if (!(err instanceof Unauthorized)) toast(err.message);
  }
  row.busy = false;
  refresh();
});

$("run-all").addEventListener("click", async () => {
  $("run-all").disabled = true;
  try {
    const { triggered } = await (await api("POST", "/api/run")).json();
    toast(`已让 ${triggered} 个账号立即运行`);
  } catch (err) {
    if (!(err instanceof Unauthorized)) toast(err.message);
  }
  setTimeout(refresh, 600);
});

/* 设置 */

let serverSettings = null;
let formEdited = false;

function durationError(s, minMinutes) {
  const m = /^(?:(\d+(?:\.\d+)?)h)?(?:(\d+(?:\.\d+)?)m)?(?:(\d+(?:\.\d+)?)s)?$/.exec(s.trim());
  if (!s.trim() || !m || (!m[1] && !m[2] && !m[3])) return "写成 90m、6h 这样";
  const min = (+m[1] || 0) * 60 + (+m[2] || 0) + (+m[3] || 0) / 60;
  return min < minMinutes ? `最少 ${minMinutes}m` : "";
}

function formValue() {
  return {
    interval: $("interval").value.trim(),
    jitter: $("jitter").value.trim(),
    bvid: $("bvid").value.trim(),
    do_dynamic: $("dynamic").checked,
  };
}

function validateForm() {
  const v = formValue();
  const errors = {
    interval: durationError(v.interval, 10),
    jitter: v.jitter === "0" ? "" : durationError(v.jitter, 0),
    bvid: /^BV[0-9A-Za-z]{10}$/.test(v.bvid) ? "" : "BV 号是 BV 开头的 12 位",
  };
  for (const [k, msg] of Object.entries(errors)) {
    document.querySelector(`[data-error="${k}"]`).textContent = msg;
    $(k).setAttribute("aria-invalid", msg ? "true" : "false");
  }
  const invalid = Object.values(errors).some(Boolean);
  $("save").disabled = !formEdited || invalid;
  $("reset").disabled = !formEdited;
}

function fillForm(st) {
  $("interval").value = st.interval;
  $("jitter").value = st.jitter;
  $("bvid").value = st.bvid;
  $("dynamic").checked = st.do_dynamic;
  formEdited = false;
  $("form-error").textContent = "";
  validateForm();
}

function renderSettings(st) {
  serverSettings = st;
  if (!formEdited) fillForm(st);
}

$("settings").addEventListener("input", () => {
  formEdited = true;
  validateForm();
});

$("reset").addEventListener("click", () => serverSettings && fillForm(serverSettings));

$("settings").addEventListener("submit", async (e) => {
  e.preventDefault();
  $("save").disabled = true;
  $("save").textContent = "正在保存";
  $("form-error").textContent = "";
  try {
    const saved = await (await api("PUT", "/api/settings", formValue())).json();
    fillForm(saved);
    toast("设置已保存");
    refresh();
  } catch (err) {
    if (!(err instanceof Unauthorized)) $("form-error").textContent = err.message;
    validateForm();
  }
  $("save").textContent = "保存设置";
});

/* 日志 */

let logSig = "";

function scrollLogToEnd() {
  $("log").scrollTop = $("log").scrollHeight;
}

function renderLogs(lines) {
  const sig = `${lines.length}|${lines[lines.length - 1] || ""}`;
  if (sig === logSig) return;
  logSig = sig;
  const box = $("log");
  const atBottom = box.scrollHeight - box.scrollTop - box.clientHeight < 24;
  const frag = document.createDocumentFragment();
  if (!lines.length) frag.append(el("span", "t", "还没有日志"));
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
  if (atBottom) scrollLogToEnd();
}

/* 轮询状态 */

async function refresh() {
  try {
    const s = await (await api("GET", "/api/status")).json();
    renderZone(s);
    renderStats(s.accounts || []);
    renderAccounts(s.accounts || []);
    renderSettings(s.settings);
    renderLogs(s.logs || []);
    $("version").textContent = `bili-ip-anchor ${s.version}`;
    $("conn-dot").className = "dot ok";
    $("conn-text").textContent = `已连接 · ${s.version}`;
    $("banner").hidden = true;
  } catch (err) {
    if (err instanceof Unauthorized) return;
    $("conn-dot").className = "dot bad";
    $("conn-text").textContent = "连接中断，正在重试";
    $("banner").textContent = `连不上服务，显示的是之前的数据：${err.message}`;
    $("banner").hidden = false;
  }
}

/* 扫码添加账号 */

const QR_TEXT = {
  waiting: "用 B 站 App 扫一扫",
  scanned: "已扫码，请在手机上确认登录",
  expired: "二维码已过期",
};

let qrPoll = null;
let qrURL = null;

function stopQR() {
  clearInterval(qrPoll);
  qrPoll = null;
}

function setQRState(text, kind) {
  $("qr-state").textContent = text;
  $("qr-state").className = kind ? `is-${kind}` : "";
}

function qrFailed(text, kind) {
  stopQR();
  setQRState(text, kind);
  document.querySelector(".qr-frame").classList.add("is-dim");
  $("qr-renew").hidden = false;
}

async function startQR() {
  stopQR();
  document.querySelector(".qr-frame").classList.add("is-dim");
  $("qr-renew").hidden = true;
  setQRState("正在获取二维码");
  try {
    const blob = await (await api("POST", "/api/qr")).blob();
    if (qrURL) URL.revokeObjectURL(qrURL);
    qrURL = URL.createObjectURL(blob);
    $("qr-img").src = qrURL;
    document.querySelector(".qr-frame").classList.remove("is-dim");
    setQRState(QR_TEXT.waiting);
    qrPoll = setInterval(pollQR, 2000);
  } catch (err) {
    if (err instanceof Unauthorized) { $("qr").close(); return; }
    qrFailed(err.message, "error");
  }
}

async function pollQR() {
  try {
    const { state, mid } = await (await api("GET", "/api/qr")).json();
    if (state === "success") {
      stopQR();
      $("qr").close();
      toast(`已添加账号 UID ${mid}，正在运行第一轮`);
      location.hash = "#accounts";
      refresh();
    } else if (state === "expired" || state === "none") {
      qrFailed(QR_TEXT.expired);
    } else if (QR_TEXT[state]) {
      setQRState(QR_TEXT[state], state === "scanned" ? "scanned" : "");
    }
  } catch (err) {
    if (err instanceof Unauthorized) { $("qr").close(); return; }
    qrFailed(err.message, "error");
  }
}

function openQR() {
  $("qr").showModal();
  startQR();
}

$("qr-open").addEventListener("click", openQR);
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
