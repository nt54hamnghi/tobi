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
		Use:   "list",
		Short: "List all tags",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("list called")
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

// listNotes recursively traverses the directory at root and lists all '.md' files
// while ignoring the .git folder. It returns absolute paths to the discovered files.
// Returns an error if the root path is not a valid directory.
func listNotes(root string) ([]string, error) {
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", root)
	}

	notes := make([]string, 0, 128)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
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
func filterGitTracked(root string, paths []string) ([]string, error) {
	filtered := make([]string, 0, len(paths))

	rfs := osfs.New(root, osfs.WithBoundOS())
	ps, err := gitignore.ReadPatterns(rfs, nil)
	if err != nil {
		return nil, err
	}
	matcher := gitignore.NewMatcher(ps)

	for _, path := range paths {
		relPath, err := filepath.Rel(root, path)
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
