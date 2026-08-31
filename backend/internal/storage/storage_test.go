package storage

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestNewAssociationRecord(t *testing.T) {
	digest := strings.Repeat("a", 64)
	record, err := NewAssociationRecord(digest, []string{"docs/z", "a", "docs/z", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if record.Version != 1 || record.SHA256 != digest {
		t.Fatalf("unexpected record: %#v", record)
	}
	if want := []string{"a", "b", "docs/z"}; !reflect.DeepEqual(record.Paths, want) {
		t.Fatalf("paths = %q, want %q", record.Paths, want)
	}
}

func TestNewAssociationRecordRejectsInvalidInput(t *testing.T) {
	validDigest := strings.Repeat("a", 64)
	tests := []struct {
		name   string
		digest string
		paths  []string
		want   error
	}{
		{name: "digest", digest: "invalid", paths: []string{"file"}, want: ErrInvalidSHA256},
		{name: "empty path", digest: validDigest, paths: []string{""}, want: ErrInvalidPath},
		{name: "absolute", digest: validDigest, paths: []string{"/file"}, want: ErrInvalidPath},
		{name: "backslash", digest: validDigest, paths: []string{`dir\file`}, want: ErrInvalidPath},
		{name: "single dot", digest: validDigest, paths: []string{"."}, want: ErrInvalidPath},
		{name: "single dot dot", digest: validDigest, paths: []string{".."}, want: ErrInvalidPath},
		{name: "dot", digest: validDigest, paths: []string{"dir/./file"}, want: ErrInvalidPath},
		{name: "dot dot", digest: validDigest, paths: []string{"dir/../file"}, want: ErrInvalidPath},
		{name: "duplicate separator", digest: validDigest, paths: []string{"dir//file"}, want: ErrInvalidPath},
		{name: "trailing separator", digest: validDigest, paths: []string{"dir/"}, want: ErrInvalidPath},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewAssociationRecord(test.digest, test.paths)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestNewAssociationRecordAllowsNoPaths(t *testing.T) {
	record, err := NewAssociationRecord(strings.Repeat("a", 64), nil)
	if err != nil {
		t.Fatal(err)
	}
	if record.Paths == nil || len(record.Paths) != 0 {
		t.Fatalf("paths = %#v, want non-nil empty slice", record.Paths)
	}
}
