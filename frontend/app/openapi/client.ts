import createClient from "openapi-fetch"

import type { paths } from "./schema"

export const api = createClient<paths>({
  baseUrl: "",
  credentials: "same-origin",
})

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly body: unknown,
  ) {
    super(
      typeof body === "object" &&
        body !== null &&
        "message" in body &&
        typeof body.message === "string"
        ? body.message
        : `HTTP ${status}`,
    )
  }
}

export const apiKeys = {
  entries: <T>(parentId: number | null, query: T) => ["entries", parentId, query] as const,
} as const
