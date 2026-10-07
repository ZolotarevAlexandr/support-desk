import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeAll, expect, it, vi } from "vitest";
import { AuthContext } from "./auth";
import { TicketWorkflow } from "./workflow";
import type { Ticket, User } from "./api";
const requester: User = {
  id: 1,
  name: "Alice",
  email: "alice@example.com",
  role: "user",
};
const operator: User = {
  id: 2,
  name: "Alex",
  email: "alex@example.com",
  role: "operator",
};
const ticket: Ticket = {
  id: 10,
  requester_id: 1,
  requester_name: "Alice",
  assignee_id: 2,
  assignee_name: "Alex",
  title: "Printer offline",
  description: "Needs help",
  category_id: 1,
  status: "in_progress",
  priority: "medium",
  created_at: "2026-10-02T00:00:00Z",
  updated_at: "2026-10-02T00:00:00Z",
};
beforeAll(() => {
  HTMLDialogElement.prototype.showModal = function () {
    this.setAttribute("open", "");
  };
  HTMLDialogElement.prototype.close = function () {
    this.removeAttribute("open");
  };
});
afterEach(() => vi.restoreAllMocks());
function panel(user = operator, value = ticket) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <AuthContext.Provider value={{ user, setUser: () => {} }}>
        <TicketWorkflow
          ticket={value}
          canEdit={user.role === "operator"}
          onEdit={() => {}}
          onDelete={async () => {}}
        />
      </AuthContext.Provider>
    </QueryClientProvider>,
  );
  return client;
}
it("requires a resolution message and submits a named action", async () => {
  const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response(JSON.stringify({ ...ticket, status: "resolved" }), {
      status: 200,
    }),
  );
  panel();
  const u = userEvent.setup();
  await u.click(screen.getByRole("button", { name: "Resolve…" }));
  await u.click(screen.getByRole("button", { name: "Mark resolved" }));
  expect(fetch).not.toHaveBeenCalled();
  await u.type(
    screen.getByLabelText("Resolution message"),
    "Installed a new driver",
  );
  await u.click(screen.getByRole("button", { name: "Mark resolved" }));
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  expect(JSON.parse(fetch.mock.calls[0][1]!.body as string)).toEqual({
    action: "resolve",
    message: "Installed a new driver",
  });
});
it("keeps failed resolution input and explains the error", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response(
      JSON.stringify({ error: { message: "Ticket is no longer in progress" } }),
      { status: 409 },
    ),
  );
  panel();
  const u = userEvent.setup();
  await u.click(screen.getByRole("button", { name: "Resolve…" }));
  await u.type(
    screen.getByLabelText("Resolution message"),
    "Investigated and fixed",
  );
  await u.click(screen.getByRole("button", { name: "Mark resolved" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "no longer in progress",
  );
  expect(screen.getByLabelText("Resolution message")).toHaveValue(
    "Investigated and fixed",
  );
});
it("offers requester confirmation only on their resolved ticket", async () => {
  const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response(JSON.stringify({ ...ticket, status: "closed" }), {
      status: 200,
    }),
  );
  panel(requester, { ...ticket, status: "resolved" });
  await userEvent.click(screen.getByRole("button", { name: "Issue fixed" }));
  await waitFor(() => expect(fetch).toHaveBeenCalled());
  expect(JSON.parse(fetch.mock.calls[0][1]!.body as string)).toEqual({
    action: "confirm",
  });
  expect(
    screen.queryByRole("button", { name: "Resolve…" }),
  ).not.toBeInTheDocument();
});
it("asks for an explanation before returning work to the operator", async () => {
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValue(new Response(JSON.stringify(ticket), { status: 200 }));
  panel(requester, { ...ticket, status: "resolved" });
  const u = userEvent.setup();
  await u.click(screen.getByRole("button", { name: "Still need help" }));
  await u.type(
    screen.getByLabelText("What still needs attention?"),
    "Printing still fails",
  );
  await u.click(screen.getByRole("button", { name: "Send back to operator" }));
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  expect(JSON.parse(fetch.mock.calls[0][1]!.body as string)).toEqual({
    action: "request_changes",
    message: "Printing still fails",
  });
});
it("searches operators, saves a reassignment, and offers queue return", async () => {
  const next = {
    ...operator,
    id: 3,
    name: "Blake",
    email: "blake@example.com",
  };
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockImplementation(
      async (input) =>
        new Response(
          JSON.stringify(
            String(input).endsWith("/operators")
              ? [operator, next]
              : { ...ticket, assignee_id: 3, assignee_name: "Blake" },
          ),
          { status: 200 },
        ),
    );
  panel();
  const u = userEvent.setup();
  await u.click(screen.getByRole("button", { name: "Reassign…" }));
  await u.type(screen.getByLabelText("Find an operator"), "blake");
  await u.click(await screen.findByRole("button", { name: /Blake/ }));
  expect(screen.getByRole("button", { name: "Return to queue" })).toBeVisible();
  await u.click(screen.getByRole("button", { name: "Save assignment" }));
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  const call = fetch.mock.calls.find(([url]) =>
    String(url).endsWith("/actions"),
  );
  expect(JSON.parse(call![1]!.body as string)).toEqual({
    action: "reassign",
    assignee_id: 3,
  });
});
it("other operators see waiting feedback without requester confirmation buttons", () => {
  panel(operator, { ...ticket, status: "resolved" });
  expect(
    screen.getByText("Waiting for the requester to confirm the resolution."),
  ).toBeVisible();
  expect(
    screen.queryByRole("button", { name: "Issue fixed" }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "Still need help" }),
  ).not.toBeInTheDocument();
});

it("starts an existing assigned open ticket while preserving its operator", async () => {
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValue(new Response(JSON.stringify(ticket), { status: 200 }));
  panel(operator, { ...ticket, status: "open" });
  expect(
    screen.queryByRole("button", { name: "Resolve…" }),
  ).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Start work" }));
  await waitFor(() => expect(fetch).toHaveBeenCalled());
  expect(JSON.parse(fetch.mock.calls[0][1]!.body as string)).toEqual({
    action: "reassign",
    assignee_id: 2,
  });
});
