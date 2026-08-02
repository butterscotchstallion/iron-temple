import { test, expect } from "@playwright/test";

test("renders the app shell", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: /iron temple/i })).toBeVisible();
  await expect(page.getByText("StrongLifts 5x5", { exact: true })).toBeVisible();
});

test("rest timer counts down from 3:00", async ({ page }) => {
  await page.goto("/");
  const remaining = page.getByTestId("rest-remaining");
  await expect(remaining).toHaveText("3:00");
  await page.getByRole("button", { name: "Start" }).click();
  // After ~1.5s it should have ticked below the start value.
  await expect(remaining).not.toHaveText("3:00", { timeout: 3000 });
});
