import { useSWRConfig } from "swr"
import useSWRMutation from "swr/mutation"

import { api, ApiError } from "../client"
import { apiKeys } from "../keys"

export const oidcLoginURL = "/api/v1/auth/login"

export function useLogout() {
  const { mutate } = useSWRConfig()

  const { trigger, ...state } = useSWRMutation(apiKeys.auth.logout(), async () => {
    const { error, response } = await api.POST("/api/v1/auth/logout")

    if (error || !response.ok) {
      throw new ApiError(response.status, error)
    }
  })

  const logout = async () => {
    await trigger()
    // clear all cache keys belonging to this user.
    await mutate(() => true, undefined, { revalidate: false })
    window.location.replace("/")
  }

  return {
    ...state,
    logout,
  }
}
