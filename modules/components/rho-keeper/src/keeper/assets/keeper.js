/* rho keeper
 *
 * The page is a shell and the data arrives after. Everything here is about two
 * things: showing what the server last said, and never showing something the
 * server has not said.
 *
 * The stream is the source of truth rather than the fetch. A person leaves this
 * page open, somebody changes something in a terminal, and the page stops
 * showing a machine that no longer exists. Every render comes from a state the
 * server sent, so there is no path by which the page can be showing a guess.
 *
 * Nothing is written to the row it came from. A change posts, the server does it
 * through the same functions the CLI uses, and the answer arrives over the
 * stream. A page that guessed would be right until it was not.
 */

"use strict";

const state = { data: null, filter: "", view: "overview" };
let treeBuilt = false;

const $ = (id) => document.getElementById(id);
const el = (tag, className, text) => {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
};

function esc(value) {
  return String(value === undefined || value === null ? "" : value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

/* -------------------------------------------------------------------------- */
/*  Navigation                                                                 */
/* -------------------------------------------------------------------------- */

const TITLES = {
  overview: "Overview",
  modules: "Modules",
  contracts: "Contracts & plugins",
  docs: "Docs",
  cli: "CLI",
  system: "System",
  module: "Module",
};

function show(view, name) {
  state.view = TITLES[view] ? view : "unknown";
  document.querySelectorAll(".view").forEach((node) => {
    node.hidden = node.dataset.view !== state.view;
  });
  document.querySelectorAll("[data-nav]").forEach((node) => {
    if (node.dataset.nav === state.view) node.setAttribute("aria-current", "page");
    else node.removeAttribute("aria-current");
  });
  const title = TITLES[state.view] || "No such page";
  $("crumb").textContent = state.view === "module" && name ? "Module — " + name : title;
  // A section reached by a link is not highlighted as though it were the one a
  // person is in; the module page belongs to the modules section.
  const owning = state.view === "module" ? "modules" : state.view;
  document.querySelectorAll("[data-nav]").forEach((node) => {
    if (node.dataset.nav === owning) node.setAttribute("aria-current", "page");
    else node.removeAttribute("aria-current");
  });
}

// where reads the address: which section, and what within it.
//
// Everything after the section belongs to it, not just the first word of it.
// `#/docs/handbook/installing` is the docs section and the page
// handbook/installing; taking only `handbook` loads the handbook's front page
// for every page under it, which is a menu that looks right and opens the same
// thing.
function where() {
  const rest = (location.hash || "").replace(/^#\//, "");
  const cut = rest.indexOf("/");
  if (cut < 0) return { section: rest, name: "" };
  return { section: rest.slice(0, cut), name: rest.slice(cut + 1) };
}

// route reads the address. `#/modules/rho-sql` is that module's page, which is
// what a person expects from clicking its name and what a bookmark of it
// reopens.
function route() {
  const { section, name } = where();
  show(section || "overview", name);
  if (section === "module" && name) openModule(name);
  if (section === "docs" && name) loadDoc(name);
}

window.addEventListener("hashchange", route);
// A module's documentation is its own page under the module's name, so a
// bookmark of it reopens it.
window.addEventListener("hashchange", () => {
  const { section, name } = where();
  if (section === "module-docs" && name) loadModuleDoc(name);
});

/* -------------------------------------------------------------------------- */
/*  Rendering                                                                  */
/* -------------------------------------------------------------------------- */

function render() {
  const data = state.data;
  if (!data) return;
  const s = data.state;

  $("brand-ver").textContent = "v" + (s.keeper.version || "0.0.0");
  $("local-home").textContent = s.home || "—";
  $("home").textContent = s.keeper.config ? "settings in " + s.keeper.config : "";
  $("read").textContent = "read at " + data.read;

  const missing = (s.contracts || []).filter((c) => c.status !== "ok").length;
  $("nav-modules").textContent = s.modules.length;
  $("nav-contracts").textContent = (s.contracts || []).length;

  renderCards(s, missing);
  renderMatrix(s.contracts);
  renderModules(s.modules);
  renderContracts(s.contracts);
  renderPlugins(s.modules);
  renderErrors(s.errors);
}

function renderCards(s, missing) {
  const enabled = s.modules.filter((m) => m.enabled).length;
  const cards = [
    { n: s.modules.length, l: "Installed" },
    { n: enabled, l: "Enabled" },
    { n: s.modules.length - enabled, l: "Not enabled" },
    {
      n: missing,
      l: missing === 1 ? "Unfilled contract" : "Unfilled contracts",
      tone: missing ? "bad" : "",
    },
    {
      n: s.toolchain.installed ? "ok" : "missing",
      l: "Toolchain",
      tone: s.toolchain.installed ? "" : "bad",
    },
  ];
  $("cards").innerHTML = cards
    .map(
      (c) =>
        `<div class="card${c.tone ? " " + c.tone : ""}"><span class="n">${esc(
          String(c.n)
        )}</span><span class="l">${esc(c.l)}</span></div>`
    )
    .join("");
}

function statusPill(status) {
  if (status === "ok") return '<span class="pill ok">ok</span>';
  if (status === "missing") return '<span class="pill missing">missing</span>';
  return '<span class="pill conflict">more than one</span>';
}

function renderMatrix(contracts) {
  const list = contracts || [];
  $("matrix").innerHTML = list
    .map(
      (c) => `<tr>
      <td class="name">${esc(c.component)}.${esc(c.name)}</td>
      <td>${(c.filled || []).map((f) => esc(f)).join(", ") || '<span class="none">—</span>'}</td>
      <td>${c.one ? "exactly one" : "every enabled"}</td>
      <td>${statusPill(c.status)}</td>
    </tr>`
    )
    .join("");
  $("matrix-empty").hidden = list.length > 0;
}

function renderContracts(contracts) {
  const list = contracts || [];
  $("contracts").innerHTML = list
    .map(
      (c) => `<tr>
      <td class="name">${esc(c.component)}.${esc(c.name)}
        <span class="sub2">${c.description ? esc(c.description) : ""}</span></td>
      <td>${statusPill(c.status)}</td>
    </tr>`
    )
    .join("");
  $("contracts-empty").hidden = list.length > 0;
}

function renderPlugins(modules) {
  const plugins = (modules || []).filter((m) => m.kind === "plugin");
  $("plugins").innerHTML = plugins
    .map(
      (m) => `<tr>
      <td class="name"><a class="plain" href="#/module/${encodeURIComponent(m.name)}">${esc(
        m.name
      )}</a><span class="sub2">${esc(m.version)}</span></td>
      <td>${(m.implements || []).map((p) => `<span class="chip">${esc(p)}</span>`).join("") || '<span class="none">unspecified</span>'}</td>
      <td>${toggle(m)}</td>
    </tr>`
    )
    .join("");
  $("plugins-empty").hidden = plugins.length > 0;
}

function renderModules(modules) {
  const needle = state.filter.trim().toLowerCase();
  const rows = (modules || []).filter((m) => {
    if (!needle) return true;
    return (
      (m.name || "").toLowerCase().includes(needle) ||
      (m.description || "").toLowerCase().includes(needle) ||
      (m.provides || []).join(" ").toLowerCase().includes(needle) ||
      (m.implements || []).join(" ").toLowerCase().includes(needle)
    );
  });

  $("modules").innerHTML = rows.map(moduleRow).join("");
  $("modules-empty").hidden = rows.length > 0;
}

function moduleRow(m) {
  const others = (m.installedVersions || []).filter((v) => v !== m.version).length;
  const about = [];
  about.push(m.description ? esc(m.description) : '<span class="none">no description</span>');
  if (m.unreadable) about.push('<span class="none">' + esc(m.unreadable) + "</span>");

  let sub = "";
  if (m.extends) sub += '<span class="sub2">extends ' + esc(m.extends) + "</span>";
  if (m.command) sub += '<span class="sub2">rho ' + esc(m.command) + "</span>";
  if ((m.provides || []).length)
    sub +=
      '<span class="sub2">provides ' +
      m.provides.map((p) => '<span class="chip">' + esc(p) + "</span>").join("") +
      "</span>";

  return `<tr data-open="${esc(m.name)}" class="openable">
    <td class="name">${esc(m.name)}</td>
    <td class="kind">${esc(m.kind)}</td>
    <td class="version">${esc(m.version)}${
    others ? '<span class="also">+' + others + " more</span>" : ""
  }</td>
    <td>${toggle(m)}</td>
    <td class="about">${about.join(" ")}${sub}</td>
    <td class="right">${docsCell(m)}</td>
  </tr>`;
}

// toggle is the whole control: the state and the thing that changes it, in one
// widget.
//
// A status beside a button says the same thing twice and can disagree with
// itself. A switch cannot: it is on, or it is off, and pressing it is the one
// way that changes.
function toggle(m) {
  return `<button class="toggle" role="switch" aria-pressed="${m.enabled}" data-name="${esc(
    m.name
  )}" title="${esc(m.enabled ? "in use" : "installed, not in use")}">
    <span class="track"><span class="knob"></span></span>
    <span class="label">${m.enabled ? "on" : "off"}</span>
  </button>`;
}

// docsCell is the documentation a module has: a link when it has some, and
// nothing when it does not. A badge that opens an empty page is a badge that
// lies about what a module carries.
// docsCell is what a module documents about itself.
//
// The badge opens the documentation and nothing else. The row around it opens
// the module, and a badge that went to the module instead would be a badge that
// says "docs" and shows a settings form — a link whose label is not where it
// goes.
function docsCell(m) {
  if (m.hasDocs) {
    return `<a class="docbadge" href="#/module-docs/${encodeURIComponent(m.name)}" data-nodetail="1">docs</a>`;
  }
  if (m.hasSettings) {
    return `<a class="docbadge" href="#/module/${encodeURIComponent(m.name)}" data-nodetail="1">settings</a>`;
  }
  return '<span class="none">—</span>';
}

function statusText(m) {
  if (m.enabled) return '<span class="pill ok">in use</span>';
  if (m.record) return '<span class="pill neutral">switched off</span>';
  return '<span class="pill neutral">never enabled</span>';
}

function renderErrors(errors) {
  const list = errors || [];
  $("errors-panel").hidden = list.length === 0;
  $("errors").innerHTML = list.map((e) => "<li>" + esc(e) + "</li>").join("");
}

/* -------------------------------------------------------------------------- */
/*  The stream                                                                 */
/* -------------------------------------------------------------------------- */

function connect() {
  const conn = $("conn");
  const source = new EventSource("/api/v1/events");

  source.addEventListener("state", (event) => {
    state.data = JSON.parse(event.data);
    conn.dataset.state = "live";
    $("conn-text").textContent =
      "connected" + (state.data.state.keeper.addr ? " (" + portOf(state.data.state.keeper.addr) + ")" : "");
    render();
    // The menu is built from the modules as well as the project's own pages, so
    // it is built once the modules are known — and rebuilt if they change.
    if (!treeBuilt) {
      treeBuilt = true;
      loadDocTree();
    }
  });

  source.addEventListener("error", () => {
    // The browser reconnects on its own, and saying so is more useful than a
    // page that has quietly stopped updating.
    conn.dataset.state = "down";
    $("conn-text").textContent = "reconnecting";
  });
}

function portOf(addr) {
  const index = String(addr).lastIndexOf(":");
  return index >= 0 ? addr.slice(index + 1) : addr;
}

/* -------------------------------------------------------------------------- */
/*  One module                                                                  */
/* -------------------------------------------------------------------------- */

// openModule is one module in full: what it is, what it documents, and what it
// is set to.
//
// It is fetched rather than derived from the list, because a detail page that
// built itself out of the row it was opened from would be unable to show
// anything the row did not carry — and the row carries a summary on purpose.
async function openModule(name) {
  let detail;
  try {
    const response = await fetch("/api/v1/modules/" + encodeURIComponent(name));
    if (!response.ok) throw new Error("no such module");
    detail = await response.json();
  } catch (err) {
    $("detail-title").textContent = name;
    $("detail-facts").innerHTML = "";
    $("detail-settings").innerHTML = '<p class="empty">This module is not installed.</p>';
    return;
  }

  $("detail-title").textContent = detail.name;
  $("detail-kind").textContent = detail.kind + (detail.type ? " · " + detail.type : "");
  $("detail-config").textContent = detail.configFile || "";

  const facts = [
    ["Kind", detail.kind],
    ["Version", detail.version + ((detail.versions || []).length > 1 ? "  (of " + detail.versions.length + " installed)" : "")],
    ["Enabled", detail.enabled ? "yes" : detail.record ? "no — switched off" : "no — never enabled"],
    ["Description", detail.description || "—"],
    ["Import path", detail.importPath || "—"],
    ["Main", detail.main || "—"],
    ["Command", detail.command ? "rho " + detail.command : "—"],
    ["Extends", detail.extends || "—"],
    ["Provides", (detail.provides || []).join(", ") || "—"],
    ["Implements", (detail.implements || []).join(", ") || "—"],
    ["Versions", (detail.versions || []).join(", ")],
    ["Documentation", detail.docsPath || "—"],
  ];
  $("detail-facts").innerHTML = facts
    .map(([k, v]) => `<div><dt>${esc(k)}</dt><dd>${esc(v)}</dd></div>`)
    .join("");

  $("detail-actions").innerHTML = `
    ${toggle(detail)}
    ${detail.hasDocs ? '<a class="docbtn" href="#/module-docs/' + encodeURIComponent(detail.name) + '">read its documentation</a>' : ""}`;

  renderSettings(detail);
}

// renderSettings is the form for what a module says it can be configured with.
//
// Every control is a real control rather than a read-only value, because a page
// that shows a setting and will not let it be changed is a page that has found a
// question and refused to let it be asked. The command that sets it is printed
// beneath, because a change made here must be repeatable in a terminal.
function renderSettings(detail) {
  const settings = detail.settings || [];
  if (!settings.length) {
    $("detail-settings").innerHTML =
      '<p class="empty">This module declares no settings. It is configured by its manifest, not by a file.</p>';
    return;
  }
  $("detail-settings").innerHTML = settings
    .map((s) => {
      const input =
        s.choices && s.choices.length
          ? `<select data-key="${esc(s.name)}">${s.choices
              .map((c) => `<option value="${esc(c)}"${c === s.value ? " selected" : ""}>${esc(c)}</option>`)
              .join("")}</select>`
          : `<input type="text" data-key="${esc(s.name)}" value="${esc(s.value)}">`;
      return `<div class="setting${s.changed ? " changed" : ""}">
        <label>${esc(s.name)}</label>
        ${s.description ? `<div class="about">${esc(s.description)}</div>` : ""}
        <div class="row">${input}<button data-set="${esc(s.name)}" data-name="${esc(
        detail.name
      )}">save</button></div>
        ${
          s.changed && s.default
            ? `<div class="cmdline">default: ${esc(s.default)} — ${esc(detail.name)} runs on this until it is put back</div>`
            : `<div class="cmdline">${esc(settingCommand(detail.name, s.name))}</div>`
        }
      </div>`;
    })
    .join("");
}

// settingCommand is what a person would run to set a value by hand.
//
// It is assembled here rather than stored, so it is the command as this page
// spells it — and a change made through the page is one they can repeat without
// being told what the page did.
function settingCommand(module, key) {
  const current = (state.data?.state?.modules || []).find((m) => m.name === module);
  const version = current ? current.version : "";
  return "rho keeper setting " + module + " " + key + "=…   # " + module + "@" + version;
}

async function setSetting(button) {
  const module = button.dataset.name;
  const key = button.dataset.set;
  const field = $("detail-settings").querySelector(`[data-key="${CSS.escape(key)}"]`);
  if (!field) return;
  button.disabled = true;
  let action;
  try {
    const response = await fetch("/api/v1/modules/" + encodeURIComponent(module) + "/settings", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ key: key, value: field.value }),
    });
    action = await response.json();
  } catch (err) {
    action = { ok: false, command: "rho keeper setting " + module + " " + key, error: String(err) };
  }
  $("last-panel").hidden = false;
  $("last-command").textContent = action.command || "";
  $("last-outcome").textContent = action.ok ? action.message : action.error;
  $("last-outcome").className = action.ok ? "" : "bad";
  button.disabled = false;
  if (action.ok) openModule(module);
}

