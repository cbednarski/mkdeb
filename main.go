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
	args := os.Args

	if len(args) < 2 {
		showUsage()
	}

	switch args[1] {
	case "archs":
		showArchs()
	case "build":
		buildCommand := flag.NewFlagSet("build", flag.ExitOnError)
		version := buildCommand.String("version", "1.0", "Package version")
		target := buildCommand.String("target", "", "Target folder with generated filename")
		buildCommand.Parse(args[2:])
		build(checkConfig(buildCommand.Args()), *version, *target)
	case "init":
		initialize()
	case "validate":
		commandArgs := flag.Args()

		validate(checkConfig(commandArgs))
	default:
		showUsage()
	}
	os.Exit(0)
}

func checkConfig(args []string) string {
	if len(args) < 1 {
		fmt.Printf("Missing config file\n")
		os.Exit(1)
	}
	if len(args) > 1 {
		fmt.Printf("Too many arguments\n")
		os.Exit(1)
	}
	return args[0]
}

// getAbsPaths takes a relative path to a file and returns both the containing
// directory and the absolute path to the file.
//
// Example: cat -> /bin /bin/cat
func getAbsPaths(filename string) (string, string) {
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
	workdir, filename := getAbsPaths(config)
	err = os.Chdir(workdir)
	handleError(err)
	defer os.Chdir(back)

	// Validate
	p, err := deb.NewPackageSpecFromFile(filename)
	handleError(err)
	handleError(p.Validate(false))
}

func build(config, version, target string) {
	// Change to config path
	back, err := os.Getwd()
	handleError(err)

	// Get the working directory to cd into and the absolute path to the file
	workdir, abspath := getAbsPaths(config)
	err = os.Chdir(workdir)
	handleError(err)
	defer os.Chdir(back)

	p, err := deb.NewPackageSpecFromFile(abspath)
	handleError(err)

	// Set version
	p.Version = version

	// Set target filename
	if target == "" {
		target = workdir
	} else {
		if !isDir(target) {
			handleError(fmt.Errorf("%q is not a directory", target))
		}
	}

	// Validate
	handleError(p.Validate(true))

	// Build
	handleError(p.Build(target))
	fmt.Printf("Built package %s\n", path.Join(target, p.Filename()))
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
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

  mkdeb build -version=1.2.0 config.json

  Options:

    -version (required) Package version

    -target (optional) output artifact to this path

  By default the build artifact

  The build command will change to the directory where the config file is
  located, so paths should always be specified relative to the config file.

PACKAGING CONFIGURATION

  Required Fields

  - package: The name of your package
  - version: Must adhere to debian version syntax.
  - architecture: CPU arch for your binaries, or "all"
  - maintainer: Your Name <email@example.com>
  - description: Brief explanation of your package

  Optional Fields

  - depends: Other packages you depend on. E.g: "python" or "curl (>= 7.0.0)"
  - conflicts: Packages your package are not compatible with
  - breaks: Packages your package breaks
  - replaces: Packages your package replaces
  - homepage: URL to your project homepage or source repository, if you have one

  For more details on how to specify various config options, refer to the
  debian package specification:

  - https://www.debian.org/doc/debian-policy/ch-controlfields.html
  - https://www.debian.org/doc/manuals/debian-faq/ch-pkg_basics.en.html

PACKAGING LAYOUT

  autoPath

  mkdeb will automatically include any files deb-pkg, the default autoPath
  directory. For example, the following files will be automatically included and
  installed to their corresponding paths:

    deb-pkg/etc/mysqld/my.conf  -> /etc/mysqld/my.conf
    deb-pkg/usr/bin/mysqld      -> /usr/bin/mysqld

  You can override this behavior by setting autoPath to - (dash character) and /
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

  - tempPath: Controls where intermediate files are written during the build.
    This defaults to the system temp directory.

  - upgradeConfigs: Indicates whether apt should replace files under /etc when
    installing a new package version. By default these files are not upgraded.

  - preserveSymlinks: By default contents of symlink targets are copied. This
    option writes symlinks to the archive instead.

LICENSE

  Copyright 2016 Chris Bednarski <banzaimonkey@gmail.com>, and others

  Portions of mkdeb are licensed under the MIT, BSD and Go Licenses. Please
  refer to the project source for full license text and details.

  https://github.com/cbednarski/mkdeb
`
