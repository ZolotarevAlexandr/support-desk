import { test, expect, type Page } from "@playwright/test";
const run = Date.now();
const password = "strong-password-123";
async function register(page: Page, name: string, operator = false) {
  await page.goto("/register");
  await page.getByLabel("Name", { exact: true }).fill(name);
  await page
    .getByLabel("Email", { exact: true })
    .fill(`${name}-${run}@example.com`);
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByLabel("Confirm password").fill(password);
  if (operator) {
    await page
      .getByRole("combobox", { name: "Account type" })
      .selectOption("operator");
    await page
      .getByLabel("Invitation code")
      .fill(process.env.E2E_INVITE_CODE || "private-invite");
  }
  await page
    .getByRole("button", { name: "Create account", exact: true })
    .click();
  await expect(
    page.getByRole("heading", {
      name: operator ? "Operator queue" : "My tickets",
      exact: true,
    }),
  ).toBeVisible();
}
test("operator workflow, requester feedback/confirmation, and user isolation", async ({
  browser,
}) => {
  const userContext = await browser.newContext();
  const opContext = await browser.newContext();
  const otherContext = await browser.newContext();
  const user = await userContext.newPage();
  const op = await opContext.newPage();
  const other = await otherContext.newPage();
  await register(user, "requester");
  await user.getByRole("link", { name: "Create ticket", exact: true }).click();
  await user
    .getByLabel("Title", { exact: true })
    .fill("Printer needs a driver");
  await user
    .getByLabel("Description", { exact: true })
    .fill("The office printer is offline.");
  await user
    .getByRole("combobox", { name: "Category", exact: true })
    .selectOption({ label: "Hardware" });
  await user.getByRole("button", { name: "Submit ticket" }).click();
  await expect(
    user.getByRole("heading", { name: "Printer needs a driver" }),
  ).toBeVisible();
  const ticketPath = new URL(user.url()).pathname;
  await register(op, "operator", true);
  const queueRow = op
    .locator("tr")
    .filter({ has: op.getByRole("link", { name: /Printer needs a driver/ }) });
  await queueRow.getByRole("button", { name: /Take ticket/ }).click();
  await expect(
    queueRow.getByRole("button", { name: /Take ticket/ }),
  ).toHaveCount(0);
  await op.getByRole("link", { name: /Printer needs a driver/ }).click();
  const management = op.getByRole("region", { name: "Request management" });
  await expect(
    management.getByText("In progress", { exact: true }),
  ).toBeVisible();
  await op.getByRole("button", { name: "Reassign…" }).click();
  await op.getByLabel("Find an operator").fill("operator");
  await op.getByRole("button", { name: "Return to queue" }).click();
  await op.getByRole("button", { name: "Save assignment" }).click();
  await expect(
    management.getByText("Waiting for an operator", { exact: true }),
  ).toBeVisible();
  await op.getByRole("button", { name: "Take ticket", exact: true }).click();
  await expect(
    management.getByText("In progress", { exact: true }),
  ).toBeVisible();
  await op
    .getByLabel("Add a message")
    .fill("Investigating the printer driver.");
  await op.getByRole("button", { name: "Send message" }).click();
  await expect(
    op.getByText("Investigating the printer driver.", { exact: true }),
  ).toBeVisible();
  async function resolve(message: string) {
    await op.getByRole("button", { name: "Resolve…" }).click();
    await op.getByLabel("Resolution message").fill(message);
    await op.getByRole("button", { name: "Mark resolved" }).click();
    await expect(
      management.getByText("Resolved", { exact: true }),
    ).toBeVisible();
  }
  await resolve("The driver is installed. Please try again.");
  await user.reload();
  await expect(
    user.getByText("The driver is installed. Please try again.", {
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    user.getByRole("button", { name: "Issue fixed", exact: true }),
  ).toBeVisible();
  await user
    .getByRole("button", { name: "Still need help", exact: true })
    .click();
  await user
    .getByLabel("What still needs attention?")
    .fill("Printing still fails after restart.");
  await user.getByRole("button", { name: "Send back to operator" }).click();
  await expect(
    user
      .getByRole("region", { name: "Request management" })
      .getByText("In progress", { exact: true }),
  ).toBeVisible();
  await op.reload();
  await expect(
    op.getByText("Printing still fails after restart.", { exact: true }),
  ).toBeVisible();
  await resolve("Updated the print server configuration.");
  await user.reload();
  await user.getByRole("button", { name: "Issue fixed", exact: true }).click();
  await expect(user.getByText(/This ticket is closed/)).toBeVisible();
  await expect(user.getByLabel("Add a message")).toHaveCount(0);
  await register(other, "other");
  await other.goto(ticketPath);
  await expect(other.getByRole("alert")).toContainText("Resource not found");
  await other.goto("/tickets");
  await expect(
    other.getByRole("heading", { name: "No tickets found" }),
  ).toBeVisible();
  await user.goto("/tickets");
  await user.getByLabel("Search", { exact: true }).fill("Printer");
  await user
    .getByRole("combobox", { name: "Status", exact: true })
    .selectOption("closed");
  await user.reload();
  await expect(user.getByLabel("Search", { exact: true })).toHaveValue(
    "Printer",
  );
  await expect(
    user.getByRole("link", { name: /Printer needs a driver/ }),
  ).toBeVisible();
  await user.getByRole("button", { name: "Sign out" }).click();
  await expect(
    user.getByRole("heading", { name: "Welcome back" }),
  ).toBeVisible();
  await user.goto(ticketPath);
  await expect(
    user.getByRole("heading", { name: "Welcome back" }),
  ).toBeVisible();
  await op.reload();
  await op.getByText("More actions", { exact: true }).click();
  await op.getByRole("button", { name: "Reopen…" }).click();
  await op
    .getByLabel("Reason for reopening")
    .fill("Checking another affected workstation.");
  await op.getByRole("button", { name: "Reopen ticket", exact: true }).click();
  await expect(op.getByLabel("Add a message")).toBeVisible();
  await expect(
    op.getByText("Checking another affected workstation.", { exact: true }),
  ).toBeVisible();
  for (let i = 1; i <= 76; i++)
    expect(
      (
        await op.request.post(`/api/v1${ticketPath}/comments`, {
          data: { body: `History message ${i}` },
        })
      ).status(),
    ).toBe(201);
  await op.reload();
  await expect(
    op.getByText("History message 76", { exact: true }),
  ).toBeVisible();
  await op.getByRole("button", { name: "Previous", exact: true }).click();
  await expect(op.getByText(/Page 3 of 4/)).toBeVisible();
  await expect(op.getByText("History message 76", { exact: true })).toHaveCount(
    0,
  );
  await op.getByRole("button", { name: "Previous", exact: true }).click();
  await expect(op.getByText(/Page 2 of 4/)).toBeVisible();
  await op.getByRole("button", { name: "Previous", exact: true }).click();
  await expect(
    op.getByText("Investigating the printer driver.", { exact: true }),
  ).toBeVisible();
  await expect(op.getByText("History message 76", { exact: true })).toHaveCount(
    0,
  );
  const deleteButton = op.getByRole("button", {
    name: "Delete ticket",
    exact: true,
  });
  if (!(await deleteButton.isVisible()))
    await op.getByText("More actions", { exact: true }).click();
  await deleteButton.click();
  await op.getByRole("button", { name: "Confirm deletion" }).click();
  await expect(
    op.getByRole("heading", { name: "Operator queue", exact: true }),
  ).toBeVisible();
  await userContext.close();
  await opContext.close();
  await otherContext.close();
});
test("operator category CRUD and ticket pagination", async ({ page }) => {
  await register(page, "manager", true);
  await page.getByRole("link", { name: "Categories", exact: true }).click();
  await page.getByLabel("Name", { exact: true }).fill(`Category ${run}`);
  await page.getByLabel("Description", { exact: true }).fill("Browser test");
  await page
    .getByRole("button", { name: "Create category", exact: true })
    .click();
  const row = page.locator("article").filter({
    has: page.getByRole("heading", { name: `Category ${run}`, exact: true }),
  });
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "Edit category", exact: true }).click();
  await page.getByLabel("Name", { exact: true }).fill(`Updated ${run}`);
  await page
    .getByRole("button", { name: "Save category", exact: true })
    .click();
  const edited = page.locator("article").filter({
    has: page.getByRole("heading", { name: `Updated ${run}`, exact: true }),
  });
  await expect(edited).toBeVisible();
  await edited
    .getByRole("button", { name: "Delete category", exact: true })
    .click();
  await page.getByRole("button", { name: "Confirm deletion" }).click();
  await expect(edited).toHaveCount(0);
  const categories = await (
    await page.request.get("/api/v1/categories")
  ).json();
  for (let i = 0; i < 26; i++) {
    const r = await page.request.post("/api/v1/tickets", {
      data: {
        title: `Pagination ${run} ${i}`,
        description: "Pagination test",
        category_id: categories[0].id,
      },
    });
    expect(r.status()).toBe(201);
  }
  await page.goto(`/tickets?search=Pagination+${run}`);
  await expect(page.getByText(/26 results/)).toBeVisible();
  await page.getByRole("button", { name: "Next", exact: true }).click();
  await expect(page).toHaveURL(/page=2/);
  await expect(page.locator("tbody tr")).toHaveCount(1);
  await page.reload();
  await expect(page.locator("tbody tr")).toHaveCount(1);
  const all = await (
    await page.request.get(
      `/api/v1/tickets?search=Pagination+${run}&page_size=100`,
    )
  ).json();
  for (const t of all.items)
    expect(
      (await page.request.delete(`/api/v1/tickets/${t.id}`)).status(),
    ).toBe(204);
});
