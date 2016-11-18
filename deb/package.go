// Package deb is used to build debian binary packages (.deb), based on the
// debian package specification. It does not handle source, source package, or
// changes.
//
// See https://www.debian.org/doc/debian-policy/ch-controlfields.html
// See https://www.debian.org/doc/manuals/debian-faq/ch-pkg_basics.en.html
package deb

import (
	"archive/tar"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/blakesmith/ar"
	"github.com/klauspost/pgzip"
)

const (
	debianBinary = "2.0\n"
)

var (
	reDepends     = regexp.MustCompile(`^[a-zA-Z0-9_-]+( \((>|>=|<|<=|=) ([0-9][0-9a-zA-Z.-]*?)\))?$`)
	reReplacesEtc = regexp.MustCompile(`^[a-zA-Z0-9_-]+( \(<< ([0-9][0-9a-zA-Z.-]*?)\))?$`)

	controlFiles = []string{
		"preinst",
		"postinst",
		"prerm",
		"postrm",
	}

	supportedArchitectures = []string{
		"all", // This is used for non-binary packages
		"amd64",
		"arm64",
		"armel",
		"armhf",
		"i386",
		"mips",
		"mipsel",
		"powerpc",
		"ppc64el",
		"s390x",
	}
)

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
	PreserveSymlinks bool `json:"preserveSymlinks,omitempty"`

	// If UpgradeConfigs is true we will exclude /etc from conffiles, allowing
	// the package to update config files when it is upgraded
	UpgradeConfigs bool `json:"upgradeConfigs,omitempty"`

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

// DefaultPackageSpec includes default values for package specifications. This
// simplifies configuration so a user need only specify required fields to build
func DefaultPackageSpec() *PackageSpec {
	return &PackageSpec{
		Section:  "default",
		Priority: "extra",
		AutoPath: "deb-pkg",
	}
}

// NewPackageSpecFromJSON creates a PackageSpec from JSON data
func NewPackageSpecFromJSON(data []byte) (*PackageSpec, error) {
	p := DefaultPackageSpec()
	err := json.Unmarshal(data, p)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// NewPackageSpecFromFile creates a PackageSpec from a JSON file
func NewPackageSpecFromFile(filename string) (*PackageSpec, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	return NewPackageSpecFromJSON(data)
}

// Validate checks the syntax of various text fields in PackageSpec to verify
// that they conform to the debian package specification. Errors from this call
// should be passed to the user so they can fix errors in their config file.
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
	if !hasString(supportedArchitectures, p.Architecture) {
		return fmt.Errorf("Arch %q is not supported; expected one of %s", p.Architecture, strings.Join(supportedArchitectures, ", "))
	}
	for _, dep := range p.Depends {
		if !reDepends.MatchString(dep) {
			return fmt.Errorf("Dependency %q is invalid; expected something like 'libc (= 5.1.2)' matching %q", dep, reDepends.String())
		}
	}
	for _, replace := range p.Replaces {
		if !reReplacesEtc.MatchString(replace) {
			return fmt.Errorf("Replacement %q is invalid; expected something like 'libc (<< 5.1.2)' matching %q", replace, reReplacesEtc.String())
		}
	}
	for _, conflict := range p.Conflicts {
		if !reReplacesEtc.MatchString(conflict) {
			return fmt.Errorf("Conflict %q is invalid; expected something like 'libc (<< 5.1.2)' matching %q", conflict, reReplacesEtc.String())
		}
	}
	for _, breaks := range p.Breaks {
		if !reReplacesEtc.MatchString(breaks) {
			return fmt.Errorf("Break %q is invalid; expected something like 'libc (<< 5.1.2)' matching %q", breaks, reReplacesEtc.String())
		}
	}
	return nil
}

// Filename derives the standard debian filename as package-version-arch.deb
// based on the data specified in PackageSpec.
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

	// Create the control file archive
	controlArchive, err := ioutil.TempFile(p.TempPath, "control")
	if err != nil {
		return fmt.Errorf("Failed creating temp file: %s", err)
	}
	defer file.Close()
	// Write control files to it
	if err := p.CreateControlArchive(controlArchive); err != nil {
		return fmt.Errorf("Failed to compress control files: %s", err)
	}
	// Reset the cursor so we can io.Copy from the beginning of the file
	if _, err := controlArchive.Seek(0, 0); err != nil {
		return err
	}
	// Copy the control file archive into ar (.deb)
	if err := writeFileToAr(archive, baseHeader, "control.tar.gz", controlArchive); err != nil {
		return err
	}

	// Create the data file archive
	dataArchive, err := ioutil.TempFile(p.TempPath, "control")
	if err != nil {
		return fmt.Errorf("Failed creating temp file: %s", err)
	}
	defer file.Close()
	// Write data files to it
	if err := p.CreateDataArchive(dataArchive); err != nil {
		return fmt.Errorf("Failed to compress data files: %s", err)
	}
	// Reset the cursor so we can io.Copy from the beginning of the file
	if _, err := dataArchive.Seek(0, 0); err != nil {
		return err
	}
	// Copy the data archive into the ar (.deb)
	if err := writeFileToAr(archive, baseHeader, "data.tar.gz", dataArchive); err != nil {
		return err
	}

	return nil
}

