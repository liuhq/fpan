package database

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/liuhq/fpan/internal/models"
	"gorm.io/gorm"
)

func TestNormalizeListOptionsDefaults(t *testing.T) {
	got, err := normalizeListOptions(ListEntriesOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := ListEntriesOptions{
		Page: 1, Size: 100, Sort: SortAscending, SortBy: EntrySortName, Type: EntryTypeAll,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeListOptions() = %#v, want %#v", got, want)
	}
}

func TestNormalizeListOptionsRejectsInvalidValues(t *testing.T) {
	tests := []ListEntriesOptions{
		{Page: -1},
		{Size: 101},
		{Sort: "sideways"},
		{SortBy: "size"},
		{Type: "blob"},
	}
	for _, input := range tests {
		if _, err := normalizeListOptions(input); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("normalizeListOptions(%#v) error = %v, want ErrInvalidInput", input, err)
		}
	}
}

func TestEscapeLike(t *testing.T) {
	if got, want := escapeLike(`a_b%\\c`), `a\_b\%\\\\c`; got != want {
		t.Fatalf("escapeLike() = %q, want %q", got, want)
	}
}

func TestValidateBlobMetadata(t *testing.T) {
	validSHA := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	tests := []struct {
		name string
		blob *models.Blob
		want bool
	}{
		{name: "valid", blob: &models.Blob{SHA256: validSHA, Size: 0}},
		{name: "nil", blob: nil, want: true},
		{name: "short hash", blob: &models.Blob{SHA256: "abc"}, want: true},
		{name: "uppercase hash", blob: &models.Blob{SHA256: strings.ToUpper(validSHA)}, want: true},
		{name: "negative size", blob: &models.Blob{SHA256: validSHA, Size: -1}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateBlobMetadata(tt.blob); (got != nil) != tt.want {
				t.Fatalf("validateBlobMetadata() error = %v, wantError=%t", got, tt.want)
			}
		})
	}
}

func TestTranslateError(t *testing.T) {
	if !errors.Is(translateError(gorm.ErrRecordNotFound), ErrNotFound) {
		t.Fatal("record-not-found was not translated")
	}
	if !errors.Is(translateError(gorm.ErrDuplicatedKey), ErrConflict) {
		t.Fatal("duplicate-key was not translated")
	}
	if !errors.Is(translateError(gorm.ErrForeignKeyViolated), ErrConflict) {
		t.Fatal("foreign-key violation was not translated")
	}
}