/* -------------------------------------------------------------------------- */
/*  The CLI                                                                     */
/* -------------------------------------------------------------------------- */

// loadCommands is what this toolchain accepts, read from it.
//
// A list written here would be a list that was true when it was written, and a
// page of commands a machine does not have is worse than a page of none.
async function loadCommands() {
  let commands = [];
  try {
    commands = await (await fetch("/api/v1/cli")).json();
  } catch (err) {
    commands = [];
  }
  if (!commands.length) {
    $("cli").innerHTML = '<p class="none">The toolchain could not be read.</p>';
    return;
  }
  // A command is its tool, and whatever follows it. A tool on its own is the
  // tool's own name once — falling back to the tool for a missing argument is
  // how a page ends up saying "rho-deps rho-deps".
  const toolchain = commands.filter((c) => c.kind === "toolchain" && c.help);
  const fromComponents = commands.filter((c) => c.kind && c.kind !== "toolchain");

  const parts = [];
  if (toolchain.length) {
    // The toolchain's own help, in the toolchain's own words, listing what this
    // machine can do — which is a shorter list than a written-down one, and the
    // true one.
    parts.push('<section class="cmd"><h3>rho</h3><pre><code>' + esc(toolchain[0].help) + "</code></pre></section>");
  } else if (fromComponents.length) {
    parts.push(
      '<p class="none">The toolchain could not be asked what it can do, so this is ' +
        "what it declares it has.</p>"
    );
  } else {
    parts.push('<p class="none">No Rho installation was found, so there is no toolchain to describe.</p>');
  }

  if (fromComponents.length) {
    parts.push('<div class="block-head"><h2>From installed components</h2></div>');
    parts.push(
      fromComponents
        .map((c) => {
          const name = "rho " + (c.invocation || "").trim();
          return `<div class="comp">
        <code class="name">${esc(name)}</code>
        <span class="about">${esc(c.description || "")}</span>
        <span class="from">${esc(c.kind)}</span>
      </div>`;
        })
        .join("")
    );
  }
  $("cli").innerHTML = parts.join("");
}

