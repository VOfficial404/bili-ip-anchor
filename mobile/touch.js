// MyGo 的网页版把手指当成鼠标：按住拖动不会滚动页面。这里把手指在画布上的拖动换成滚轮事件，
// 松手后按速度继续滑一段；轻点仍然是点击。只在手机 App 里用（mobile.go 把它插进 index.html）。
(() => {
  const SLOP = 8 // 移动超过这么多像素才算拖动
  const FRICTION = 0.95 // 惯性每 16ms 剩下的速度
  let t = null // 正在拖动的手指
  let fling = 0
  let self = false // 正在派发自己造的事件

  // 界面按下时会 setPointerCapture；手指在它处理之前就抬起来时浏览器会抛错，整个界面跟着退出
  const capture = Element.prototype.setPointerCapture
  Element.prototype.setPointerCapture = function (id) {
    try { capture.call(this, id) } catch {}
  }

  const wheel = (target, x, y, dx, dy) =>
    target.dispatchEvent(new WheelEvent("wheel", {
      clientX: x, clientY: y, deltaX: dx, deltaY: dy, deltaMode: 0, bubbles: true, cancelable: true,
    }))

  const ours = (e) => !self && e.pointerType === "touch"

  addEventListener("pointerdown", (e) => {
    if (!ours(e)) return
    if (t) {
      e.stopImmediatePropagation() // 第二根手指不算点击
      return
    }
    cancelAnimationFrame(fling)
    if (e.target.tagName !== "CANVAS") return
    t = { id: e.pointerId, target: e.target, x0: e.clientX, y0: e.clientY, x: e.clientX, y: e.clientY, at: e.timeStamp, vx: 0, vy: 0, dragging: false }
  }, true)

  addEventListener("pointermove", (e) => {
    if (!ours(e) || !t || e.pointerId !== t.id) return
    if (!t.dragging) {
      if (Math.hypot(e.clientX - t.x0, e.clientY - t.y0) < SLOP) return
      t.dragging = true
      // 让界面放开这次按下；位置在窗口外，不会算作点击
      self = true
      t.target.dispatchEvent(new PointerEvent("pointercancel", {
        pointerId: t.id, pointerType: "touch", isPrimary: true, clientX: -1e5, clientY: -1e5, bubbles: true,
      }))
      self = false
    }
    e.stopImmediatePropagation()
    const dx = t.x - e.clientX, dy = t.y - e.clientY
    const dt = Math.max(1, e.timeStamp - t.at)
    t.vx = 0.8 * (dx / dt) + 0.2 * t.vx
    t.vy = 0.8 * (dy / dt) + 0.2 * t.vy
    t.x = e.clientX
    t.y = e.clientY
    t.at = e.timeStamp
    if (dx || dy) wheel(t.target, t.x, t.y, dx, dy)
  }, true)

  const end = (e) => {
    if (!ours(e) || !t || e.pointerId !== t.id) return
    const d = t
    t = null
    if (!d.dragging) return
    e.stopImmediatePropagation()
    if (e.type !== "pointerup" || e.timeStamp - d.at > 100) return // 停住后才松手：不滑
    let vx = d.vx, vy = d.vy, last = performance.now()
    const step = (now) => {
      const dt = Math.min(50, now - last)
      last = now
      const k = Math.pow(FRICTION, dt / 16)
      vx *= k
      vy *= k
      if (Math.abs(vx) < 0.02 && Math.abs(vy) < 0.02) return
      wheel(d.target, d.x, d.y, vx * dt, vy * dt)
      fling = requestAnimationFrame(step)
    }
    fling = requestAnimationFrame(step)
  }
  addEventListener("pointerup", end, true)
  addEventListener("pointercancel", end, true)
})()
