// Package deb is used to build debian binary packages (.deb), based on the
// debian package specification. It does not handle source, source package, or
// changes.
//
// See https://www.debian.org/doc/debian-policy/ch-controlfields.html
// See https://www.debian.org/doc/manuals/debian-faq/ch-pkg_basics.en.html
package deb

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/blakesmith/ar"
)

const DebianBinary = "2.0\n"

// PackageSpec is parsed from JSON and
type PackageSpec struct {
	// Binary data
	Preinst  string            `json:"preinst,omitempty"`
	Postinst string            `json:"postinst,omitempty"`
	Prerm    string            `json:"prerm,omitempty"`
	Postrm   string            `json:"postrm,omitempty"`
	Files    map[string]string `json:"files,omitempty"`

	// If TempPath is specified we will build the archives there instead of in /tmp
	TempPath string `json:"tempath,omitempty"`

	// If PreserveSymlinks is true we will archive any symlinks instead of the
	// contents of the files they points to.
	PreserveSymlinks bool `json:"preservesymlinks,omitempty"`

	// If AutoPath is specified we will use the contents of that directory to build the deb
	AutoPath string `json:"autopath"`

	// Binary Debian Control File - Required fields
	Package      string `json:"package"`
	Version      string `json:"version"`
	Architecture string `json:"arch"`
	Maintainer   string `json:"maintainer"`
	Description  string `json:"description"`

	// Optional Fields
	Depends   []string `json:"depends,omitempty"`
	Conflicts []string `json:"conflicts,omitempty"`
	Breaks    []string `json:"breaks,omitempty"`
	Replaces  []string `json:"replaces,omitempty"`
	Section   string   `json:"section,omitempty"`
	Priority  string   `json:"priority,omitempty"`
	Homepage  string   `json:"homepage,omitempty"`

	// Derived fields
	InstalledSize int64 // Kilobytes, rounded up. Derived from file sizes.
}

func DefaultPackageSpec() *PackageSpec {
	return &PackageSpec{
		Section:  "default",
		Priority: "extra",
		AutoPath: "deb-pkg",
	}
}

