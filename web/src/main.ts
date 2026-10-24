import "@fontsource/fira-code/latin-400.css";
import "@fontsource/fira-code/latin-500.css";
import "@fontsource/fira-code/latin-600.css";
import "@fontsource/fira-sans/latin-400.css";
import "@fontsource/fira-sans/latin-500.css";
import "@fontsource/fira-sans/latin-600.css";
import "./style.css";
import { renderAddForm } from "./addform";
import { byId, button, h } from "./dom";
import { SLIDER, downtime, pct, percentFromPosition, positionFromPercent } from "./format";
import { createGraph } from "./graph";
import { renderInspector, type EditHost } from "./inspector";
import { presets } from "./presets";
import { renderStart, type StartHost } from "./start";
import { decodeState, encodeState, type AppState } from "./state";
import type { ApiError, CheckResult, Diff, EditOp, GraphData, GraphNode, Override, Trace, TraceNode } from "./types";
import { loadCore, type Core } from "./wasm";

const EDIT_DEBOUNCE_MS = 200;
const TOP_LEVERS = 5;

const editor = byId<HTMLTextAreaElement>("editor");
const errorsEl = byId<HTMLUListElement>("errors");
const slidersEl = byId<HTMLDivElement>("sliders");
const answerEl = byId<HTMLElement>("answer");
const tilesEl = byId<HTMLElement>("tiles");
const liveEl = byId<HTMLElement>("live");
const findingsEl = byId<HTMLDivElement>("findings");
const derivationEl = byId<HTMLDivElement>("derivation");
const inspectorEl = byId<HTMLDivElement>("inspector");
const drawerEl = byId<HTMLElement>("drawer");
const drawerTitle = byId<HTMLElement>("drawer-title");
const addEl = byId<HTMLDivElement>("add");
const resultsEl = byId<HTMLElement>("results");
const uncertaintyEl = byId<HTMLInputElement>("uncertainty");
const shareBtn = byId<HTMLButtonElement>("share");
const shareLabel = byId<HTMLSpanElement>("share-label");
const themeBtn = byId<HTMLButtonElement>("theme");
const resetBtn = byId<HTMLButtonElement>("reset");
const startEl = byId<HTMLElement>("start");
const helpEl = byId<HTMLDialogElement>("help");
const graph = createGraph(byId("graph"));

let core: Core;
let overrides: Record<string, number> = {};
let removed: string[] = [];
let graphYaml = "";
let graphData: GraphData | null = null;
let selectedId: string | null = null;

function overrideList(): Override[] {
  return Object.entries(overrides)
    .filter(([name]) => !removed.includes(name))
    .map(([component, percent]) => ({ component, percent }));
}

function showErrors(errors: readonly ApiError[]): void {
  errorsEl.replaceChildren(...errors.map((e) => h("li", "", e.line ? `line ${e.line}: ${e.msg}` : e.msg)));
  editor.classList.toggle("invalid", errors.length > 0);
}

function componentNodes(): GraphNode[] {
  return (graphData?.nodes ?? []).filter((n) => n.kind === "component" && n.component);
}

// ---- summary -------------------------------------------------------------------------------------

function tile(label: string, value: string, sub = "", bad = false): HTMLElement {
  return h("div", `tile${bad ? " bad" : ""}`, h("span", "tile-label", label), h("span", "tile-value", value), sub && h("span", "tile-sub", sub));
}

/** How much of the error budget the computed availability spends: over 100% means the target is missed. */
function budget(r: CheckResult): HTMLElement {
  const used = (1 - r.availability) / (1 - r.target);
  const level = used > 1 ? "bad" : used > 0.8 ? "warn" : "good";
  const fill = h("div", "budget-fill");
  fill.style.setProperty("--used", `${Math.min(used, 1) * 100}%`);
  const track = h("div", "budget-track", fill);
  track.setAttribute("role", "img");
  track.setAttribute("aria-label", `Error budget ${used > 1 ? "exceeded" : "used"}: ${(used * 100).toFixed(0)}%`);
  return h(
    "div",
    `budget ${level}`,
    h("div", "budget-text", h("span", "", "Error budget used"), h("strong", "", `${(used * 100).toFixed(0)}%`)),
    track,
    h("div", "budget-text", h("span", "", `${downtime(r.availability, r.window_days)} of ${downtime(r.target, r.window_days)} allowed`), h("span", "", `per ${r.window_days}d`)),
  );
}

