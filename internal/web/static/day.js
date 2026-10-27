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

const labels = { yes: "YES", ok: "OK", no: "NO", unknown: "?" };

// Mirror my saved status into my row of the group list, and show the
// summary the server worded for the new state.
function showMine(value, summaryText) {
  const row = document.querySelector(".group li[data-me]");
  if (row) {
    row.dataset.status = value;
    const icon = document.getElementById("icon-" + value);
    if (icon) row.querySelector(".mini").replaceChildren(icon.content.cloneNode(true));
    row.querySelector(".status-word").textContent = labels[value];
  }
  const summary = document.querySelector(".summary");
  if (summary && summaryText) summary.textContent = summaryText;
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
    const send = async (confirmed) => {
      const body = new URLSearchParams({ status: btn.value });
      if (confirmed) body.set("confirm", "1");
      const res = await fetch(picker.action, {
        method: "POST",
        headers: { Accept: "application/json" },
        body,
      });
      return [res, await res.json().catch(() => ({}))];
    };
    try {
      let [res, data] = await send(false);
      // Pulling out of a locked-in day cancels it: the server asks first.
      if (res.status === 409 && data.confirm) {
        if (!confirm(data.confirm)) {
          select(previous ? previous.value : "");
          return;
        }
        [res, data] = await send(true);
      }
      if (!res.ok) throw new Error(data.error || "Couldn't save that. Please try again.");
      if (data.cancelled) {
        // The banner, the admin's buttons and everyone's view change.
        location.reload();
        return;
      }
      select(data.status);
      showMine(data.status, data.summary);
      // A note goes with an answer: offered once there is one, and gone
      // (the server deletes it) when the status goes back to [?].
      if (noteForm) {
        noteForm.hidden = data.status === "unknown";
        if (data.status === "unknown") { noteForm.note.value = ""; showMyNote(""); }
      }
    } catch (err) {
      select(previous ? previous.value : "");
      showError(err.message);
    }
  });
}

const noteForm = document.querySelector("form.note-form");

function showMyNote(text) {
  const note = document.querySelector(".group li[data-me] .note");
  if (note) note.textContent = text;
}

if (noteForm) {
  noteForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    const btn = noteForm.querySelector("button");
    showError("");
    try {
      const res = await fetch(noteForm.action, {
        method: "POST",
        headers: { Accept: "application/json" },
        body: new URLSearchParams({ note: noteForm.note.value }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || "Couldn't save that. Please try again.");
      noteForm.note.value = data.note;
      showMyNote(data.note);
      btn.textContent = "Saved";
      setTimeout(() => { btn.textContent = "Save"; }, 1500);
    } catch (err) {
      showError(err.message);
    }
  });
}

// Admin actions ask first. Without JS, the server's own confirm page does.
document.querySelectorAll("form[data-confirm]").forEach((form) => {
  form.addEventListener("submit", (e) => {
    if (!confirm(form.dataset.confirm)) e.preventDefault();
  });
});

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
