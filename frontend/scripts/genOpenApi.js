import { mkdir, writeFile } from "fs/promises"

import openapiTS, { astToString } from "openapi-typescript"

const source = new URL("../../api/openapi.yaml", import.meta.url)
const dest = new URL("../app/openapi/schema.d.ts", import.meta.url)

console.log(`Generating openapi-ts from ${source} to ${dest}`)

const ast = await openapiTS(source)
const contents = astToString(ast)

await mkdir(new URL(".", dest), { recursive: true })
await writeFile(dest, contents)
