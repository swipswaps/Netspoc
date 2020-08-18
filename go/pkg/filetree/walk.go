package filetree

import (
	"github.com/hknutzen/Netspoc/go/pkg/abort"
	"github.com/hknutzen/Netspoc/go/pkg/conf"
	"github.com/hknutzen/Netspoc/go/pkg/fileop"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Context struct {
	Path string
	Data string
	ipV6 bool
}
type parser func(*Context)

// Read input from file and process it by function which is given as argument.
func processFile(input *Context, fn parser) {
	content, err := ioutil.ReadFile(input.Path)
	if err != nil {
		abort.Msg("Can't read %s: %s", input.Path, err)
	}
	input.Data = string(content)
	fn(input)
}

func Walk(fname string, fn parser) {
	input := &Context{ipV6: conf.Conf.IPV6}

	// Handle toplevel file.
	if !fileop.IsDir(fname) {
		input.Path = fname
		processFile(input, fn)
		return
	}

	// Handle toplevel Directory
	files, err := ioutil.ReadDir(fname)
	if err != nil {
		panic(err)
	}
	ipvDir := "ipv6"
	if conf.Conf.IPV6 {
		ipvDir = "ipv4"
	}
	for _, file := range files {
		base := file.Name()
		ignore := conf.Conf.IgnoreFiles
		// Skip hidden file, special file/directory, ignored file.
		if base[0] == '.' || base == "config" || base == "raw" ||
			ignore.MatchString(base) {
			continue
		}
		name := filepath.Join(fname, base)
		err = filepath.Walk(name,
			func(fname string, file os.FileInfo, err error) error {
				if err != nil {
					// Abort filepath.Walk.
					return err
				}
				copy := *input
				input := &copy
				input.Path = fname

				base := path.Base(fname)

				// Handle ipv6 / ipv4 subdirectory or file.
				if base == ipvDir {
					input.ipV6 = base == "ipv6"
				}

				isDir := file.IsDir()

				// Skip hidden and ignored file.
				if base[0] == '.' || ignore.MatchString(base) {
					if isDir {
						return filepath.SkipDir
					} else {
						return nil
					}
				}

				if !isDir {
					processFile(input, fn)
				}
				return nil
			})

		if err != nil {
			abort.Msg("while walking path %q: %v\n", fname, err)
		}
	}
}
