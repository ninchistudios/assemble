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

const noteForm = document.querySelector("form.note-form");

function showMyNote(text) {
  const note = document.querySelector(".group li[data-me] .note");
  if (note) note.textContent = text;
}

// Saving my status: one request at a time, always for the latest choice.
// A tap while a save is in flight only changes `wanted`; when the save
// returns, the loop sends `wanted` if it has moved on. Only the outcome for
// the final choice touches the page, so a slow early response can't undo a
// later tap, and writes reach the server in the order they were made.
const pressed = picker && picker.querySelector('button.pick[aria-pressed="true"]');
let saved = pressed ? pressed.value : "unknown";
let wanted = saved;
let saving = null;     // the running save loop, if any
let noteSaving = false;
let pendingNav = null; // where a swipe asked to go while a save was running

async function postStatus(value, confirmed) {
  const body = new URLSearchParams({ status: value });
  if (confirmed) body.set("confirm", "1");
  const res = await fetch(picker.action, { method: "POST", headers: { Accept: "application/json" }, body });
  return [res, await res.json().catch(() => ({}))];
}

async function saveLoop() {
  while (wanted !== saved) {
    const value = wanted;
    let [res, data] = await postStatus(value, false);
    // Pulling out of a locked-in day cancels it: the server asks first.
    if (res.status === 409 && data.confirm) {
      if (wanted !== value) continue; // already superseded by a newer tap
      if (!confirm(data.confirm)) {
        wanted = saved;
        select(saved);
        pendingNav = null; // stay here: the answer didn't change
        return;
      }
      [res, data] = await postStatus(value, true);
    }
    if (!res.ok) throw new Error(data.error || "Couldn't save that. Please try again.");
    saved = data.status;
    if (data.cancelled) {
      // The banner, the admin's buttons and everyone's view change.
      if (!pendingNav) location.reload();
      return;
    }
    if (wanted === saved) {
      showMine(saved, data.summary);
      // A note goes with an answer: offered once there is one, and gone
      // (the server deletes it) when the status goes back to [?].
      if (noteForm) {
        noteForm.hidden = saved === "unknown";
        if (saved === "unknown") { noteForm.note.value = ""; showMyNote(""); }
      }
    }
  }
}

// After a save settles, go wherever a swipe asked to while it ran.
function settled() {
  if (saving || noteSaving || !pendingNav) return;
  const href = pendingNav;
  pendingNav = null;
  location.href = href;
}

if (picker && picker.getAttribute("aria-disabled") !== "true") {
  picker.addEventListener("click", (e) => {
    const btn = e.target.closest("button.pick");
    if (!btn) return;
    e.preventDefault();
    wanted = btn.value;
    select(wanted); // show the choice at once
    showError("");
    if (saving) return; // the running loop will pick it up
    saving = saveLoop()
      .catch((err) => {
        wanted = saved;
        select(saved);
        showError(err.message);
        pendingNav = null;
      })
      .finally(() => {
        saving = null;
        settled();
      });
  });
}

if (noteForm) {
  noteForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    const btn = noteForm.querySelector("button");
    const sent = noteForm.note.value;
    showError("");
    noteSaving = true;
    try {
      const res = await fetch(noteForm.action, {
        method: "POST",
        headers: { Accept: "application/json" },
        body: new URLSearchParams({ note: sent }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || "Couldn't save that. Please try again.");
      // Only tidy the field to what was saved if nothing was typed since.
      if (noteForm.note.value === sent) noteForm.note.value = data.note;
      showMyNote(data.note);
      btn.textContent = "Saved";
      setTimeout(() => { btn.textContent = "Save"; }, 1500);
    } catch (err) {
      showError(err.message);
      pendingNav = null;
    } finally {
      noteSaving = false;
      settled();
    }
  });
}

// Admin actions ask first. Without JS the server shows its own confirm
// page, so the lock-in override is only added once this confirm is given.
document.querySelectorAll("form[data-confirm]").forEach((form) => {
  form.addEventListener("submit", (e) => {
    if (!confirm(form.dataset.confirm)) {
      e.preventDefault();
      return;
    }
    if (form.hasAttribute("data-override") && !form.querySelector('input[name="override"]')) {
      const input = document.createElement("input");
      input.type = "hidden";
      input.name = "override";
      input.value = "1";
      form.append(input);
    }
  });
});

// Navigation by swipe and arrow keys, following the page's own links.
// Leaving while a save is in flight would abort it, so wait for it.
function go(href) {
  if (saving || noteSaving) {
    pendingNav = href;
    return;
  }
  location.href = href;
}

const prev = document.querySelector('a[rel="prev"]');
const next = document.querySelector('a[rel="next"]');
// The arrow links too: a tap on one waits for a pending save.
[prev, next].forEach((link) => {
  if (!link) return;
  link.addEventListener("click", (e) => {
    if (!saving && !noteSaving) return;
    e.preventDefault();
    go(link.href);
  });
});

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
  if (link) go(link.href);
}, { passive: true });

document.addEventListener("keydown", (e) => {
  if (e.target.closest("input, textarea, select")) return;
  if (e.key === "ArrowLeft" && prev) go(prev.href);
  if (e.key === "ArrowRight" && next) go(next.href);
});
