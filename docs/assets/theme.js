/* Purpose: Applies the planning docs theme and builds the page outline.
   Used by: every page that links docs/assets/planning.css.
   Responsibilities: Remember the theme choice and link to headings in the current document.
   Important: the choice is local to this browser. It does not change the desktop app theme. */
(function () {
  var key = "kubesift-docs-theme";
  var root = document.documentElement;
  var docsIndex = new URL("../index.html", document.currentScript.src);

  function addOutline(main) {
    if (!main) return;
    var headings = main.querySelectorAll("h2, h3");
    if (!headings.length) return;

    var outline = document.createElement("details");
    outline.className = "page-outline";
    var summary = document.createElement("summary");
    summary.textContent = "On this page";
    var nav = document.createElement("nav");
    nav.setAttribute("aria-label", "On this page");
    var list = document.createElement("ul");
    headings.forEach(function (heading) {
      var section = heading.parentElement;
      var target = !heading.id && section.matches("section[id]") && section.firstElementChild === heading ? section : heading;
      if (!target.id) {
        var slug = heading.textContent.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "section";
        var id = slug;
        for (var number = 2; document.getElementById(id); number++) id = slug + "-" + number;
        heading.id = id;
      }
      var item = document.createElement("li");
      if (heading.tagName === "H3") item.className = "outline-subsection";
      var link = document.createElement("a");
      link.href = "#" + encodeURIComponent(target.id);
      link.textContent = heading.textContent.trim();
      item.appendChild(link);
      list.appendChild(item);
    });
    nav.appendChild(list);
    outline.append(summary, nav);
    main.before(outline);
    var wide = window.matchMedia("(min-width: 1100px)");
    outline.open = wide.matches;
    wide.addEventListener("change", function () { outline.open = wide.matches; });
  }
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
    if (!rail) {
      rail = document.createElement("aside");
      rail.className = "rail";
      var nav = document.createElement("nav");
      nav.setAttribute("aria-label", "Documentation");
      var index = document.createElement("a");
      index.href = docsIndex.href;
      index.textContent = "KubeSift planning index";
      nav.appendChild(index);
      rail.appendChild(nav);
      document.body.prepend(rail);
    }
    addOutline(document.querySelector("main"));
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
