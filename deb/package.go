// Package deb is used to build debian binary packages (.deb), based on the
// debian package specification. It does not handle source, source package, or
// changes.
//
// The bulk of the configuration options and functionalty are associated with
// PackageSpec. Refer to that section for more details.
//
// References
//
// https://www.debian.org/doc/debian-policy/ch-controlfields.html
//
// https://www.debian.org/doc/manuals/debian-faq/ch-pkg_basics.en.html
package deb

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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

// PackageSpec is parsed from JSON and initializes both build time parameters
// and the metadata inside the .deb package.
//
// Required Fields
//
// The following fields are required by the debian package specification:
//
// Package is the name of your package, and typically matches the name of your
// main program.
//
// Version is a debian version string. See the reference for more details.
// This field is not currently validated except to verify that it is specified,
// but if the syntax is invalid you will not be able to install the package.
//
// Architecture is the CPU architecture your package is compiled for. If your
// package does not include a compiled binary you can set this to "all".
//
// Maintainer should indicate contact information for the package, such as
// Chris Bednarski <chris@example.com>
//
// Description should briefly explain what your package is used for. Only a
// single line is currently supported.
//
// Optional Fields
//
// Depends is used to specify whether your package depends on other packages.
// Dependencies should be specified using the following syntax
//
//	"depends": [
//	    "curl (>= 7.0.0)",
//	    "python (= 2.7.12)",
//	    "tree"
//	]
//
// Conflicts, Breaks, and Replaces work in a very similar way. For additional
// information on when you should use optional fields and how to specify them,
// refer to the debian package specification.
//
// Homepage should link to your package's source repository, if applicable.
// Otherwise link to your website.
//
// Control Scripts
//
// You may need to perform additional setup (or cleanup) when (un)installing a
// package. You can do this through the control scripts: preinst, postinst,
// prerm, and postrm.
//
// These are commonly used to create users, start or stop services, or perform
// cleanup when a package is uninstalled.
//
// AutoPath
//
// The Build method is designed to automatically fill in most of the build
// configuration based on files it finds on the filesystem. If AutoPath is set
// to a non-empty value it will be scanned for pre/post/inst/rm scripts as well
// as configuration files and binaries to be automatically included in the .deb.
//
// To disable the automatic behavior set AutoPath to an empty string or dash "-".
// Whether or not AutoPath is used you may supplement the list of files to be
// included by specifying the Files field.
//
// Build Time Options
//
// TempPath controls where intermediate files are written during the build. This
// defaults to the system temp directory (usually /tmp).
//
// UpgradeConfigs causes a package upgrade to replace all of the config files.
// By default files under /etc are left as-is when upgrading a package so you
// can keep changes made to your config files, but if you want to upgrade the
// config files themselves you will need to set UpgradeConfigs to true.
//
// PreserveSymlinks writes symlinks to the archive. By default the contents of
// the file the symlink is pointing to is copied into the .deb package.
//
// Derived Fields
//
// InstalledSize is calculated based on the total size of your files and control
// scripts. You should not specify this yourself.
//
// For details on how to use pre/post/inst/rm and various .deb-specific fields
// please refere to the debian package specification:
//
// https://www.debian.org/doc/debian-policy/ch-controlfields.html
//
// https://www.debian.org/doc/manuals/debian-faq/ch-pkg_basics.en.html
type PackageSpec struct {
	// Binary Debian Control File - Required fields
	Package      string
	Version      string `json:"-"`
	Architecture string
	Maintainer   string
	Description  string

	// Optional Fields
	Depends   []string
	Conflicts []string `json:",omitempty"`
	Breaks    []string `json:",omitempty"`
	Replaces  []string `json:",omitempty"`
	Section   string   // Defaults to "default"
	Priority  string   // Defaults to "extra"
	Homepage  string

	// Control Scripts
	Preinst  string
	Postinst string
	Prerm    string
	Postrm   string

	// Build time options
	AutoPath         string // Defaults to "deb-pkg"
	Files            map[string]string
	TempPath         string `json:",omitempty"`
	PreserveSymlinks bool   `json:",omitempty"`
	UpgradeConfigs   bool   `json:",omitempty"`

	// Derived fields
	InstalledSize int64 `json:"-"` // Kilobytes, rounded up. Derived from file sizes.
}

