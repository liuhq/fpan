import useSWR, { useSWRConfig } from "swr"
import useSWRMutation from "swr/mutation"

import { api, ApiError } from "../client"
import { apiKeys, isEntriesKeyForParent } from "../keys"
import type { paths } from "../schema"
import type { FolderId, ParentId } from "../types"

export function useFolder(id: FolderId) {
  return useSWR(apiKeys.folders("get", id), async () => {
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
    apiKeys.folders("create", parentId),
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

export function useUpdateFolder(sourceParentId: ParentId) {
  const { mutate } = useSWRConfig()

  const { trigger, ...state } = useSWRMutation(
    apiKeys.folders("update", sourceParentId),
    async (_, { arg: { id, ...body } }: { arg: UpdateFolderInput & { id: FolderId } }) => {
      const { data, error, response } = await api.PUT("/api/v1/folders/{id}", {
        params: {
          path: { id },
        },
        body:
          "parent_id" in body && body.parent_id !== sourceParentId
            ? body
            : { display: body.display },
      })

      if (error) {
        throw new ApiError(response.status, error)
      }

      return data
    },
  )

  const renameFolder = async (id: FolderId, display: NonNullable<UpdateFolderInput["display"]>) => {
    const folder = await trigger({ id, display })
    await Promise.all([
      mutate((key) => isEntriesKeyForParent(key, sourceParentId)),
      mutate(apiKeys.folders("get", id), folder, { revalidate: false }),
    ])
    return folder
  }

  const moveFolder = async (id: FolderId, { display, parent_id }: UpdateFolderInput) => {
    const folder = await trigger({ id, display, parent_id })
    await Promise.all([
      mutate((key) => isEntriesKeyForParent(key, sourceParentId)),
      mutate(apiKeys.folders("get", id), folder, { revalidate: false }),
    ])
    if (parent_id !== undefined) {
      await mutate((key) => isEntriesKeyForParent(key, parent_id))
    }
    return folder
  }

  return {
    ...state,
    renameFolder,
    moveFolder,
  }
}

export function useDeleteFolder(parentId: ParentId) {
  const { mutate } = useSWRConfig()

  const { trigger, ...state } = useSWRMutation(
    apiKeys.folders("delete"),
    async (_, { arg: id }: { arg: FolderId }) => {
      const { data, error, response } = await api.DELETE("/api/v1/folders/{id}", {
        params: { path: { id } },
      })

      if (error) {
        throw new ApiError(response.status, error)
      }

      return data
    },
  )

  const deleteFolder = async (id: FolderId) => {
    await trigger(id)
    await Promise.all([
      mutate((key) => isEntriesKeyForParent(key, parentId)),
      mutate(apiKeys.folders("get", id), undefined, { revalidate: false }),
    ])
  }

  return {
    ...state,
    deleteFolder,
  }
}