function renderSummary(r: CheckResult, base: CheckResult, diff: Diff | null): void {
  const value = h("div", "hero-value", h("span", "big", pct(r.availability)), h("span", `chip ${r.verdict}`, r.verdict));
  if (diff) {
    const dpp = (r.availability - base.availability) * 100;
    value.append(h("span", "delta", `was ${pct(base.availability)} (${dpp >= 0 ? "+" : ""}${dpp.toFixed(4)} pp)`));
  }
  const hero = h("div", "hero", h("div", "answer-main", h("p", "service-name", r.service), value), budget(r), h("p", "advice", r.recommendation));

  const u = r.uncertainty;
  const tiles = h(
    "div",
    "tiles",
    tile("Target", pct(r.target), `${downtime(r.target, r.window_days)} allowed / ${r.window_days}d`),
    tile(
      "Dependency ceiling",
      r.ceiling === null ? "n/a" : pct(r.ceiling),
      r.ceiling === null ? "mark components owned: true" : "everything you own perfect",
      r.ceiling !== null && r.ceiling < r.target,
    ),
    tile("Recommended tier", r.tier === null ? "below 99%" : pct(r.tier), u ? "from the pessimistic p5 estimate" : "rounded down from the computed value"),
  );
  if (r.method === "bdd") {
    tiles.append(tile("Single points of failure", String(r.spofs.length), r.spofs.slice(0, 3).join(", ") + (r.spofs.length > 3 ? ", ..." : "") || "none"));
  }
  const top = r.levers[0];
  if (top) tiles.append(tile("Biggest lever", top.component, `+${(top.improvement_potential * 100).toFixed(4)} pp if made perfect`));
  if (r.naive !== undefined && Math.abs(r.naive - r.availability) > 1e-12) {
    tiles.append(tile("Naive multiplication", pct(r.naive), "ignores shared components"));
  }
  if (u) {
    tiles.append(
      tile("Chance of meeting target", `${(u.p_meets_target * 100).toFixed(1)}%`, `${u.samples} samples`, u.p_meets_target < 0.9),
      tile("Spread (p5 to p95)", `${pct(u.p5)} to ${pct(u.p95)}`, `median ${pct(u.p50)}`),
    );
  }
  answerEl.replaceChildren(hero);
  // The header repeats the headline figure so it stays in view while a slider far below the result is dragged.
  liveEl.replaceChildren(h("span", "live-value", pct(r.availability)), h("span", `chip ${r.verdict}`, r.verdict));
  liveEl.hidden = false;
  tilesEl.replaceChildren(tiles);
  renderFindings(r);
}

function renderFindings(r: CheckResult): void {
  const body: Node[] = [];
  if (r.levers.length) {
    const table = h("table");
    table.append(h("thead", "", h("tr", "", h("th", "", "Biggest levers"), h("th", "", "Availability"), h("th", "", "Gain if made perfect"))));
    const tbody = h("tbody");
    for (const l of r.levers.slice(0, TOP_LEVERS)) {
      tbody.append(h("tr", "", h("td", "", l.component), h("td", "", pct(l.availability)), h("td", "", `+${(l.improvement_potential * 100).toFixed(4)} pp`)));
    }
    table.append(tbody);
    body.push(table);
  }
  if (r.method === "bdd") {
    body.push(h("p", "note", r.spofs.length ? `Single points of failure: ${r.spofs.join(", ")}` : "No single points of failure."));
    if (r.cut_sets.length) {
      const pairs = r.cut_sets.map((p) => p.join(" + ")).join("; ");
      body.push(h("p", "note", `Pairs that take it down together: ${pairs}${r.cut_sets_truncated ? ", ..." : ""}`));
    }
  }
  for (const w of r.warnings) body.push(h("p", "warning", w));
  if (removed.length) body.push(h("p", "note", `Removed in this what-if: ${removed.join(", ")}`));
  findingsEl.replaceChildren(...body);
}

// ---- derivation ----------------------------------------------------------------------------------

function traceItem(n: TraceNode): HTMLLIElement {
  const li = h("li", "", `${n.label}: `, h("span", "p", pct(n.probability)));
  if (n.shared_components?.length) {
    li.append(
      h("span", "shared", `shared components: ${n.shared_components.join(", ")}. The parts below are not independent, so this is the exact value, not a product.`),
    );
  }
  if (n.children?.length) li.append(h("ul", "", ...n.children.map(traceItem)));
  return li;
}

function renderDerivation(t: Trace): void {
  derivationEl.replaceChildren(
    h("p", "note", `${t.service} works ${pct(t.root.probability)} of the time because:`),
    h("ul", "", traceItem(t.root)),
  );
}

// ---- selection and editing -----------------------------------------------------------------------

