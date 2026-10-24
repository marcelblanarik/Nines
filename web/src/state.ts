/** Everything the app needs to reproduce a view: the model text and the slider overrides. */
export interface AppState {
  yaml: string;
  /** Component name to overridden availability (percent). */
  overrides: Record<string, number>;
  /** Components removed in the what-if (the model text itself is untouched). */
  removed: string[];
  /** Whether the uncertainty run is on. */
  uncertainty: boolean;
}

const VERSION = "v1.";

async function pipe(bytes: Uint8Array, stream: CompressionStream | DecompressionStream): Promise<Uint8Array> {
  const out = new Response(new Blob([bytes as Uint8Array<ArrayBuffer>]).stream().pipeThrough(stream));
  return new Uint8Array(await out.arrayBuffer());
}

function toBase64Url(bytes: Uint8Array): string {
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
}

function fromBase64Url(text: string): Uint8Array {
  const bin = atob(text.replaceAll("-", "+").replaceAll("_", "/"));
  return Uint8Array.from(bin, (c) => c.charCodeAt(0));
}

/** Deflate then base64url, so the whole model fits in a URL fragment and never reaches a server. */
export async function encodeState(state: AppState): Promise<string> {
  const json = JSON.stringify({ y: state.yaml, s: state.overrides, r: state.removed, u: state.uncertainty });
  const packed = await pipe(new TextEncoder().encode(json), new CompressionStream("deflate"));
  return VERSION + toBase64Url(packed);
}

function isOverrides(v: unknown): v is Record<string, number> {
  return (
    typeof v === "object" &&
    v !== null &&
    !Array.isArray(v) &&
    Object.values(v).every((x) => typeof x === "number" && Number.isFinite(x))
  );
}

/** Returns null for anything that is not a state this app produced. */
export async function decodeState(fragment: string): Promise<AppState | null> {
  const text = fragment.replace(/^#/, "");
  if (!text.startsWith(VERSION)) return null;
  try {
    const bytes = await pipe(fromBase64Url(text.slice(VERSION.length)), new DecompressionStream("deflate"));
    const data: unknown = JSON.parse(new TextDecoder().decode(bytes));
    if (typeof data !== "object" || data === null) return null;
    const { y, s, r, u } = data as { y?: unknown; s?: unknown; r?: unknown; u?: unknown };
    if (typeof y !== "string" || !isOverrides(s)) return null;
    // r and u are newer than the first links, so they may be missing.
    const removed = Array.isArray(r) && r.every((x) => typeof x === "string") ? (r as string[]) : [];
    return { yaml: y, overrides: s, removed, uncertainty: u === true };
  } catch {
    return null;
  }
}
