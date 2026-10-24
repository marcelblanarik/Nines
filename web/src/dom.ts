export type Child = Node | string | null | false;

/** Builds an element. Strings become text nodes, so user-supplied names can never inject markup. */
export function h<K extends keyof HTMLElementTagNameMap>(tag: K, className = "", ...kids: Child[]): HTMLElementTagNameMap[K] {
  const el = document.createElement(tag);
  if (className) el.className = className;
  for (const kid of kids) if (kid) el.append(kid);
  return el;
}

export function byId<T extends HTMLElement>(id: string): T {
  const el = document.getElementById(id);
  if (!el) throw new Error(`missing #${id}`);
  return el as T;
}

/** A labelled form control: <label>text<control/></label>. */
export function field(text: string, control: HTMLElement, className = "f"): HTMLLabelElement {
  return h("label", className, h("span", "", text), control);
}

export function numberInput(value: number, opts: { min?: number; max?: number; step?: number } = {}): HTMLInputElement {
  const input = h("input");
  input.type = "number";
  input.value = String(value);
  if (opts.min !== undefined) input.min = String(opts.min);
  if (opts.max !== undefined) input.max = String(opts.max);
  input.step = String(opts.step ?? "any");
  return input;
}

export function textInput(value: string, placeholder = ""): HTMLInputElement {
  const input = h("input");
  input.type = "text";
  input.value = value;
  input.placeholder = placeholder;
  input.autocomplete = "off";
  input.spellcheck = false;
  return input;
}

export function checkbox(checked: boolean): HTMLInputElement {
  const input = h("input");
  input.type = "checkbox";
  input.checked = checked;
  return input;
}

/** A <select> with the given [value, label] options. */
export function select(options: readonly (readonly [string, string])[], value = ""): HTMLSelectElement {
  const el = h("select");
  for (const [v, label] of options) {
    const o = h("option", "", label);
    o.value = v;
    el.append(o);
  }
  // Leave the browser's default (the first option) unless the requested value is one of the options.
  if (options.some(([v]) => v === value)) el.value = value;
  return el;
}

export function button(text: string, className = "", onClick?: () => void): HTMLButtonElement {
  const b = h("button", `btn ${className}`.trim(), text);
  b.type = "button";
  if (onClick) b.addEventListener("click", onClick);
  return b;
}
