package cmd

import (
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gotest.tools/v3/fs"
)

func Test_extractFrontMatter(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
		wantErr  error
	}{
		{
			name:     "valid frontmatter",
			input:    "---\ntags: [one, two]\n---\nContent here",
			expected: "tags: [one, two]",
			wantErr:  nil,
		},
		{
			name:     "multiple separators",
			input:    "---\ntags: [one, two]\n---\nContent with\n---More content\n---",
			expected: `tags: [one, two]`,
			wantErr:  nil,
		},
		{
			name:     "whitespace in frontmatter",
			input:    "---\n\ntags: [one, two]\n\n---\n",
			expected: "tags: [one, two]",
			wantErr:  nil,
		},
		{
			name:    "empty frontmatter",
			input:   "---\n---\nContent here",
			wantErr: ErrEmptyFrontMatter,
		},
		{
			name:    "no frontmatter",
			input:   "Content here",
			wantErr: ErrNoFrontMatter,
		},
		{
			name:    "no closing delimiter",
			input:   "---\ntags: [test]",
			wantErr: ErrInvalidFrontMatter,
		},
		{
			name:    "no opening delimiter",
			input:   "tags: [test]---",
			wantErr: ErrNoFrontMatter,
		},
		{
			name:    "no new line after opening delimiter",
			input:   "---tags: [test]\n---\n",
			wantErr: ErrInvalidFrontMatter,
		},
		{
			name:     "no new line after closing delimiter",
			input:    "---\ntags: [test]\n---",
			expected: "tags: [test]",
			wantErr:  nil,
		},
		{
			name:     "whitespace around delimiters",
			input:    "\n---\ntags: [one, two]\n---\n",
			expected: "tags: [one, two]",
			wantErr:  ErrNoFrontMatter,
		},
	}

	r := require.New(t)

	for _, tt := range testCases {
		t.Run(tt.name, func(_ *testing.T) {
			actual, err := extractFrontMatter(strings.NewReader(tt.input))

			if tt.wantErr != nil {
				r.ErrorIs(err, tt.wantErr)
			} else {
				r.NoError(err)
				r.Equal(tt.expected, actual)
			}
		})
	}
}

func Test_listNotes(t *testing.T) {
	testCases := []struct {
		name string
		dir  *fs.Dir
		want []string
	}{
		{
			name: "single",
			dir: fs.NewDir(t, "test",
				fs.WithFile("note.md", "# Test"),
			),
			want: []string{"note.md"},
		},
		{
			name: "multiple",
			dir: fs.NewDir(t, "test",
				fs.WithFiles(map[string]string{
					"note1.md": "# Test 1",
					"note2.md": "# Test 2",
				}),
			),
			want: []string{"note1.md", "note2.md"},
		},
		{
			name: "mixed file types",
			dir: fs.NewDir(t, "test",
				fs.WithFiles(map[string]string{
					"note1.md": "# Test 1",
					"t.txt":    "Text file",
					"t.json":   `{"key": "value"}`,
					"t.sh":     "#!/bin/bash",
				}),
			),
			want: []string{"note1.md"},
		},
		{
			name: "nested",
			dir: fs.NewDir(t, "test",
				fs.WithDir("level1",
					fs.WithFile("note.md", "# Nested"),
				),
			),
			want: []string{"level1/note.md"},
		},
		{
			name: "deeply nested",
			dir: fs.NewDir(t, "test",
				fs.WithDir("level1",
					fs.WithDir("level2",
						fs.WithFile("note.md", "# Deep"),
					),
				),
			),
			want: []string{"level1/level2/note.md"},
		},
		{
			name: ".git skipped",
			dir: fs.NewDir(t, "test",
				fs.WithFile("note.md", "# Note"),
				fs.WithDir(".git",
					fs.WithFile("file.md", "# Ignored"),
				),
			),
			want: []string{"note.md"},
		},
		{
			name: "empty directory",
			dir:  fs.NewDir(t, "test"),
			want: []string{},
		},
	}

	r := require.New(t)
	for _, tt := range testCases {
		defer tt.dir.Remove()
		t.Run(tt.name, func(_ *testing.T) {
			root, err := newDirPath(tt.dir.Path())
			r.NoError(err)

			notes, err := listNotes(root)
			r.NoError(err)

			// Convert absolute paths to relative paths for comparison
			relPaths := make([]string, len(notes))
			for i, path := range notes {
				relPath, err := filepath.Rel(root.String(), path)
				r.NoError(err)
				relPaths[i] = relPath
			}

			// Sort both slices for reliable comparison
			sort.Strings(relPaths)
			sort.Strings(tt.want)

			r.Equal(tt.want, relPaths)
		})
	}
}

