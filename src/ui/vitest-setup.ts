// Registers @testing-library/jest-dom's matchers (toBeInTheDocument,
// toBeDisabled, toHaveTextContent, …) on Vitest's `expect`, and augments the
// matcher types project-wide (this file is in tsconfig `include`).
import "@testing-library/jest-dom/vitest";
