// Invite page enhancements: copy, share and a confirm before disabling.
// Without JS the link is still selectable in its field and disabling still
// works, just without the confirm.
"use strict";

document.querySelectorAll("[data-copy]").forEach((btn) => {
  // The clipboard API needs a secure context: https, or localhost.
  if (!navigator.clipboard) return;
  btn.hidden = false;
  btn.addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(btn.dataset.copy);
      btn.textContent = "Copied";
    } catch {
      btn.textContent = "Copy failed";
    }
    setTimeout(() => { btn.textContent = "Copy"; }, 2000);
  });
});

document.querySelectorAll("[data-share]").forEach((btn) => {
  // Phones offer the system share sheet, straight into the group chat.
  if (!navigator.share) return;
  btn.hidden = false;
  btn.addEventListener("click", () => {
    navigator.share({ title: document.title, url: btn.dataset.share }).catch(() => {});
  });
});

document.querySelectorAll("form[data-confirm]").forEach((form) => {
  form.addEventListener("submit", (e) => {
    if (!confirm(form.dataset.confirm)) e.preventDefault();
  });
});

// Tapping the field selects the whole link for a manual copy.
document.querySelectorAll("input.link").forEach((input) => {
  input.addEventListener("focus", () => input.select());
});
