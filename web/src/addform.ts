import { button, checkbox, field, h, numberInput, select, textInput } from "./dom";
import type { EditHost } from "./inspector";
import type { EditOp } from "./types";

const DEFAULT_AVAILABILITY = 99.9;

/** Renders the "add a component" form: a new component, and how the service depends on it. */
export function renderAddForm(container: HTMLElement, host: EditHost): void {
  const name = textInput("", "e.g. payments-api");
  const percent = numberInput(DEFAULT_AVAILABILITY, { step: 0.001 });
  const owned = checkbox(false);
  const how = select([
    ["required", "Required: the service needs it (in series)"],
    ["redundant", "Redundant: an alternative to another component"],
    ["unused", "Not used yet: just add it"],
  ]);
  const alternativeTo = select(host.components().map((c): [string, string] => [c, c]));
  const alternativeField = field("Alternative to", alternativeTo);
  const error = h("p", "form-error");
  error.setAttribute("role", "alert");

  const sync = (): void => {
    alternativeField.hidden = how.value !== "redundant";
  };
  how.addEventListener("change", sync);
  sync();

  const add = (): void => {
    const componentName = name.value.trim();
    const p = Number(percent.value);
    if (!componentName) {
      error.textContent = "Give the component a name.";
      return;
    }
    if (percent.value.trim() === "" || !Number.isFinite(p)) {
      error.textContent = "Availability must be a number.";
      return;
    }
    const ops: EditOp[] = [{ op: "add_component", name: componentName, component: { kind: "point", percent: p, owned: owned.checked } }];
    if (how.value === "required") ops.push({ op: "add_requirement", name: componentName });
    if (how.value === "redundant") ops.push({ op: "add_redundant", name: componentName, to: alternativeTo.value });
    const message = host.apply(ops);
    error.textContent = message ?? "";
    if (message === null) name.value = "";
  };

  container.replaceChildren(
    h(
      "form",
      "inspect",
      h("div", "f-row", field("Name", name), field("Availability (%)", percent)),
      field("How the service uses it", how),
      alternativeField,
      field("Owned by my team", owned, "f check"),
      h("div", "actions", button("Add to model", "primary", add)),
      error,
      h("p", "note", "Edits made here rewrite the model text in canonical form, so comments in it are dropped."),
    ),
  );
  container.querySelector("form")?.addEventListener("submit", (e) => {
    e.preventDefault();
    add();
  });
}
