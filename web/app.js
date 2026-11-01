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

/* 时间显示 */

function fmtTime(iso) {
  const d = new Date(iso);
  const pad = (n) => String(n).padStart(2, "0");
  const today = new Date();
  const sameDay = d.toDateString() === today.toDateString();
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  return sameDay ? `今天 ${hm}` : `${d.getMonth() + 1}月${d.getDate()}日 ${hm}`;
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

/* 状态渲染 */

let settingsDirty = false;
let lastLogCount = -1;

function renderZone(s) {
  const place = $("place");
  const meta = $("place-meta");
  meta.replaceChildren();
  const z = s.zone;
  if (!z) {
    place.textContent = s.zone_error ? "查不到" : "查询中";
    place.classList.add("is-pending");
    if (s.zone_error) meta.append(span(`查询失败：${s.zone_error}`, "error"));
    return;
  }
  place.classList.remove("is-pending");
  const domestic = z.country === "中国";
  const parts = domestic ? [z.province, z.city] : [z.country, z.province];
  place.textContent = [...new Set(parts.filter(Boolean))].join(" ") || "未知地区";
  if (z.isp) meta.append(span(z.isp));
  if (z.addr) {
    const code = document.createElement("code");
    code.textContent = z.addr;
    meta.append(code);
  }
}

function span(text, cls) {
  const el = document.createElement("span");
  el.textContent = text;
  if (cls) el.className = cls;
  return el;
}

function renderAccount(s) {
  const acc = $("account");
  acc.replaceChildren();
  if (s.logged_in) {
    acc.append(span(s.uname || "已登录", "state-ok"));
    $("qr-open").textContent = "重新扫码";
    $("qr-open").className = "secondary";
    $("bili-logout").hidden = false;
  } else {
    acc.append(span("未登录", "state-bad"));
    $("qr-open").textContent = "扫码登录";
    $("qr-open").className = "primary";
    $("bili-logout").hidden = true;
  }
}

function renderRun(s) {
  const last = $("last");
  last.replaceChildren();
  $("last-error").textContent = "";
  if (s.running) {
    last.append(span("正在运行"));
  } else if (!s.last) {
    last.append(span("还没有运行过", "muted"));
  } else {
    last.append(span(fmtTime(s.last.at)), " ");
    last.append(s.last.ok ? span("成功", "state-ok") : span("失败", "state-bad"));
    if (s.last.error) $("last-error").textContent = s.last.error;
  }
  const btn = $("run");
  btn.disabled = s.running;
  btn.textContent = s.running ? "运行中" : "立即运行";

  $("next").textContent = s.running || !s.next ? "—" : `${fmtTime(s.next)}（${fmtRelative(s.next)}）`;
}

function renderSettings(st) {
  if (settingsDirty) return;
  $("interval").value = st.interval;
  $("jitter").value = st.jitter;
  $("bvid").value = st.bvid;
  $("dynamic").checked = st.do_dynamic;
}

function renderLogs(lines) {
  if (lines.length === lastLogCount && lines.length < 300) return;
  lastLogCount = lines.length;
  const box = $("log");
  const atBottom = box.scrollHeight - box.scrollTop - box.clientHeight < 24;
  box.textContent = lines.join("\n");
  if (atBottom) box.scrollTop = box.scrollHeight;
}

async function refresh() {
  try {
    const s = await (await api("GET", "/api/status")).json();
    renderZone(s);
    renderAccount(s);
    renderRun(s);
    renderSettings(s.settings);
    renderLogs(s.logs || []);
    $("version").textContent = s.version;
  } catch (err) {
    if (!(err instanceof Unauthorized)) $("last-error").textContent = `连不上服务：${err.message}`;
  }
}

/* 操作 */

$("run").addEventListener("click", async () => {
  $("run").disabled = true;
  try {
    await api("POST", "/api/run");
    setTimeout(refresh, 600);
  } catch (err) {
    $("last-error").textContent = err.message;
    $("run").disabled = false;
  }
});

$("bili-logout").addEventListener("click", async () => {
  if (!confirm("退出后服务器上的会话会被注销并删除，需要重新扫码才能继续。确定退出？")) return;
  try {
    await api("POST", "/api/bili-logout");
    refresh();
  } catch (err) {
    if (!(err instanceof Unauthorized)) alert(err.message);
  }
});

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

/* 扫码登录 */

const QR_TEXT = {
  waiting: "用 B 站 App 扫一扫",
  scanned: "已扫码，请在手机上确认登录",
  expired: "二维码已过期",
  success: "登录成功",
};

let qrPoll = null;
let qrURL = null;

function stopQR() {
  clearInterval(qrPoll);
  qrPoll = null;
}

async function startQR() {
  stopQR();
  const frame = document.querySelector(".qr-frame");
  frame.classList.add("is-dim");
  $("qr-renew").hidden = true;
  $("qr-state").textContent = "正在获取二维码";
  try {
    const blob = await (await api("POST", "/api/qr")).blob();
    if (qrURL) URL.revokeObjectURL(qrURL);
    qrURL = URL.createObjectURL(blob);
    $("qr-img").src = qrURL;
    frame.classList.remove("is-dim");
    $("qr-state").textContent = QR_TEXT.waiting;
    qrPoll = setInterval(pollQR, 2000);
  } catch (err) {
    if (err instanceof Unauthorized) { $("qr").close(); return; }
    $("qr-state").textContent = err.message;
    $("qr-renew").hidden = false;
  }
}

async function pollQR() {
  try {
    const { state } = await (await api("GET", "/api/qr")).json();
    if (QR_TEXT[state]) $("qr-state").textContent = QR_TEXT[state];
    if (state === "expired" || state === "none") {
      stopQR();
      document.querySelector(".qr-frame").classList.add("is-dim");
      $("qr-renew").hidden = false;
    }
    if (state === "success") {
      stopQR();
      setTimeout(() => { $("qr").close(); refresh(); }, 900);
    }
  } catch (err) {
    stopQR();
    if (err instanceof Unauthorized) { $("qr").close(); return; }
    $("qr-state").textContent = err.message;
    $("qr-renew").hidden = false;
  }
}

$("qr-open").addEventListener("click", () => {
  $("qr").showModal();
  startQR();
});
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
