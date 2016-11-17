// Package deb is used to build debian binary packages (.deb), based on the
// debian package specification. It does not handle source, source package, or
// changes.
//
// See https://www.debian.org/doc/debian-policy/ch-controlfields.html
package deb

import (
	"encoding/json"
	"fmt"
	"io"
	"path"

	"github.com/mitchellh/packer/template"
)

type Package struct {
	PackageSpec *PackageSpec
	Control     []byte // control.tar.gz
}

type PackageSpec struct {
	// Binary data
	Preinst   string
	Postinst  string
	Prerm     string
	Postrm    string
	DataFiles []string

	// Build configuration
	PkgPath string // pkg path is where we'll look for binaries
	EtcPath string // etc path is where we'll look for configs

	// Binary Debian Control File - Required fields
	Package      string
	Version      string
	Architecture string
	Maintainer   string
	Description  string

	//
	InstalledSize int // Kilobytes, rounded up. Derived from file sizes.
	Depends       []string
	Conflicts     []string
	Breaks        []string
	Replaces      []string
	Section       string
	Priority      string
	Homepage      string
}

func DefaultPackageSpec() *PackageSpec {
	return &PackageSpec{
		Section:  "default",
		Priority: "extra",
		EtcPath:  path.Join("pkg", "etc"),
		PkgPath:  "pkg",
	}
}

func NewPackageSpecFromJson(data []bytes) (*PackageSpec, error) {
	// Default
	p := DefaultPackageSpec()
	err := json.Unmarshal(data, p)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (p *PackageSpec) Validate() err {
	// Verify required fields are specified
	if p.Package == "" {
		return fmt.Errorf("Package must be specified")
	}
	if p.Version == "" {
		return fmt.Errorf("Version must be specified")
	}
	if p.Architecture == "" {
		return fmt.Errorf("Architecture must be specified")
	}
	if p.Maintainer == "" {
		return fmt.Errorf("Maintainer must be specified")
	}
	if p.Description == "" {
		return fmt.Errorf("Description must be specified")
	}
}

func (p *PackageSpec) Filename() string {
	return fmt.Sprintf("%s-%s-%s.deb", p.Package, p.Version, p.Architecture)
}

func (p *PackageSpec) Build(target string) error {
	err := p.Validate()
	if err != nil {
		return err
	}
}

func (p *PackageSpec) RenderControlFile(wr io.Writer) error {
	t, err := template.Parse(ControlFileTemplate)
	if err != nil {
		// If this fails at runtime then the template is broken. This is a
		// developer error, not a user error.
		return panic(err)
	}
	return t.Execute(wr, p)
}

const ControlFileTemplate = `Package: {{ .Package }}
Version: {{ .Version }}
Architecture: {{ .Architecture}}
Maintainer: {{ .Maintainer }}
Installed-Size: {{ .InstalledSize }}
Depends: {{ .Depends }}
Conflicts: {{ .Conflicts }}
Breaks: {{ .Breaks }}
Replaces: {{ .Replaces }}
Section: {{ .Section }}
Priority: {{ .Priority }}
Homepage: {{ .Homepage }}
Description: {{ .Description }}
`
