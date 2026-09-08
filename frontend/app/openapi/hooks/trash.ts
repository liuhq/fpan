import useSWR, { useSWRConfig } from "swr"
import useSWRMutation from "swr/mutation"

import { api, ApiError } from "../client"
import { apiKeys, isEntriesKeyForParent, isTrashDetailKey } from "../keys"
import type { paths } from "../schema"
import type { FileId, FolderId } from "../types"

type TrashItemsOutput =
  paths["/api/v1/trash"]["get"]["responses"]["200"]["content"]["application/json"]

export function useTrash() {
  return useSWR(apiKeys.trash.detail(), async () => {
    const { data, error, response } = await api.GET("/api/v1/trash")

    if (error) {
      throw new ApiError(response.status, error)
    }

    return data
  })
}

export type DeleteItemInput = paths["/api/v1/trash/{type}/{id}"]["delete"]["parameters"]["path"]

export function useDeleteTrash() {
  const { mutate } = useSWRConfig()

  const { trigger, ...state } = useSWRMutation(
    apiKeys.trash.delete(),
    async (_, { arg }: { arg?: DeleteItemInput }) => {
      const { data, error, response } = await (arg === undefined
        ? api.DELETE("/api/v1/trash")
        : api.DELETE("/api/v1/trash/{type}/{id}", {
            params: {
              path: {
                id: arg.id,
                type: arg.type,
              },
            },
          }))

      if (error) {
        throw new ApiError(response.status, error)
      }

      return data
    },
  )

  const emptyTrash = async () => {
    await trigger()
    await mutate<TrashItemsOutput>(apiKeys.trash.detail(), { items: [] }, { revalidate: false })
  }

  const deleteFromTrash = async (id: FileId | FolderId, type: DeleteItemInput["type"]) => {
    await trigger({ id, type })
    await mutate((key) => isTrashDetailKey(key))
  }

  return {
    ...state,
    emptyTrash,
    deleteFromTrash,
  }
}

export type RestoreItemInput =
  paths["/api/v1/trash/{type}/{id}/restore"]["post"]["parameters"]["path"]

export function useRestoreTrash() {
  const { mutate } = useSWRConfig()

  const { trigger, ...state } = useSWRMutation(
    apiKeys.trash.restore(),
    async (_, { arg }: { arg: RestoreItemInput }) => {
      const { data, error, response } = await api.POST("/api/v1/trash/{type}/{id}/restore", {
        params: {
          path: {
            id: arg.id,
            type: arg.type,
          },
        },
      })

      if (error) {
        throw new ApiError(response.status, error)
      }

      return data
    },
  )

  const restoreFromTrash = async ({ id, type }: RestoreItemInput) => {
    const restored = await trigger({ id, type })
    const detailKey =
      restored.type === "file"
        ? apiKeys.files.detail(restored.id)
        : apiKeys.folders.detail(restored.id)

    await Promise.all([
      mutate((key) => isTrashDetailKey(key)),
      mutate((key) => isEntriesKeyForParent(key, restored.parent_id)),
      mutate(detailKey, restored, { revalidate: false }),
    ])

    return restored
  }

  return {
    ...state,
    restoreFromTrash,
  }
}
