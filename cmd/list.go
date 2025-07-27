package cmd

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	set "github.com/deckarep/golang-set/v2"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
)

func NewListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list [path]",
		Short:   "List all tags",
		Aliases: []string{"ls", "l"},
		Args:    cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				p, exist := os.LookupEnv("OBSIDIAN_VAULT_PATH")
				if !exist {
					return fmt.Errorf("path not provided and OBSIDIAN_VAULT_PATH is not set")
				}
				args = append(args, p)
			}

			root, err := newDirPath(args[0])
			if err != nil {
				return err
			}

			isIgnored, err := loadIgnoredTags(filepath.Join(root.String(), ".tobiignore"))
			if err != nil {
				return err
			}

			ns, err := listNotes(root)
			if err != nil {
				return err
			}

			tags := collectTags(ns.notes).Difference(isIgnored)

			fmt.Printf("%+v\n", tags)
			fmt.Printf("%d\n", ns.hash)

			return nil
		},
	}

	return cmd
}

func loadIgnoredTags(ignoreFile string) (set.Set[string], error) {
	lines := set.NewSet[string]()

	b, err := os.ReadFile(ignoreFile)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return lines, nil
		case errors.Is(err, fs.ErrPermission):
			log.Printf("permission denied: %s", ignoreFile)
			return lines, nil
		default:
			return nil, err
		}
	}

	for l := range strings.Lines(string(b)) {
		l = strings.TrimSuffix(l, "\n")
		if l == "" {
			continue
		}
		lines.Add(l)
	}

	return lines, nil
}

func collectTags(notes set.Set[string]) set.Set[string] {
	var wg sync.WaitGroup

	ch := make(chan []string, notes.Cardinality())

	for n := range set.Elements(notes) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tags, err := processFile(n)
			if err != nil {
				log.Printf("failed to process file %s: %v", n, err)
				ch <- nil
				return
			}
			ch <- tags
		}()
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	allTags := set.NewSetWithSize[string](1024)
	for tags := range ch {
		allTags.Append(tags...)
	}
	return allTags
}

func processFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	yml, err := extractFrontMatter(f)
	if errors.Is(err, ErrEmptyFrontMatter) || errors.Is(err, ErrNoFrontMatter) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return extractTagsFromYAML([]byte(yml))
}

var (
	ErrInvalidFrontMatter = errors.New("invalid frontmatter")
	ErrEmptyFrontMatter   = errors.New("empty frontmatter")
	ErrNoFrontMatter      = errors.New("no frontmatter")
)

// extractFrontMatter reads from the given reader and extracts YAML frontmatter
// content enclosed between '---' delimiters, returning the frontmatter as a string.
// Returns an error if delimiters are missing or frontmatter is empty.
func extractFrontMatter(r io.Reader) (string, error) {
	sep := "---"

	scanner := bufio.NewScanner(r)
	if scanner.Scan() {
		t := scanner.Text()
		if !strings.HasPrefix(t, sep) {
			return "", ErrNoFrontMatter
		}
		if t != sep {
			return "", ErrInvalidFrontMatter
		}
	}

	var (
		s   strings.Builder
		end bool
	)

	for scanner.Scan() {
		t := scanner.Text()
		if t == sep {
			end = true
			break
		}
		s.WriteString(t + "\n")
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	if !end {
		return "", ErrInvalidFrontMatter
	}

	yml := strings.TrimSpace(s.String())
	if len(yml) == 0 {
		return "", ErrEmptyFrontMatter
	}

	return yml, nil
}

// extractTagsFromYAML parses YAML frontmatter data and extracts the "tags" field,
// returning the tags as a slice of strings. Returns an error if the YAML is invalid.
func extractTagsFromYAML(data []byte) ([]string, error) {
	var fm struct {
		Tags []string `yaml:"tags"`
	}

	if err := yaml.Unmarshal(data, &fm); err != nil {
		return nil, err
	}

	for i := range fm.Tags {
		fm.Tags[i] = strings.TrimPrefix(fm.Tags[i], "#")
	}

	return fm.Tags, nil
}

// dirPath is a path to a valid directory.
type dirPath string

func newDirPath(path string) (dirPath, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", path)
	}
	return dirPath(path), nil
}

func (d dirPath) String() string {
	return string(d)
}

type noteSet struct {
	notes set.Set[string]
	hash  uint64
}

// listNotes recursively traverses the directory at root and lists all '.md' files
// while ignoring the .git folder. It returns absolute paths to the discovered files.
//
// Returns an error if the root path is not a valid directory.
func listNotes(root dirPath) (noteSet, error) {
	h := fnv.New64a()
	m, err := newGitIgnoredMatcher(root)
	if err != nil {
		return noteSet{}, err
	}

	notes := set.NewSet[string]()
	err = filepath.WalkDir(root.String(), func(path string, d fs.DirEntry, err error) error {
		// Skip directory entry if there's an error
		if err != nil {
			return nil
		}

		// Skip .git directory
		if d.Name() == ".git" {
			return filepath.SkipDir
		}

		if d.Type().IsRegular() && filepath.Ext(path) == ".md" {
			// matchFile will returns an error if the path can't be made relative to root.
			// However, this is not possible in WalkDir, so ignoring error is safe.
			skip, _ := m.matchFile(path)
			if skip {
				return nil
			}

			info, err := d.Info()
			// Skip files where we can't get info. Info() returns fs.ErrNotExist if the file
			// has been removed or renamed since the directory read. Since we're only reading
			// (not modifying files), this should never happen. However, we log the error
			// as a safeguard to warn anyone against accidentally modifying files during traversal.
			if err != nil {
				log.Printf("failed to get file info for %s: %v", path, err)
				return nil
			}

			h.Write([]byte(path))
			binary.Write(h, binary.LittleEndian, info.ModTime().Unix())

			notes.Add(path)
		}

		return nil
	})
	if err != nil {
		return noteSet{}, err
	}

	return noteSet{
		notes: notes,
		hash:  h.Sum64(),
	}, nil
}

type gitignoreMatcher struct {
	gitignore.Matcher
	root dirPath
}

func newGitIgnoredMatcher(root dirPath) (gitignoreMatcher, error) {
	rfs := osfs.New(root.String(), osfs.WithBoundOS())

	ps, err := gitignore.ReadPatterns(rfs, nil)
	if err != nil {
		return gitignoreMatcher{}, err
	}

	return gitignoreMatcher{
		gitignore.NewMatcher(ps),
		root,
	}, nil
}

func (m *gitignoreMatcher) matchFile(absPath string) (bool, error) {
	relPath, err := filepath.Rel(m.root.String(), absPath)
	if err != nil {
		return false, err
	}
	s := strings.Split(relPath, string(filepath.Separator))
	return m.Match(s, false), nil
}
