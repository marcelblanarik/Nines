import { describe, expect, it } from "vitest";
import { decodeState, encodeState, type AppState } from "./state";

const sample: AppState = {
  yaml: "service: s\ntarget: 99.9\ncomponents:\n  a: {availability: 99.95}\nrequires: a\n# ünïcode ✓\n",
  overrides: { a: 99.99, "db-primary": 99.5 },
  removed: ["db-replica"],
  uncertainty: true,
};

describe("state codec", () => {
  it("round-trips the model and the overrides exactly", async () => {
    const fragment = await encodeState(sample);
    expect(await decodeState(fragment)).toEqual(sample);
    expect(await decodeState("#" + fragment)).toEqual(sample);
  });

  it("produces a URL-safe fragment", async () => {
    const fragment = await encodeState(sample);
    expect(fragment).toMatch(/^v1\.[A-Za-z0-9_-]+$/);
  });

  it("compresses a long model well below its raw size", async () => {
    const yaml = Array.from({ length: 200 }, (_, i) => `  dep-${i}: {availability: 99.9}`).join("\n");
    const fragment = await encodeState({ yaml, overrides: {}, removed: [], uncertainty: false });
    expect(fragment.length).toBeLessThan(yaml.length / 3);
  });

  it("still opens links made before removals and uncertainty existed", async () => {
    const old = { y: sample.yaml, s: sample.overrides };
    const bytes = new TextEncoder().encode(JSON.stringify(old));
    const packed = new Uint8Array(await new Response(new Blob([bytes]).stream().pipeThrough(new CompressionStream("deflate"))).arrayBuffer());
    const fragment = "v1." + btoa(String.fromCharCode(...packed)).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
    expect(await decodeState(fragment)).toEqual({ yaml: sample.yaml, overrides: sample.overrides, removed: [], uncertainty: false });
  });

  it("rejects fragments it did not produce", async () => {
    expect(await decodeState("")).toBeNull();
    expect(await decodeState("v2.abc")).toBeNull();
    expect(await decodeState("v1.%%%")).toBeNull();
    expect(await decodeState("v1.AAAA")).toBeNull();
  });
});
