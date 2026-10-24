import cytoscape from "cytoscape";
import dagre from "cytoscape-dagre";
import type { GraphData } from "./types";

cytoscape.use(dagre);

export interface GraphView {
  render(data: GraphData): void;
  /** Called with a graph node id when a node is clicked, or null when the background is. */
  onSelect(handler: (id: string | null) => void): void;
  /** Highlight one node; null clears. */
  select(id: string | null): void;
  /** Mark components removed in the what-if. */
  markRemoved(components: readonly string[]): void;
  /** Outline the components that are single points of failure. */
  markSpofs(components: readonly string[]): void;
  /** Re-read colours from CSS, e.g. after a light/dark switch. */
  restyle(): void;
  /** Zooms about the centre of the view; above 1 zooms in. */
  zoomBy(factor: number): void;
  /** Shows the whole graph. */
  fit(): void;
  /** Pans so the node is not under `region` (page coordinates), e.g. a drawer that opened over the graph. */
  keepClear(id: string, region: DOMRect): void;
}

/** Breathing room between a node and the edge of whatever was moved away from. */
const KEEP_CLEAR_PX = 16;
/** A region narrower than this share of the graph is a side drawer; a wider one is a sheet from the bottom. */
const SIDE_DRAWER_MAX_SHARE = 0.8;

const FIT_PADDING_PX = 24;

const CHAR_PX = 6.4;
const LINE_PX = 15;
const PAD_PX = 18;

/** Node box size from its label; cytoscape's own "label" sizing is deprecated. */
function boxSize(label: string): { w: number; h: number } {
  const lines = label.split("\n");
  return { w: Math.max(...lines.map((l) => l.length)) * CHAR_PX + PAD_PX, h: lines.length * LINE_PX + PAD_PX / 2 };
}

function css(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

function stylesheet(): cytoscape.StylesheetJson {
  const text = css("--text");
  return [
    {
      selector: "node",
      style: {
        label: "data(label)",
        "text-wrap": "wrap",
        "text-valign": "center",
        "text-halign": "center",
        "font-size": 12,
        "font-family": css("--font"),
        color: text,
        "background-color": css("--node"),
        "border-width": 1,
        "border-color": css("--control"),
        shape: "round-rectangle",
        width: "data(w)",
        height: "data(h)",
      },
    },
    { selector: 'node[kind = "service"]', style: { "background-color": css("--accent"), "border-color": css("--accent"), color: css("--accent-ink"), "font-weight": "bold" } },
    { selector: 'node[kind != "component"][kind != "service"]', style: { shape: "hexagon", "background-color": css("--op"), "font-size": 10 } },
    { selector: "node[?owned]", style: { "border-width": 3, "border-color": css("--accent") } },
    { selector: "node.removed", style: { opacity: 0.35, "border-style": "dashed" } },
    { selector: "node:selected", style: { "overlay-color": css("--accent"), "overlay-opacity": 0.18, "overlay-padding": 6 } },
    { selector: "node.spof", style: { "border-width": 3, "border-color": css("--bad") } },
    {
      selector: "edge",
      style: {
        width: 1.5,
        "line-color": css("--edge"),
        "target-arrow-color": css("--edge"),
        "target-arrow-shape": "triangle",
        "curve-style": "bezier",
        label: "data(label)",
        "font-size": 9,
        color: css("--muted"),
        "text-background-color": css("--surface"),
        "text-background-opacity": 1,
        "text-background-padding": "2px",
      },
    },
    { selector: 'edge[kind = "group"]', style: { "line-style": "dashed" } },
  ];
}

export function createGraph(container: HTMLElement): GraphView {
  // The graph fills its own pane, so the wheel and pinch zoom it and dragging pans it.
  const cy = cytoscape({ container, style: stylesheet(), minZoom: 0.2, maxZoom: 2.5 });
  // What the view looked like right after the last fit, to tell whether the user has since zoomed or panned.
  let fitted = { zoom: 0, x: 0, y: 0 };
  const fitToView = (): void => {
    cy.fit(undefined, FIT_PADDING_PX);
    fitted = { zoom: cy.zoom(), x: cy.pan().x, y: cy.pan().y };
  };
  const untouched = (): boolean => cy.zoom() === fitted.zoom && cy.pan().x === fitted.x && cy.pan().y === fitted.y;
  // A pane that was hidden (another view on a narrow screen) has no size, so the first fit has to wait until it shows.
  let fitPending = false;
  new ResizeObserver(() => {
    cy.resize();
    if (container.clientWidth === 0) return;
    // The pane changed size: keep the whole graph in view unless the user has moved it.
    if (fitPending || untouched()) fitToView();
    fitPending = false;
  }).observe(container);
  let spofs: readonly string[] = [];
  let removed: readonly string[] = [];

  const applySpofs = (): void => {
    cy.nodes().removeClass("spof");
    cy.nodes().filter((n) => spofs.includes(String(n.data("component")))).addClass("spof");
  };

  const applyRemoved = (): void => {
    cy.nodes().removeClass("removed");
    cy.nodes().filter((n) => removed.includes(String(n.data("component")))).addClass("removed");
  };

  return {
    onSelect(handler) {
      cy.on("tap", (e) => {
        if (e.target === cy) handler(null);
        else if (e.target.isNode()) handler(String(e.target.id()));
      });
    },
    select(id) {
      cy.nodes().unselect();
      if (id) cy.getElementById(id).select();
    },
    markRemoved(components) {
      removed = components;
      applyRemoved();
    },
    render(data) {
      cy.elements().remove();
      cy.add([
        ...data.nodes.map((n) => ({
          group: "nodes" as const,
          data: { id: n.id, label: n.label, kind: n.kind, owned: n.owned ?? false, component: n.component ?? "", ...boxSize(n.label) },
        })),
        ...data.edges.map((e, i) => ({
          group: "edges" as const,
          data: { id: `e${i}`, source: e.from, target: e.to, label: e.label ?? "", kind: e.kind },
        })),
      ]);
      cy.layout({ name: "dagre", rankDir: "TB", nodeSep: 24, rankSep: 40, animate: false } as cytoscape.LayoutOptions).run();
      cy.resize();
      fitPending = container.clientWidth === 0;
      if (!fitPending) fitToView();
      applySpofs();
      applyRemoved();
    },
    markSpofs(components) {
      spofs = components;
      applySpofs();
    },
    restyle() {
      cy.style(stylesheet());
    },
    zoomBy(factor) {
      cy.zoom({ level: cy.zoom() * factor, renderedPosition: { x: container.clientWidth / 2, y: container.clientHeight / 2 } });
    },
    fit: fitToView,
    keepClear(id, region) {
      const node = cy.getElementById(id);
      if (node.empty()) return;
      const c = container.getBoundingClientRect();
      const bb = node.renderedBoundingBox({});
      const box = { left: c.left + bb.x1, right: c.left + bb.x2, top: c.top + bb.y1, bottom: c.top + bb.y2 };
      const covered = box.right > region.left && box.left < region.right && box.bottom > region.top && box.top < region.bottom;
      if (!covered) return;
      if (region.width < c.width * SIDE_DRAWER_MAX_SHARE) cy.panBy({ x: region.left - box.right - KEEP_CLEAR_PX, y: 0 });
      else cy.panBy({ x: 0, y: region.top - box.bottom - KEEP_CLEAR_PX });
    },
  };
}
