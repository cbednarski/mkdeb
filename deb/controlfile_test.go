// This file contains tests for control file rendering. They are in a separate
// file because they are somewhat verbose.

package deb

import (
	"path"
	"testing"
)

func TestRenderControlFileBasic(t *testing.T) {
	p, err := NewPackageSpecFromFile(path.Join("test-fixtures", "example-basic.json"))
	if err != nil {
		t.Fatal(err)
	}
	p.Version = "0.1.0"

	expected := `Package: mkdeb
Version: 0.1.0
Architecture: amd64
Maintainer: Chris Bednarski <banzaimonkey@gmail.com>
Installed-Size: 0
Section: default
Priority: extra
Homepage: https://github.com/cbednarski/mkdeb
Description: A CLI tool for building debian packages
`
	buf, err := p.RenderControlFile()
	if err != nil {
		t.Fatal(err)
	}

	if string(buf) != expected {
		t.Fatalf("Control file did not match expected\n%s\n--Found--\n%s\n", expected, string(buf))
	}
}

func TestRenderControlFileWithDepends(t *testing.T) {
	p, err := NewPackageSpecFromFile(path.Join("test-fixtures", "example-depends.json"))
	if err != nil {
		t.Fatal(err)
	}

	p.Conflicts = []string{}
	p.Version = "0.1.0"

	expected := `Package: mkdeb
Version: 0.1.0
Architecture: amd64
Maintainer: Chris Bednarski <banzaimonkey@gmail.com>
Installed-Size: 0
Depends: wget, tree
Section: default
Priority: extra
Homepage: https://github.com/cbednarski/mkdeb
Description: A CLI tool for building debian packages
`
	buf, err := p.RenderControlFile()
	if err != nil {
		t.Fatal(err)
	}

	if string(buf) != expected {
		t.Fatalf("Control file did not match expected\n%s\n--Found--\n%s\n", expected, string(buf))
	}
}

func TestRenderControlFileWithPreDepends(t *testing.T) {
	p, err := NewPackageSpecFromFile(path.Join("test-fixtures", "example-predepends.json"))
	if err != nil {
		t.Fatal(err)
	}

	p.Conflicts = []string{}
	p.Version = "0.1.0"

	expected := `Package: mkdeb
Version: 0.1.0
Architecture: amd64
Maintainer: Chris Bednarski <banzaimonkey@gmail.com>
Installed-Size: 0
Pre-Depends: wget, tree
Section: default
Priority: extra
Homepage: https://github.com/cbednarski/mkdeb
Description: A CLI tool for building debian packages
`
	buf, err := p.RenderControlFile()
	if err != nil {
		t.Fatal(err)
	}

	if string(buf) != expected {
		t.Fatalf("Control file did not match expected\n%s\n--Found--\n%s\n", expected, string(buf))
	}
}

func TestRenderControlFileWithReplaces(t *testing.T) {
	p, err := NewPackageSpecFromFile(path.Join("test-fixtures", "example-replaces.json"))
	if err != nil {
		t.Fatal(err)
	}

	p.Replaces = []string{"debpkg"}
	p.Version = "0.1.0"

	expected := `Package: mkdeb
Version: 0.1.0
Architecture: amd64
Maintainer: Chris Bednarski <banzaimonkey@gmail.com>
Installed-Size: 0
Depends: wget, tree
Conflicts: debpkg
Replaces: debpkg
Section: default
Priority: extra
Homepage: https://github.com/cbednarski/mkdeb
Description: A CLI tool for building debian packages
`
	buf, err := p.RenderControlFile()
	if err != nil {
		t.Fatal(err)
	}

	if string(buf) != expected {
		t.Fatalf("Control file did not match expected\n%s\n--Found--\n%s\n", expected, string(buf))
	}
}
