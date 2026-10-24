import { button, field, h, numberInput, textInput } from "./dom";
import type { Preset } from "./presets";
import type { ApiError } from "./types";

/** A model file is a few KB; refusing anything much larger keeps a wrong file (a log, a binary) from freezing the editor. */
export const MAX_UPLOAD_BYTES = 512 * 1024;
const SHOWN_ERRORS = 5;
const STARTER_ROWS = 2;
const DEFAULT_TARGET = 99.9;
const DEFAULT_AVAILABILITY = 99.9;
// Placeholder until the sample models are published somewhere stable.
const SAMPLES_URL = "https://github.com/marcelblanarik/nines";
const SVG_NS = "http://www.w3.org/2000/svg";

/** What the start section needs from the app. */
export interface StartHost {
  /** Errors for the model text, empty when it is valid. */
  validate(yaml: string): readonly ApiError[];
  /** Makes the text the working model and shows the result. */
  open(yaml: string): void;
}

export interface NewComponent {
  name: string;
  availability: number;
}

/** A float the YAML reader will not take for an integer: 99 becomes 99.0. */
function yamlNumber(n: number): string {
  return Number.isInteger(n) ? n.toFixed(1) : String(n);
}

/** The model text for a service that needs every one of its components. JSON strings are valid YAML, so names are always quoted safely. */
export function buildModelYaml(service: string, target: number, components: readonly NewComponent[]): string {
  return [
    `service: ${JSON.stringify(service)}`,
    `target: ${yamlNumber(target)}`,
    "window: 30d",
    "",
    "components:",
    ...components.map((c) => `  ${JSON.stringify(c.name)}: {availability: ${yamlNumber(c.availability)}}`),
    "",
    "requires:",
    "  all:",
    ...components.map((c) => `    - ${JSON.stringify(c.name)}`),
    "",
  ].join("\n");
}