func NewPackageSpecFromJson(data []byte) (*PackageSpec, error) {
	p := DefaultPackageSpec()
	err := json.Unmarshal(data, p)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func NewPackageSpecFromFile(filename string) (*PackageSpec, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	return NewPackageSpecFromJson(data)
}

func (p *PackageSpec) Validate() error {
	// Verify required fields are specified
	missing := []string{}
	if p.Package == "" {
		missing = append(missing, "package")
	}
	if p.Version == "" {
		missing = append(missing, "version")
	}
	if p.Architecture == "" {
		missing = append(missing, "architecture")
	}
	if p.Maintainer == "" {
		missing = append(missing, "maintainer")
	}
	if p.Description == "" {
		missing = append(missing, "description")
	}
	if len(missing) > 0 {
		return fmt.Errorf("These required fields are missing: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (p *PackageSpec) Filename() string {
	return fmt.Sprintf("%s-%s-%s.deb", p.Package, p.Version, p.Architecture)
}

// Build creates a .deb file in the target directory. The name is defived from
// Filename().
func (p *PackageSpec) Build(target string) error {
	err := p.Validate()
	if err != nil {
		return err
	}
	// 1. Create binary package (tar.gz format)
	// 2. Create control file package (tar.gz format)
	// 3. Create .deb / package (ar archive format)

	// debian-binary
	// control.tar.gz
	// data.tar.gz

	file, err := os.Create(target)
	if err != nil {
		return err
	}

	archive := ar.NewWriter(file)
	archive.WriteGlobalHeader()

	archiveCreationTime := time.Now()

	baseHeader := ar.Header{
		ModTime: archiveCreationTime,
		Uid:     0,
		Gid:     0,
		Mode:    644,
	}

	// Write the debian binary version (hard-coded to 2.0)
	if err := writeBytesToAr(archive, baseHeader, "debian-binary", []byte("2.0\n")); err != nil {
		return err
	}

	// Write the control files
	controlArchive, err := p.CreateControlArchive()
	if err != nil {
		return fmt.Errorf("Failed to compress control files: %s", err)
	}
	if err := writeFileToAr(archive, baseHeader, "control.tar.gz", controlArchive); err != nil {
		return err
	}

	// Write the data files
	dataArchive, err := p.CreateDataArchive()
	if err != nil {
		return fmt.Errorf("Failed to compress data files: %s", err)
	}
	if err := writeFileToAr(archive, baseHeader, "data.tar.gz", dataArchive); err != nil {
		return err
	}

	return nil
}

// RenderControlFile creates a debian control file for this package.
func (p *PackageSpec) RenderControlFile(wr io.Writer) error {
	t, err := template.New("controlfile").Funcs(template.FuncMap{"join": join}).Parse(ControlFileTemplate)
	if err != nil {
		panic(err)
	}
	return t.Execute(wr, p)
}

// ListFiles returns a list of files that will be included in the archive. This
// is a list of Path => Name pairs representing the file on disk and where we
// are going to store it in the archive.
func (p *PackageSpec) ListFiles() ([]string, error) {
	files := []string{}

	exclude := []string{
		"preinst",
		"postinst",
		"prerm",
		"postrm",
	}

	// First, grab all the files in AutoPath that are not control files
	if p.AutoPath != "" {
		if err := filepath.Walk(p.AutoPath, func(filepath string, info os.FileInfo, err error) error {
			// Skip directories
			if info.IsDir() {
				return nil
			}
			// Skip control files
			if hasString(exclude, path.Base(filepath)) {
				return nil
			}
			files = append(files, filepath)
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return files, nil
}

// List control files returns a list of control files for this package
func (p *PackageSpec) ListControlFiles() ([]string, error) {
	files := []string{}

	// This is ugly but means we don't have to use reflection

	if p.Preinst != "" {
		files = append(files, p.Preinst)
	} else if p.AutoPath != "" {
		filepath := path.Join(p.AutoPath, "preinst")
		if fileExists(filepath) {
			files = append(files, filepath)
		}
	}

	if p.Postinst != "" {
		files = append(files, p.Postinst)
	} else if p.AutoPath != "" {
		filepath := path.Join(p.AutoPath, "postinst")
		if fileExists(filepath) {
			files = append(files, filepath)
		}
	}

	if p.Prerm != "" {
		files = append(files, p.Prerm)
	} else if p.AutoPath != "" {
		filepath := path.Join(p.AutoPath, "prerm")
		if fileExists(filepath) {
			files = append(files, filepath)
		}
	}

	if p.Postrm != "" {
		files = append(files, p.Postrm)
	} else if p.AutoPath != "" {
		filepath := path.Join(p.AutoPath, "postrm")
		if fileExists(filepath) {
			files = append(files, filepath)
		}
	}

	return files, nil
}

// CalculateSize returns the size in Kilobytes of all files in the package.
func (p *PackageSpec) CalculateSize() (int64, error) {
	size := int64(0)

	files, err := p.ListFiles()
	if err != nil {
		return 0, err
	}

	controlFiles, err := p.ListControlFiles()
	if err != nil {
		return 0, err
	}

	// Merge list of control files and data files so we can get the whole size
	files = append(files, controlFiles...)

	for _, file := range files {
		var fileinfo os.FileInfo
		var err error = nil
		if p.PreserveSymlinks {
			fileinfo, err = os.Lstat(file)
		} else {
			fileinfo, err = os.Stat(file)
		}
		if err != nil {
			return 0, fmt.Errorf("Failed to stat %q: %s", file, err)
		}
		size += fileinfo.Size()
	}

	// Convert size from bytes to kilobytes. If there is a remainder, round up.
	if size%1024 > 0 {
		size = size/1024 + 1
	} else {
		size = size / 1024
	}

	return size, nil
}

// CalculateChecksums produces the contents of the md5sums file with the
// following format:
//
//	checksum  file1
//	checksum  file2
//
// All files in ListFiles are included
func (p *PackageSpec) CalculateChecksums() ([]byte, error) {
	data := []byte{}
	files, err := p.ListFiles()
	if err != nil {
		return data, err
	}

	for _, file := range files {
		if sum, err := md5SumFile(file); err != nil {
			return data, err
		} else {
			normFile, err := p.NormalizeFilename(file)
			if err != nil {
				return data, err
			}
			data = append(data, []byte(sum+"  "+normFile+"\n")...)
		}
	}

	return data, nil
}

// CreateDataArchive
func (p *PackageSpec) CreateDataArchive() (*os.File, error) {
	return nil, nil
}

// CreateControlTarball
func (p *PackageSpec) CreateControlArchive() (*os.File, error) {
	return nil, nil
}

// NormalizeFilename converts a local filename into a target archive filename
// by either using the PackageSpec.Files map or by stripping the AutoPath prefix
// from the file path. For example, deb-pkg/etc/blah will become ./etc/blah and
// a file mapped from config to /etc/config will become ./etc/config in the archive
func (p *PackageSpec) NormalizeFilename(filename string) (string, error) {
	if target, ok := p.Files[filename]; ok {
		return path.Join(".", target), nil
	}
	fpath, err := filepath.Rel(p.AutoPath, filename)
	if err != nil {
		return "", err
	}
	return path.Join(".", fpath), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hasString(items []string, search string) bool {
	for _, item := range items {
		if item == search {
			return true
		}
	}
	return false
}

func md5SumFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}

	hash := md5.New()
	_, err = io.Copy(hash, file)
	if err != nil {
		return "", err
	}

	sum := hash.Sum([]byte{})
	return hex.EncodeToString(sum), nil
}

func writeBytesToAr(archive *ar.Writer, header ar.Header, name string, data []byte) error {
	header.Name = name
	// This will cause data truncation on 32-bit go arch for files around 2gb.
	// In that case we can't do this in memory anyway so you should use
	// writeFileToAr() instead.
	header.Size = int64(len(data))
	if err := archive.WriteHeader(&header); err != nil {
		return fmt.Errorf("Failed writing header for %q: %s", name, err)
	}
	if _, err := archive.Write(data); err != nil {
		return fmt.Errorf("Failed writing data for %q: %s", name, err)
	}
	return nil
}

func writeFileToAr(archive *ar.Writer, header ar.Header, name string, file *os.File) error {
	header.Name = name
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("Failed to stat %q to write ar header size: %s", file.Name(), err)
	}

	header.Size = info.Size()
	if err := archive.WriteHeader(&header); err != nil {
		return fmt.Errorf("Failed writing ar header for %q: %s", name, err)
	}
	if _, err := io.Copy(archive, file); err != nil {
		return fmt.Errorf("Failed copying ar data for %q: %s", name, err)
	}
	return nil
}

func join(s []string) string {
	return strings.Join(s, " ")
}

const ControlFileTemplate = `Package: {{ .Package }}
Version: {{ .Version }}
Architecture: {{ .Architecture}}
Maintainer: {{ .Maintainer }}
Installed-Size: {{ .InstalledSize }}
{{- if (len .Depends) gt 0 }}
Depends: {{ join .Depends }}
{{- end -}}
{{- if (len .Conflicts) gt 0 }}
Conflicts: {{ join .Conflicts }}
{{- end -}}
{{- if (len .Breaks) gt 0 }}
Breaks: {{ join .Breaks }}
{{- end -}}
{{- if (len .Replaces) gt 0 }}
Replaces: {{ join .Replaces }}
{{- end }}
Section: {{ .Section }}
Priority: {{ .Priority }}
Homepage: {{ .Homepage }}
Description: {{ .Description }}
`
