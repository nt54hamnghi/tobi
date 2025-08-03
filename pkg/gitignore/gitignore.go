package gitignore

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

const (
	commentPrefix  = "#"
	gitDir         = ".git"
	gitignoreFile  = ".gitignore"
	tobiignoreFile = ".tobiignore"
)

// .git/info/exclude
var infoExcludeFile = filepath.Join(gitDir, "info", "exclude")

type RepoRootMatcher struct {
	Root AbsolutePath
	gitignore.Matcher
}

func NewRepoRootMatcher(root AbsolutePath) (RepoRootMatcher, error) {
	ps, err := ReadPatterns(root)
	if err != nil {
		return RepoRootMatcher{}, err
	}

	return RepoRootMatcher{root, gitignore.NewMatcher(ps)}, nil
}

func (m *RepoRootMatcher) MatchFile(path AbsolutePath) bool {
	parts := splitPath(path.String())
	return m.Match(parts, false)
}

// ReadPatterns reads gitignore patterns from the repository, starting with
// .git/info/exclude at the repository root, then recursively traversing the
// directory structure to read all .gitignore and .tobiignore files.
//
// Patterns are returned in ascending order of priority (last higher), with
// nested .gitignore and .tobiignore files overriding parent patterns. Nested .git folders
// are not supported.
func ReadPatterns(root AbsolutePath) ([]gitignore.Pattern, error) {
	// load patterns from .git/info/exclude
	// Errors are acceptable. We'll just start with a nil slice.
	ps, _ := readIgnoreFile(root.join(infoExcludeFile))

	err := filepath.WalkDir(root.String(), func(path string, d fs.DirEntry, err error) error {
		// Return out of WalkDir as soon as there's an error
		if err != nil {
			return err
		}

		// Skip .git directory
		if d.IsDir() {
			if d.Name() == gitDir {
				return filepath.SkipDir
			}

			m := gitignore.NewMatcher(ps)
			if m.Match(splitPath(path), true) {
				return filepath.SkipDir
			}
		}

		// files are walked in lexical order, so .tobiignore files override .gitignore files
		// TODO: test this assumption
		if d.Type().IsRegular() && (d.Name() == gitignoreFile || d.Name() == tobiignoreFile) {
			// Since root is absolute when we pass it to WalkDir, path is absolute.
			// It's safe to construct AbsolutePath directly from path.
			p := NewAbsolutePathUnchecked(path)
			subps, err := readIgnoreFile(p)
			if err != nil {
				return err
			}
			ps = append(ps, subps...)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return ps, nil
}

// readIgnoreFile reads and parses patterns from a gitignore file.
// Skips comment lines (#) and empty lines. Handles .git/info/exclude files
// by applying their patterns at the repository root level.
func readIgnoreFile(ignoreFile AbsolutePath) ([]gitignore.Pattern, error) {
	path := ignoreFile.String()
	domain := splitPath(filepath.Dir(path))

	if strings.HasSuffix(path, infoExcludeFile) {
		// .git/info/exclude patterns apply to repository root, not .git/info directory
		// so we move up 2 levels
		domain = domain[:len(domain)-2]
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ps := []gitignore.Pattern{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		l := scanner.Text()
		if strings.HasPrefix(l, commentPrefix) {
			continue
		}
		if len(strings.TrimSpace(l)) == 0 {
			continue
		}
		ps = append(ps, gitignore.ParsePattern(l, domain))
	}

	return ps, nil
}

type AbsolutePath struct {
	path string
}

func NewAbsolutePath(path string) (AbsolutePath, error) {
	if !filepath.IsAbs(path) {
		return AbsolutePath{}, fmt.Errorf("%s is not an absolute path", path)
	}
	return AbsolutePath{path}, nil
}

// NewAbsolutePathUnchecked creates an AbsolutePath without checking if the path is absolute.
// This is useful when you are certain the path is absolute. Use with caution since if the
// invariant is violated, other functions expecting absolute paths might return unexpected
// results.
func NewAbsolutePathUnchecked(path string) AbsolutePath {
	return AbsolutePath{path}
}

func (a AbsolutePath) String() string {
	return a.path
}

func (a AbsolutePath) join(elem ...string) AbsolutePath {
	e := append([]string{a.path}, elem...)
	return AbsolutePath{path: filepath.Join(e...)}
}

func splitPath(path string) []string {
	return strings.Split(path, string(os.PathSeparator))
}