/** A key for the selected node that survives a rebuild of the graph: components by name, operators by position. */
function selectionKey(): string | null {
  const n = graphData?.nodes.find((x) => x.id === selectedId);
  if (!n) return null;
  return n.component ? `c:${n.component}` : n.node !== undefined ? `n:${n.node}:${n.kind}` : n.id;
}

function findByKey(key: string | null): GraphNode | null {
  if (!key || !graphData) return null;
  return graphData.nodes.find((n) => (n.component ? `c:${n.component}` : n.node !== undefined ? `n:${n.node}:${n.kind}` : n.id) === key) ?? null;
}

const host: EditHost = {
  components: () => componentNodes().map((n) => n.component ?? ""),
  apply(ops: EditOp[]): string | null {
    if (!ops.length) return null;
    const r = core.edit(editor.value, ops);
    if (!r.ok) return r.errors.map((e) => e.msg).join("; ");
    // A component whose availability was just edited takes the new value; a slider what-if left over from
    // before would otherwise keep showing the old one.
    const edited = new Set(ops.flatMap((o) => (o.op === "update_component" ? [o.name] : [])));
    const kept = Object.fromEntries(Object.entries(overrides).filter(([name]) => !edited.has(name)));
    load(r.result.yaml, kept, removed, selectionKey());
    return null;
  },
};

/** Selects a node and opens its editor, or clears the selection and closes it. `byUser` is a click or key press, not a rebuild. */
function select(id: string | null, byUser = false): void {
  selectedId = id;
  graph.select(id);
  const node = graphData?.nodes.find((n) => n.id === id) ?? null;
  if (!node) {
    closeDrawer(byUser);
    return;
  }
  renderInspector(inspectorEl, node, host);
  drawerTitle.textContent = node.kind === "component" ? "Edit component" : node.kind === "service" ? "Service" : "Edit operator";
  openDrawer(node.id, byUser);
}

// ---- editor drawer -------------------------------------------------------------------------------

/** How long the drawer takes to slide in; keep in step with --drawer-ms in style.css. */
const DRAWER_MS = 200;
let returnFocus: HTMLElement | null = null;

function openDrawer(nodeId: string, byUser: boolean): void {
  const wasOpen = drawerEl.classList.contains("open");
  if (!wasOpen) returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  drawerEl.classList.add("open");
  drawerEl.inert = false;
  if (byUser) drawerTitle.focus({ preventScroll: true });
  // Once it has slid in, move the graph if the drawer now covers the selected node.
  if (!wasOpen || byUser) window.setTimeout(() => graph.keepClear(nodeId, drawerEl.getBoundingClientRect()), DRAWER_MS);
}

function closeDrawer(restoreFocus: boolean): void {
  if (!drawerEl.classList.contains("open")) return;
  drawerEl.classList.remove("open");
  drawerEl.inert = true;
  if (restoreFocus && returnFocus?.isConnected) returnFocus.focus({ preventScroll: true });
  returnFocus = null;
}

// ---- sliders -------------------------------------------------------------------------------------

function buildSliders(): void {
  const rows: HTMLElement[] = [];
  for (const node of componentNodes()) {
    const name = node.component ?? "";
    const base = node.availability ?? 100;
    const input = h("input");
    input.type = "range";
    input.min = String(SLIDER.min);
    input.max = String(SLIDER.max);
    input.step = String(SLIDER.step);
    input.setAttribute("aria-label", `${name} availability`);
    input.title = "Drag to try a different availability. Double-click to reset.";
    const out = h("output");
    const toggle = button("", "ghost small");
    const row = h("div", "slider", h("span", "name", name), input, out, h("span", "row-actions", button("Edit", "ghost small", () => select(node.id, true)), toggle));

    const show = (): void => {
      const percent = overrides[name] ?? base;
      const gone = removed.includes(name);
      const position = positionFromPercent(percent);
      input.value = String(position);
      input.style.setProperty("--p", `${(position / SLIDER.max) * 100}%`);
      input.disabled = gone;
      out.textContent = gone ? "removed" : pct(percent / 100);
      toggle.textContent = gone ? "Restore" : "Remove";
      toggle.title = gone ? "Put it back in the what-if" : "What if this component were not in the model?";
      row.classList.toggle("changed", name in overrides && !gone);
      row.classList.toggle("removed", gone);
    };
    input.addEventListener("input", () => {
      overrides = { ...overrides, [name]: percentFromPosition(Number(input.value)) };
      show();
      scheduleRefresh();
    });
    input.addEventListener("dblclick", () => {
      const { [name]: _dropped, ...rest } = overrides;
      overrides = rest;
      show();
      scheduleRefresh();
    });
    toggle.addEventListener("click", () => {
      removed = removed.includes(name) ? removed.filter((r) => r !== name) : [...removed, name];
      show();
      refresh();
    });
    show();
    rows.push(row);
  }
  slidersEl.replaceChildren(...rows);
}

