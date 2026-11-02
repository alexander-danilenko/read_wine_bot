import createClient, { type Middleware } from "openapi-fetch";
import { webApp } from "../telegram";
import type { components, paths } from "./schema";

export type Giveaway = components["schemas"]["GiveawayBody"];
export type Channel = components["schemas"]["ListChannelBody"];
export type Me = components["schemas"]["MeBody"];
export type Review = components["schemas"]["ReviewOutputBody"];
export type ApiError = components["schemas"]["APIError"];
export type ErrorCode = ApiError["code"];

// Every API request carries Telegram's signed launch data; the server trusts nothing else (ADR 0009).
const auth: Middleware = {
  onRequest({ request }) {
    request.headers.set("Authorization", `tma ${webApp?.initData ?? ""}`);
    return request;
  },
};

export const api = createClient<paths>({ baseUrl: window.location.origin });
api.use(auth);

// Failure is what a failed call left: the API's error, or none when the network failed.
export class Failure extends Error {
  constructor(readonly problem: ApiError | undefined) {
    super(problem?.code ?? "network");
  }

  get code(): ErrorCode {
    return this.problem?.code ?? "internal";
  }

  get field(): string | undefined {
    return this.problem?.field;
  }
}

// must returns the data of an API call, or throws a Failure.
export async function must<D>(call: Promise<{ data?: D; error?: unknown }>): Promise<D> {
  let result: { data?: D; error?: unknown };
  try {
    result = await call;
  } catch {
    throw new Failure(undefined);
  }

  if (result.error !== undefined) throw new Failure(result.error as ApiError);
  return result.data as D;
}
