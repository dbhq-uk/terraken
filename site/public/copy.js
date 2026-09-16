// The only script on this site.
//
// It is a file rather than an inline block on purpose: with nothing inline,
// _headers can send script-src 'self' with no 'unsafe-inline' and no external
// origin. An inline handler anywhere would mean loosening that for every page.
//
// The buttons ship with the `hidden` attribute set and are revealed here, so a
// visitor without JavaScript never sees a copy control that cannot copy. The
// command itself is selectable text either way.

const CLEAR_AFTER_MS = 1600;

/**
 * navigator.clipboard needs a secure context, which this site always has - but
 * a permission can still be refused, and Firefox rejects the promise rather
 * than resolving false. The fallback is the old textarea trick, which is
 * synchronous and works wherever execCommand still does.
 */
async function write(text) {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch (e) {
    // fall through
  }
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.top = "-1000px";
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch (e) {
    return false;
  }
}

// aria-live on a region that is empty at load, so the result is announced when
// it changes. Announcing through the button's own label instead would rename
// the control the user just pressed, which reads as a different button.
const status = document.createElement("div");
status.className = "sr-only";
status.setAttribute("role", "status");
status.setAttribute("aria-live", "polite");
document.body.appendChild(status);

for (const btn of document.querySelectorAll("[data-copy]")) {
  const text = btn.getAttribute("data-copy");
  if (!text) continue;
  btn.hidden = false;
  btn.setAttribute("aria-label", `Copy the command: ${text}`);
  const label = btn.querySelector(".install-copy-label") || btn;
  let timer;

  btn.addEventListener("click", async () => {
    const ok = await write(text);
    label.textContent = ok ? "Copied" : "Press Ctrl+C";
    btn.classList.toggle("is-done", ok);
    status.textContent = ok ? "Command copied to the clipboard" : "Copy failed - select the command and copy it";
    window.clearTimeout(timer);
    timer = window.setTimeout(() => {
      label.textContent = "Copy";
      btn.classList.remove("is-done");
      status.textContent = "";
    }, CLEAR_AFTER_MS);
  });
}
