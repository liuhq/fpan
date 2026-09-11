import { useState } from "react"

import { parseLoginURL } from "~/lib/auth"

/**
 * Mock login
 */
export function useMockLogin(returnTo: string) {
  const loginURL = parseLoginURL(returnTo)
  const isMockMode = import.meta.env.DEV && import.meta.env.MODE === "mock"
  const [isLoggingIn, setIsLoggingIn] = useState(false)
  const [loginError, setLoginError] = useState<string | null>(null)

  const handleLogin = async (evt: React.MouseEvent) => {
    if (!isMockMode) {
      return
    }
    evt.preventDefault()

    if (isLoggingIn) {
      return
    }

    setIsLoggingIn(true)
    setLoginError(null)

    try {
      const response = await fetch(loginURL, { cache: "no-store" })
      if (!response.ok) {
        throw new Error(`Mock login failed with status ${response.status}`)
      }
      window.location.replace(returnTo)
    } catch (error) {
      setLoginError(error instanceof Error ? error.message : "Mock login failed")
      setIsLoggingIn(false)
    }
  }

  return {
    isLoggingIn,
    loginError,
    handleLogin,
  }
}