/* -------------------------------------------------------------------------- */
/*  The system                                                                  */
/* -------------------------------------------------------------------------- */

async function loadSystem() {
  let info;
  try {
    info = await (await fetch("/api/v1/system")).json();
  } catch (err) {
    $("system-facts").innerHTML = '<div><dt>Unavailable</dt><dd>' + esc(String(err)) + "</dd></div>";
    return;
  }
  const facts = [
    ["Keeper", "v" + (info.keeper || "—")],
    ["Rho", info.rho && info.rho.version ? info.rho.version : "—"],
    ["Go", info.go || "—"],
    ["OS", info.os || "—"],
    ["Architecture", info.arch || "—"],
    ["Rho home", info.home || "—"],
    ["Components", info.counts.components + " (" + info.counts.enabled + " enabled in total)"],
    ["Plugins", info.counts.plugins],
    ["Contracts", info.counts.contracts + (info.counts.unfilled ? ", " + info.counts.unfilled + " unfilled" : "")],
  ];
  if (info.rho && info.rho.missing && info.rho.missing.length) {
    facts.push(["Missing", info.rho.missing.join(", ")]);
  }
  $("system-facts").innerHTML = facts
    .map(([k, v]) => `<div><dt>${esc(k)}</dt><dd>${esc(v)}</dd></div>`)
    .join("");

  $("system-drivers").innerHTML = (info.drivers || [])
    .map(
      (d) =>
        `<tr><td>${esc(d.name)}</td><td class="${
          d.found ? "version" : "absent"
        }">${d.found ? esc(d.version) : "not installed"}</td></tr>`
    )
    .join("");
  $("nav-system").textContent = info.os ? "" : "";
}

