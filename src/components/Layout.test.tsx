import { render, screen } from "@testing-library/react";
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

    expect(screen.getByRole("link", { name: "title" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "sessionLog" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "settings" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "config" })).not.toBeInTheDocument();
    expect(screen.getAllByRole("link").map((link) => link.getAttribute("href"))).toEqual([
      "/",
      "/sessions",
      "/settings",
    ]);
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
