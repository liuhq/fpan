import { zValidator } from "@hono/zod-validator"
import { Hono } from "hono"
import z from "zod"

const hono = new Hono()
  .get(
    "/",
    zValidator(
      "query",
      z.object({
        id: z.nanoid().optional(),
      }),
    ),
    (ctx) => {
      const query = ctx.req.valid("query")
      return ctx.json(
        {
          ok: true,
          path: "/dir:GET",
          id: query.id,
        },
        201,
      )
    },
  )
  .post("/", (ctx) => {
    return ctx.text("/dir:POST")
  })

export default hono