/* -------------------------------------------------------------------------- */
/*  Documentation                                                               */
/* -------------------------------------------------------------------------- */

// The documentation is a tree, so it is shown as one: a menu of pages beside the
// page, and the page's own headings beside that. A long document scrolled with
// no way to see how long it is, or to jump, is a document a person gives up on.
let docFiles = [];

// The menu is three things in one: the project's own documentation, and then
// the documentation of what is installed, split by what it is. They are one menu
// because they are one question — what can I read about this machine — and a
// person looking for the handbook and a person looking for what a module says
// about itself are looking in the same place.
async function loadDocTree() {
  let files = [];
  let available = false;
  try {
    available = (await (await fetch("/api/v1/docs/available")).json()).available;
    if (available) files = await (await fetch("/api/v1/docs")).json();
  } catch (err) {
    files = [];
  }
  docFiles = files || [];

  const parts = [];
  if (docFiles.length) {
    const bySection = {};
    for (const file of docFiles) {
      (bySection[file.section] = bySection[file.section] || []).push(file);
    }
    parts.push('<li class="group">Rho documentation</li>');
    for (const section of Object.keys(bySection).sort()) {
      if (section !== ".") parts.push('<li class="group sub">' + esc(section) + "</li>");
      for (const file of bySection[section]) {
        parts.push(
          '<li><a href="#/docs/' + file.path + '" data-doc="' + esc(file.path) + '">' +
            esc(file.title) + "</a></li>"
        );
      }
    }
  } else {
    parts.push(
      '<li class="empty">No documentation found. Set RHO_DOCS_ROOT, or run ' +
        "<code>rho install</code> where the source is.</li>"
    );
  }

  const modules = (state.data?.state?.modules || []);
  for (const [kind, label] of [["component", "Components"], ["plugin", "Plugins"]]) {
    const of = modules.filter((m) => m.kind === kind && m.hasDocs);
    if (!of.length) continue;
    parts.push('<li class="group" data-group="' + kind + '">' + esc(label) + "</li>");
    for (const m of of) {
      parts.push(
        '<li><a href="#/module-docs/' + encodeURIComponent(m.name) + '" data-mod="' +
          esc(m.name) + '">' + esc(m.name) + "</a></li>"
      );
    }
  }
  $("doc-files").innerHTML = parts.join("");
  markDocSelection();
}

