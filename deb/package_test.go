package deb

import (
	"path"
	"testing"
)

func TestDefaultPackageSpec(t *testing.T) {
	p := DefaultPackageSpec()
	expected := "deb-pkg"
	if p.AutoPath != expected {
		t.Fatalf("Expected AutoPath to be %q, got %q", expected, p.AutoPath)
	}
}

func TestFilename(t *testing.T) {
	p := &PackageSpec{
		Package:      "mkdeb",
		Version:      "0.1.0",
		Architecture: "amd64",
	}
	expected := "mkdeb-0.1.0-amd64.deb"
	if p.Filename() != expected {
		t.Fatalf("Expected filename to be %q, got %q", expected, p.Filename)
	}
}

func TestValidate(t *testing.T) {
	p, err := NewPackageSpecFromFile(path.Join("test-fixtures", "example-basic.json"))
	if err != nil {
		t.Fatalf("Failed to load fixture: %s", err)
	}

	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}

	p2 := &PackageSpec{}
	err = p2.Validate()
	expected := "These required fields are missing: package, version, architecture, maintainer, description"
	if err.Error() != expected {
		t.Fatalf("-- Expected --\n%s\n-- Found --\n%s\n", expected, err.Error())
	}
}

func TestListControlFiles(t *testing.T) {
	p, err := NewPackageSpecFromFile(path.Join("test-fixtures", "example-basic.json"))
	p.AutoPath = path.Join("test-fixtures", "package1")
	if err != nil {
		t.Fatalf("Failed to load fixture: %s", err)
	}

	files, err := p.ListControlFiles()
	if err != nil {
		t.Fatalf("Failed to list control files: %s", err)
	}

	search := "test-fixtures/package1/preinst"
	if !hasString(files, search) {
		t.Errorf("Unable to find %q in %+v", search, files)
	}
}

func TestListFiles(t *testing.T) {
	p, err := NewPackageSpecFromFile(path.Join("test-fixtures", "example-basic.json"))
	p.AutoPath = path.Join("test-fixtures", "package1")
	if err != nil {
		t.Fatalf("Failed to load fixture: %s", err)
	}

	files, err := p.ListFiles()
	if err != nil {
		t.Fatal(err)
	}

	configPath := path.Join("test-fixtures", "package1", "etc", "package1", "config")
	if !hasString(files, configPath) {
		t.Errorf("%q is missing: %+v", configPath, files)
	}

	binaryPath := path.Join("test-fixtures", "package1", "usr", "local", "bin", "package1")
	if !hasString(files, binaryPath) {
		t.Errorf("%q is missing: %+v", binaryPath, files)
	}
}

func TestCalculateSize(t *testing.T) {
	// find deb/test-fixtures/package1/ | xargs cat 2>/dev/null | wc -c
	// divide by 1024 to go from bytes => kilobytes
	p, err := NewPackageSpecFromFile(path.Join("test-fixtures", "example-basic.json"))
	p.AutoPath = path.Join("test-fixtures", "package1")
	if err != nil {
		t.Fatalf("Failed to load fixture: %s", err)
	}

	expected := int64(1)
	size, err := p.CalculateSize()
	if err != nil {
		t.Fatal(err)
	}
	if size != expected {
		t.Errorf("Expected %d got %d", expected, size)
	}
}
