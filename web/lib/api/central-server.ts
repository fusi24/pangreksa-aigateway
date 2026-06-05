import "server-only";

/**
 * Thrown when the Central Server returns a non-2xx response.
 */
export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: string
  ) {
    super(`Central Server error ${status}: ${body}`);
    this.name = "ApiError";
  }
}

/**
 * Server-side typed fetch helper for the Central Server.
 * Attaches ADMIN_API_KEY header — never exposed to the browser.
 *
 * @param path - API path starting with /
 * @param init - Optional RequestInit overrides
 * @param jwt - Optional user JWT to forward as X-User-Token header
 * @returns Parsed JSON response typed as T
 * @throws {ApiError} On non-2xx status
 */
export async function centralFetch<T>(
  path: string,
  init?: RequestInit,
  jwt?: string
): Promise<T> {
  const base = process.env.CENTRAL_SERVER_URL;
  const key = process.env.ADMIN_API_KEY;

  if (!base || !key) {
    throw new Error(
      "CENTRAL_SERVER_URL and ADMIN_API_KEY must be set in environment"
    );
  }

  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    Authorization: `Bearer ${key}`,
  };

  if (jwt) {
    headers["X-User-Token"] = jwt;
  }

  const res = await fetch(`${base}${path}`, {
    ...init,
    headers: {
      ...headers,
      ...(init?.headers as Record<string, string> | undefined),
    },
    cache: "no-store",
  });

  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new ApiError(res.status, text);
  }

  return res.json() as Promise<T>;
}
