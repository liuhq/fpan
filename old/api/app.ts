import { Hono } from "hono"

import dir from "#dir"

const app = new Hono().route("/dir", dir)

export default app
export type AppType = typeof app