// markDocSelection highlights whatever is on screen, in the menu.
//
// A menu with nothing marked does not say where a person is, which on a list of
// this length is the difference between a menu and a wall of names.
function markDocSelection() {
  const { section, name } = where();
  document.querySelectorAll("[data-doc]").forEach((a) => {
    if (section === "docs" && a.dataset.doc === name) a.setAttribute("aria-current", "page");
    else a.removeAttribute("aria-current");
  });
  document.querySelectorAll("[data-mod]").forEach((a) => {
    if (section === "module-docs" && a.dataset.mod === name) a.setAttribute("aria-current", "page");
    else a.removeAttribute("aria-current");
  });
}

async function loadDoc(path) {
  // With no page named, this is the documentation's own front page. Falling back
  // to nothing would leave whatever was on screen — which after a module's
  // documentation is a module's documentation, on a page whose menu is the
  // project's.
  const index = docFiles.find((f) => f.path === "index") || docFiles[0];
  const wanted = path || (index && index.path);
  if (!wanted) {
    $("docs").innerHTML = '<p class="none">No documentation found.</p>';
    $("doc-title").textContent = "Documentation";
    $("docs-hint").textContent = "";
    $("toc").hidden = true;
    markDocSelection();
    return;
  }
  let doc;
  try {
    doc = await (await fetch("/api/v1/docs/" + wanted)).json();
  } catch (err) {
    $("docs").innerHTML = '<p class="none">There is no such page.</p>';
    return;
  }
  document.querySelectorAll("[data-doc]").forEach((a) => {
    if (a.dataset.doc === wanted) a.setAttribute("aria-current", "page");
    else a.removeAttribute("aria-current");
  });
  $("doc-title").textContent = doc.title || wanted;
  $("docs").innerHTML = doc.body || '<p class="none">This page is empty.</p>';
  $("docs-hint").textContent = (doc.breadcrumb || []).join(" / ");
  renderToc(doc.sections || []);
  markDocSelection();
}

