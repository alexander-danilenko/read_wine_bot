import createClient, { type Middleware } from "openapi-fetch";
import type { components, paths } from "./schema";

export type ApiError = components["schemas"]["APIError"];
export type ErrorCode = ApiError["code"];

// Telegram's signed launch data; empty when the page was not opened from Telegram.
export function initData(): string {
  return window.Telegram?.WebApp?.initData ?? "";
}

// Every API request carries the launch data; the server trusts nothing else (ADR 0009).
const auth: Middleware = {
  onRequest({ request }) {
    request.headers.set("Authorization", `tma ${initData()}`);
    return request;
  },
};

export const api = createClient<paths>({ baseUrl: window.location.origin });
api.use(auth);
