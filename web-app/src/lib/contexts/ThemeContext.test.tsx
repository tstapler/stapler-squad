import { render, screen, act, waitFor } from "@testing-library/react";
import { ThemeProvider, useTheme } from "./ThemeContext";
import { vars } from "@/styles/theme-contract.css";

// Jest stubs .css.ts files with a proxy; supply the real shape (`var(--name__hash)`) vanilla-extract emits.
jest.mock("@/styles/theme-contract.css", () => ({
  vars: {
    color: { primary: "var(--color-primary__h1)", statusDot: { running: "var(--color-statusDot-running__h2)" } },
  },
}));

const netflix = {
  id: "netflix",
  label: "Netflix",
  base: "dark",
  tokens: { "color.primary": "#e50914", "color.statusDot.running": "#46d369", "color.nope": "#000" },
};

function propName(cssVar: string): string {
  return /^var\((--[^),\s]+)/.exec(cssVar)![1];
}

function Probe() {
  const { theme, setTheme, customThemes } = useTheme();
  return (
    <div>
      <span data-testid="theme">{theme}</span>
      <span data-testid="count">{customThemes.length}</span>
      <button onClick={() => setTheme("custom:netflix")}>netflix</button>
      <button onClick={() => setTheme("clean")}>clean</button>
    </div>
  );
}

describe("ThemeProvider user themes", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("style");
    global.fetch = jest.fn().mockResolvedValue({ ok: true, json: async () => ({ themes: [netflix] }) });
  });

  it("applies token overrides for a custom theme and clears them on switch", async () => {
    render(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("count").textContent).toBe("1"));

    act(() => screen.getByText("netflix").click());
    const style = document.documentElement.style;
    expect(style.getPropertyValue(propName(vars.color.primary))).toBe("#e50914");
    expect(style.getPropertyValue(propName(vars.color.statusDot.running))).toBe("#46d369");

    act(() => screen.getByText("clean").click());
    expect(style.getPropertyValue(propName(vars.color.primary))).toBe("");
  });

  it("re-applies a persisted custom theme once /api/themes answers", async () => {
    localStorage.setItem("stapler-theme", "custom:netflix");
    render(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>,
    );
    await waitFor(() =>
      expect(document.documentElement.style.getPropertyValue(propName(vars.color.primary))).toBe("#e50914"),
    );
  });

  it("keeps a theme chosen while /api/themes is in flight", async () => {
    localStorage.setItem("stapler-theme", "custom:netflix");
    let resolveFetch!: (v: unknown) => void;
    global.fetch = jest.fn().mockReturnValue(new Promise((r) => (resolveFetch = r)));
    render(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>,
    );
    act(() => screen.getByText("clean").click());
    await act(async () => resolveFetch({ ok: true, json: async () => ({ themes: [netflix] }) }));
    expect(screen.getByTestId("theme").textContent).toBe("clean");
    expect(document.documentElement.style.getPropertyValue(propName(vars.color.primary))).toBe("");
  });

  it("falls back to the default when the persisted custom theme is gone", async () => {
    localStorage.setItem("stapler-theme", "custom:missing");
    render(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("theme").textContent).toBe("clean"));
    expect(localStorage.getItem("stapler-theme")).toBe("clean");
    expect(localStorage.getItem("stapler-theme-custom")).toBeNull();
  });

  it("caches the applied custom theme for the FOUC script", async () => {
    render(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("count").textContent).toBe("1"));
    act(() => screen.getByText("netflix").click());
    const cache = JSON.parse(localStorage.getItem("stapler-theme-custom")!);
    expect(cache.id).toBe("custom:netflix");
    expect(cache.props[propName(vars.color.primary)]).toBe("#e50914");
  });
});
