import { describe, expect, it } from "vitest";
import { SLIDER, downtime, pct, percentFromPosition, positionFromPercent } from "./format";

describe("pct matches the CLI", () => {
  it.each([
    [0.98018993, "98.02%"],
    [0.9995, "99.95%"],
    [0.999, "99.90%"],
    [0.99909988, "99.91%"],
    [0.99999, "99.999%"],
    [1, "100.00%"],
    [0.972, "97.20%"],
  ])("%s", (f, want) => expect(pct(f)).toBe(want));
});

describe("downtime", () => {
  it("scales the unit of precision", () => {
    expect(downtime(0.9995, 30)).toBe("21.6 min");
    expect(downtime(0.98, 30)).toBe("864 min");
    expect(downtime(0.999999, 30)).toBe("0.04 min");
  });
});

describe("percent slider", () => {
  it("moves in 0.1 steps", () => {
    expect(percentFromPosition(positionFromPercent(90))).toBe(90);
    expect(percentFromPosition(positionFromPercent(90) + 1)).toBe(90.1);
    expect(percentFromPosition(positionFromPercent(99.8) + 1)).toBe(99.9);
  });
  it("tops out at 99.999%, one step after 99.9%", () => {
    expect(percentFromPosition(SLIDER.max)).toBe(99.999);
    expect(percentFromPosition(SLIDER.max - 1)).toBe(99.9);
    expect(percentFromPosition(SLIDER.max + 5)).toBe(99.999);
    expect(positionFromPercent(100)).toBe(SLIDER.max);
    expect(positionFromPercent(99.99)).toBe(SLIDER.max);
  });
  it("starts above 50%, the lowest the model accepts", () => {
    expect(percentFromPosition(SLIDER.min)).toBe(50.1);
    expect(positionFromPercent(10)).toBe(SLIDER.min);
  });
  it("round-trips whole-tenth values", () => {
    for (const p of [51.3, 90, 99, 99.5, 99.9]) {
      expect(percentFromPosition(positionFromPercent(p))).toBe(p);
    }
  });
});
