package cmd

import (
	"path/filepath"
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
			name:     "whitespace around delimiters",
			input:    "\n---\ntags: [one, two]\n---\n",
			expected: "tags: [one, two]",
			wantErr:  nil,
		},
		{
			name:    "empty frontmatter",
			input:   "---\n---\nContent here",
			wantErr: ErrEmptyFrontMatter,
		},
		{
			name:    "no closing delimiter",
			input:   "---\ntags: [test]",
			wantErr: ErrInvalidFrontMatter,
		},
		{
			name:    "no opening delimiter",
			input:   "tags: [test]---",
			wantErr: ErrInvalidFrontMatter,
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
		t.Run(tt.name, func(t *testing.T) {
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

func Test_filteredGitTracked(t *testing.T) {
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
						"note2.md":   "content",
						"note3.md":   "content",
						".gitignore": "note3.md",
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
		t.Run(tt.name, func(t *testing.T) {
			root, err := newDirPath(tt.dir.Path())
			r.NoError(err)

			notes, err := listNotes(root)
			r.NoError(err)

			filtered, err := filterGitTracked(root, notes)
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
