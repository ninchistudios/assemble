// Day page: one tap sets my status without a reload, and a sideways swipe
// (or the arrow keys) moves a day. Without JS the status buttons are plain
// form submits and the arrows are plain links.
"use strict";

const picker = document.querySelector("form.picker");
const errorBox = document.querySelector(".picker-error");

function showError(msg) {
  if (!errorBox) return;
  errorBox.textContent = msg;
  errorBox.hidden = !msg;
}

function select(value) {
  picker.querySelectorAll("button.pick").forEach((b) => {
    b.setAttribute("aria-pressed", String(b.value === value));
  });
}

if (picker && picker.getAttribute("aria-disabled") !== "true") {
  picker.addEventListener("click", async (e) => {
    const btn = e.target.closest("button.pick");
    if (!btn) return;
    e.preventDefault();
    const previous = picker.querySelector('button.pick[aria-pressed="true"]');
    // Show the choice at once; put it back if the server says no.
    select(btn.value);
    showError("");
    try {
      const body = new URLSearchParams({ status: btn.value });
      const res = await fetch(picker.action, {
        method: "POST",
        headers: { Accept: "application/json" },
        body,
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || "Couldn't save that. Please try again.");
      select(data.status);
    } catch (err) {
      select(previous ? previous.value : "");
      showError(err.message);
    }
  });
}

// Navigation by swipe and arrow keys, following the page's own links.
const prev = document.querySelector('a[rel="prev"]');
const next = document.querySelector('a[rel="next"]');
let startX = null, startY = null;

document.addEventListener("touchstart", (e) => {
  if (e.touches.length !== 1) return;
  startX = e.touches[0].clientX;
  startY = e.touches[0].clientY;
}, { passive: true });

document.addEventListener("touchend", (e) => {
  if (startX === null) return;
  const dx = e.changedTouches[0].clientX - startX;
  const dy = e.changedTouches[0].clientY - startY;
  startX = startY = null;
  // A deliberate sideways swipe, not a scroll.
  if (Math.abs(dx) < 60 || Math.abs(dx) < Math.abs(dy) * 2) return;
  const link = dx < 0 ? next : prev;
  if (link) location.href = link.href;
}, { passive: true });

document.addEventListener("keydown", (e) => {
  if (e.target.closest("input, textarea, select")) return;
  if (e.key === "ArrowLeft" && prev) location.href = prev.href;
  if (e.key === "ArrowRight" && next) location.href = next.href;
});
