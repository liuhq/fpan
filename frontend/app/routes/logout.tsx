import { redirectDocument } from "react-router"

import { api, ApiError } from "~/openapi/client"

export async function clientAction() {
  const { error, response } = await api.POST("/api/v1/auth/logout")

  if (!response.ok) {
    throw new ApiError(response.status, error)
  }

  return redirectDocument("/login")
}
