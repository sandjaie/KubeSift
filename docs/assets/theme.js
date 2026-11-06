/* Purpose: Applies the planning docs light or dark theme before paint and adds the sidebar switch.
   Used by: every page that links docs/assets/planning.css.
   Responsibilities: honor a saved choice, otherwise follow the system, and remember the switch.
   Important: the choice is local to this browser. It does not change the desktop app theme. */
(function () {
  var key = "kubesift-docs-theme";
  var root = document.documentElement;
  try {
    var saved = localStorage.getItem(key);
    if (saved === "dark" || saved === "light") root.dataset.theme = saved;
  } catch (e) {}

  function chosen() {
    return root.dataset.theme === "dark" || root.dataset.theme === "light";
  }

  function dark() {
    if (root.dataset.theme === "dark") return true;
    if (root.dataset.theme === "light") return false;
    return window.matchMedia("(prefers-color-scheme: dark)").matches;
  }

  document.addEventListener("DOMContentLoaded", function () {
    var rail = document.querySelector(".rail");
    if (!rail) return;
    var button = document.createElement("button");
    button.type = "button";
    button.className = "theme";
    function label() {
      var on = dark();
      button.textContent = on ? "Light mode" : "Dark mode";
      button.setAttribute("aria-pressed", on ? "true" : "false");
    }
    button.addEventListener("click", function () {
      var next = dark() ? "light" : "dark";
      root.dataset.theme = next;
      try { localStorage.setItem(key, next); } catch (e) {}
      label();
    });
    var mark = rail.querySelector(".mark");
    if (mark) mark.insertAdjacentElement("afterend", button);
    else rail.appendChild(button);
    label();
    window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", function () {
      if (!chosen()) label();
    });
  });
})();
