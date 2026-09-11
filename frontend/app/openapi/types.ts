import type { components, paths } from "./schema"

export type EntriesQuery = NonNullable<paths["/api/v1/entries"]["get"]["parameters"]["query"]>

export type NormalizedEntriesQuery = Required<Omit<EntriesQuery, "filter">> &
  Pick<EntriesQuery, "filter">

export type ParentId = number | null
export type FolderId = number
export type FileId = number

export type FileEntry = components["schemas"]["File"]
export type FolderEntry = components["schemas"]["Folder"]
export type Entry = components["schemas"]["Entry"]
export type Share = components["schemas"]["Share"]