// ---- refresh -------------------------------------------------------------------------------------

function rebuildGraph(yaml: string, restoreKey: string | null): void {
  const g = core.graph(yaml);
  if (!g.ok) return;
  const previous = new Map(componentNodes().map((n) => [n.component, n.availability]));
  graphData = g.result;
  graphYaml = yaml;
  const names = new Set(componentNodes().map((n) => n.component));
  const current = new Map(componentNodes().map((n) => [n.component, n.availability]));
  // Keep a slider override only while its component still exists and its model value is unchanged: a changed
  // value in the text is the new starting point, and the old what-if would hide it.
  overrides = Object.fromEntries(
    Object.entries(overrides).filter(([name]) => names.has(name) && (!previous.has(name) || previous.get(name) === current.get(name))),
  );
  removed = removed.filter((name) => names.has(name));
  graph.render(g.result);
  buildSliders();
  renderAddForm(addEl, host);
  select(findByKey(restoreKey)?.id ?? null);
}

function refresh(restoreKey: string | null = null): void {
  const yaml = editor.value;
  const opts = { uncertainty: uncertaintyEl.checked };
  const base = core.check(yaml, opts);
  if (!base.ok) {
    showErrors(base.errors);
    answerEl.classList.add("stale");
    tilesEl.classList.add("stale");
    return;
  }
  showErrors([]);
  answerEl.classList.remove("stale");
  tilesEl.classList.remove("stale");

  if (yaml !== graphYaml) rebuildGraph(yaml, restoreKey);

  let shown = base.result;
  let diff: Diff | null = null;
  const list = overrideList();
  if (list.length || removed.length) {
    const w = core.whatif(yaml, list, removed, opts);
    if (w.ok) {
      diff = w.result;
      shown = w.result.after;
    } else {
      showErrors(w.errors);
    }
  }
  renderSummary(shown, base.result, diff);
  graph.markSpofs(shown.spofs);
  graph.markRemoved(removed);

  const ex = core.explain(yaml, list, removed);
  if (ex.ok) renderDerivation(ex.result);
  else derivationEl.replaceChildren(h("p", "note", ex.errors.map((e) => e.msg).join("; ")));

  const state: AppState = { yaml, overrides, removed, uncertainty: opts.uncertainty };
  void encodeState(state).then((fragment) => history.replaceState(null, "", `#${fragment}`));
}

let framePending = false;
function scheduleRefresh(): void {
  if (framePending) return;
  framePending = true;
  requestAnimationFrame(() => {
    framePending = false;
    refresh();
  });
}

let editTimer: number | undefined;

function load(yaml: string, nextOverrides: Record<string, number>, nextRemoved: string[], restoreKey: string | null = null): void {
  resultsEl.hidden = false; // shown before the graph is drawn, so it has a size to fit to
  editor.value = yaml;
  overrides = nextOverrides;
  removed = nextRemoved;
  graphYaml = "";
  refresh(restoreKey);
}

// ---- start section -------------------------------------------------------------------------------

function scrollTo(el: HTMLElement): void {
  const calm = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  el.scrollIntoView({ block: "start", behavior: calm ? "auto" : "smooth" });
}

/** Takes the reader back up to the upload and form cards, with the first control focused. */
function goToStart(): void {
  scrollTo(startEl);
  startEl.querySelector<HTMLElement>("button")?.focus({ preventScroll: true });
}

/** Brings the result into view and moves keyboard focus to it, once a model has been provided. */
function showResult(): void {
  scrollTo(resultsEl);
  answerEl.focus({ preventScroll: true });
}

const startHost: StartHost = {
  validate: (yaml) => {
    const r = core.check(yaml, { uncertainty: false });
    return r.ok ? [] : r.errors;
  },
  open: (yaml) => {
    load(yaml, {}, []);
    showResult();
  },
};

const ZOOM_STEP = 1.25;

// ---- keyboard shortcuts --------------------------------------------------------------------------

/** Keys and what they do, shown in the help dialog. Keep in step with onShortcut. */
const SHORTCUTS: readonly (readonly [keys: readonly string[], does: string])[] = [
  [["?"], "Show this list"],
  [["f"], "Fit the whole graph in view"],
  [["+", "-"], "Zoom the graph in or out"],
  [["u"], "Turn the uncertainty run on or off"],
  [["t"], "Switch between light and dark"],
  [["n"], "Jump back to the upload and form"],
  [["Esc"], "Close the editor"],
];

