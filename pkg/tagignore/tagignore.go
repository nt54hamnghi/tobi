package tagignore

import (
	"bufio"
	"errors"
	"io/fs"
	"log"
	"os"
	"strings"

	set "github.com/deckarep/golang-set/v2"
	"github.com/gobwas/glob"
)

type TagGlobs struct {
	Globs []glob.Glob
}

func (tg *TagGlobs) Match(tag string) bool {
	for _, g := range tg.Globs {
		if g.Match(tag) {
			return true
		}
	}
	return false
}

func NewTagGlobs(path string) (TagGlobs, error) {
	lines, err := readIgnorePatterns(path)
	if err != nil {
		return TagGlobs{}, err
	}

	// TODO: this can be run in parallel
	globs := make([]glob.Glob, 0, lines.Cardinality())
	for l := range set.Elements(lines) {
		g, err := glob.Compile(l)
		if err != nil {
			return TagGlobs{}, err
		}
		globs = append(globs, g)
	}

	return TagGlobs{Globs: globs}, nil
}

func readIgnorePatterns(path string) (set.Set[string], error) {
	lines := set.NewSet[string]()

	f, err := os.Open(path)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return lines, nil
		case errors.Is(err, fs.ErrPermission):
			log.Printf("permission denied to read %s", path)
			return lines, nil
		default:
			return nil, err
		}
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		l := strings.TrimSpace(scanner.Text())
		if len(l) == 0 {
			continue
		}
		// lines prefixed with # are comments
		if strings.HasPrefix(l, "#") {
			continue
		}
		lines.Add(l)
	}

	return lines, nil
}
