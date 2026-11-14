/** Standard API response envelope (mirrors the server's `api-response.utils`). */
export interface ApiResponse<T = unknown> {
  success: boolean;
  data?: T;
  message?: string;
  error?: {
    code: string;
    message: string;
    details?: unknown;
  };
  metadata?: {
    page?: number;
    limit?: number;
    total?: number;
    totalPages?: number;
  };
}

/** Thrown for non-2xx responses and for envelopes without data. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details?: unknown;
  readonly url: string;
  /**
   * True when `message` came from the server's error envelope; false when it
   * is the client's fallback (the HTTP status line), which says little.
   */
  readonly fromServer: boolean;

  constructor(init: {
    status: number;
    code: string;
    message: string;
    details?: unknown;
    url: string;
    fromServer?: boolean;
  }) {
    super(init.message);
    this.name = "ApiError";
    this.status = init.status;
    this.code = init.code;
    this.details = init.details;
    this.url = init.url;
    this.fromServer = init.fromServer ?? false;
  }
}

/**
 * The request never reached the server: `fetch` itself rejected (offline, DNS,
 * CORS, a dropped connection). Extends `TypeError`, which is what `fetch`
 * throws, so `instanceof TypeError` checks keep working.
 */
export class NetworkError extends TypeError {
  readonly url: string;

  constructor(url: string, options?: { cause?: unknown }) {
    super(`Could not reach ${url}`, options);
    this.name = "NetworkError";
    this.url = url;
  }
}

/**
 * A 401 from the session guard (`SESSION_INVALID`), with when its request was
 * sent (a `performance.now()` timestamp).
 */
export interface UnauthorizedEvent {
  path: string;
  sentAt: number;
}

/** `ErrorCode.SESSION_INVALID` in `@groot/core/errors`, the session guard's 401 code. */
const SESSION_INVALID = "SESSION_INVALID";

type Params = Record<string, unknown>;

const BASE_URL = "/api/v1";

function buildUrl(path: string, params?: Params): string {
  const url = /^https?:/.test(path) ? path : `${BASE_URL}${path.startsWith("/") ? "" : "/"}${path}`;
  if (!params) return url;
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== null) search.set(key, String(value));
  }
  const query = search.toString();
  return query ? `${url}?${query}` : url;
}

/**
 * API client: the single surface for backend calls, on plain `fetch`. Unwraps
 * the `{ success, data }` envelope and throws an `ApiError` on failure so
 * callers get typed values directly. The session cookie travels with every
 * request.
 */
/** Per-request extras every verb accepts. */
export interface ExtraOptions {
  /** Headers added to the request (a step-up token, an idempotency key). */
  headers?: Record<string, string>;
}

export interface RequestOptions extends ExtraOptions {
  params?: Params;
  body?: unknown;
  /** The endpoint may answer with no data; return `undefined` instead of throwing. */
  emptyOk?: boolean;
}

async function readEnvelope<T>(response: Response): Promise<ApiResponse<T> | undefined> {
  const text = await response.text();
  if (!text) return undefined;
  try {
    return JSON.parse(text) as ApiResponse<T>;
  } catch {
    return undefined;
  }
}

class ApiClient {
  private readonly unauthorizedHandlers = new Set<(event: UnauthorizedEvent) => void>();

  /**
   * Run `handler` when the session guard rejects a request (a 401 coded
   * `SESSION_INVALID`). Any other 401, such as a wrong password, only throws.
   * Returns an unsubscribe function.
   */
  onUnauthorized(handler: (event: UnauthorizedEvent) => void): () => void {
    this.unauthorizedHandlers.add(handler);
    return () => {
      this.unauthorizedHandlers.delete(handler);
    };
  }

