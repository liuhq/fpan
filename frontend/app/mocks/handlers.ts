import { HttpResponse, delay, http as untypedHttp } from "msw"
import { createOpenApiHttp } from "openapi-msw"

import type { paths } from "~/openapi/schema"

import { type Entry, type StoredFile, type StoredShare, store } from "./store"

const http = createOpenApiHttp<paths>()
const sessionKey = "fpan:mock:authenticated"
const oidcStateKey = "fpan:mock:oidc-state"
const returnToKey = "fpan:mock:return-to"

function apiError(status: number, message: string) {
  return HttpResponse.json({ code: status * 10, message }, { status })
}

function loggedIn() {
  return sessionStorage.getItem(sessionKey) !== "false"
}

function requireAuth() {
  return loggedIn() ? undefined : apiError(401, "authentication required")
}

function safeReturnTo(value: string | null) {
  if (!value) return "/"
  try {
    const target = new URL(value, window.location.origin)
    const rejectedPaths = new Set(["/login", "/logout"])
    return target.origin === window.location.origin && !rejectedPaths.has(target.pathname)
      ? `${target.pathname}${target.search}${target.hash}`
      : "/"
  } catch {
    return "/"
  }
}

function numericParam(value: string | readonly string[] | undefined) {
  const parsed = Number(value)
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : undefined
}

type ListOptions = {
  page: number
  size: number
  sort: "asc" | "desc"
  sortBy: "name" | "created_at" | "updated_at"
  filter: string
  type: "all" | "file" | "folder"
}

function listOptions(request: Request): ListOptions | Response {
  const query = new URL(request.url).searchParams
  const page = Number(query.get("page") ?? 1)
  const size = Number(query.get("size") ?? 100)
  const sort = query.get("sort") ?? "asc"
  const sortBy = query.get("sort_by") ?? "name"
  const type = query.get("type") ?? "all"
  if (!Number.isInteger(page) || page < 1) return apiError(400, "page must be a positive integer")
  if (!Number.isInteger(size) || size < 1 || size > 100)
    return apiError(400, "size must be between 1 and 100")
  if (sort !== "asc" && sort !== "desc") return apiError(400, "sort must be asc or desc")
  if (sortBy !== "name" && sortBy !== "created_at" && sortBy !== "updated_at")
    return apiError(400, "sort_by must be name, created_at, or updated_at")
  if (type !== "all" && type !== "file" && type !== "folder")
    return apiError(400, "type must be all, file, or folder")
  return { page, size, sort, sortBy, filter: query.get("filter") ?? "", type }
}

function paginate(items: Entry[], options: ListOptions) {
  const filtered = items.filter(
    (item) =>
      (options.type === "all" || item.type === options.type) &&
      item.display.toLocaleLowerCase().includes(options.filter.toLocaleLowerCase()),
  )
  filtered.sort((left, right) => {
    const result =
      options.sortBy === "name"
        ? left.display.localeCompare(right.display)
        : left[options.sortBy] - right[options.sortBy]
    return options.sort === "asc" ? result : -result
  })
  const start = (options.page - 1) * options.size
  return {
    items: filtered.slice(start, start + options.size),
    total: filtered.length,
    page: options.page,
    size: options.size,
  }
}

async function hashBytes(bytes: Uint8Array) {
  const digest = await crypto.subtle.digest("SHA-256", bytes.slice().buffer as ArrayBuffer)
  return [...new Uint8Array(digest)].map((value) => value.toString(16).padStart(2, "0")).join("")
}

function activeTarget(type: "file" | "folder", id: number) {
  return store.activeEntry(type, id)
}

function shareResponse(item: StoredShare) {
  const { password: _, ...body } = item
  return body
}

function shareAccess(token: string, password: string | null) {
  const item = store.shares.find((candidate) => candidate.token === token)
  if (!item) return { error: apiError(404, "resource not found") }
  if (
    (item.password !== null && item.password !== password) ||
    (item.expires_at !== null && item.expires_at <= Math.floor(Date.now() / 1000)) ||
    (item.max_downloads !== null && item.download_count >= item.max_downloads)
  ) {
    return { error: apiError(403, "share access denied") }
  }
  const entry = activeTarget(item.entry_type, item.entry_id)
  if (!entry) return { error: apiError(404, "resource not found") }
  return { item, entry }
}

