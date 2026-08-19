package models

import (
	"reflect"
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestBlobSchema(t *testing.T) {
	model := parseSchema(t, &Blob{})

	checks := model.ParseCheckConstraints()
	check, ok := checks["chk_blobs_sha256_format"]
	if !ok {
		t.Fatal("missing SHA256 format check")
	}
	if check.Constraint != "sha256 ~ '^[0-9a-f]{64}$'" {
		t.Fatalf("unexpected SHA256 check: %q", check.Constraint)
	}

	sha256 := model.LookUpField("SHA256")
	if sha256 == nil || !sha256.PrimaryKey || sha256.DataType != "char(64)" {
		t.Fatalf("SHA256 must be a 64-character primary key: %#v", sha256)
	}
}

func TestFileLogicalPathSchema(t *testing.T) {
	model := parseSchema(t, &File{})

	assertNullableParent(t, model)
	assertIndex(t, model, "idx_files_parent", false, "parent_id")
	assertIndex(
		t,
		model,
		"uidx_files_parent_display_active",
		true,
		"parent_id",
		"display",
	)
	assertIndex(t, model, "uidx_files_root_display_active", true, "display")
	assertIndexWhere(
		t,
		model,
		"uidx_files_parent_display_active",
		"deleted_at IS NULL AND parent_id IS NOT NULL",
	)
	assertIndexWhere(
		t,
		model,
		"uidx_files_root_display_active",
		"deleted_at IS NULL AND parent_id IS NULL",
	)
	assertOnDelete(t, model, "Parent", "CASCADE")
	assertOnDelete(t, model, "Blob", "RESTRICT")
}

func TestFolderLogicalPathSchema(t *testing.T) {
	model := parseSchema(t, &Folder{})

	assertNullableParent(t, model)
	assertIndex(t, model, "idx_folders_parent", false, "parent_id")
	assertIndex(
		t,
		model,
		"uidx_folders_parent_display_active",
		true,
		"parent_id",
		"display",
	)
	assertIndex(t, model, "uidx_folders_root_display_active", true, "display")
	assertIndexWhere(
		t,
		model,
		"uidx_folders_parent_display_active",
		"deleted_at IS NULL AND parent_id IS NOT NULL",
	)
	assertIndexWhere(
		t,
		model,
		"uidx_folders_root_display_active",
		"deleted_at IS NULL AND parent_id IS NULL",
	)
	assertOnDelete(t, model, "Parent", "CASCADE")
}

func parseSchema(t *testing.T, value any) *schema.Schema {
	t.Helper()

	model, err := schema.Parse(value, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse %T schema: %v", value, err)
	}

	return model
}

func assertNullableParent(t *testing.T, model *schema.Schema) {
	t.Helper()

	parentID := model.LookUpField("ParentID")
	if parentID == nil {
		t.Fatal("missing ParentID field")
	}
	if parentID.NotNull {
		t.Fatal("ParentID must remain nullable so NULL represents the root")
	}
}

func assertIndex(
	t *testing.T,
	model *schema.Schema,
	name string,
	unique bool,
	wantFields ...string,
) {
	t.Helper()

	index := findIndex(t, model, name)
	if (index.Class == "UNIQUE") != unique {
		t.Fatalf("index %q uniqueness = %q, want unique=%t", name, index.Class, unique)
	}

	gotFields := make([]string, 0, len(index.Fields))
	for _, field := range index.Fields {
		gotFields = append(gotFields, field.DBName)
	}
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("index %q fields = %v, want %v", name, gotFields, wantFields)
	}
}

func assertIndexWhere(t *testing.T, model *schema.Schema, name, want string) {
	t.Helper()

	if got := findIndex(t, model, name).Where; got != want {
		t.Fatalf("index %q WHERE = %q, want %q", name, got, want)
	}
}

func findIndex(t *testing.T, model *schema.Schema, name string) *schema.Index {
	t.Helper()

	for _, index := range model.ParseIndexes() {
		if index.Name == name {
			return index
		}
	}

	t.Fatalf("missing index %q", name)
	return nil
}

func assertOnDelete(t *testing.T, model *schema.Schema, relationName, want string) {
	t.Helper()

	relation, ok := model.Relationships.Relations[relationName]
	if !ok {
		t.Fatalf("missing %q relationship", relationName)
	}

	constraint := relation.ParseConstraint()
	if constraint == nil {
		t.Fatalf("missing %q relationship constraint", relationName)
	}
	if constraint.OnDelete != want {
		t.Fatalf("%q OnDelete = %q, want %q", relationName, constraint.OnDelete, want)
	}
}
