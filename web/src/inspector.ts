import { button, checkbox, field, h, numberInput, select, textInput } from "./dom";
import type { ComponentInfo, EditOp, GraphNode } from "./types";

/** What the edit forms need from the app. */
export interface EditHost {
  /** Names of the components in the model, in order. */
  components(): readonly string[];
  /** Applies the edits to the model text. Returns an error message, or null on success. */
  apply(ops: EditOp[]): string | null;
}

const OPERATOR_TITLES: Record<string, string> = {
  all: "All of: every member must work",
  any: "Any of: at least one member must work",
  k_of_n: "K of N: at least k members must work",
  retry: "Retry: tries a component several times",
  fallback: "Fallback: a second option when the first fails",
  cached: "Cache: served from cache or from the origin",
  optional: "Optional: only some requests need it",
};

const RATIO_LABELS: Record<string, string> = {
  fallback: "Share of requests the fallback can serve (%)",
  cached: "Cache hit ratio (%)",
  optional: "Share of requests that need it (%)",
};

function num(input: HTMLInputElement, what: string): number {
  const v = Number(input.value);
  if (input.value.trim() === "" || !Number.isFinite(v)) throw new Error(`${what} must be a number`);
  return v;
}

/** Runs a form action, turning a thrown validation message into the form's error line. */
function guarded(errorEl: HTMLElement, host: EditHost, build: () => EditOp[]): () => void {
  return () => {
    try {
      errorEl.textContent = host.apply(build()) ?? "";
    } catch (e) {
      errorEl.textContent = e instanceof Error ? e.message : String(e);
    }
  };
}

function componentForm(n: GraphNode, info: ComponentInfo, host: EditHost): HTMLElement {
  const name = n.component ?? "";
  const error = h("p", "form-error");
  error.setAttribute("role", "alert");

  const kind = select(
    [
      ["point", "A percentage"],
      ["interval", "An uncertain range"],
      ["counts", "Measured counts"],
    ],
    info.kind,
  );
  const percent = numberInput(info.kind === "point" ? info.percent : n.availability ?? 99.9, { step: 0.001 });
  const lo = numberInput(info.kind === "interval" ? info.lo : info.percent, { step: 0.001 });
  const hi = numberInput(info.kind === "interval" ? info.hi : info.percent, { step: 0.001 });
  const good = numberInput(info.kind === "counts" ? info.good : 0, { min: 0, step: 1 });
  const total = numberInput(info.kind === "counts" ? info.total : 0, { min: 0, step: 1 });
  const owned = checkbox(info.owned);
  const source = textInput(info.source, "e.g. vendor-sla, measured");
  const transient = numberInput(info.transient_fraction, { min: 0, max: 1, step: 0.05 });
  const group = select([["", "None"], ...host.components().filter((c) => c !== name).map((c): [string, string] => [c, c])], info.group);

  const point = h("div", "f-row", field("Availability (%)", percent));
  const range = h("div", "f-row", field("Lower bound (%)", lo), field("Upper bound (%)", hi));
  const counts = h("div", "f-row", field("Good", good), field("Total", total));
  const showKind = (): void => {
    point.hidden = kind.value !== "point";
    range.hidden = kind.value !== "interval";
    counts.hidden = kind.value !== "counts";
  };
  kind.addEventListener("change", showKind);
  showKind();

  const apply = guarded(error, host, () => {
    const component: Partial<ComponentInfo> = {
      kind: kind.value as ComponentInfo["kind"],
      owned: owned.checked,
      source: source.value.trim(),
      transient_fraction: num(transient, "Transient fraction"),
      group: group.value,
    };
    if (kind.value === "point") component.percent = num(percent, "Availability");
    if (kind.value === "interval") Object.assign(component, { lo: num(lo, "Lower bound"), hi: num(hi, "Upper bound") });
    if (kind.value === "counts") Object.assign(component, { good: num(good, "Good"), total: num(total, "Total") });
    return [{ op: "update_component", name, component }];
  });
  const remove = guarded(error, host, () => {
    if (!window.confirm(`Delete ${name} from the model? Every place that uses it is removed too.`)) return [];
    return [{ op: "delete_component", name }];
  });

  return h(
    "form",
    "inspect",
    h("h3", "", name),
    field("How availability is known", kind),
    point,
    range,
    counts,
    h("div", "f-row", field("Owned by my team", owned, "f check"), field("Source", source)),
    h(
      "div",
      "f-row",
      field("Share of failures a retry fixes (0 to 1)", transient),
      field("Shares fate with (group)", group),
    ),
    h("div", "actions", button("Apply", "primary", apply), button("Delete component", "ghost danger", remove)),
    error,
  );
}

function operatorForm(n: GraphNode, host: EditHost): HTMLElement {
  const index = n.node ?? -1;
  const params = n.params ?? { members: 0 };
  const error = h("p", "form-error");
  error.setAttribute("role", "alert");
  const body: HTMLElement[] = [];
  let apply: (() => void) | null = null;

  if (n.kind === "all" || n.kind === "any") {
    const kind = select(
      [
        ["all", "All of (every member must work)"],
        ["any", "Any of (redundancy: one is enough)"],
      ],
      n.kind,
    );
    body.push(field("Behaviour", kind));
    apply = guarded(error, host, () => [{ op: "set_node", index, kind: kind.value as "all" | "any" }]);
  } else if (n.kind === "k_of_n") {
    const k = numberInput(params.k ?? 1, { min: 1, step: 1 });
    body.push(field(`k (out of ${params.members} members)`, k));
    apply = guarded(error, host, () => [{ op: "set_node", index, k: num(k, "k") }]);
  } else if (n.kind === "retry") {
    const attempts = numberInput(params.attempts ?? 1, { min: 1, step: 1 });
    body.push(field("Attempts", attempts));
    apply = guarded(error, host, () => [{ op: "set_node", index, attempts: num(attempts, "Attempts") }]);
  } else if (n.kind in RATIO_LABELS) {
    const ratio = numberInput(Number(((params.ratio ?? 0) * 100).toFixed(4)), { min: 0, max: 100, step: 1 });
    body.push(field(RATIO_LABELS[n.kind] ?? "Ratio (%)", ratio));
    apply = guarded(error, host, () => [{ op: "set_node", index, ratio: num(ratio, "Ratio") / 100 }]);
  }

  const out: HTMLElement[] = [h("h3", "", OPERATOR_TITLES[n.kind] ?? n.kind), ...body];
  if (apply) out.push(h("div", "actions", button("Apply", "primary", apply)));

  if (n.kind === "all" || n.kind === "any" || n.kind === "k_of_n") {
    const names = host.components();
    if (names.length) {
      const member = select(names.map((c): [string, string] => [c, c]));
      const add = guarded(error, host, () => [{ op: "add_child", index, name: member.value }]);
      out.push(h("div", "f-row add-member", field("Add a member", member), button("Add", "ghost", add)));
    }
  }
  out.push(error);
  return h("form", "inspect", ...out);
}

/** Renders the editor for the selected graph node, or a hint when nothing is selected. */
export function renderInspector(container: HTMLElement, node: GraphNode | null, host: EditHost): void {
  container.replaceChildren();
  if (!node) {
    container.append(h("p", "note", "Click a node in the graph, or press Edit on a component below, to change it here."));
    return;
  }
  const form = node.kind === "component" && node.info ? componentForm(node, node.info, host) : node.kind === "service" ? null : operatorForm(node, host);
  container.append(form ?? h("p", "note", "This is the service itself. Edit its name or target in the model text."));
  container.querySelector("form")?.addEventListener("submit", (e) => e.preventDefault());
}
