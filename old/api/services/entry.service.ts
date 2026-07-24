import type { DbClient } from "#db/client.ts"
import { blobTable, fileTable } from "#db/schema.ts"
import { NotFoundError, AccessDeniedError } from "#errors"
import type { InsertBlob } from "#services/blob.service.ts"
import { createFileService, type InsertFile } from "#services/file.service.ts"
import { createShareService, type SelectShare } from "#services/share.service.ts"

export const createEntryService = (db: DbClient) => {
  const fileSrv = createFileService(db)
  const shareSrv = createShareService(db)

  return {
    saveFile: async (param: {
      name: InsertFile["name"]
      sha256: InsertBlob["sha256"]
      size: InsertBlob["size"]
      folderId: InsertFile["folder_id"]
      mimeType: InsertFile["mime_type"]
    }) => {
      const { id: fileId } = await db.transaction(async (tx) => {
        await tx.insert(blobTable).values({ sha256: param.sha256, size: param.size }).returning()
        const [savedFile] = await tx
          .insert(fileTable)
          .values({
            name: param.name,
            sha256: param.sha256,
            folder_id: param.folderId,
            mime_type: param.mimeType,
          })
          .returning({ id: fileTable.id })

        if (!savedFile) throw new NotFoundError("Save failed")
        return savedFile
      })

      return fileSrv.getBlob(fileId)
    },
    accessShare: async (token: SelectShare["token"], hashedPasswd?: string) => {
      const [shared] = await shareSrv.findByToken(token)

      if (!shared) throw new NotFoundError(`Invalid token: ${token}`)
      if (shared.revoked_at) throw new AccessDeniedError(`Shared revoked at ${shared.revoked_at}`)

      if (shared.password_hash) {
        if (!hashedPasswd) throw new AccessDeniedError("Password required")
        if (hashedPasswd !== shared.password_hash) throw new AccessDeniedError("Invalid password")
      }

      const [downloaded] = await shareSrv.recordDownload(token)
      if (!downloaded) throw new AccessDeniedError("Shared expired or limit reached")

      const [file] = await fileSrv.findById(downloaded.file_id)
      if (!file) throw new NotFoundError("File is not exist")

      return file
    },
  } as const
}