function blobResponse(bytes: Uint8Array) {
  return new HttpResponse(bytes.slice().buffer, {
    status: 200,
    headers: {
      "Content-Type": "application/octet-stream",
      "Content-Length": String(bytes.byteLength),
    },
  })
}

async function uploadedFile(request: Request, parentId: number | null) {
  let display = ""
  let mimeType = "application/octet-stream"
  let bytes: Uint8Array
  if (request.headers.get("Content-Type")?.startsWith("multipart/form-data")) {
    const part = (await request.formData()).get("file")
    if (!(part instanceof File)) return { error: apiError(400, "file is required") }
    display = part.name.trim()
    mimeType = part.type || mimeType
    bytes = new Uint8Array(await part.arrayBuffer())
  } else {
    const disposition = request.headers.get("Content-Disposition")
    const encodedName = disposition?.match(/filename\*=UTF-8''([^;]+)/i)?.[1]
    const plainName = disposition?.match(/filename="?([^";]+)"?/i)?.[1]
    display = decodeURIComponent(
      encodedName ?? plainName ?? request.headers.get("X-File-Name") ?? "",
    ).trim()
    mimeType = request.headers.get("X-File-Type") || mimeType
    bytes = new Uint8Array(await request.arrayBuffer())
  }
  if (!display) return { error: apiError(400, "display is required") }
  if (parentId !== null && !store.activeFolder(parentId))
    return { error: apiError(404, "resource not found") }
  if (store.siblingNameExists(display, parentId))
    return { error: apiError(409, "resource conflict") }
  const hash = await hashBytes(bytes)
  const now = Math.floor(Date.now() / 1000)
  store.blobs.set(hash, bytes)
  const result: StoredFile = {
    type: "file",
    id: store.nextFileId++,
    display,
    parent_id: parentId,
    mime_type: mimeType,
    blob: { sha256: hash, size: bytes.byteLength, created_at: now },
    created_at: now,
    updated_at: now,
    deleted_at: null,
  }
  store.files.push(result)
  return { result }
}

