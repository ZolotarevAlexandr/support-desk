import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { AuthContext } from "./auth";
import { Comments } from "./comments";
import type { Ticket } from "./api";
afterEach(() => vi.restoreAllMocks());
it("shows new activity but allows navigating to earlier conversation pages", async () => {
  const ticket: Ticket = {
    id: 10,
    requester_id: 1,
    requester_name: "Alice",
    assignee_id: 2,
    assignee_name: "Alex",
    category_id: 1,
    title: "Printer",
    description: "Broken",
    priority: "medium",
    status: "in_progress",
    created_at: "2026-10-02T00:00:00Z",
    updated_at: "2026-10-02T00:00:00Z",
  };
  const messages = Array.from({ length: 81 }, (_, i) => ({
    id: i + 1,
    author_id: 2,
    author_name: "Alex",
    author_role: "operator",
    body: `Message ${i + 1}`,
    created_at: ticket.created_at,
    updated_at: ticket.created_at,
  }));
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const page = Number(
      new URL(String(input), "http://test").searchParams.get("page"),
    );
    return new Response(
      JSON.stringify({
        items: messages.slice((page - 1) * 25, page * 25),
        page,
        page_size: 25,
        total: 81,
      }),
      { status: 200 },
    );
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <AuthContext.Provider
        value={{
          user: {
            id: 1,
            name: "Alice",
            email: "alice@example.com",
            role: "user",
          },
          setUser: () => {},
        }}
      >
        <Comments ticket={ticket} />
      </AuthContext.Provider>
    </QueryClientProvider>,
  );
  expect(await screen.findByText("Message 81", { exact: true })).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "Previous" }));
  expect(await screen.findByText("Message 51", { exact: true })).toBeVisible();
  expect(
    screen.queryByText("Message 81", { exact: true }),
  ).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Previous" }));
  expect(await screen.findByText("Message 26", { exact: true })).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "Previous" }));
  expect(await screen.findByText("Message 1", { exact: true })).toBeVisible();
  expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();
});
