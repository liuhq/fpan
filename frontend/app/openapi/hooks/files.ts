import useSWR, { useSWRConfig } from "swr"
import useSWRMutation from "swr/mutation"

import { api, ApiError } from "../client"
import { apiKeys, isEntriesKeyForParent } from "../keys"
import type { paths } from "../schema"
import type { FileId, ParentId } from "../types"

export function useFile(id: FileId) {
  return useSWR(apiKeys.files.detail(id), async () => {
    const { data, error, response } = await api.GET("/api/v1/files/{id}", {
      params: { path: { id } },
    })

    if (error) {
      throw new ApiError(response.status, error)
    }

    return data
  })
}

export type UpdateFileInput =
  paths["/api/v1/files/{id}"]["put"]["requestBody"]["content"]["application/json"]

export function useUpdateFile(id: FileId, srcParentId: ParentId) {
  const { mutate } = useSWRConfig()

  const { trigger, ...state } = useSWRMutation(
    apiKeys.files.update(id),
    async (_, { arg }: { arg: UpdateFileInput }) => {
      const body: UpdateFileInput = {}
      if ("display" in arg && arg.display !== undefined) {
        body.display = arg.display
      }
      if ("parent_id" in arg && arg.parent_id !== undefined) {
        body.parent_id = arg.parent_id
      }

      const { data, error, response } = await api.PUT("/api/v1/files/{id}", {
        params: {
          path: { id },
        },
        body,
      })

      if (error) {
        throw new ApiError(response.status, error)
      }

      return data
    },
  )

  const renameFile = async (display: NonNullable<UpdateFileInput["display"]>) => {
    const file = await trigger({ display })
    await Promise.all([
      mutate((key) => isEntriesKeyForParent(key, srcParentId)),
      mutate(apiKeys.files.detail(id), file, { revalidate: false }),
    ])
    return file
  }

  const moveFile = async (destParentId: ParentId) => {
    const file = await trigger({ parent_id: destParentId })
    const parentIds = destParentId === srcParentId ? [srcParentId] : [srcParentId, destParentId]
    await Promise.all([
      ...parentIds.map((parentId) => mutate((key) => isEntriesKeyForParent(key, parentId))),
      mutate(apiKeys.files.detail(id), file, { revalidate: false }),
    ])
    return file
  }

  return {
    ...state,
    renameFile,
    moveFile,
  }
}

export function useDeleteFile(id: FileId, parentId: ParentId) {
  const { mutate } = useSWRConfig()

  const { trigger, ...state } = useSWRMutation(apiKeys.files.delete(id), async () => {
    const { data, error, response } = await api.DELETE("/api/v1/files/{id}", {
      params: {
        path: { id },
      },
    })

    if (error) {
      throw new ApiError(response.status, error)
    }

    return data
  })

  const deleteFile = async () => {
    await trigger()
    await Promise.all([
      mutate((key) => isEntriesKeyForParent(key, parentId)),
      mutate(apiKeys.files.detail(id), undefined, { revalidate: false }),
    ])
  }

  return {
    ...state,
    deleteFile,
  }
}

async function $uploadByForm(file: File, parentId: ParentId) {
  const options = {
    body: { file },
    bodySerializer: () => {
      const formData = new FormData()
      formData.append("file", file)
      return formData
    },
  }

  return parentId === null
    ? api.POST("/api/v1/files", options)
    : api.POST("/api/v1/folders/{id}/files", {
        ...options,
        params: { path: { id: parentId } },
      })
}

function $contentDisposition(filename: string) {
  const encoded = encodeURIComponent(filename).replace(
    /['()*]/g,
    (character) => `%${character.charCodeAt(0).toString(16).toUpperCase()}`,
  )
  return `attachment; filename*=UTF-8''${encoded}`
}

async function $uploadByStream(file: File, parentId: ParentId) {
  const header = {
    "Content-Disposition": $contentDisposition(file.name),
    ...(file.type ? { "X-File-Type": file.type } : {}),
  }
  const options = {
    params: { header },
    headers: { "Content-Type": "application/octet-stream" },
    body: file,
    bodySerializer: (body: Blob) => body,
  }

  return parentId === null
    ? api.POST("/api/v1/files/stream", options)
    : api.POST("/api/v1/folders/{id}/files/stream", {
        ...options,
        params: {
          path: { id: parentId },
          header,
        },
      })
}

export function useCreateFile(parentId: ParentId) {
  const { mutate } = useSWRConfig()

  const { trigger, ...state } = useSWRMutation(
    apiKeys.files.create(parentId),
    async (_, { arg }: { arg: { file: File; stream: boolean } }) => {
      const { data, error, response } = await (arg.stream
        ? $uploadByStream(arg.file, parentId)
        : $uploadByForm(arg.file, parentId))

      if (error) {
        throw new ApiError(response.status, error)
      }

      return data
    },
  )

  const uploadByForm = async (file: File) => {
    const fileInfo = await trigger({ file, stream: false })
    await mutate((key) => isEntriesKeyForParent(key, parentId))
    return fileInfo
  }

  const uploadByStream = async (file: File) => {
    const fileInfo = await trigger({ file, stream: true })
    await mutate((key) => isEntriesKeyForParent(key, parentId))
    return fileInfo
  }

  return {
    ...state,
    uploadByForm,
    uploadByStream,
  }
}
