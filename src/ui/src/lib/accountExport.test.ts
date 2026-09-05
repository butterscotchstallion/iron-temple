import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { downloadAccountExport, filenameFrom } from "./accountExport";

describe("filenameFrom", () => {
  it("reads the quoted name the API sends", () => {
    expect(
      filenameFrom('attachment; filename="iron-temple-ada-2026-09-05.json"'),
    ).toBe("iron-temple-ada-2026-09-05.json");
  });

  it("reads an unquoted name", () => {
    expect(filenameFrom("attachment; filename=export.json")).toBe("export.json");
  });

  it("falls back when the header is absent", () => {
    expect(filenameFrom(null)).toBe("iron-temple-export.json");
  });

  it("falls back when the header names no file", () => {
    expect(filenameFrom("attachment")).toBe("iron-temple-export.json");
  });

  // A filename is a name. A header that answers with a path is not answering
  // the question, so it is refused outright rather than trimmed into shape.
  it.each([
    'attachment; filename="../../etc/passwd"',
    'attachment; filename="/etc/passwd"',
    'attachment; filename="..\\\\windows\\\\system32"',
  ])("refuses a path: %s", (header) => {
    expect(filenameFrom(header)).toBe("iron-temple-export.json");
  });
});

describe("downloadAccountExport", () => {
  let click: ReturnType<typeof vi.fn>;
  let createObjectURL: ReturnType<typeof vi.fn>;
  let revokeObjectURL: ReturnType<typeof vi.fn>;
  let anchor: HTMLAnchorElement;

  beforeEach(() => {
    vi.useFakeTimers();

    click = vi.fn();
    anchor = document.createElement("a");
    anchor.click = click;
    // Only the anchor is intercepted; anything else the code under test creates
    // should still come from the real document.
    vi.spyOn(document, "createElement").mockImplementation(((tag: string) =>
      tag === "a"
        ? anchor
        : (Object.getPrototypeOf(document).createElement as typeof document.createElement).call(
            document,
            tag,
          )) as typeof document.createElement);

    createObjectURL = vi.fn(() => "blob:export");
    revokeObjectURL = vi.fn();
    vi.stubGlobal("URL", {
      ...URL,
      createObjectURL,
      revokeObjectURL,
    });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  function respondWith(body: string, init: ResponseInit = {}) {
    const fetchMock = vi.fn(async () => new Response(body, { status: 200, ...init }));
    vi.stubGlobal("fetch", fetchMock);
    return fetchMock;
  }

  it("saves the response under the name the server chose", async () => {
    respondWith('{"formatVersion":1}', {
      headers: { "Content-Disposition": 'attachment; filename="iron-temple-ada.json"' },
    });

    await downloadAccountExport();

    expect(createObjectURL).toHaveBeenCalledOnce();
    expect(anchor.download).toBe("iron-temple-ada.json");
    expect(anchor.href).toContain("blob:export");
    expect(click).toHaveBeenCalledOnce();
  });

  // The session is a cookie, so the request has to carry it.
  it("sends credentials", async () => {
    const fetchMock = respondWith("{}");

    await downloadAccountExport();

    const [, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(init.credentials).toBe("same-origin");
  });

  it("asks the API path the client is configured with", async () => {
    const fetchMock = respondWith("{}");

    await downloadAccountExport();

    const [url] = fetchMock.mock.calls[0] as unknown as [string];
    expect(url).toBe("/api/v1/me/export");
  });

  // Revoking synchronously saves an empty file in Safari, so the URL has to
  // outlive the click.
  it("keeps the object URL alive past the click", async () => {
    respondWith("{}");

    await downloadAccountExport();
    expect(revokeObjectURL).not.toHaveBeenCalled();

    vi.advanceTimersByTime(10_000);
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:export");
  });

  // Believing you have a backup you do not have is the failure worth avoiding,
  // so a refused export must reject rather than quietly save nothing.
  it("rejects when the server refuses", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("nope", { status: 500 })),
    );

    await expect(downloadAccountExport()).rejects.toThrow("500");
    expect(click).not.toHaveBeenCalled();
  });

  it("rejects when the request never lands", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new TypeError("Failed to fetch");
      }),
    );

    await expect(downloadAccountExport()).rejects.toThrow();
    expect(click).not.toHaveBeenCalled();
  });
});
