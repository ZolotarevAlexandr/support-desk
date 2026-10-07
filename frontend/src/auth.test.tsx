import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { describe, it, expect, vi, afterEach } from "vitest";
import { AuthContext, AuthPage } from "./auth";
const user = {
  id: 1,
  name: "Alice",
  email: "alice@example.com",
  role: "user" as const,
};
afterEach(() => vi.restoreAllMocks());
function renderForm() {
  const setUser = vi.fn();
  render(
    <MemoryRouter>
      <AuthContext.Provider value={{ user: null, setUser }}>
        <AuthPage register />
      </AuthContext.Provider>
    </MemoryRouter>,
  );
  return setUser;
}
async function fill() {
  const u = userEvent.setup();
  await u.type(screen.getByLabelText("Name"), "Alice");
  await u.type(screen.getByLabelText("Email"), "alice@example.com");
  await u.type(
    screen.getByLabelText("Password", { exact: true }),
    "strong-password-123",
  );
  await u.type(
    screen.getByLabelText("Confirm password"),
    "strong-password-123",
  );
  return u;
}
describe("registration", () => {
  it("reveals the invitation field only for operators", async () => {
    renderForm();
    expect(screen.queryByLabelText(/Invitation code/)).not.toBeInTheDocument();
    await userEvent.selectOptions(
      screen.getByLabelText("Account type"),
      "operator",
    );
    expect(screen.getByLabelText(/Invitation code/)).toBeRequired();
    await userEvent.selectOptions(
      screen.getByLabelText("Account type"),
      "user",
    );
    expect(screen.queryByLabelText(/Invitation code/)).not.toBeInTheDocument();
  });
  it("validates confirmation before sending", async () => {
    const fetch = vi.spyOn(globalThis, "fetch");
    renderForm();
    const u = await fill();
    await u.clear(screen.getByLabelText("Confirm password"));
    await u.type(screen.getByLabelText("Confirm password"), "different");
    await u.click(screen.getByRole("button", { name: "Create account" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Passwords do not match",
    );
    expect(fetch).not.toHaveBeenCalled();
  });
  it("signs the account in on successful registration", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify(user), { status: 201 }),
    );
    const setUser = renderForm();
    const u = await fill();
    await u.click(screen.getByRole("button", { name: "Create account" }));
    await waitFor(() => expect(setUser).toHaveBeenCalledWith(user));
  });
  it("shows server failures without losing the form", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(
        JSON.stringify({ error: { message: "Email already exists" } }),
        { status: 409 },
      ),
    );
    renderForm();
    const u = await fill();
    await u.click(screen.getByRole("button", { name: "Create account" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Email already exists",
    );
    expect(screen.getByLabelText("Email")).toHaveValue("alice@example.com");
  });
});
