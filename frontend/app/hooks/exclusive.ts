import { useCallback, useRef } from "react"

export class ConcurrentMutationError extends Error {
  constructor() {
    super("Another mutation is already in progress")
    this.name = "ConcurrentMutationError"
  }
}

export function useExclusiveAction() {
  const inFlightRef = useRef(false)

  return useCallback(async <T>(action: () => Promise<T>): Promise<T> => {
    if (inFlightRef.current) {
      throw new ConcurrentMutationError()
    }

    inFlightRef.current = true

    try {
      return await action()
    } finally {
      inFlightRef.current = false
    }
  }, [])
}
