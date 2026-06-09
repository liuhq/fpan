/**
 * Cycle Prevention (Application-Layer Invariant)
 * -----------------------------------------------
 * The database schema only blocks direct self-references (id == parent_id).
 * It CANNOT detect multi-level cycles (e.g. A -> B -> A) that arise when a
 * folder is moved beneath one of its own descendants.
 *
 * REQUIREMENT: Before committing any operation that changes `parent_id`,
 * the application MUST verify that the new parent is not the moved entry
 * itself nor any of its descendants. Walk the ancestor chain of the new
 * parent up to root ('111111111111111111111'); if the moved entry's id is
 * encountered, reject the move.
 *
 * Implementations MUST bound the walk (max depth or a visited set) so that
 * any pre-existing corrupt cycle cannot cause an infinite loop.
 *
 * This check and the UPDATE must run inside a single transaction to avoid
 * concurrent reparenting races that could still introduce a cycle.
 *
 *
 *
 * Recursive Soft-Delete (Application-Layer Invariant)
 * ---------------------------------------------------
 * Setting `deleted_at` on a folder does NOT cascade to its children; the
 * schema's ON DELETE CASCADE only applies to hard deletes, not soft deletes.
 * Left unhandled, children remain "active" under a deleted parent.
 *
 * REQUIREMENT: When soft-deleting a folder, the application MUST set the same
 * `deleted_at` timestamp on the entire subtree (the folder and all of its
 * descendants) within one transaction.
 *
 * On RESTORE (clearing `deleted_at`), the application MUST likewise restore
 * the subtree and resolve name collisions against the partial unique index
 * idx_entries_name (parent_id, name WHERE deleted_at IS NULL), e.g. by
 * renaming or prompting the user.
 */

--- !!! Run `PRAGMA foreign_keys = ON;` when db connects
--- !!! HARDCODE: root = '111111111111111111111'

CREATE TABLE IF NOT EXISTS entries (
    id TEXT PRIMARY KEY,
    parent_id TEXT NOT NULL,
    TYPE TEXT NOT NULL CHECK (TYPE IN ('file', 'folder')),
    name TEXT NOT NULL,
    mime_type TEXT,
    size INTEGER CHECK (
        size IS NULL
        OR size >= 0
    ),
    storage_id TEXT,
    sha256 TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    deleted_at INTEGER,
    CHECK (
        (type = 'file')
        OR (
            type = 'folder'
            AND storage_id IS NULL
            AND size IS NULL
            AND sha256 IS NULL
        )
    ),
    -- `ON DELETE CASCADE` when empty trash bin is emptied
    FOREIGN KEY (parent_id) REFERENCES entries (id) ON DELETE CASCADE
) WITHOUT ROWID;

-------------
--- INDEX ---
-------------
CREATE UNIQUE INDEX IF NOT EXISTS idx_entries_name ON entries (parent_id, name)
WHERE
    deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_entries_parent ON entries (parent_id);

CREATE INDEX IF NOT EXISTS idx_entries_parent_active ON entries (parent_id)
WHERE
    deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_entries_deleted ON entries (deleted_at)
WHERE
    deleted_at IS NOT NULL;

---------------
--- TRIGGER ---
---------------
CREATE TRIGGER trg_no_type_change
BEFORE UPDATE OF type ON entries
FOR EACH ROW
WHEN OLD.type != NEW.type
BEGIN
    SELECT RAISE(ABORT, 'type is immutable');
END;

-- Prevent self reference from being created
CREATE TRIGGER trg_insert_no_self_reference
BEFORE INSERT ON entries
FOR EACH ROW
WHEN NEW.id = NEW.parent_id AND NEW.id != '111111111111111111111'
BEGIN
    SELECT RAISE(ABORT, 'only root may be self-referential');
END;

CREATE TRIGGER trg_update_no_self_reference
BEFORE UPDATE OF parent_id ON entries
FOR EACH ROW
WHEN NEW.id = NEW.parent_id AND NEW.id != '111111111111111111111'
BEGIN
    SELECT RAISE(ABORT, 'only root may be self-referential');
END;

-- Check `parent_id` is a folder
CREATE TRIGGER trg_insert_parent_must_be_folder
BEFORE INSERT ON entries
FOR EACH ROW
WHEN NEW.id != NEW.parent_id
AND (SELECT type FROM entries WHERE id = NEW.parent_id AND deleted_at IS NULL) IS NOT 'folder'
BEGIN
    SELECT RAISE(ABORT, 'parent must be a folder');
END;

CREATE TRIGGER trg_update_parent_must_be_folder
BEFORE UPDATE OF parent_id ON entries
FOR EACH ROW
WHEN NEW.id != NEW.parent_id
AND (SELECT type FROM entries WHERE id = NEW.parent_id AND deleted_at IS NULL) IS NOT 'folder'
BEGIN
    SELECT RAISE(ABORT, 'parent must be a folder');
END;

-- Update `updated_at`
CREATE TRIGGER trg_entries_updated_at
AFTER UPDATE ON entries
FOR EACH ROW
WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE entries SET updated_at = unixepoch() WHERE id = NEW.id;
END;

-- Prevent root folder from being deleted
CREATE TRIGGER trg_protect_root
BEFORE DELETE ON entries
FOR EACH ROW
WHEN OLD.id = '111111111111111111111'
BEGIN
    SELECT RAISE(ABORT, 'cannot delete root folder');
END;

CREATE TRIGGER trg_protect_root_softdelete
BEFORE UPDATE OF deleted_at ON entries
FOR EACH ROW
WHEN OLD.id = '111111111111111111111'
AND NEW.deleted_at IS NOT NULL
BEGIN
    SELECT RAISE(ABORT, 'cannot delete root folder');
END;

-- Prevent `parent_id` of root folder from being modified
CREATE TRIGGER trg_protect_root_reparent
BEFORE UPDATE OF parent_id ON entries
FOR EACH ROW
WHEN OLD.id = '111111111111111111111'
AND NEW.parent_id != '111111111111111111111'
BEGIN
    SELECT RAISE(ABORT, 'cannot move root folder');
END;

------------
--- INIT ---
------------
INSERT
    OR IGNORE INTO entries (
        id,
        parent_id,
        type,
        name,
        created_at,
        updated_at
    )
VALUES (
        '111111111111111111111',
        '111111111111111111111',
        'folder',
        'root',
        0,
        0
    );