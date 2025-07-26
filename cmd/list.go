package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

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
				p, err := defaultVaultPath()
				if err != nil {
					return err
				}
				args = append(args, p)
			}

			root, err := newDirPath(args[0])
			if err != nil {
				return err
			}

			isIgnored, err := readIgnoredTags(filepath.Join(root.String(), ".tobiignore"))
			if err != nil {
				return err
			}

			notes, err := listGitTrackedNotes(root)
			if err != nil {
				return err
			}

			tags := collectTags(notes)
			tags = slices.DeleteFunc(tags, func(t string) bool {
				return isIgnored[t]
			})
			fmt.Printf("%+v", tags)

			return nil
		},
	}

	return cmd
}

func readIgnoredTags(ignoreFile string) (map[string]bool, error) {
	b, err := os.ReadFile(ignoreFile)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return nil, nil
		case errors.Is(err, fs.ErrPermission):
			log.Printf("permission denied: %s", ignoreFile)
			return nil, nil
		default:
			return nil, err
		}
	}

	lines := make(map[string]bool)
	for l := range strings.Lines(string(b)) {
		l = strings.TrimSuffix(l, "\n")
		if l == "" {
			continue
		}
		lines[l] = true
	}

	return lines, nil
}

func collectTags(notes []string) []string {
	var wg sync.WaitGroup

	ch := make(chan []string, len(notes))

	for _, n := range notes {
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

	allTags := make([]string, 0, 1024)
	for tags := range ch {
		allTags = append(allTags, tags...)
	}
	sort.Strings(allTags)
	return slices.Compact(allTags)
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

// listGitTrackedNotes finds all '.md' files in the root directory and filters them
// based on .gitignore files in the directory. It returns only files that should be tracked by Git.
//
// Returns an error if unable to read files or .gitignore patterns.
func listGitTrackedNotes(root dirPath) ([]string, error) {
	paths, err := listNotes(root)
	if err != nil {
		return nil, err
	}

	filtered := make([]string, 0, len(paths))

	rfs := osfs.New(root.String(), osfs.WithBoundOS())
	ps, err := gitignore.ReadPatterns(rfs, nil)
	if err != nil {
		return nil, err
	}
	matcher := gitignore.NewMatcher(ps)

	for _, path := range paths {
		relPath, err := filepath.Rel(root.String(), path)
		if err != nil {
			return nil, err
		}
		s := strings.Split(relPath, string(filepath.Separator))
		if !matcher.Match(s, false) {
			filtered = append(filtered, path)
		}
	}

	return filtered, nil
}

// listNotes recursively traverses the directory at root and lists all '.md' files
// while ignoring the .git folder. It returns absolute paths to the discovered files.
//
// Returns an error if the root path is not a valid directory.
func listNotes(root dirPath) ([]string, error) {
	notes := make([]string, 0, 128)

	err := filepath.WalkDir(root.String(), func(path string, d fs.DirEntry, err error) error {
		// Skip directory entry if there's an error
		if err != nil {
			return nil
		}

		// Skip .git directory
		if d.Name() == ".git" {
			return filepath.SkipDir
		}

		if d.Type().IsRegular() && filepath.Ext(path) == ".md" {
			notes = append(notes, path)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return notes, nil
}
