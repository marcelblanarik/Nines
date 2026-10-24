import { describe, expect, it } from "vitest";
import { buildModelYaml } from "./start";

describe("buildModelYaml", () => {
  it("lists every component under components and requires.all", () => {
    const yaml = buildModelYaml("shop", 99.9, [
      { name: "db", availability: 99.95 },
      { name: "api", availability: 99.9 },
    ]);
    expect(yaml).toBe(
      [
        'service: "shop"',
        "target: 99.9",
        "window: 30d",
        "",
        "components:",
        '  "db": {availability: 99.95}',
        '  "api": {availability: 99.9}',
        "",
        "requires:",
        "  all:",
        '    - "db"',
        '    - "api"',
        "",
      ].join("\n"),
    );
  });

  it("writes whole numbers as floats so the model reader keeps them numeric", () => {
    const yaml = buildModelYaml("s", 99, [{ name: "x", availability: 100 }]);
    expect(yaml).toContain("target: 99.0");
    expect(yaml).toContain("availability: 100.0");
  });

  it("quotes names that would otherwise change the YAML", () => {
    const yaml = buildModelYaml('a: "b"', 99.9, [{ name: "yes: {x}, # no", availability: 99.9 }]);
    expect(yaml).toContain('service: "a: \\"b\\""');
    expect(yaml).toContain('  "yes: {x}, # no": {availability: 99.9}');
    expect(yaml).toContain('    - "yes: {x}, # no"');
  });
});
