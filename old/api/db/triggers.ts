import { getTableConfig } from "drizzle-orm/sqlite-core"

import { ROOT } from "../constants.ts"
import { blobTable, fileTable, folderTable } from "./schema.ts"

const files = getTableConfig(fileTable).name
const blobs = getTableConfig(blobTable).name
const folders = getTableConfig(folderTable).name

export const triggerStatements: string[] = [
  `CREATE TRIGGER IF NOT EXISTS trg_blob_ref_inc
AFTER INSERT ON ${files}
BEGIN
  UPDATE ${blobs} SET ref_count = ref_count + 1 WHERE sha256 = NEW.sha256;
END;`,

  `CREATE TRIGGER IF NOT EXISTS trg_blob_ref_dec
AFTER DELETE ON ${files}
BEGIN
  UPDATE ${blobs} SET ref_count = ref_count - 1 WHERE sha256 = OLD.sha256;
END;`,

  `CREATE TRIGGER IF NOT EXISTS trg_blob_ref_move
AFTER UPDATE OF sha256 ON ${files}
WHEN OLD.sha256 IS NOT NEW.sha256
BEGIN
  UPDATE ${blobs} SET ref_count = ref_count - 1 WHERE sha256 = OLD.sha256;
  UPDATE ${blobs} SET ref_count = ref_count + 1 WHERE sha256 = NEW.sha256;
END;`,

  `CREATE TRIGGER IF NOT EXISTS trg_protect_root_delete
BEFORE DELETE ON ${folders}
WHEN OLD.id = '${ROOT}'
BEGIN
  SELECT RAISE(ABORT, 'cannot delete root folder');
END;`,

  `CREATE TRIGGER IF NOT EXISTS trg_protect_root_update
BEFORE UPDATE ON ${folders}
WHEN OLD.id = '${ROOT}'
  AND (NEW.parent_id IS NOT NULL
    OR NEW.deleted_at IS NOT NULL
    OR NEW.name IS NOT OLD.name)
BEGIN
  SELECT RAISE(ABORT, 'root folder is immutable');
END;`,
].map((s) => s.trim())