// renderToc is a page's own headings, as the menu beside it.
//
// It is built from the headings the document declares rather than from the
// rendered HTML, so that a heading a person cannot see — one inside something
// that is collapsed — is still in the list of what the page holds.
function renderToc(sections) {
  const toc = $("toc");
  if (!sections.length) {
    toc.hidden = true;
    toc.innerHTML = "";
    return;
  }
  toc.hidden = false;
  toc.innerHTML =
    '<div class="h">On this page</div>' +
    sections
      .map((s) => `<a class="lv${Math.min(s.level, 4)}" href="#${esc(s.id)}">${esc(s.title)}</a>`)
      .join("");
}

async function loadModuleDoc(name) {
  show("docs");
  $("doc-title").textContent = name;
  try {
    const doc = await (await fetch("/api/v1/modules/" + encodeURIComponent(name) + "/docs")).json();
    $("docs").innerHTML = doc.body;
    $("docs-hint").textContent = "documentation of " + name;
    renderToc(doc.sections || []);
  } catch (err) {
    $("docs").innerHTML = '<p class="none">This module carries no documentation.</p>';
    $("docs-hint").textContent = name;
    $("toc").hidden = true;
  }
  markDocSelection();
}

/* -------------------------------------------------------------------------- */
/*  Changing something                                                          */
/* -------------------------------------------------------------------------- */