// RenderControlFile creates a debian control file for this package.
func (p *PackageSpec) RenderControlFile() ([]byte, error) {
	t, err := template.New("controlfile").Funcs(template.FuncMap{"join": join}).Parse(controlFileTemplate)
	if err != nil {
		// This should only happen if the template itself is messed up, which
		// means the code has an error (not a user error)
		panic(err)
	}
	buf := &bytes.Buffer{}
	err = t.Execute(buf, p)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ListFiles returns a list of files that will be included in the archive. This
// is a list of Path => Name pairs representing the file on disk and where we
// are going to store it in the archive.
func (p *PackageSpec) ListFiles() ([]string, error) {
	files := []string{}

	// First, grab all the files in AutoPath that are not control files
	if p.AutoPath != "" {
		if err := filepath.Walk(p.AutoPath, func(filepath string, info os.FileInfo, err error) error {
			// Skip directories
			if info.IsDir() {
				return nil
			}
			// Skip control files
			if hasString(controlFiles, path.Base(filepath)) {
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

// ListEtcFiles lists all of the configuration files that are packaged under /etc
// in the archive so they can be added to conffiles. These will be normalized
// to include a leading /
func (p *PackageSpec) ListEtcFiles() ([]string, error) {
	etcFiles := []string{}

	// If UpgradeConfigs is set we'll return an empty list. This prevents the
	// config files from receiving special treatment during package upgrades and
	// updates them like regular files.
	if p.UpgradeConfigs {
		return etcFiles, nil
	}

	files, err := p.ListFiles()
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		normFile, err := p.NormalizeFilename(file)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(normFile, "etc") {
			etcFiles = append(etcFiles, "/"+normFile)
		}
	}
	return etcFiles, nil
}

// ListControlFiles returns a list of optional control scripts including
// pre/post/inst/rm that are used in this package.
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
		var err error
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
		sum, err := md5SumFile(file)
		if err != nil {
			return data, err
		}
		normFile, err := p.NormalizeFilename(file)
		if err != nil {
			return data, err
		}
		data = append(data, []byte(sum+"  "+normFile+"\n")...)
	}

	return data, nil
}

// CreateDataArchive creates
func (p *PackageSpec) CreateDataArchive(file *os.File) error {
	return nil
}

// CreateControlArchive creates the control.tar.gz part of the .deb package
// This includes:
//
//	conffiles
//	md5sums
//	control
//	pre/post/inst/rm scripts (if any)
//
// You must pass in a file handle that is open for writing.
func (p *PackageSpec) CreateControlArchive(file *os.File) error {
	// Create a compressed archive stream
	zipwriter := pgzip.NewWriter(file)
	defer zipwriter.Close()
	archive := tar.NewWriter(zipwriter)
	defer archive.Close()

	header := tar.Header{
		Mode:    644,
		Uid:     0,
		Gid:     0,
		ModTime: time.Now(),
		Uname:   "root",
		Gname:   "root",
	}

	// Add md5sums
	sumData, err := p.CalculateChecksums()
	if err != nil {
		return err
	}
	sumHeader := header
	sumHeader.Name = "md5sums"
	sumHeader.Size = int64(len(sumData))
	archive.WriteHeader(&sumHeader)
	archive.Write(sumData)

	// Add conffiles
	confFiles, err := p.ListEtcFiles()
	if err != nil {
		return err
	}
	confData := []byte(strings.Join(confFiles, "\n") + "\n")
	confHeader := header
	confHeader.Name = "conffiles"
	confHeader.Size = int64(len(confData))
	archive.WriteHeader(&confHeader)
	archive.Write(confData)

	// Add control file
	controlData, err := p.RenderControlFile()
	if err != nil {
		return err
	}
	controlHeader := header
	controlHeader.Name = "control"
	controlHeader.Size = int64(len(controlData))
	archive.WriteHeader(&controlHeader)
	archive.Write(controlData)

	// Add control scripts
	scripts, err := p.ListControlFiles()
	if err != nil {
		return err
	}
	for _, script := range scripts {
		scriptData, err := ioutil.ReadFile(script)
		if err != nil {
			return fmt.Errorf("Failed reading script %q: %s", script, err)
		}

		scriptHeader := header
		scriptHeader.Name, err = p.NormalizeFilename(script)
		if err != nil {
			return err
		}
		scriptHeader.Size = int64(len(scriptData))
		archive.WriteHeader(&scriptHeader)
		archive.Write(scriptData)
	}

	return nil
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
	return strings.Join(s, ", ")
}

const controlFileTemplate = `Package: {{ .Package }}
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
