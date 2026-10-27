// Notices: dismiss with a tap, without a reload. Without JS the dismiss
// button is a form post that comes back to the same page.
"use strict";

document.querySelectorAll(".notices form.dismiss").forEach((form) => {
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const item = form.closest("li");
    item.hidden = true;
    try {
      const res = await fetch(form.action, { method: "POST", headers: { Accept: "application/json" } });
      if (!res.ok) throw new Error();
      item.remove();
      const list = document.querySelector(".notices");
      if (list && !list.children.length) list.remove();
    } catch {
      item.hidden = false;
    }
  });
});
