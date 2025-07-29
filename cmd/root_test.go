package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	set "github.com/deckarep/golang-set/v2"
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
			root, err := newVaultPath(tt.dir.Path())
			r.NoError(err)

			ns, err := listNotes(root)
			r.NoError(err)

			// Convert absolute paths to relative paths for comparison
			notes := set.Sorted(ns.notes)
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
			actual, err := processFile(filepath.Join(tt.dir.Path(), "note.md"))
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
			want: []string{},
		},
		{
			name: "file does not exist",
			dir:  fs.NewDir(t, "test"),
			want: []string{},
		},
	}

	r := require.New(t)

	for _, tt := range testCases {
		defer tt.dir.Remove()

		t.Run(tt.name, func(_ *testing.T) {
			root, err := newVaultPath(tt.dir.Path())
			r.NoError(err)

			actual, err := loadIgnoredTags(root)
			r.NoError(err)

			r.Equal(tt.want, set.Sorted(actual))
		})
	}
}

func Test_collectTags(t *testing.T) {
	testCases := []struct {
		name        string
		dir         *fs.Dir
		ignoredTags set.Set[string]
		want        map[string]int
	}{
		{
			name: "single file",
			dir: fs.NewDir(t, "test",
				fs.WithFile("note1.md", "---\ntags: [golang, cobra, cli]\n---\nContent"),
			),
			ignoredTags: set.NewSet[string](),
			want: map[string]int{
				"golang": 1,
				"cobra":  1,
				"cli":    1,
			},
		},
		{
			name: "multiple files",
			dir: fs.NewDir(t, "test",
				fs.WithFiles(map[string]string{
					"note1.md": "---\ntags: [golang, cobra]\n---\nContent",
					"note2.md": "---\ntags: [golang, cli]\n---\nContent",
					"note3.md": "---\ntags: [cobra]\n---\nContent",
				}),
			),
			ignoredTags: set.NewSet[string](),
			want: map[string]int{
				"golang": 2,
				"cobra":  2,
				"cli":    1,
			},
		},
		{
			name: "skip files with errors",
			dir: fs.NewDir(t, "test",
				fs.WithFiles(map[string]string{
					"valid.md":   "---\ntags: [golang]\n---\nContent",
					"invalid.md": "---\ntags: [invalid: yaml\n---\nContent", // Invalid YAML
				}),
			),
			ignoredTags: set.NewSet[string](),
			want: map[string]int{
				"golang": 1,
			},
		},
		{
			name: "ignored tags are filtered out",
			dir: fs.NewDir(t, "test",
				fs.WithFiles(map[string]string{
					"note1.md": "---\ntags: [golang, cobra, daily]\n---\nContent",
					"note2.md": "---\ntags: [golang, daily, personal]\n---\nContent",
				}),
			),
			ignoredTags: set.NewSet("daily", "personal"),
			want: map[string]int{
				"golang": 2,
				"cobra":  1,
			},
		},
		{
			name: "all tags ignored",
			dir: fs.NewDir(t, "test",
				fs.WithFile("note1.md", "---\ntags: [daily, personal]\n---\nContent"),
			),
			ignoredTags: set.NewSet("daily", "personal"),
			want:        map[string]int{},
		},
		{
			name: "empty noteSet",
			dir:  fs.NewDir(t, "test"),
			want: map[string]int{},
		},
		{
			name: "hash prefix removal",
			dir: fs.NewDir(t, "test",
				fs.WithFile("note1.md", "---\ntags: [\"#golang\", golang]\n---\nContent"),
			),
			ignoredTags: set.NewSet[string](),
			want: map[string]int{
				"golang": 2,
			},
		},
	}

	r := require.New(t)

	for _, tt := range testCases {
		defer tt.dir.Remove()

		t.Run(tt.name, func(_ *testing.T) {
			// Create noteSet from test directory
			root, err := newVaultPath(tt.dir.Path())
			r.NoError(err)
			ns, err := listNotes(root)
			r.NoError(err)

			// Test collectTags
			result := collectTags(ns, tt.ignoredTags)

			// Verify results
			r.Equal(tt.want, result.Tags)
			r.Equal(ns.hash, result.Hash) // Hash should be preserved
		})
	}
}

func Test_newTagCountsFromCache(t *testing.T) {
	testCases := []struct {
		name string
		dir  *fs.Dir
		want tagCounts
	}{
		{
			name: "reads from .tobi.json",
			dir: fs.NewDir(t, "test",
				fs.WithFile(".tobi.json", `{"tags":{"golang":5,"cobra":3},"hash":12345678901234567890}`),
			),
			want: tagCounts{
				Tags: map[string]int{
					"golang": 5,
					"cobra":  3,
				},
				Hash: 12345678901234567890,
			},
		},
	}

	r := require.New(t)

	for _, tt := range testCases {
		defer tt.dir.Remove()

		t.Run(tt.name, func(_ *testing.T) {
			root, err := newVaultPath(tt.dir.Path())
			r.NoError(err)

			result, err := newTagCountsFromCache(root)
			r.NoError(err)

			r.Equal(tt.want, result)
		})
	}
}

func Test_tagCounts_writeCache(t *testing.T) {
	testCases := []struct {
		name      string
		tagCounts tagCounts
		wantJSON  string
	}{
		{
			name: "writes to .tobi.json with proper formatting",
			tagCounts: tagCounts{
				Tags: map[string]int{
					"golang": 5,
					"cobra":  3,
				},
				Hash: 12345678901234567890,
			},
			wantJSON: "{\n\t\"tags\": {\n\t\t\"cobra\": 3,\n\t\t\"golang\": 5\n\t},\n\t\"hash\": 12345678901234567890\n}\n",
		},
	}

	r := require.New(t)

	for _, tt := range testCases {
		t.Run(tt.name, func(_ *testing.T) {
			dir := fs.NewDir(t, "test")
			defer dir.Remove()

			root, err := newVaultPath(dir.Path())
			r.NoError(err)

			// Write cache
			err = tt.tagCounts.writeCache(root)
			r.NoError(err)

			// Verify file was created at correct location
			content, err := os.ReadFile(root.cachePath())
			r.NoError(err)

			// Verify JSON content matches expected format
			r.Equal(tt.wantJSON, string(content))
		})
	}
}
