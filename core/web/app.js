"use strict";

const $ = (id) => document.getElementById(id);

class Unauthorized extends Error {}

// body 是对象时按 JSON 发；raw 为 true 时原样发（上传图片）
async function api(method, path, body, raw = false) {
  const headers = { "X-Requested-With": "bia" };
  if (body !== undefined && !raw) headers["Content-Type"] = "application/json";
  if (raw && body.type) headers["Content-Type"] = body.type;
  const res = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : raw ? body : JSON.stringify(body),
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

// report 把错误显示成提示条，面板被锁时不提示
function report(err) {
  if (!(err instanceof Unauthorized)) toast(err.message);
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

function button(text, cls, iconName) {
  const b = el("button", cls);
  b.type = "button";
  if (iconName) b.append(icon(iconName));
  const label = el("span", null, text);
  b.append(label);
  b.label = label;
  return b;
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

function fmtDate(iso) {
  const d = new Date(iso);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

function fmtRelative(iso) {
  const sec = Math.round((new Date(iso) - Date.now()) / 1000);
  const abs = Math.abs(sec);
  let text;
  if (abs < 60) text = "不到 1 分钟";
  else if (abs < 3600) text = `${Math.round(abs / 60)} 分钟`;
  else if (abs < 86400 * 2) {
    const h = Math.floor(abs / 3600);
    const m = Math.round((abs % 3600) / 60);
    text = m ? `${h} 小时 ${m} 分钟` : `${h} 小时`;
  } else text = `${Math.floor(abs / 86400)} 天`;
  return sec >= 0 ? `${text}后` : `${text}前`;
}

function fmtDuration(sec) {
  const m = Math.floor(sec / 60);
  return m >= 60 ? `${Math.floor(m / 60)}:${pad(m % 60)}:${pad(sec % 60)}` : `${m}:${pad(sec % 60)}`;
}

function fmtCount(n) {
  if (n >= 1e8) return `${(n / 1e8).toFixed(1)} 亿`;
  if (n >= 1e4) return `${(n / 1e4).toFixed(1)} 万`;
  return String(n);
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

/* 确认对话框 */

let confirmAction = null;

function askConfirm(title, text, ok, action) {
  $("confirm-title").textContent = title;
  $("confirm-text").textContent = text;
  $("confirm-ok").textContent = ok;
  confirmAction = action;
  $("confirm").showModal();
  $("confirm-ok").focus();
}

$("confirm-cancel").addEventListener("click", () => $("confirm").close());
$("confirm-ok").addEventListener("click", () => {
  $("confirm").close();
  if (confirmAction) confirmAction();
});

/* 概览和侧边栏 */

let status = null;

function zonePlace(z) {
  const parts = z.country === "中国" ? [z.province, z.city] : [z.country, z.province];
  return [...new Set(parts.filter(Boolean))].join(" ") || "未知地区";
}

// 主页属地显示的那一级：国内是省份，国外是国家
function zoneRegion(z) {
  return z.country === "中国" && z.province ? z.province : z.country;
}

function targetRegion() {
  return status && status.zone ? zoneRegion(status.zone) : "";
}

function whereLabel(s) {
  const who = s.mode === "local" ? "本机" : "这台服务器";
  return s.settings.proxy ? `${who}（经代理）` : who;
}

function renderZone(s) {
  $("hero-label").textContent = `B 站把${whereLabel(s)}识别为`;
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
  if (s.zone_checked_at) meta.append(el("span", "muted tiny", `查询于 ${fmtTime(s.zone_checked_at)}`));
}

function renderSide(s) {
  $("conn-dot").className = "dot ok";
  $("conn-text").textContent = `${s.mode === "local" ? "本机运行" : "已连接"} · ${s.version}`;
  $("conn-host").textContent = s.mode === "local" ? "在这台设备上运行" : location.host;
  // 本机模式（手机 App）的面板密码是随机的，锁上就进不来了
  $("signout").hidden = s.mode === "local";
  $("conn-mode-icon").setAttribute("href", s.mode === "local" ? "#i-laptop" : "#i-server");
  let egress = s.zone ? `${s.zone.addr} · ${zoneRegion(s.zone)}` : s.zone_error ? "出口 IP 查询失败" : "出口 IP 查询中";
  if (s.settings.proxy) egress = `经代理 ${egress}`;
  $("conn-egress").textContent = egress;
  $("conn-egress").parentElement.title = s.zone ? `B 站看到的出口 IP：${s.zone.addr}（${zonePlace(s.zone)} ${s.zone.isp || ""}）` : "B 站看到的出口 IP";
}

function renderStats(list) {
  let bad = 0, authed = 0, matched = 0, next = null;
  for (const a of list) {
    if (!a.logged_in || (a.last && !a.last.ok)) bad++;
    if (a.app_auth) {
      authed++;
      if (a.home && a.home.matched_since) matched++;
    }
    if (a.next && (!next || a.next < next)) next = a.next;
  }
  $("stat-count").textContent = list.length;
  $("stat-match").textContent = authed ? `${matched} / ${authed}` : "—";
  $("stat-match-sub").textContent = authed ? `${authed} 个账号已授权查询` : "授权查询后显示";
  $("stat-bad").textContent = bad;
  $("stat-bad").classList.toggle("bad", bad > 0);
  $("stat-next").textContent = next ? fmtTime(next) : "—";
  $("stat-next-sub").textContent = next ? fmtRelative(next) : "没有排程";

  const notLoggedIn = list.filter((a) => !a.logged_in).length;
  const badge = $("nav-badge");
  badge.hidden = list.length === 0;
  badge.textContent = notLoggedIn || list.length;
  badge.classList.toggle("is-bad", notLoggedIn > 0);
}

/* 账号 */

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

// hdslbURL 只接受 B 站图床的图片，加上缩略图参数
function hdslbURL(src, size) {
  if (!/^(https?:)?\/\/[a-z0-9.-]+\.hdslb\.com\//i.test(src || "")) return "";
  return src.replace(/^(https?:)?\/\//, "https://") + size;
}

function accountState(a) {
  if (a.running) return ["运行中", "var(--accent)"];
  if (a.busy) return [`正在${a.busy}`, "var(--accent)"];
  if (!a.logged_in) return ["登录失效", "var(--danger)"];
  if (!a.last) return ["等待首次运行", "var(--muted)"];
  return a.last.ok ? ["正常", "var(--success)"] : ["上次失败", "var(--warning)"];
}

// tile 是账号卡片里的一格：标题、主要内容、说明和按钮
function tile(title, iconName) {
  const box = el("div", "tile");
  const head = el("p", "tile-title");
  head.append(icon(iconName), title);
  const value = el("p", "tile-value");
  const sub = el("p", "tile-sub");
  const actions = el("div", "tile-actions");
  box.append(head, value, sub, actions);
  return { box, value, sub, actions };
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
  const chips = el("div", "chips");
  const who = el("div", "acc-who");
  who.append(nameLine, chips);

  const relogin = button("重新扫码", "primary");
  const run = button("立即运行", null, "play");
  const check = button("刷新资料", "icon-only", "refresh");
  check.title = "刷新资料：只查询硬币、Cookie 和主页属地，不发心跳";
  const remove = button("退出", "icon-only danger", "logout");
  remove.title = "退出这个账号";
  const actions = el("div", "acc-actions");
  actions.append(relogin, run, check, remove);

  const head = el("div", "acc-head");
  head.append(avatar, who, actions);

  const home = tile("主页属地", "pin");
  const target = tile("目标属地", "globe");
  const times = tile("运行", "play");
  const cookie = tile("Cookie", "cookie");
  const grid = el("div", "acc-grid");
  grid.append(home.box, target.box, times.box, cookie.box);

  const authBtn = button("授权查询", "small", "shield");
  authBtn.title = "用 B 站 App 再扫一次码，授权本程序查询这个账号主页显示的 IP 属地";
  const unauthBtn = button("取消授权", "small link");
  home.actions.append(authBtn, unauthBtn);
  const cookieBtn = button("刷新 Cookie", "small", "refresh");
  cookie.actions.append(cookieBtn);

  const err = el("p", "acc-error");
  const li = el("li", "card acc");
  li.append(head, grid, err);

  const row = { el: li, avatar, img, initial, name, state, chips, relogin, run, check, remove, home, target, times, cookie, authBtn, unauthBtn, cookieBtn, err, face: null, label: "", mid };
  relogin.addEventListener("click", () => openQR());
  run.addEventListener("click", () => accountAction(row, "POST", "run"));
  check.addEventListener("click", () => accountAction(row, "POST", "check", "资料已刷新"));
  cookieBtn.addEventListener("click", () => accountAction(row, "POST", "cookie/refresh", "Cookie 已刷新"));
  authBtn.addEventListener("click", () => openQR({ mid, label: row.label }));
  unauthBtn.addEventListener("click", () => askConfirm(`取消「${row.label}」的主页属地授权？`, "会在 B 站撤销这份 App 授权，之后不再显示主页属地。不影响定时任务。", "取消授权",
    () => accountAction(row, "DELETE", "appauth", "已取消授权")));
  remove.addEventListener("click", () => askConfirm(`退出「${row.label}」？`, "这份会话会在 B 站注销并删除，之后不再为这个账号运行。", "退出",
    () => accountAction(row, "DELETE", "", `已退出 ${row.label}`)));
  return row;
}

async function accountAction(row, method, action, done) {
  row.busy = true;
  setRowButtons(row);
  try {
    await api(method, `/api/accounts/${row.mid}${action ? "/" + action : ""}`);
    if (done) toast(done);
  } catch (err) {
    report(err);
  }
  row.busy = false;
  refresh();
}

function setRowButtons(row) {
  const a = row.data;
  if (!a) return;
  const busy = row.busy || a.running || Boolean(a.busy);
  row.relogin.hidden = a.logged_in;
  row.run.hidden = !a.logged_in;
  row.run.disabled = busy;
  row.run.label.textContent = a.running ? "运行中" : "立即运行";
  row.check.disabled = busy || !a.logged_in;
  row.remove.disabled = row.busy;
  row.cookieBtn.disabled = busy || !a.logged_in || !a.cookie.can_refresh;
  row.cookieBtn.label.textContent = a.busy === "刷新 Cookie" ? "正在刷新" : "刷新 Cookie";
  row.cookieBtn.title = a.cookie.can_refresh ? "现在就换一份新的 Cookie" : "这个账号是旧版本扫码登录的，重新扫码一次之后才能刷新";
  row.authBtn.hidden = a.app_auth;
  row.authBtn.disabled = busy || !a.logged_in;
  row.unauthBtn.hidden = !a.app_auth;
  row.unauthBtn.disabled = busy;
}

function chip(text, cls) {
  return el("span", `chip ${cls || ""}`, text);
}

function updateRow(row, a) {
  row.data = a;
  row.label = a.uname || `UID ${a.mid}`;
  row.name.textContent = row.label;
  row.name.title = row.label;
  row.initial.textContent = initials(row.label);
  row.avatar.style.background = avatarColor(row.label);

  const face = hdslbURL(a.face, "@96w_96h_1c.webp");
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

  const chips = [chip(`UID ${a.mid}`, "mono")];
  if (a.stats) {
    chips.push(chip(`Lv${a.stats.level}`, `lv lv${a.stats.level}`));
    if (a.stats.vip) chips.push(chip(a.stats.vip, "vip"));
    chips.push(chip(`硬币 ${a.stats.coins}`));
    if (a.stats.bcoins) chips.push(chip(`B币 ${a.stats.bcoins}`));
  }
  row.chips.replaceChildren(...chips);

  // 目标属地
  const target = targetRegion();
  const z = status && status.zone;
  row.target.value.textContent = z ? zoneRegion(z) : "查询中";
  row.target.value.classList.toggle("is-pending", !z);
  row.target.sub.textContent = z ? [z.addr, z.isp].filter(Boolean).join(" · ") : "";

  // 主页属地
  const h = a.home;
  const hv = row.home.value;
  hv.replaceChildren();
  hv.classList.remove("is-pending");
  let homeSub = "";
  if (!a.app_auth) {
    hv.textContent = "未授权查询";
    hv.classList.add("is-pending");
    homeSub = "B 站只在 App 接口里提供主页属地，需要额外授权一次才能显示。";
  } else if (!h || (!h.text && !h.error)) {
    hv.textContent = "查询中";
    hv.classList.add("is-pending");
  } else if (h.error && !h.text) {
    hv.textContent = "查不到";
    hv.classList.add("is-pending");
    homeSub = h.error;
  } else {
    hv.append(h.text);
    if (h.matched_since) {
      hv.append(el("span", "badge state match", "已是目标属地"));
      homeSub = `${fmtTime(h.matched_since)} 起一致`;
    } else if (target) {
      hv.append(el("span", "badge state differ", `还不是${target}`));
      const next = a.next ? `下一轮（${fmtTime(a.next)}）` : "下一轮";
      homeSub = `预计${next}之后几小时内变为${target}，B 站没有公开具体规则`;
    }
    if (h.error) homeSub = `上次查询失败：${h.error}`;
    else if (h.checked_at) homeSub += `${homeSub ? "。" : ""}查询于 ${fmtTime(h.checked_at)}`;
  }
  row.home.sub.textContent = homeSub;

  // 运行
  const tv = row.times.value;
  if (a.running) tv.textContent = "正在运行";
  else if (a.next) tv.textContent = `下次 ${fmtTime(a.next)}`;
  else tv.textContent = "—";
  const lines = [];
  if (a.next && !a.running) lines.push(fmtRelative(a.next));
  if (a.last) lines.push(`上次 ${fmtTime(a.last.at)} ${a.last.ok ? "成功" : "失败"}`);
  if (a.last_ok) lines.push(`上次从目标 IP 发出请求：${fmtTime(a.last_ok)}`);
  row.times.sub.textContent = lines.join(" · ") || "还没有运行过";

  // Cookie
  const cv = row.cookie.value;
  cv.classList.remove("bad", "warn", "is-pending");
  if (a.cookie.expires) {
    const days = Math.ceil((new Date(a.cookie.expires) - Date.now()) / 86400000);
    cv.textContent = days > 0 ? `还有 ${days} 天` : "已过期";
    if (days <= 0) cv.classList.add("bad");
    else if (days < 14) cv.classList.add("warn");
    row.cookie.sub.textContent = `到 ${fmtDate(a.cookie.expires)}${a.cookie.can_refresh ? "，B 站要求时会自动刷新" : ""}`;
  } else {
    cv.textContent = "有效期未知";
    cv.classList.add("is-pending");
    row.cookie.sub.textContent = a.cookie.can_refresh ? "下次运行时会查询" : "旧版本扫码登录的账号，重新扫码后可以看到有效期并刷新";
  }

  if (!a.logged_in) row.err.textContent = "这个账号的会话已失效，点「重新扫码」用同一个账号登录即可恢复。";
  else row.err.textContent = a.last && !a.last.ok ? a.last.error || "" : "";
  setRowButtons(row);
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
      box.querySelector("[data-qr]").addEventListener("click", () => openQR());
    }
  }
  $("run-all").hidden = empty;
  $("run-all").disabled = !list.some((a) => a.logged_in && !a.running && !a.busy);
}

$("run-all").addEventListener("click", async () => {
  $("run-all").disabled = true;
  try {
    const { triggered } = await (await api("POST", "/api/run")).json();
    toast(`已让 ${triggered} 个账号立即运行`);
  } catch (err) {
    report(err);
  }
  setTimeout(refresh, 600);
});

/* 设置 */

let serverSettings = null;
let formEdited = false;
let formVideos = [];

function durationError(s, minMinutes) {
  const m = /^(?:(\d+(?:\.\d+)?)h)?(?:(\d+(?:\.\d+)?)m)?(?:(\d+(?:\.\d+)?)s)?$/.exec(s.trim());
  if (!s.trim() || !m || (!m[1] && !m[2] && !m[3])) return "写成 90m、6h 这样";
  const min = (+m[1] || 0) * 60 + (+m[2] || 0) + (+m[3] || 0) / 60;
  return min < minMinutes ? `最少 ${minMinutes}m` : "";
}

function proxyError(s) {
  s = s.trim();
  if (!s) return "";
  const m = /^(?:(https?|socks5h?):\/\/)?(?:[^@/\s]+@)?[^:/\s]+:\d{1,5}\/?$/.exec(s);
  if (!m) return "写成 http://127.0.0.1:7890 或 socks5://127.0.0.1:7891";
  return "";
}

function formValue() {
  return {
    interval: $("interval").value.trim(),
    jitter: $("jitter").value.trim(),
    proxy: $("proxy").value.trim(),
    videos: formVideos.slice(),
    do_dynamic: $("dynamic").checked,
    dynamic_text: $("dynamic-text").value.trim(),
  };
}

function validateForm() {
  const v = formValue();
  const errors = {
    interval: durationError(v.interval, 10),
    jitter: v.jitter === "0" ? "" : durationError(v.jitter, 0),
    proxy: proxyError(v.proxy),
    videos: v.videos.length ? "" : "至少要有一个心跳视频",
    dynamic_text: "",
  };
  for (const [k, msg] of Object.entries(errors)) {
    document.querySelector(`[data-error="${k}"]`).textContent = msg;
    const input = { videos: "video-add", dynamic_text: "dynamic-text" }[k] || k;
    $(input).setAttribute("aria-invalid", msg ? "true" : "false");
  }
  const invalid = Object.values(errors).some(Boolean);
  $("save").disabled = !formEdited || invalid;
  $("reset").disabled = !formEdited;
  renderPreview();
}

function fillForm(st) {
  $("interval").value = st.interval;
  $("jitter").value = st.jitter;
  $("proxy").value = st.proxy || "";
  $("dynamic").checked = st.do_dynamic;
  $("dynamic-text").value = st.dynamic_text || "";
  formVideos = (st.videos || []).slice();
  formEdited = false;
  $("form-error").textContent = "";
  renderVideos();
  validateForm();
}

function markEdited() {
  formEdited = true;
  validateForm();
}

function renderSettings(st) {
  serverSettings = st;
  if (!formEdited) fillForm(st);
}

$("settings").addEventListener("input", (e) => {
  if (e.target.id === "video-add" || e.target.id === "image-file") return;
  if (e.target.id === "proxy") $("proxy-result").textContent = "";
  markEdited();
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

/* 代理测试 */

$("proxy-test").addEventListener("click", async () => {
  const proxy = $("proxy").value.trim();
  const out = $("proxy-result");
  if (!proxy) {
    out.className = "tiny muted";
    out.textContent = "留空表示直连，不需要测试。";
    return;
  }
  $("proxy-test").disabled = true;
  out.className = "tiny muted";
  out.textContent = "正在经过代理访问 B 站…";
  try {
    const z = await (await api("POST", "/api/proxy/test", { proxy })).json();
    out.className = "tiny ok";
    out.textContent = `可用。B 站看到的是 ${zonePlace(z)}${z.isp ? " · " + z.isp : ""} · ${z.addr}，主页属地会变成${zoneRegion(z)}。`;
  } catch (err) {
    if (err instanceof Unauthorized) return;
    out.className = "tiny error";
    out.textContent = err.message;
  }
  $("proxy-test").disabled = false;
});

/* 心跳视频 */

const videoInfo = new Map(); // BV 号 → 视频信息，或 { error }

async function loadVideo(bvid) {
  if (videoInfo.has(bvid)) return videoInfo.get(bvid);
  let info;
  try {
    info = await (await api("GET", `/api/videos/${bvid}`)).json();
  } catch (err) {
    if (err instanceof Unauthorized) throw err;
    info = { bvid, error: err.message };
  }
  videoInfo.set(bvid, info);
  return info;
}

function renderVideos() {
  const ul = $("videos");
  ul.replaceChildren();
  formVideos.forEach((bvid) => {
    const li = el("li", "video");
    const cover = el("span", "cover");
    const body = el("div", "video-body");
    const title = el("a", "video-title", bvid);
    title.href = `https://www.bilibili.com/video/${bvid}`;
    title.target = "_blank";
    title.rel = "noopener noreferrer";
    const meta = el("p", "muted tiny");
    meta.append(el("span", "mono", bvid));
    body.append(title, meta);
    const remove = button("移除", "icon-only small", "trash");
    remove.title = "移除这个视频";
    remove.disabled = formVideos.length <= 1;
    remove.addEventListener("click", () => {
      formVideos = formVideos.filter((v) => v !== bvid);
      renderVideos();
      markEdited();
    });
    li.append(cover, body, remove);
    ul.append(li);

    loadVideo(bvid).then((info) => {
      if (info.error) {
        meta.append(el("span", "error", ` · ${info.error}`));
        return;
      }
      title.textContent = info.title;
      title.title = info.title;
      meta.append(` · ${info.owner} · ${fmtDuration(info.duration)} · ${fmtCount(info.views)} 播放`);
      const src = hdslbURL(info.cover, "@160w_100h_1c.webp");
      if (src) {
        const img = el("img");
        img.alt = "";
        img.referrerPolicy = "no-referrer";
        img.src = src;
        cover.append(img);
      }
    }).catch(() => {});
  });
}

async function addVideos() {
  const input = $("video-add");
  const found = [...new Set(input.value.match(/BV[0-9A-Za-z]{10}/g) || [])];
  const errBox = document.querySelector('[data-error="videos"]');
  if (!found.length) {
    errBox.textContent = "没找到 BV 号。粘贴视频链接或 BV 开头的 12 位编号";
    return;
  }
  $("video-add-btn").disabled = true;
  const bad = [];
  for (const bvid of found) {
    if (formVideos.includes(bvid)) continue;
    if (formVideos.length >= 20) { bad.push("最多 20 个视频"); break; }
    try {
      const info = await loadVideo(bvid);
      if (info.error) { bad.push(`${bvid}：${info.error}`); continue; }
    } catch { break; }
    formVideos.push(bvid);
  }
  $("video-add-btn").disabled = false;
  input.value = "";
  renderVideos();
  markEdited();
  if (bad.length) errBox.textContent = bad.join("；");
}

$("video-add-btn").addEventListener("click", addVideos);
$("video-add").addEventListener("keydown", (e) => {
  if (e.key === "Enter") {
    e.preventDefault();
    addVideos();
  }
});

/* 动态图片 */

let images = [];
let imagesSig = "";

function renderImages(list) {
  images = list;
  const sig = list.map((i) => i.name).join(",");
  if (sig === imagesSig) return;
  imagesSig = sig;
  const ul = $("images");
  ul.replaceChildren();
  for (const img of list) {
    const li = el("li", "thumb");
    const pic = el("img");
    pic.alt = "动态图片";
    pic.loading = "lazy";
    pic.src = `/api/dynamic/images/${img.name}`;
    const del = button("删除", "thumb-del icon-only", "trash");
    del.title = "删除这张图片";
    del.addEventListener("click", async () => {
      del.disabled = true;
      try {
        await api("DELETE", `/api/dynamic/images/${img.name}`);
      } catch (err) {
        report(err);
      }
      refresh();
    });
    li.append(pic, del);
    ul.append(li);
  }
  if (list.length < 9) {
    const li = el("li", "thumb add");
    const add = button("添加图片", null, "image");
    add.addEventListener("click", () => $("image-file").click());
    li.append(add);
    ul.append(li);
  }
  renderPreview();
}

$("image-file").addEventListener("change", async (e) => {
  const files = [...e.target.files];
  e.target.value = "";
  for (const f of files) {
    try {
      await api("POST", "/api/dynamic/images", f, true);
    } catch (err) {
      report(err);
      if (err instanceof Unauthorized) return;
    }
  }
  if (files.length) toast(`已添加 ${files.length} 张图片`);
  refresh();
});

function renderPreview() {
  const box = $("dynamic-preview");
  const text = $("dynamic-text").value.trim();
  const n = images.length;
  if (!$("dynamic").checked) {
    box.textContent = "现在不发动态，只上报心跳。";
    return;
  }
  const parts = [];
  if (text) parts.push(`文字「${text.length > 24 ? text.slice(0, 24) + "…" : text}」`);
  if (n) parts.push(`${n} 张图片`);
  box.textContent = parts.length ? `每轮会发：${parts.join(" + ")}，几秒后删掉。` : "没有文字也没有图片时，每轮发一张内置图片，几秒后删掉。";
}

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
    status = s;
    renderZone(s);
    renderSide(s);
    renderStats(s.accounts || []);
    renderAccounts(s.accounts || []);
    renderSettings(s.settings);
    renderImages(s.images || []);
    renderLogs(s.logs || []);
    $("version").textContent = `bili-ip-anchor ${s.version}`;
    $("banner").hidden = true;
  } catch (err) {
    if (err instanceof Unauthorized) return;
    $("conn-dot").className = "dot bad";
    $("conn-text").textContent = "连接中断，正在重试";
    $("banner").textContent = `连不上服务，显示的是之前的数据：${err.message}`;
    $("banner").hidden = false;
  }
}

/* 扫码：添加账号，或授权查询某个账号的主页属地 */

const QR_TEXT = {
  waiting: "用 B 站 App 扫一扫",
  scanned: "已扫码，请在手机上确认",
  expired: "二维码已过期",
};

let qrPoll = null;
let qrURL = null;
let qrTarget = null; // null 表示添加账号，否则是 { mid, label }

function qrPath() {
  return qrTarget ? `/api/accounts/${qrTarget.mid}/appauth` : "/api/qr";
}

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
    const blob = await (await api("POST", qrPath())).blob();
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
    const { state, mid } = await (await api("GET", qrPath())).json();
    if (state === "success") {
      stopQR();
      $("qr").close();
      if (qrTarget) toast(`已授权，正在查询「${qrTarget.label}」的主页属地`);
      else {
        toast(`已添加账号 UID ${mid}，正在运行第一轮`);
        location.hash = "#accounts";
      }
      refresh();
      setTimeout(refresh, 3000);
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

function openQR(target = null) {
  qrTarget = target;
  if (target) {
    $("qr-title").textContent = "授权查询主页属地";
    $("qr-desc").textContent = `用「${target.label}」的 B 站 App 扫码并确认。这份授权只用来查询这个账号主页显示的 IP 属地，随时可以取消。`;
  } else {
    $("qr-title").textContent = "添加 B 站账号";
    $("qr-desc").textContent = "用 B 站 App 扫码并确认。已经添加过的账号再扫一次，会更新它的会话。";
  }
  $("qr").showModal();
  startQR();
}

$("qr-open").addEventListener("click", () => openQR());
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
