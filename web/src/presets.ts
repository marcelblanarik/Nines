import fanOut from "./presets/fan-out.yaml?raw";
import threeTier from "./presets/three-tier.yaml?raw";
import twoRegions from "./presets/two-regions.yaml?raw";

export interface Preset {
  id: string;
  name: string;
  yaml: string;
}

export const presets: readonly Preset[] = [
  { id: "three-tier", name: "3-tier app", yaml: threeTier },
  { id: "two-regions", name: "Active/active across two regions", yaml: twoRegions },
  { id: "fan-out", name: "20-way fan-out", yaml: fanOut },
];
