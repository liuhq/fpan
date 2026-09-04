import type { paths } from "./schema"

export type EntriesQuery = NonNullable<paths["/api/v1/entries"]["get"]["parameters"]["query"]>

export type NormalizedEntriesQuery = Required<Omit<EntriesQuery, "filter">> &
  Pick<EntriesQuery, "filter">

export type ParentId = number | null
export type FolderId = number