  private async send(
    method: string,
    path: string,
    options: RequestOptions = {},
  ): Promise<Response> {
    const headers = new Headers({ accept: "application/json", ...options.headers });
    let body: BodyInit | undefined;
    if (options.body instanceof FormData) {
      body = options.body; // the browser sets multipart/form-data with the boundary
    } else if (options.body !== undefined) {
      headers.set("content-type", "application/json");
      body = JSON.stringify(options.body);
    }

    const sentAt = performance.now();
    let response: Response;
    try {
      response = await fetch(buildUrl(path, options.params), {
        method,
        headers,
        body,
        credentials: "include",
      });
    } catch (error) {
      // `fetch` rejects with a TypeError only when the request could not be made.
      if (error instanceof TypeError) throw new NetworkError(path, { cause: error });
      throw error;
    }
    if (response.ok) return response;
    const parsed = await readEnvelope(response);
    const error = new ApiError({
      status: response.status,
      code: parsed?.error?.code ?? `HTTP_${response.status}`,
      message:
        parsed?.error?.message ??
        (response.statusText || `Request failed with status ${response.status}`),
      details: parsed?.error?.details,
      url: path,
      fromServer: typeof parsed?.error?.message === "string",
    });
    if (error.code === SESSION_INVALID) {
      for (const handler of this.unauthorizedHandlers) handler({ path, sentAt });
    }
    throw error;
  }

  private async envelope<T>(response: Response): Promise<ApiResponse<T>> {
    return (await readEnvelope<T>(response)) ?? { success: true };
  }

  private unwrap<T>(envelope: ApiResponse<T>, path: string): T {
    if (envelope.data === undefined || envelope.data === null) {
      throw new ApiError({
        status: 200,
        code: envelope.error?.code ?? "NO_DATA",
        message: envelope.error?.message ?? "No data returned",
        url: path,
      });
    }
    return envelope.data;
  }

  private async call<T>(method: string, path: string, options?: RequestOptions): Promise<T> {
    return this.unwrap<T>(await this.envelope<T>(await this.send(method, path, options)), path);
  }

  /**
   * Any verb with the full set of options: query params, a JSON (or
   * FormData) body, extra headers such as a step-up token, and `emptyOk`
   * for an endpoint that may answer with no data (as DELETE does), in which
   * case the result is whatever data came back or `undefined`.
   */
  async request<T>(method: string, path: string, options: RequestOptions = {}): Promise<T> {
    const verb = method.toUpperCase();
    if (!options.emptyOk) return this.call<T>(verb, path, options);
    const envelope = await this.envelope<T>(await this.send(verb, path, options));
    return envelope.data as T;
  }

  async get<T>(url: string, params?: Params, options?: ExtraOptions): Promise<T> {
    return this.call<T>("GET", url, { ...options, params });
  }

  async post<T>(url: string, data?: unknown, options?: ExtraOptions): Promise<T> {
    return this.call<T>("POST", url, { ...options, body: data });
  }

  async put<T>(url: string, data?: unknown, options?: ExtraOptions): Promise<T> {
    return this.call<T>("PUT", url, { ...options, body: data });
  }

  async patch<T>(url: string, data?: unknown, options?: ExtraOptions): Promise<T> {
    return this.call<T>("PATCH", url, { ...options, body: data });
  }

  /**
   * DELETE. Unlike the other verbs, DELETE may legitimately return no body
   * (204 No Content), so this does not throw on missing data — it returns
   * whatever data is present (typed by the caller) or `undefined`.
   */
  async delete<T = unknown>(url: string, data?: unknown, options?: ExtraOptions): Promise<T> {
    return this.request<T>("DELETE", url, { ...options, body: data, emptyOk: true });
  }

  /** Upload `FormData` — the browser sets the multipart Content-Type + boundary. */
  async postForm<T>(url: string, formData: FormData): Promise<T> {
    return this.call<T>("POST", url, { body: formData });
  }

  /** Download a binary file as a `Blob` (no envelope unwrapping). */
  async getBlob(url: string, params?: Params): Promise<Blob> {
    const response = await this.send("GET", url, { params });
    return response.blob();
  }
}

export const apiClient = new ApiClient();
