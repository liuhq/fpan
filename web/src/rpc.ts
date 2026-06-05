import type { AppType } from "@fpan/api"
import { hc } from "hono/client"

const client = hc<AppType>("/api")

export default client
