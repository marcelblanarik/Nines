// Mirrors the JSON produced by the Go core (schema_version 1). Availabilities are fractions unless noted.

export interface Uncertainty {
  samples: number;
  seed: number;
  mean: number;
  p5: number;
  p50: number;
  p95: number;
  p_meets_target: number;
}

export interface Lever {
  component: string;
  availability: number;
  owned: boolean;
  importance: number;
  improvement_potential: number;
}

export interface CheckResult {
  schema_version: number;
  service: string;
  target: number;
  window_days: number;
  method: "bdd" | "monte_carlo";
  availability: number;
  availability_ci?: [number, number];
  naive?: number;
  ceiling: number | null;
  verdict: "achievable" | "marginal" | "infeasible";
  tier: number | null;
  uncertainty?: Uncertainty;
  levers: Lever[];
  spofs: string[];
  cut_sets: string[][];
  cut_sets_truncated?: boolean;
  warnings: string[];
  recommendation: string;
}

export interface TraceNode {
  kind: string;
  label: string;
  probability: number;
  shared_components?: string[];
  children?: TraceNode[];
}

export interface Trace {
  schema_version: number;
  service: string;
  root: TraceNode;
}

export interface DiffRow {
  label: string;
  before: number;
  after: number;
  delta_pp: number;
  delta_downtime_min: number;
}

export interface Diff {
  schema_version: number;
  changes: string[];
  rows: DiffRow[];
  before: CheckResult;
  after: CheckResult;
}

export interface ComponentInfo {
  kind: "point" | "interval" | "counts";
  percent: number;
  lo: number;
  hi: number;
  good: number;
  total: number;
  owned: boolean;
  source: string;
  transient_fraction: number;
  group: string;
}

/** An operator's tunable parameters; those that do not apply to its kind are absent or zero. */
export interface NodeParams {
  k?: number;
  attempts?: number;
  ratio?: number;
  members: number;
}

export interface GraphNode {
  id: string;
  label: string;
  kind: string;
  owned?: boolean;
  component?: string;
  /** Percentage, for components. */
  availability?: number;
  info?: ComponentInfo;
  /** Index of an operator node in the requires tree, which structural edits address it by. */
  node?: number;
  params?: NodeParams;
}

export interface GraphEdge {
  from: string;
  to: string;
  label?: string;
  kind: "requires" | "group";
}

export interface GraphData {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export interface ApiError {
  line?: number;
  msg: string;
}

export type ApiResponse<T> = { ok: true; result: T } | { ok: false; errors: ApiError[] };

/** A what-if override: set a component to a percentage. */
export interface Override {
  component: string;
  percent: number;
}

/** One structural change to the model, applied by the core in order. */
export type EditOp =
  | { op: "add_component"; name: string; component: Partial<ComponentInfo> }
  | { op: "update_component"; name: string; component: Partial<ComponentInfo> }
  | { op: "delete_component"; name: string }
  | { op: "add_requirement"; name: string }
  | { op: "add_redundant"; name: string; to: string }
  | { op: "add_child"; index: number; name: string }
  | { op: "set_node"; index: number; kind?: "all" | "any"; k?: number; attempts?: number; ratio?: number };

export interface CheckOptions {
  uncertainty: boolean;
}
