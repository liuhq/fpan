import { createAsync, query } from "@solidjs/router"
import { parseResponse } from "hono/client"
import type { DetailedError } from "hono/client"
import { createSignal } from "solid-js"

import client from "#/rpc.ts"

const getDirQuery = query(async () => {
  const res = await parseResponse(client.dir.$get({ query: { id: "123" } })).catch(
    (e: DetailedError) => {
      console.error(e)
    },
  )
  return res
}, "dir")

const Home = () => {
  const [count, setCount] = createSignal(0)
  const data = createAsync(() => getDirQuery())

  return (
    <>
      <section>
        <div>{data()?.path}</div>
        <button type="button" class="ds-btn ds-btn-primary" onClick={() => setCount((c) => c + 1)}>
          Count is {count()}
        </button>
      </section>
    </>
  )
}

export default Home