async function act(button) {
  const name = button.dataset.name;
  const what = button.getAttribute("aria-pressed") === "true" ? "disable" : "enable";
  button.disabled = true;

  let action;
  try {
    const response = await fetch("/api/v1/modules/" + encodeURIComponent(name) + "/" + what, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "{}",
    });
    action = await response.json();
  } catch (err) {
    action = { ok: false, command: "rho module " + what + " " + name, error: String(err) };
  }

  // The command is shown whether it worked or not. A person who was refused is
  // precisely the one who wants to see what was tried.
  $("last-panel").hidden = false;
  $("last-command").textContent = action.command || "";
  $("last-outcome").textContent = action.ok ? action.message : action.error;
  $("last-outcome").className = action.ok ? "" : "bad";

  button.disabled = false;
  // The stream sends the new state. If it does not arrive, the button is
  // re-enabled above and the page still works, which is why nothing here writes
  // the row itself.
}

/* -------------------------------------------------------------------------- */
/*  Wiring                                                                     */
/* -------------------------------------------------------------------------- */

$("filter").addEventListener("input", (event) => {
  state.filter = event.target.value;
  if (state.data) renderModules(state.data.state.modules);
});

// A link to a place on the page being read must not change the address.
//
// The router owns the hash: it is what says which section is open, and a bare
// fragment in it — `#running` — is read as a section called running, so clicking
// a heading in the page takes a reader to "No such page". The link is followed
// here instead, and the address is left as it was.
document.addEventListener("click", (event) => {
  const anchor = event.target.closest('a[href^="#"]');
  if (anchor) {
    const id = anchor.getAttribute("href").slice(1);
    // A cross-reference was rewritten to this dashboard's own route, and that is
    // the router's to follow. Only a fragment with no slash is a place on this
    // page.
    if (id && !id.includes("/")) {
      const target = document.getElementById(id);
      if (target) {
        event.preventDefault();
        target.scrollIntoView({ behavior: "smooth", block: "start" });
        return;
      }
    }
  }
  const button = event.target.closest(".toggle");
  if (button) {
    act(button);
    return;
  }
  const save = event.target.closest("button[data-set]");
  if (save) {
    setSetting(save);
    return;
  }
  // A link inside a row is a link. The badge and a module's name both go
  // somewhere of their own, and a row that opened itself under either would send
  // a person somewhere they did not click.
  const link = event.target.closest("a[href^='#/']");
  if (link) {
    location.hash = link.getAttribute("href");
    return;
  }
  // Anything else in the row is the module.
  const row = event.target.closest("tr[data-open]");
  if (row) location.hash = "#/module/" + encodeURIComponent(row.dataset.open);
});

route();
loadSystem();
loadCommands();
connect();
