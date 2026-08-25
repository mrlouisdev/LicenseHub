package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationChecksumCanonicalizesCRLF(t *testing.T) {
	lf := []byte("CREATE TABLE fixture (id integer);\n-- next\n")
	crlf := []byte("CREATE TABLE fixture (id integer);\r\n-- next\r\n")
	if checksumBytes(lf) != checksumBytes(crlf) {
		t.Fatal("LF and CRLF forms must have one canonical checksum")
	}
	changed := []byte("CREATE TABLE fixture (id bigint);\n-- next\n")
	if checksumBytes(lf) == checksumBytes(changed) {
		t.Fatal("a semantic SQL change must change the checksum")
	}
}

func TestLegacyMigrationChecksumRequiresExactCanonicalContent(t *testing.T) {
	data := []byte("-- fixture\n")
	canonical := checksumBytes(data)
	legacyMigrationChecksums["fixture.up.sql"] = struct {
		legacy    string
		canonical string
	}{"0123456789abcdef", canonical}
	t.Cleanup(func() { delete(legacyMigrationChecksums, "fixture.up.sql") })

	if !migrationChecksumMatches("fixture.up.sql", "0123456789abcdef", data) {
		t.Fatal("known legacy checksum and canonical content should match")
	}
	if migrationChecksumMatches("fixture.up.sql", "0123456789abcdef", []byte("-- changed\n")) {
		t.Fatal("legacy checksum must not permit changed migration content")
	}
}

func TestLegacyMigrationCanonicalManifestMatchesFiles(t *testing.T) {
	for name, known := range legacyMigrationChecksums {
		data, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if got := checksumBytes(data); got != known.canonical {
			t.Errorf("%s canonical checksum = %s, manifest = %s", name, got, known.canonical)
		}
	}
}
