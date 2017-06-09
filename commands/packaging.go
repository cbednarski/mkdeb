package commands

import (
	"context"
	"flag"
	"fmt"

	"github.com/google/subcommands"
)

type PackagingCmd struct {
}

func (*PackagingCmd) Name() string     { return "packaging" }
func (*PackagingCmd) Synopsis() string { return "show help for packaging" }
func (*PackagingCmd) Usage() string {
	return packagingHelp
}

func (p *PackagingCmd) SetFlags(f *flag.FlagSet) {

}

func (p *PackagingCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {

	fmt.Println(packagingHelp)
	return subcommands.ExitSuccess
}

const packagingHelp = `PACKAGING CONFIGURATION

  Required Fields

  - package: The name of your package
  - version: Must adhere to debian version syntax.
  - architecture: CPU arch for your binaries, or "all"
  - maintainer: Your Name <email@example.com>
  - description: Brief explanation of your package

  Optional Fields

  - depends: Other packages you depend on. E.g: "python" or "curl (>= 7.0.0)"
  - preDepends: Other packages you depend on which is required to be available
    to configure your package.
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

`