// DefaultPackageSpec includes default values for package specifications. This
// simplifies configuration so a user need only specify required fields to build
func DefaultPackageSpec() *PackageSpec {
	return &PackageSpec{
		Section:   "default",
		Priority:  "extra",
		AutoPath:  "deb-pkg",
		Depends:   make([]string, 0),
		Conflicts: make([]string, 0),
		Breaks:    make([]string, 0),
		Replaces:  make([]string, 0),
		Files:     make(map[string]string, 0),
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
// Filename() so you can find it with:
//
//	path.Join(target, PackageSpec.Filename())
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

	err = os.MkdirAll(target, 0755)
	if err != nil {
		return fmt.Errorf("Unable to create target directory %q: %s", target, err)
	}

	file, err := os.Create(path.Join(target, p.Filename()))
	if err != nil {
		return fmt.Errorf("Failed to create build target: %s", err)
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
		return fmt.Errorf("Failed to write debian-binary: %s", err)
	}

	if err := p.CreateControlArchive("control.tar.gz"); err != nil {
		return fmt.Errorf("Failed to compress control files: %s", err)
	}
	defer os.Remove("control.tar.gz")
	// Copy the control file archive into ar (.deb)
	if err := writeFileToAr(archive, baseHeader, "control.tar.gz", "control.tar.gz"); err != nil {
		return err
	}

	if err := p.CreateDataArchive("data.tar.gz"); err != nil {
		return fmt.Errorf("Failed to compress data files: %s", err)
	}
	defer os.Remove("data.tar.gz")
	// Copy the data archive into the ar (.deb)
	if err := writeFileToAr(archive, baseHeader, "data.tar.gz", "data.tar.gz"); err != nil {
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
	if p.AutoPath != "" && p.AutoPath != "-" && FileExists(p.AutoPath) {
		if err := filepath.Walk(p.AutoPath, func(filepath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
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
	} else if p.AutoPath != "" && p.AutoPath != "-" {
		filepath := path.Join(p.AutoPath, "preinst")
		if FileExists(filepath) {
			files = append(files, filepath)
		}
	}

	if p.Postinst != "" {
		files = append(files, p.Postinst)
	} else if p.AutoPath != "" && p.AutoPath != "-" {
		filepath := path.Join(p.AutoPath, "postinst")
		if FileExists(filepath) {
			files = append(files, filepath)
		}
	}

	if p.Prerm != "" {
		files = append(files, p.Prerm)
	} else if p.AutoPath != "" && p.AutoPath != "-" {
		filepath := path.Join(p.AutoPath, "prerm")
		if FileExists(filepath) {
			files = append(files, filepath)
		}
	}

	if p.Postrm != "" {
		files = append(files, p.Postrm)
	} else if p.AutoPath != "" && p.AutoPath != "-" {
		filepath := path.Join(p.AutoPath, "postrm")
		if FileExists(filepath) {
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
// All files returned by ListFiles() are included
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
func (p *PackageSpec) CreateDataArchive(target string) error {
	file, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("Failed to create data archive %q: %s", target, err)
	}
	defer file.Close()

	// Create a compressed archive stream
	zipwriter := pgzip.NewWriter(file)
	defer zipwriter.Close()
	archive := tar.NewWriter(zipwriter)
	defer archive.Close()

	header := tar.Header{
		Uid:   0,
		Gid:   0,
		Uname: "root",
		Gname: "root",
	}

	files, err := p.ListFiles()
	if err != nil {
		return err
	}
	for _, filename := range files {
		dataFile, err := os.Open(filename)
		if err != nil {
			return err
		}
		defer dataFile.Close()

		info, err := dataFile.Stat()
		if err != nil {
			return err
		}

		target, err := p.NormalizeFilename(filename)
		if err != nil {
			return err
		}

		fileHeader := header
		fileHeader.Name = target
		fileHeader.Size = info.Size()
		fileHeader.Mode = 755
		// fileHeader.Mode = int64(info.Mode().Perm())
		fileHeader.ModTime = info.ModTime()
		archive.WriteHeader(&fileHeader)
		_, err = io.Copy(archive, dataFile)
		if err != nil {
			return err
		}
	}
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
func (p *PackageSpec) CreateControlArchive(target string) error {
	file, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("Failed to create control archive %q: %s", target, err)
	}
	defer file.Close()

	// Create a compressed archive stream
	zipwriter := gzip.NewWriter(file)
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
		scriptHeader.Mode = 755
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
	if p.AutoPath != "" && p.AutoPath != "-" {
		fpath, err := filepath.Rel(p.AutoPath, filename)
		if err != nil {
			return "", err
		}
		return path.Join(".", fpath), nil
	}
	return "", fmt.Errorf("Not sure what to do with %q because it is not specified in files and autopath is disabled", filename)
}

// FileExists returns true if the specified file/dir exists and we can stat it
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// SupportedArchitectures lists the architectures that are accepted by the validator
func SupportedArchitectures() []string {
	return supportedArchitectures
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

func writeFileToAr(archive *ar.Writer, header ar.Header, source string, dest string) error {
	header.Name = source
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("Failed to stat %q to write ar header size: %s", file.Name(), err)
	}

	header.Size = info.Size()
	if err := archive.WriteHeader(&header); err != nil {
		return fmt.Errorf("Failed writing ar header for %q: %s", source, err)
	}
	if _, err := io.Copy(archive, file); err != nil {
		return fmt.Errorf("Failed copying ar data for %q: %s", source, err)
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