function renderHelp(): void {
  const rows = SHORTCUTS.flatMap(([keys, does]) => [
    h("dt", "", ...keys.flatMap((k, i) => (i ? [" or ", h("kbd", "", k)] : [h("kbd", "", k)]))),
    h("dd", "", does),
  ]);
  helpEl.replaceChildren(
    h("div", "panel help-inner", Object.assign(h("h2", "", "Keyboard shortcuts"), { id: "help-title" }), h("dl", "shortcuts", ...rows), h("div", "actions", button("Close", "primary", () => helpEl.close()))),
  );
}

function typingIn(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && (target.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName));
}

function onShortcut(e: KeyboardEvent): void {
  // The help dialog handles Esc itself, and letters typed into a field are text.
  if (e.ctrlKey || e.metaKey || e.altKey || helpEl.open) return;
  if (e.key === "Escape") {
    if (selectedId) select(null, true);
    return;
  }
  if (typingIn(e.target)) return;
  switch (e.key) {
    case "?":
      helpEl.showModal();
      break;
    case "f":
      graph.fit();
      break;
    case "+":
    case "=":
      graph.zoomBy(ZOOM_STEP);
      break;
    case "-":
      graph.zoomBy(1 / ZOOM_STEP);
      break;
    case "u":
      uncertaintyEl.click();
      break;
    case "t":
      toggleTheme();
      break;
    case "n":
      goToStart();
      break;
    default:
      return;
  }
  e.preventDefault();
}

// ---- theme ---------------------------------------------------------------------------------------

const THEME_KEY = "nines-theme";

function effectiveTheme(): "light" | "dark" {
  const set = document.documentElement.dataset.theme;
  if (set === "light" || set === "dark") return set;
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function toggleTheme(): void {
  const next = effectiveTheme() === "dark" ? "light" : "dark";
  document.documentElement.dataset.theme = next;
  try {
    localStorage.setItem(THEME_KEY, next);
  } catch {
    // not remembered, but the switch itself still works
  }
  graph.restyle();
}

// ---- wiring --------------------------------------------------------------------------------------

async function start(): Promise<void> {
  byId("drawer-close").addEventListener("click", () => select(null, true));
  renderHelp();
  byId("help-btn").addEventListener("click", () => helpEl.showModal());
  document.addEventListener("keydown", onShortcut);
  byId("zoom-in").addEventListener("click", () => graph.zoomBy(ZOOM_STEP));
  byId("zoom-out").addEventListener("click", () => graph.zoomBy(1 / ZOOM_STEP));
  byId("zoom-fit").addEventListener("click", () => graph.fit());
  answerEl.replaceChildren(h("p", "note", "Loading the calculation core…"));

  try {
    core = await loadCore();
  } catch (e) {
    // The results are hidden at this point, so the message goes where the reader is looking.
    startEl.replaceChildren(h("p", "warning", `Could not start the WebAssembly core: ${e instanceof Error ? e.message : String(e)}`));
    return;
  }

  themeBtn.addEventListener("click", toggleTheme);
  editor.addEventListener("input", () => {
    window.clearTimeout(editTimer);
    editTimer = window.setTimeout(() => refresh(selectionKey()), EDIT_DEBOUNCE_MS);
  });
  uncertaintyEl.addEventListener("change", () => refresh());
  resetBtn.addEventListener("click", () => {
    overrides = {};
    removed = [];
    buildSliders();
    refresh();
  });
  shareBtn.addEventListener("click", () => {
    const done = (msg: string): void => {
      shareLabel.textContent = msg;
      window.setTimeout(() => (shareLabel.textContent = "Copy share link"), 1500);
    };
    navigator.clipboard.writeText(location.href).then(
      () => done("Copied"),
      () => done("Copy the address bar"),
    );
  });
  const apply = (s: AppState): void => {
    uncertaintyEl.checked = s.uncertainty;
    load(s.yaml, s.overrides, s.removed);
  };
  window.addEventListener("hashchange", () => {
    void decodeState(location.hash).then((s) => {
      const same =
        s && s.yaml === editor.value && JSON.stringify(s.overrides) === JSON.stringify(overrides) && JSON.stringify(s.removed) === JSON.stringify(removed);
      if (s && !same) apply(s);
    });
  });
  window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => graph.restyle());
  graph.onSelect((id) => select(id, true));

  const shared = await decodeState(location.hash);
  renderStart(startEl, startHost, presets);
  if (shared) apply(shared); // a shared link goes straight to its result; a fresh visit shows only the hero and the start cards
}

void start();