func Test_listGitTrackedNotes(t *testing.T) {
	testCases := []struct {
		name string
		dir  *fs.Dir
		want []string
	}{
		{
			name: "root .gitignore",
			dir: fs.NewDir(t, "test",
				fs.WithFiles(map[string]string{
					".gitignore": "level1/",
					"note.md":    "content",
				}),
				fs.WithDir("level1",
					fs.WithFiles(map[string]string{
						"note2.md": "content",
					}),
				),
			),
			want: []string{"note.md"},
		},
		{
			name: "subdir .gitignore",
			dir: fs.NewDir(t, "test",
				fs.WithFile("note.md", "content"),
				fs.WithDir("level1",
					fs.WithFiles(map[string]string{
						"note2.md": "content",
					}),
				),
			),
			want: []string{"note.md", "level1/note2.md"},
		},
		{
			name: "multiple .gitignore",
			dir: fs.NewDir(t, "test",
				fs.WithFiles(map[string]string{
					".gitignore": "level1/",
					"note.md":    "content",
				}),
				fs.WithDir("level1",
					fs.WithFiles(map[string]string{
						"note2.md": "content",
					}),
				),
				fs.WithDir("level2",
					fs.WithFiles(map[string]string{
						"note3.md":   "content",
						"note4.md":   "content",
						".gitignore": "note4.md",
					}),
				),
			),
			want: []string{"note.md", "level2/note3.md"},
		},
	}
	r := require.New(t)
	for _, tt := range testCases {
		defer tt.dir.Remove()
		t.Run(tt.name, func(_ *testing.T) {
			root, err := newDirPath(tt.dir.Path())
			r.NoError(err)

			filtered, err := listGitTrackedNotes(root)
			r.NoError(err)

			// Convert absolute paths to relative paths for comparison
			relPaths := make([]string, len(filtered))
			for i, path := range filtered {
				relPath, err := filepath.Rel(root.String(), path)
				r.NoError(err)
				relPaths[i] = relPath
			}

			// Sort both slices for reliable comparison
			sort.Strings(relPaths)
			sort.Strings(tt.want)

			r.Equal(tt.want, relPaths)
		})
	}
}

func Test_processFile(t *testing.T) {
	testCases := []struct {
		name    string
		dir     *fs.Dir
		want    []string
		wantErr bool
	}{
		{
			name: "valid frontmatter with tags",
			dir: fs.NewDir(t, "test",
				fs.WithFile(
					"note.md", "---\ntags: [golang, \"#cobra\"]\n---\nContent",
				),
			),
			want: []string{"golang", "cobra"},
		},
		{
			name: "no tags field",
			dir: fs.NewDir(t, "test",
				fs.WithFile(
					"note.md", "---\ntitle: Test\n---\nContent",
				),
			),
			want: nil,
		},
		{
			name: "empty tags array",
			dir: fs.NewDir(t, "test",
				fs.WithFile(
					"note.md", "---\ntags: []\n---\nContent",
				),
			),
			want: []string{},
		},
		{
			name: "no frontmatter",
			dir: fs.NewDir(t, "test",
				fs.WithFile(
					"note.md", "# Just content",
				),
			),
			want: nil,
		},
		{
			name: "empty frontmatter",
			dir: fs.NewDir(t, "test",
				fs.WithFile(
					"note.md", "---\n---\nContent",
				),
			),
			want: nil,
		},
		{
			name: "invalid frontmatter",
			dir: fs.NewDir(t, "test",
				fs.WithFile(
					"note.md", "---\ntags: [test]\nNo closing delimiter",
				),
			),
			wantErr: true,
		},
		{
			name: "invalid YAML",
			dir: fs.NewDir(t, "test",
				fs.WithFile(
					"note.md", "---\ntags: [invalid: yaml\n---\nContent",
				),
			),
			wantErr: true,
		},
	}

	r := require.New(t)

	for _, tt := range testCases {
		defer tt.dir.Remove()

		t.Run(tt.name, func(_ *testing.T) {
			actual, err := processFile(tt.dir.Path() + "/note.md")
			if tt.wantErr {
				r.Error(err)
				return
			}
			r.NoError(err)
			r.Equal(tt.want, actual)
		})
	}
}

func Test_readIgnoredTags(t *testing.T) {
	testCases := []struct {
		name string
		dir  *fs.Dir
		want []string
	}{
		{
			name: "single tag",
			dir: fs.NewDir(t, "test",
				fs.WithFile(".tobiignore", "golang"),
			),
			want: []string{"golang"},
		},
		{
			name: "multiple tags",
			dir: fs.NewDir(t, "test",
				fs.WithFile(".tobiignore", "golang\ncobra"),
			),
			want: []string{"cobra", "golang"},
		},
		{
			name: "duplicate tags",
			dir: fs.NewDir(t, "test",
				fs.WithFile(".tobiignore", "golang\ngolang\ncobra"),
			),
			want: []string{"cobra", "golang"},
		},
		{
			name: "duplicate empty lines",
			dir: fs.NewDir(t, "test",
				fs.WithFile(".tobiignore", "golang\n\n\ncobra"),
			),
			want: []string{"cobra", "golang"},
		},
		{
			name: "empty file",
			dir: fs.NewDir(t, "test",
				fs.WithFile(".tobiignore", ""),
			),
			want: nil,
		},
		{
			name: "file does not exist",
			dir:  fs.NewDir(t, "test"),
			want: nil,
		},
	}

	r := require.New(t)

	for _, tt := range testCases {
		defer tt.dir.Remove()

		t.Run(tt.name, func(_ *testing.T) {
			filePath := filepath.Join(tt.dir.Path(), ".tobiignore")

			actual, err := readIgnoredTags(filePath)
			r.NoError(err)

			actualTags := slices.Collect(maps.Keys(actual))
			sort.Strings(actualTags)
			r.Equal(tt.want, actualTags)
		})
	}
}