const regularHandlers = [
  http.get("/healthz", ({ response }) => response(200).json({ status: "ok" })),
  http.get("/readyz", ({ response }) => response(200).json({ status: "ready" })),
  http.get("/api/v1/auth/session", ({ response }) =>
    loggedIn()
      ? response(204).empty()
      : response(401).json({ code: 4010, message: "authentication required" }),
  ),
  http.get("/api/v1/auth/login", ({ request, response }) => {
    const returnTo = safeReturnTo(new URL(request.url).searchParams.get("return_to"))
    const state = crypto.randomUUID()
    sessionStorage.setItem(oidcStateKey, state)
    sessionStorage.setItem(returnToKey, returnTo)
    const callback = new URL("/api/v1/auth/callback", window.location.origin)
    callback.searchParams.set("code", "fpan-development")
    callback.searchParams.set("state", state)
    return response.untyped(HttpResponse.redirect(callback.toString(), 302))
  }),
  http.get("/api/v1/auth/callback", ({ request, response }) => {
    const query = new URL(request.url).searchParams
    if (
      query.get("code") !== "fpan-development" ||
      query.get("state") !== sessionStorage.getItem(oidcStateKey)
    ) {
      return response(400).json({ code: 4000, message: "OIDC authentication failed" })
    }
    const returnTo = safeReturnTo(sessionStorage.getItem(returnToKey))
    sessionStorage.setItem(sessionKey, "true")
    sessionStorage.removeItem(oidcStateKey)
    sessionStorage.removeItem(returnToKey)
    return response.untyped(
      HttpResponse.redirect(new URL(returnTo, window.location.origin).toString(), 302),
    )
  }),
  http.post("/api/v1/auth/logout", ({ response }) => {
    sessionStorage.setItem(sessionKey, "false")
    return response(204).empty()
  }),
  http.get("/api/v1/entries", async ({ request, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const options = listOptions(request)
    if (options instanceof Response) return response.untyped(options)
    await delay(300)
    return response(200).json(paginate(store.entries(null), options))
  }),
  http.get("/api/v1/folders/{id}/entries", async ({ params, request, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const id = numericParam(params.id)
    if (!id) return response(400).json({ code: 4000, message: "invalid id" })
    if (!store.activeFolder(id))
      return response(404).json({ code: 4040, message: "resource not found" })
    const options = listOptions(request)
    if (options instanceof Response) return response.untyped(options)
    await delay(300)
    return response(200).json(paginate(store.entries(id), options))
  }),
  http.get("/api/v1/files/{id}", ({ params, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const item = store.activeFile(numericParam(params.id) ?? 0)
    return item
      ? response(200).json(item)
      : response(404).json({ code: 4040, message: "resource not found" })
  }),
  http.get("/api/v1/folders/{id}", ({ params, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const item = store.activeFolder(numericParam(params.id) ?? 0)
    return item
      ? response(200).json(item)
      : response(404).json({ code: 4040, message: "resource not found" })
  }),
  http.post("/api/v1/folders", async ({ request, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const body = await request.json()
    const display = body.display.trim()
    const parentId = body.parent_id ?? null
    if (!display || (parentId !== null && !store.activeFolder(parentId)))
      return response(400).json({ code: 4000, message: "invalid request" })
    if (store.siblingNameExists(display, parentId))
      return response(409).json({ code: 4090, message: "resource conflict" })
    const now = Math.floor(Date.now() / 1000)
    const item = {
      type: "folder" as const,
      id: store.nextFolderId++,
      display,
      parent_id: parentId,
      created_at: now,
      updated_at: now,
      deleted_at: null,
    }
    store.folders.push(item)
    return response(201).json(item)
  }),
  http.put("/api/v1/files/{id}", async ({ params, request, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const item = store.activeFile(numericParam(params.id) ?? 0)
    if (!item) return response(404).json({ code: 4040, message: "resource not found" })
    const body = await request.json()
    if (body.display === undefined && body.parent_id === undefined)
      return response(400).json({ code: 4000, message: "empty update" })
    const display = body.display?.trim() ?? item.display
    const parentId = body.parent_id === undefined ? item.parent_id : body.parent_id
    if (!display || (parentId !== null && !store.activeFolder(parentId)))
      return response(400).json({ code: 4000, message: "invalid request" })
    if (store.siblingNameExists(display, parentId, item))
      return response(409).json({ code: 4090, message: "resource conflict" })
    item.display = display
    item.parent_id = parentId
    item.updated_at = Math.floor(Date.now() / 1000)
    return response(200).json(item)
  }),
  http.put("/api/v1/folders/{id}", async ({ params, request, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const item = store.activeFolder(numericParam(params.id) ?? 0)
    if (!item) return response(404).json({ code: 4040, message: "resource not found" })
    const body = await request.json()
    if (body.display === undefined && body.parent_id === undefined)
      return response(400).json({ code: 4000, message: "empty update" })
    const display = body.display?.trim() ?? item.display
    const parentId = body.parent_id === undefined ? item.parent_id : body.parent_id
    const descendants = store.descendantFolderIds(item.id)
    if (
      !display ||
      (parentId !== null && (!store.activeFolder(parentId) || descendants.has(parentId)))
    )
      return response(400).json({ code: 4000, message: "invalid request" })
    if (store.siblingNameExists(display, parentId, item))
      return response(409).json({ code: 4090, message: "resource conflict" })
    item.display = display
    item.parent_id = parentId
    item.updated_at = Math.floor(Date.now() / 1000)
    return response(200).json(item)
  }),
  http.delete("/api/v1/files/{id}", ({ params, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const item = store.activeFile(numericParam(params.id) ?? 0)
    if (!item) return response(404).json({ code: 4040, message: "resource not found" })
    item.deleted_at = Math.floor(Date.now() / 1000)
    return response(204).empty()
  }),
  http.delete("/api/v1/folders/{id}", ({ params, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const item = store.activeFolder(numericParam(params.id) ?? 0)
    if (!item) return response(404).json({ code: 4040, message: "resource not found" })
    const folderIds = store.descendantFolderIds(item.id)
    const fileIds = store.files
      .filter(
        (file) =>
          file.deleted_at === null && file.parent_id !== null && folderIds.has(file.parent_id),
      )
      .map(({ id }) => id)
    const deletedAt = Math.floor(Date.now() / 1000)
    for (const folder of store.folders) if (folderIds.has(folder.id)) folder.deleted_at = deletedAt
    for (const file of store.files) if (fileIds.includes(file.id)) file.deleted_at = deletedAt
    return response(200).json({ folder_ids: [...folderIds], file_ids: fileIds })
  }),
  http.post("/api/v1/files", async ({ request, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const result = await uploadedFile(request, null)
    return result.error ? response.untyped(result.error) : response(201).json(result.result)
  }),
  http.post("/api/v1/folders/{id}/files", async ({ params, request, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const result = await uploadedFile(request, numericParam(params.id) ?? 0)
    return result.error ? response.untyped(result.error) : response(201).json(result.result)
  }),
  http.post("/api/v1/files/stream", async ({ request, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const result = await uploadedFile(request, null)
    return result.error ? response.untyped(result.error) : response(201).json(result.result)
  }),
  http.post("/api/v1/folders/{id}/files/stream", async ({ params, request, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const result = await uploadedFile(request, numericParam(params.id) ?? 0)
    return result.error ? response.untyped(result.error) : response(201).json(result.result)
  }),
  http.get("/api/v1/blobs/{sha256}", ({ params, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const bytes = store.blobs.get(String(params.sha256))
    return bytes
      ? response.untyped(blobResponse(bytes))
      : response(404).json({ code: 4040, message: "resource not found" })
  }),
  http.get("/api/v1/trash", ({ response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    return response(200).json({ items: store.trash() })
  }),
  http.delete("/api/v1/trash", ({ response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    for (const item of store.trash()) store.purge(item.type, item.id)
    return response(204).empty()
  }),
  http.delete("/api/v1/trash/{type}/{id}", ({ params, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const id = numericParam(params.id) ?? 0
    const item = store.trash().find((entry) => entry.type === params.type && entry.id === id)
    if (!item) return response(404).json({ code: 4040, message: "resource not found" })
    store.purge(item.type, item.id)
    return response(204).empty()
  }),
  http.post("/api/v1/trash/{type}/{id}/restore", ({ params, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const id = numericParam(params.id) ?? 0
    const item = store.trash().find((entry) => entry.type === params.type && entry.id === id)
    if (!item) return response(404).json({ code: 4040, message: "resource not found" })
    if (store.siblingNameExists(item.display, item.parent_id, item))
      return response(409).json({ code: 4090, message: "resource conflict" })
    if (item.type === "file") {
      const stored = store.files.find((file) => file.id === item.id)
      if (stored) stored.deleted_at = null
    } else {
      const ids = store.descendantFolderIds(item.id)
      for (const folder of store.folders) if (ids.has(folder.id)) folder.deleted_at = null
      for (const file of store.files)
        if (file.parent_id !== null && ids.has(file.parent_id)) file.deleted_at = null
    }
    return response(200).json(item)
  }),
  http.post("/api/v1/shares", async ({ request, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const body = await request.json()
    if (
      !activeTarget(body.entry_type, body.entry_id) ||
      body.password === "" ||
      (body.expires_at !== undefined && body.expires_at <= Date.now() / 1000) ||
      (body.max_downloads !== undefined && body.max_downloads < 1)
    )
      return response(400).json({ code: 4000, message: "invalid request" })
    const now = Math.floor(Date.now() / 1000)
    const item: StoredShare = {
      id: store.nextShareId++,
      entry_id: body.entry_id,
      entry_type: body.entry_type,
      token: `mock-share-${crypto.randomUUID()}`,
      password: body.password ?? null,
      has_password: body.password !== undefined,
      expires_at: body.expires_at ?? null,
      max_downloads: body.max_downloads ?? null,
      download_count: 0,
      created_at: now,
      updated_at: now,
    }
    store.shares.push(item)
    return response(201).json(shareResponse(item))
  }),
  http.get("/api/v1/shares", ({ request, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const query = new URL(request.url).searchParams
    const page = Number(query.get("page") ?? 1)
    const size = Number(query.get("size") ?? 100)
    if (!Number.isInteger(page) || page < 1 || !Number.isInteger(size) || size < 1 || size > 100)
      return response(400).json({ code: 4000, message: "page and size are out of range" })
    // ES2022 has no Array#toSorted; sort a copy to keep fixture ordering stable.
    // oxlint-disable-next-line unicorn/no-array-sort
    const items = store.shares.slice().sort((a, b) => b.created_at - a.created_at || b.id - a.id)
    return response(200).json({
      items: items.slice((page - 1) * size, page * size).map(shareResponse),
      total: items.length,
      page,
      size,
    })
  }),
  http.get("/api/v1/shares/{id}", ({ params, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const item = store.shares.find(({ id }) => id === (numericParam(params.id) ?? 0))
    return item
      ? response(200).json(shareResponse(item))
      : response(404).json({ code: 4040, message: "resource not found" })
  }),
  http.put("/api/v1/shares/{id}", async ({ params, request, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const item = store.shares.find(({ id }) => id === (numericParam(params.id) ?? 0))
    if (!item) return response(404).json({ code: 4040, message: "resource not found" })
    const body = await request.json()
    if (
      body.password === "" ||
      (body.expires_at !== undefined &&
        body.expires_at !== null &&
        body.expires_at <= Date.now() / 1000) ||
      (body.max_downloads !== undefined &&
        body.max_downloads !== null &&
        body.max_downloads < item.download_count)
    )
      return response(400).json({ code: 4000, message: "invalid request" })
    if (body.password !== undefined) {
      item.password = body.password
      item.has_password = body.password !== null
    }
    if (body.expires_at !== undefined) item.expires_at = body.expires_at
    if (body.max_downloads !== undefined) item.max_downloads = body.max_downloads
    item.updated_at = Math.floor(Date.now() / 1000)
    return response(200).json(shareResponse(item))
  }),
  http.delete("/api/v1/shares/{id}", ({ params, response }) => {
    const auth = requireAuth()
    if (auth) return response.untyped(auth)
    const index = store.shares.findIndex(({ id }) => id === (numericParam(params.id) ?? 0))
    if (index < 0) return response(404).json({ code: 4040, message: "resource not found" })
    store.shares.splice(index, 1)
    return response(204).empty()
  }),
  http.get("/api/v1/s/{token}", ({ params, request, response }) => {
    const access = shareAccess(
      String(params.token),
      new URL(request.url).searchParams.get("password"),
    )
    if (access.error) return response.untyped(access.error)
    const { item, entry } = access as { item: StoredShare; entry: Entry }
    return response(200).json({
      token: item.token,
      entry,
      expires_at: item.expires_at,
      remaining_downloads:
        item.max_downloads === null ? null : item.max_downloads - item.download_count,
    })
  }),
  http.get("/api/v1/s/{token}/entries", ({ params, request, response }) => {
    const access = shareAccess(
      String(params.token),
      new URL(request.url).searchParams.get("password"),
    )
    if (access.error) return response.untyped(access.error)
    const { item } = access as { item: StoredShare }
    if (item.entry_type !== "folder")
      return response(403).json({ code: 4030, message: "share access denied" })
    const options = listOptions(request)
    if (options instanceof Response) return response.untyped(options)
    const requestedParent = new URL(request.url).searchParams.get("parent_id")
    const parentId = requestedParent === null ? item.entry_id : Number(requestedParent)
    if (!Number.isSafeInteger(parentId) || !store.descendantFolderIds(item.entry_id).has(parentId))
      return response(403).json({ code: 4030, message: "share access denied" })
    return response(200).json(paginate(store.entries(parentId), options))
  }),
  http.get("/api/v1/s/{token}/blobs/{sha256}", ({ params, request, response }) => {
    const access = shareAccess(
      String(params.token),
      new URL(request.url).searchParams.get("password"),
    )
    if (access.error) return response.untyped(access.error)
    const { item } = access as { item: StoredShare }
    const hash = String(params.sha256)
    const allowed =
      item.entry_type === "file"
        ? store.activeFile(item.entry_id)?.blob.sha256 === hash
        : store.files.some(
            (file) =>
              file.deleted_at === null &&
              file.parent_id !== null &&
              store.descendantFolderIds(item.entry_id).has(file.parent_id) &&
              file.blob.sha256 === hash,
          )
    const bytes = store.blobs.get(hash)
    if (!allowed || !bytes)
      return response(403).json({ code: 4030, message: "share access denied" })
    item.download_count++
    item.updated_at = Math.floor(Date.now() / 1000)
    return response.untyped(blobResponse(bytes))
  }),
  untypedHttp.all("/api/*", ({ request }) => {
    console.error(`[MSW] Missing API handler: ${request.method} ${request.url}`)
    return HttpResponse.error()
  }),
  untypedHttp.all("/healthz", () => HttpResponse.error()),
  untypedHttp.all("/readyz", () => HttpResponse.error()),
]

const errorHandlers = [
  http.get("/healthz", ({ response }) => response(200).json({ status: "ok" })),
  http.get("/readyz", ({ response }) =>
    response(503).json({ code: 5030, message: "mock dependencies are unavailable" }),
  ),
  http.get("/api/v1/auth/session", ({ response }) => response(204).empty()),
  untypedHttp.all("/api/*", () => HttpResponse.error()),
]

export const handlers = store.scenario === "error" ? errorHandlers : regularHandlers
