package deb

import (
	"os"
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
		t.Fatalf("Expected filename to be %q, got %q", expected, p.Filename())
	}
}

func TestValidate(t *testing.T) {
	p, err := NewPackageSpecFromFile(path.Join("test-fixtures", "example-basic.json"))
	if err != nil {
		t.Fatalf("Failed to load fixture: %s", err)
	}
	p.Version = "0.1.0"

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
	if err != nil {
		t.Fatalf("Failed to load fixture: %s", err)
	}
	p.AutoPath = path.Join("test-fixtures", "package1")

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
	if err != nil {
		t.Fatalf("Failed to load fixture: %s", err)
	}
	p.AutoPath = path.Join("test-fixtures", "package1")

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
	p, err := NewPackageSpecFromFile(path.Join("test-fixtures", "example-basic.json"))
	if err != nil {
		t.Fatalf("Failed to load fixture: %s", err)
	}
	p.AutoPath = path.Join("test-fixtures", "package1")

	// find deb/test-fixtures/package1/ | xargs cat 2>/dev/null | wc -c
	// divide by 1024 and round up remainder to go from bytes => kilobytes
	expected := int64(1)

	size, err := p.CalculateSize()
	if err != nil {
		t.Fatal(err)
	}
	if size != expected {
		t.Errorf("Expected %d got %d", expected, size)
	}
}

func TestNormalizeFilename(t *testing.T) {
	p, err := NewPackageSpecFromFile(path.Join("test-fixtures", "example-basic.json"))
	if err != nil {
		t.Fatalf("Failed to load fixture: %s", err)
	}
	p.AutoPath = path.Join("test-fixtures", "package1")

	configPath := path.Join("test-fixtures", "package1", "etc", "package1", "config")
	configExpected := "etc/package1/config"
	if filename, err := p.NormalizeFilename(configPath); err != nil {
		t.Fatal()
	} else if filename != configExpected {
		t.Errorf("Expected %q got %q", configExpected, filename)
	}

	hardcodedPath := "package1/binary"
	hardcodedExpected := "usr/local/bin/package1"
	if filename, err := p.NormalizeFilename(hardcodedPath); err != nil {
		t.Fatal()
	} else if filename != hardcodedExpected {
		t.Errorf("Expected %q got %q", hardcodedExpected, filename)
	}
}

func TestListEtcFiles(t *testing.T) {
	p, err := NewPackageSpecFromFile(path.Join("test-fixtures", "example-basic.json"))
	if err != nil {
		t.Fatalf("Failed to load fixture: %s", err)
	}
	p.AutoPath = path.Join("test-fixtures", "package1")

	files, err := p.ListEtcFiles()
	if err != nil {
		t.Fatal(err)
	}

	if len(files) == 0 {
		t.Fatalf("No config files found")
	}

	expected := "/etc/package1/config"
	if files[0] != expected {
		t.Errorf("Expected %q got %q", expected, files[0])
	}
}

func TestUpgradeConfig(t *testing.T) {
	p, err := NewPackageSpecFromFile(path.Join("test-fixtures", "example-basic.json"))
	if err != nil {
		t.Fatalf("Failed to load fixture: %s", err)
	}
	p.AutoPath = path.Join("test-fixtures", "package1")
	p.UpgradeConfigs = true

	data, err := p.ListEtcFiles()
	if err != nil {
		t.Fatal(err)
	}

	if len(data) != 0 {
		t.Errorf("Found unexpected config files in conffiles list: %+v", data)
	}
}

func TestMD5SumFile(t *testing.T) {
	sum, err := md5SumFile(path.Join("test-fixtures", "example-depends.json"))
	if err != nil {
		t.Fatal(err)
	}

	expected := "e8f79d2b1995292911388a9d13a9528d"
	if sum != expected {
		t.Errorf("Expected %q got %q", expected, sum)
	}
}

func TestCalculateChecksums(t *testing.T) {
	p, err := NewPackageSpecFromFile(path.Join("test-fixtures", "example-basic.json"))
	if err != nil {
		t.Fatalf("Failed to load fixture: %s", err)
	}
	p.AutoPath = path.Join("test-fixtures", "package1")

	expected := `adcc07f30ee844b18eab61f69f8c32c4  etc/package1/config
0940b4d946e3e2b8bbfdf5cfcf722518  usr/local/bin/package1
`

	data, err := p.CalculateChecksums()
	if err != nil {
		t.Fatal(err)
	}

	found := string(data)
	if found != expected {
		t.Errorf("--Expected--\n%s\n--Found--\n%s\n", expected, found)
	}
}

func TestCreateDataArchive(t *testing.T) {
	p, err := NewPackageSpecFromFile(path.Join("test-fixtures", "example-basic.json"))
	if err != nil {
		t.Fatalf("Failed to load fixture: %s", err)
	}
	p.AutoPath = path.Join("test-fixtures", "package1")
	p.TempPath = "test-fixtures"

	filename := "test-data.tar.gz"
	if err := p.CreateDataArchive(filename); err != nil {
		t.Fatal(err)
	}
	os.Remove(filename)
}

func TestCreateControlArchive(t *testing.T) {
	p, err := NewPackageSpecFromFile(path.Join("test-fixtures", "example-basic.json"))
	if err != nil {
		t.Fatalf("Failed to load fixture: %s", err)
	}
	p.AutoPath = path.Join("test-fixtures", "package1")
	p.TempPath = "test-fixtures"

	filename := "test-control.tar.gz"
	if err := p.CreateControlArchive(filename); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(filename)
}

func TestBuild(t *testing.T) {
	p, err := NewPackageSpecFromFile(path.Join("test-fixtures", "example-basic.json"))
	if err != nil {
		t.Fatalf("Failed to load fixture: %s", err)
	}
	p.AutoPath = path.Join("test-fixtures", "package1")
	p.Version = "0.1.0"

	err = p.Build("output")
	defer os.Remove(path.Join("output", p.Filename()))
	if err != nil {
		t.Fatal(err)
	}

}
