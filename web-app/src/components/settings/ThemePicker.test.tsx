import { render, screen, fireEvent } from "@testing-library/react";
import { ThemePicker } from "./ThemePicker";

const setTheme = jest.fn();

jest.mock("@/lib/contexts/ThemeContext", () => ({
  useTheme: () => ({
    theme: "custom:netflix",
    setTheme,
    availableThemes: ["clean"],
    customThemes: [
      { id: "netflix", label: "Netflix", description: "red on black", base: "dark", tokens: { "color.primary": "#e50914" } },
    ],
  }),
}));

describe("ThemePicker custom themes", () => {
  it("lists a user theme, marks it active, and selects it by custom id", () => {
    render(<ThemePicker />);
    const radio = screen.getByRole("radio", { name: /Netflix/ });
    expect(radio).toHaveAttribute("aria-checked", "true");
    fireEvent.click(screen.getByRole("radio", { name: /Clean/i }));
    expect(setTheme).toHaveBeenCalledWith("clean");
    fireEvent.click(radio);
    expect(setTheme).toHaveBeenCalledWith("custom:netflix");
  });
});
