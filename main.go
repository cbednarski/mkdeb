package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cbednarski/mkdeb/deb"
)

func main() {
	version := flag.String("version", "1.0", "Package version")
	flag.Parse()
	args := flag.Args()

	if len(args) == 0 {
		showUsage()
	}

	switch args[0] {
	case "archs":
		showArchs()
	case "build":
		build(checkConfig(args), *version)
	case "init":
		initialize()
	case "validate":
		validate(checkConfig(args))
	default:
		showUsage()
	}
	os.Exit(0)
}

func checkConfig(args []string) string {
	if len(args) < 2 {
		fmt.Printf("Missing config file\n")
		os.Exit(1)
	}
	return args[len(args)-1]
}

// getTarget takes a relative filename and returns its absolute directory and filename.
func getTarget(filename string) (string, string) {
	path, err := filepath.Abs(filename)
	if err != nil {
		fmt.Printf("Can't find %q", filename)
		os.Exit(1)
	}
	dir, _ := filepath.Split(path)
	return dir, path
}

func showArchs() {
	fmt.Printf("mkdeb supported architectures: %s\n", strings.Join(deb.SupportedArchitectures(), ", "))
}

// initialize creates a new mkdeb config. This function is not called init()
// because that has a special meaning in Go.
func initialize() {
	// Get abs path to PWD
	workdir, err := os.Getwd()
	handleError(err)
	workdir, err = filepath.Abs(workdir)
	handleError(err)

	// Get config file name
	target := path.Join(workdir, "mkdeb.json")
	if deb.FileExists(target) {
		handleError(fmt.Errorf("mkdir.json already exists in this directory"))
	}

	// Create config file
	file, err := os.Create(target)
	handleError(err)
	defer file.Close()

	// Create config struct
	projectName := filepath.Base(workdir)
	p := deb.DefaultPackageSpec()
	p.Package = projectName
	p.Maintainer = "Your Name <you@example.com>"
	p.Architecture = "amd64"
	p.Description = projectName + " is an awsome project for..."
	p.Homepage = "https://www.example.com/project"
	p.Files = map[string]string{projectName: "/usr/local/bin/" + projectName}

	data, err := json.MarshalIndent(p, "", "  ")
	handleError(err)

	_, err = file.Write(data)
	handleError(err)
}

func validate(config string) {
	// Change to config path
	back, err := os.Getwd()
	handleError(err)
	workdir, filename := getTarget(config)
	err = os.Chdir(workdir)
	handleError(err)
	defer os.Chdir(back)

	// Validate
	p, err := deb.NewPackageSpecFromFile(filename)
	handleError(err)
	handleError(p.Validate())
}

func build(config string, version string) {
	// Change to config path
	back, err := os.Getwd()
	handleError(err)
	workdir, filename := getTarget(config)
	err = os.Chdir(workdir)
	handleError(err)
	defer os.Chdir(back)

	p, err := deb.NewPackageSpecFromFile(filename)
	handleError(err)

	// Set version
	p.Version = version

	// Validate
	handleError(p.Validate())

	// Build
	handleError(p.Build(workdir))
	fmt.Printf("Built package %s\n", p.Filename())
}

func handleError(err error) {
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}
}

func showUsage() {
	fmt.Print(usage)
	os.Exit(1)
}

const usage = `ABOUT

  mkdeb is a tool for building debian packages

  Report issues or get updates from https://github.com/cbednarski/mkdeb

COMMANDS

  build       Build a package based on the specified config file
  init        Create a new mkdeb config file in the current directory
  archs       List supported CPU architectures
  validate    Validate your config file

BUILD COMMAND

  mkdeb build --version=1.2.0 config.json

  The build command will change to the directory where the config file is
  located, so paths should always be specified relative to the config file.

  All options except version should be specified in your config file since
  version typically changes on each build.

PACKAGING CONFIGURATION

  Required Fields

  - Package: The name of your package
  - Version: Must adhere to debian version syntax.
  - Architecture: CPU arch for your binaries, or "all"
  - Maintainer: Your Name <email@example.com>
  - Description: Brief explanation of your package

  Optional Fields

  - Depends: Other packages you depend on. E.g: "python" or "curl (>= 7.0.0)"
  - Conflicts: Packages your package are not compatible with
  - Breaks: Packages your package breaks
  - Replaces: Packages your package replaces
  - Homepage: URL to your project homepage or source repository, if you have one

  For more details on how to specify various config options, refer to the
  debian package specification:

  - https://www.debian.org/doc/debian-policy/ch-controlfields.html
  - https://www.debian.org/doc/manuals/debian-faq/ch-pkg_basics.en.html

PACKAGING LAYOUT

  AutoPath

  mkdeb will automatically include any files deb-pkg, the default AutoPath
  directory. For example, the following files will be automatically included and
  installed to their corresponding paths:

    deb-pkg/etc/mysqld/my.conf  -> /etc/mysqld/my.conf
    deb-pkg/usr/bin/mysqld      -> /usr/bin/mysqld

  You can override this behavior by setting AutoPath to - (dash character) and /
  or by using the Files map to create a custom source -> dest mapping.

  Control Scripts

  Control scripts allow you to take action at various stages of your package's
  lifecycle. These are commonly used to create users, start or stop services, or
  perform cleanup.

  By default mkdeb will use any of these files if they are present in deb-pkg:

  - preinst
  - postinst
  - prerm
  - postrm

  You can override this behavior by setting the relevant fields in your config.

BUILD OPTIONS

  The following options change how mkdeb runs when building packages.

  - TempPath: Controls where intermediate files are written during the build.
    This defaults to the system temp directory.

  - UpgradeConfigs: Indicates whether apt should replace files under /etc when
    installing a new package version. By default these files are not upgraded.

  - PreserveSymlinks: By default contents of symlink targets are copied. This
    option writes symlinks to the archive instead.

LICENSE

  Copyright 2016 Chris Bednarski <banzaimonkey@gmail.com>, and others

  Portions of mkdeb are licensed under the MIT and Go Licenses. Please refer to
  the project source for full license text and details.

  https://github.com/cbednarski/mkdeb
`
