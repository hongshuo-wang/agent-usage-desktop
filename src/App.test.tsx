import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";

vi.mock("./components/Layout", () => ({
  default: ({ children }: { children: React.ReactNode }) => <main>{children}</main>,
}));

vi.mock("./pages/Dashboard", () => ({
  default: () => <div>dashboard route</div>,
}));

describe("App routing", () => {
  beforeEach(() => {
    window.history.replaceState({}, "", "/config");
  });

  it("redirects removed routes to the dashboard", async () => {
    render(<App />);

    await waitFor(() => expect(window.location.pathname).toBe("/"));
    expect(await screen.findByText("dashboard route")).toBeInTheDocument();
  });
});
