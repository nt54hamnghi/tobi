package cmd

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

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
			var path string

			if len(args) == 0 {
				if p, err := vaultPath(); err != nil {
					return err
				} else {
					path = p
				}
			} else {
				path = args[0]
			}

			root, err := newDirPath(path)
			if err != nil {
				return err
			}

			notes, err := listNotes(root)
			if err != nil {
				return err
			}

			notes, err = filterGitTracked(root, notes)

			fmt.Println(root)
			return nil
		},
	}

	return cmd
}

var (
	ErrInvalidFrontMatter = errors.New("invalid frontmatter")
	ErrEmptyFrontMatter   = errors.New("empty frontmatter")
)

func processFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	yml, err := extractFrontMatter(f)
	if err != nil {
		return nil, err
	}

	return extractTagsFromYAML([]byte(yml))
}

// extractFrontMatter reads from the given reader and extracts YAML frontmatter
// content enclosed between '---' delimiters, returning the frontmatter as a string.
// Returns an error if delimiters are missing or frontmatter is empty.
func extractFrontMatter(r io.Reader) (string, error) {
	delim := "---\n"
	scanner := bufio.NewScanner(r)
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}

		if i := bytes.Index(data, []byte(delim)); i >= 0 {
			step := len(delim)
			return i + step, data[:i+step], nil
		}

		// If we're at EOF, we have a final, non-terminated line. Return it.
		if atEOF {
			return len(data), data, nil
		}

		// Request more data.
		return 0, nil, nil
	})

	parts := make([]string, 0, 2)
	for scanner.Scan() {
		if len(parts) == 2 {
			break
		}

		parts = append(parts, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	if len(parts) < 2 {
		return "", ErrInvalidFrontMatter
	}

	token := parts[1]
	switch {
	// Closing delimiter has a newline
	case strings.HasSuffix(token, delim):
		token = strings.TrimSuffix(token, delim)
	// Closing delimiter has no newline
	case strings.HasSuffix(token, delim[:3]):
		token = strings.TrimSuffix(token, delim[:3])
	default:
		return "", ErrInvalidFrontMatter
	}

	token = strings.TrimSpace(token)
	if len(token) == 0 {
		return "", ErrEmptyFrontMatter
	}

	return token, nil
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

// listNotes recursively traverses the directory at root and lists all '.md' files
// while ignoring the .git folder. It returns absolute paths to the discovered files.
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

// filterGitTracked filters the given file paths by applying .gitignore rules from
// root and subdirectories, returning only files that should be tracked by Git.
// Returns an error if there's an issue reading .gitignore files.
func filterGitTracked(root dirPath, paths []string) ([]string, error) {
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
