import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { it, expect, vi, afterEach } from "vitest";
import App from "./App";
import { APIError } from "./api";
afterEach(() => {
  vi.restoreAllMocks();
  window.history.replaceState({}, "", "/");
});
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <App />
    </QueryClientProvider>,
  );
  return client;
}
it("shows user navigation and clears private cache on logout", async () => {
  window.history.replaceState({}, "", "/tickets");
  const me = { id: 1, name: "Alice", email: "alice@example.com", role: "user" };
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const path = String(input);
    return path.endsWith("/auth/logout")
      ? new Response(null, { status: 204 })
      : new Response(
          JSON.stringify(
            path.endsWith("/auth/me")
              ? me
              : path.endsWith("/categories")
                ? []
                : { items: [], page: 1, page_size: 25, total: 0 },
          ),
          { status: 200 },
        );
  });
  const client = mount();
  expect(
    await screen.findByRole("heading", { name: "My tickets" }),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("link", { name: "Categories" }),
  ).not.toBeInTheDocument();
  client.setQueryData(["private-marker"], { secret: true });
  await userEvent.click(screen.getByRole("button", { name: "Sign out" }));
  expect(
    await screen.findByRole("heading", { name: "Welcome back" }),
  ).toBeInTheDocument();
  expect(client.getQueryData(["private-marker"])).toBeUndefined();
});
it("expires the session and clears cached private data", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response(
      JSON.stringify({
        id: 1,
        name: "Operator",
        email: "op@example.com",
        role: "operator",
      }),
      { status: 200 },
    ),
  );
  const client = mount();
  await waitFor(() =>
    expect(
      screen.getByRole("link", { name: "Categories" }),
    ).toBeInTheDocument(),
  );
  client.setQueryData(["private-marker"], 1);
  window.dispatchEvent(new Event("session-expired"));
  expect(
    await screen.findByRole("heading", { name: "Welcome back" }),
  ).toBeInTheDocument();
  expect(client.getQueryData(["private-marker"])).toBeUndefined();
  expect(new APIError(401, "expired").status).toBe(401);
});
