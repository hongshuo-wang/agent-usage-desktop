import { fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import Layout from "./Layout";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "zh", changeLanguage: vi.fn() },
  }),
}));

describe("Layout", () => {
  it("shows only the primary application navigation", () => {
    localStorage.setItem("au-theme", "light");

    render(
      <MemoryRouter>
        <Layout>
          <div>content</div>
        </Layout>
      </MemoryRouter>,
    );

    expect(screen.getAllByRole("link", { name: "title" })).not.toHaveLength(0);
    expect(screen.getAllByRole("link", { name: "sessionLog" })).not.toHaveLength(0);
    expect(screen.getAllByRole("link", { name: "settings" })).not.toHaveLength(0);
    expect(screen.queryByRole("link", { name: "config" })).not.toBeInTheDocument();
    expect(new Set(screen.getAllByRole("link").map((link) => link.getAttribute("href")))).toEqual(
      new Set(["/", "/sessions", "/settings/data-sources"]),
    );
    expect(screen.getByTestId("desktop-navigation")).toBeInTheDocument();
    expect(screen.getByTestId("mobile-navigation")).toBeInTheDocument();
    expect(within(screen.getByTestId("desktop-navigation")).getByRole("button", { name: "settings" })).toHaveAttribute("aria-expanded", "false");
    expect(within(screen.getByTestId("desktop-navigation")).queryByRole("link", { name: "pricing" })).not.toBeInTheDocument();
    for (const link of screen.getAllByRole("link", { name: "title" })) {
      expect(link).toHaveAttribute("aria-current", "page");
    }
    expect(screen.getAllByRole("link", { name: "sessionLog" })[0]).not.toHaveAttribute("aria-current");
    expect(screen.getAllByRole("main")).toHaveLength(1);
  });

  it("expands system destinations in the primary rail on system routes", () => {
    localStorage.setItem("au-theme", "light");
    render(
      <MemoryRouter initialEntries={["/settings/pricing"]}>
        <Layout><div>pricing content</div></Layout>
      </MemoryRouter>,
    );

    const desktop = screen.getByTestId("desktop-navigation");
    const systemGroup = within(desktop).getByRole("button", { name: "settings" });
    expect(systemGroup).toHaveAttribute("aria-expanded", "true");
    expect(within(desktop).getByRole("link", { name: "dataSources" })).toHaveAttribute("href", "/settings/data-sources");
    expect(within(desktop).getByRole("link", { name: "pricing" })).toHaveAttribute("aria-current", "page");
    expect(within(desktop).getByRole("link", { name: "indexDiagnostics" })).toHaveAttribute("href", "/settings/index-diagnostics");
    expect(within(desktop).getByRole("link", { name: "preferences" })).toHaveAttribute("href", "/settings/preferences");

    fireEvent.click(systemGroup);
    expect(systemGroup).toHaveAttribute("aria-expanded", "false");
    expect(within(desktop).queryByRole("link", { name: "pricing" })).not.toBeInTheDocument();
  });

  it("keeps the header fixed while making the main content vertically scrollable", () => {
    localStorage.setItem("au-theme", "light");
    render(
      <MemoryRouter>
        <Layout>
          <div style={{ minHeight: "1200px" }}>long settings content</div>
        </Layout>
      </MemoryRouter>,
    );

    expect(screen.getByRole("banner")).toHaveClass("sticky");
    expect(screen.getByRole("main")).toHaveClass("overflow-y-auto");
  });
});