function icon(path: string): SVGSVGElement {
  const svg = document.createElementNS(SVG_NS, "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("aria-hidden", "true");
  const p = document.createElementNS(SVG_NS, "path");
  p.setAttribute("d", path);
  svg.append(p);
  return svg;
}

function errorLine(): HTMLElement {
  const el = h("p", "form-error");
  el.setAttribute("role", "alert");
  return el;
}

function showLines(el: HTMLElement, lines: readonly string[]): void {
  el.replaceChildren(...lines.map((line) => h("span", "", line)));
}

// ---- upload --------------------------------------------------------------------------------------

function uploadCard(host: StartHost): HTMLElement {
  const error = errorLine();
  const input = h("input");
  input.type = "file";
  input.accept = ".yaml,.yml,.txt,text/yaml";
  input.hidden = true;

  const accept = async (file: File | undefined): Promise<void> => {
    if (!file) return;
    try {
      if (file.size > MAX_UPLOAD_BYTES) throw new Error(`${file.name} is larger than ${MAX_UPLOAD_BYTES / 1024} KB. A model file is normally a few KB.`);
      const yaml = await file.text();
      if (yaml.includes("\u0000")) throw new Error(`${file.name} is not a text file.`);
      const errors = host.validate(yaml);
      if (errors.length) {
        const lines = errors.slice(0, SHOWN_ERRORS).map((e) => (e.line ? `line ${e.line}: ${e.msg}` : e.msg));
        const more = errors.length > SHOWN_ERRORS ? [`and ${errors.length - SHOWN_ERRORS} more`] : [];
        showLines(error, [`${file.name} is not a valid model:`, ...lines, ...more]);
        return;
      }
      error.replaceChildren();
      host.open(yaml);
    } catch (e) {
      showLines(error, [e instanceof Error ? e.message : String(e)]);
    }
  };

  input.addEventListener("change", () => {
    void accept(input.files?.[0]);
    input.value = ""; // choosing the same file again must still fire "change"
  });

  const choose = button("Choose YAML file", "primary", () => input.click());
  const zone = h(
    "div",
    "dropzone",
    icon("M12 16V4m0 0L7 9m5-5 5 5M4 16v3a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-3"),
    h("p", "", "Drag a model file here, or"),
    choose,
    input,
  );
  zone.addEventListener("dragenter", (e) => {
    e.preventDefault();
    zone.classList.add("over");
  });
  zone.addEventListener("dragover", (e) => e.preventDefault()); // without this the browser refuses the drop
  zone.addEventListener("dragleave", (e) => {
    if (!zone.contains(e.relatedTarget as Node | null)) zone.classList.remove("over");
  });
  zone.addEventListener("drop", (e) => {
    e.preventDefault();
    zone.classList.remove("over");
    void accept(e.dataTransfer?.files[0]);
  });

  const samples = h("a", "samples", "Browse sample models on GitHub", icon("M7 17 17 7M8 7h9v9"), h("span", "sr-only", " (opens in a new tab)"));
  samples.href = SAMPLES_URL;
  samples.target = "_blank";
  samples.rel = "noopener noreferrer";

  return h(
    "div",
    "start-card",
    h("h2", "", "Upload a model"),
    h("p", "start-sub", "A YAML file with your service, its components and how they depend on each other. It is read in your browser and never uploaded."),
    zone,
    samples,
    error,
  );
}

// ---- build ---------------------------------------------------------------------------------------

interface Row {
  el: HTMLElement;
  name: HTMLInputElement;
  availability: HTMLInputElement;
  remove: HTMLButtonElement;
}

function componentRow(index: number, onRemove: (row: Row) => void): Row {
  const name = textInput("", "e.g. database");
  name.setAttribute("aria-label", `Component ${index} name`);
  const availability = numberInput(DEFAULT_AVAILABILITY, { min: 0, max: 100, step: 0.001 });
  availability.setAttribute("aria-label", `Component ${index} availability (%)`);
  const remove = button("Remove", "ghost small");
  remove.setAttribute("aria-label", `Remove component ${index}`);
  const el = h("div", "row", name, availability, remove);
  const row: Row = { el, name, availability, remove };
  remove.addEventListener("click", () => onRemove(row));
  return row;
}

function buildCard(host: StartHost): HTMLElement {
  const error = errorLine();
  const service = textInput("my-service", "e.g. checkout");
  const target = numberInput(DEFAULT_TARGET, { min: 0, max: 100, step: 0.001 });
  const list = h("div", "rows");
  let rows: Row[] = [];
  let created = 0;

  const syncRemove = (): void => {
    for (const r of rows) r.remove.disabled = rows.length === 1;
  };
  const addRow = (focus: boolean): void => {
    created += 1;
    const row = componentRow(created, (gone) => {
      rows = rows.filter((r) => r !== gone);
      gone.el.remove();
      syncRemove();
    });
    rows = [...rows, row];
    list.append(row.el);
    syncRemove();
    if (focus) row.name.focus();
  };
  for (let i = 0; i < STARTER_ROWS; i++) addRow(false);

  const create = (): void => {
    const name = service.value.trim();
    const t = Number(target.value);
    if (!name) return showLines(error, ["Give the service a name."]);
    if (target.value.trim() === "" || !Number.isFinite(t)) return showLines(error, ["The target must be a number."]);

    const filled = rows.filter((r) => r.name.value.trim() !== "");
    if (!filled.length) return showLines(error, ["Add at least one component."]);
    const components: NewComponent[] = [];
    for (const r of filled) {
      const a = Number(r.availability.value);
      if (r.availability.value.trim() === "" || !Number.isFinite(a)) return showLines(error, [`${r.name.value.trim()}: availability must be a number.`]);
      components.push({ name: r.name.value.trim(), availability: a });
    }
    const names = components.map((c) => c.name);
    const dup = names.find((n, i) => names.indexOf(n) !== i);
    if (dup) return showLines(error, [`"${dup}" appears twice. Component names must be unique.`]);

    const yaml = buildModelYaml(name, t, components);
    // The text is generated, so its line numbers mean nothing to the user: show the messages alone.
    const errors = host.validate(yaml);
    if (errors.length) return showLines(error, errors.slice(0, SHOWN_ERRORS).map((e) => e.msg));
    error.replaceChildren();
    host.open(yaml);
  };

  const form = h(
    "form",
    "start-form",
    h("div", "f-row", field("Service name", service), field("Target availability (%)", target)),
    h("div", "rows-head", h("span", "", "Component"), h("span", "", "Availability (%)"), h("span", "")),
    list,
    h("div", "actions", button("Add component", "ghost", () => addRow(true)), button("Create model", "primary", create)),
    error,
  );
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    create();
  });

  return h(
    "div",
    "start-card",
    h("h2", "", "Build one here"),
    h("p", "start-sub", "List what your service needs. Every component is required; refine how they combine afterwards in the editor."),
    form,
  );
}

/** Renders the two ways in under the hero: upload a model file, or describe a service by hand; plus ready-made examples. */
export function renderStart(container: HTMLElement, host: StartHost, examples: readonly Preset[]): void {
  container.replaceChildren(
    h("div", "start-grid", uploadCard(host), buildCard(host)),
    h("div", "examples", h("p", "", "Or start from an example:"), ...examples.map((e) => button(e.name, "ghost small", () => host.open(e.yaml)))),
  );
}
