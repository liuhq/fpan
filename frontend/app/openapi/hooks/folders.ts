import useSWR, { useSWRConfig } from "swr"
import useSWRMutation from "swr/mutation"

import { api, ApiError } from "../client"
import {
  apiKeys,
  isEntriesKey,
  isEntriesKeyForParent,
  isFileDetailKey,
  isFolderDetailKey,
} from "../keys"
import type { paths } from "../schema"
import type { FolderId, ParentId } from "../types"

export function useFolder(id: FolderId) {
  return useSWR(apiKeys.folders.detail(id), async () => {
    const { data, error, response } = await api.GET("/api/v1/folders/{id}", {
      params: { path: { id } },
    })

    if (error) {
      throw new ApiError(response.status, error)
    }

    return data
  })
}

export type CreateFolderInput = Omit<
  paths["/api/v1/folders"]["post"]["requestBody"]["content"]["application/json"],
  "parent_id"
>

export function useCreateFolder(parentId: ParentId) {
  const { mutate } = useSWRConfig()

  const { trigger, ...state } = useSWRMutation(
    apiKeys.folders.create(parentId),
    async (_, { arg: { display } }: { arg: CreateFolderInput }) => {
      const { data, error, response } = await api.POST("/api/v1/folders", {
        body: {
          display,
          parent_id: parentId,
        },
      })

      if (error) {
        throw new ApiError(response.status, error)
      }

      return data
    },
  )

  const createFolder = async (input: CreateFolderInput) => {
    const folder = await trigger(input)
    await mutate((key) => isEntriesKeyForParent(key, parentId))
    return folder
  }

  return {
    ...state,
    createFolder,
  }
}

export type UpdateFolderInput =
  paths["/api/v1/folders/{id}"]["put"]["requestBody"]["content"]["application/json"]

export function useUpdateFolder(id: FolderId, srcParentId: ParentId) {
  const { mutate } = useSWRConfig()

  const { trigger, ...state } = useSWRMutation(
    apiKeys.folders.update(id),
    async (_, { arg }: { arg: UpdateFolderInput }) => {
      const body: UpdateFolderInput = {}
      if ("display" in arg && arg.display !== undefined) {
        body.display = arg.display
      }
      if ("parent_id" in arg && arg.parent_id !== undefined) {
        body.parent_id = arg.parent_id
      }

      const { data, error, response } = await api.PUT("/api/v1/folders/{id}", {
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

  const renameFolder = async (display: NonNullable<UpdateFolderInput["display"]>) => {
    const folder = await trigger({ display })
    await Promise.all([
      mutate((key) => isEntriesKeyForParent(key, srcParentId)),
      mutate(apiKeys.folders.detail(id), folder, { revalidate: false }),
    ])
    return folder
  }

  const moveFolder = async (destParentId: ParentId) => {
    const folder = await trigger({ parent_id: destParentId })
    const parentIds = destParentId === srcParentId ? [srcParentId] : [srcParentId, destParentId]
    await Promise.all([
      ...parentIds.map((parentId) => mutate((key) => isEntriesKeyForParent(key, parentId))),
      mutate(apiKeys.folders.detail(id), folder, { revalidate: false }),
    ])
    return folder
  }

  return {
    ...state,
    renameFolder,
    moveFolder,
  }
}

export function useDeleteFolder(id: FolderId, parentId: ParentId) {
  const { mutate } = useSWRConfig()

  const { trigger, ...state } = useSWRMutation(apiKeys.folders.delete(id), async () => {
    const { data, error, response } = await api.DELETE("/api/v1/folders/{id}", {
      params: { path: { id } },
    })

    if (error) {
      throw new ApiError(response.status, error)
    }

    return data
  })

  const deleteFolder = async () => {
    const deleted = await trigger()
    const folderIds = new Set(deleted.folder_ids)
    const fileIds = new Set(deleted.file_ids)

    await Promise.all([
      mutate((key) => isEntriesKey(key) && key[1] !== null && folderIds.has(key[1]), undefined, {
        revalidate: false,
      }),
      mutate((key) => isFolderDetailKey(key) && folderIds.has(key[1]), undefined, {
        revalidate: false,
      }),
      mutate((key) => isFileDetailKey(key) && fileIds.has(key[1]), undefined, {
        revalidate: false,
      }),
    ])
    await mutate((key) => isEntriesKeyForParent(key, parentId))
  }

  return {
    ...state,
    deleteFolder,
  }
}
