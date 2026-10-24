import type { ApiResponse, CheckOptions, CheckResult, Diff, EditOp, GraphData, Override, Trace } from "./types";

/** The core, wrapped so callers get typed objects instead of JSON strings. */
export interface Core {
  check(yaml: string, opts: CheckOptions): ApiResponse<CheckResult>;
  explain(yaml: string, overrides: Override[], removes: string[]): ApiResponse<Trace>;
  whatif(yaml: string, overrides: Override[], removes: string[], opts: CheckOptions): ApiResponse<Diff>;
  /** Applies structural edits and returns the edited model as YAML (canonical form, comments dropped). */
  edit(yaml: string, ops: EditOp[]): ApiResponse<{ yaml: string }>;
  graph(yaml: string): ApiResponse<GraphData>;
}

interface RawCore {
  check(yaml: string, opts: string): string;
  explain(yaml: string, sets: string, removes: string): string;
  whatif(yaml: string, sets: string, removes: string, opts: string): string;
  edit(yaml: string, ops: string): string;
  graph(yaml: string): string;
}

declare class Go {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const el = document.createElement("script");
    el.src = src;
    el.onload = () => resolve();
    el.onerror = () => reject(new Error(`could not load ${src}`));
    document.head.append(el);
  });
}

/** Loads nines.wasm (the only requests the app itself makes; the page's Umami script is separate) and returns the typed core. */
export async function loadCore(): Promise<Core> {
  await loadScript("./wasm_exec.js");
  const go = new Go();
  const { instance } = await WebAssembly.instantiateStreaming(fetch("./nines.wasm"), go.importObject);
  void go.run(instance);
  const raw = (globalThis as unknown as { nines?: RawCore }).nines;
  if (!raw) throw new Error("the WebAssembly core did not register itself");
  const parse = <T>(json: string): ApiResponse<T> => JSON.parse(json) as ApiResponse<T>;
  const json = (v: unknown, empty: boolean): string => (empty ? "" : JSON.stringify(v));
  return {
    check: (yaml, opts) => parse(raw.check(yaml, JSON.stringify(opts))),
    explain: (yaml, overrides, removes) =>
      parse(raw.explain(yaml, json(overrides, !overrides.length), json(removes, !removes.length))),
    whatif: (yaml, overrides, removes, opts) =>
      parse(raw.whatif(yaml, json(overrides, !overrides.length), json(removes, !removes.length), JSON.stringify(opts))),
    edit: (yaml, ops) => parse(raw.edit(yaml, JSON.stringify(ops))),
    graph: (yaml) => parse(raw.graph(yaml)),
  };
}
