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

func Test_collectTags(t *testing.T) {
	noIgnore := func(string) bool {
		return false
	}

	testCases := []struct {
		name      string
		dir       *fs.Dir
		filter    func(string) bool
		want      map[string]int
		wantTotal int
	}{
		{
			name: "single file",
			dir: fs.NewDir(t, "test",
				fs.WithFile("note1.md", "---\ntags: [golang, cobra, cli]\n---\nContent"),
			),
			filter: noIgnore,
			want: map[string]int{
				"golang": 1,
				"cobra":  1,
				"cli":    1,
			},
			wantTotal: 3,
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
			filter: noIgnore,
			want: map[string]int{
				"golang": 2,
				"cobra":  2,
				"cli":    1,
			},
			wantTotal: 5,
		},
		{
			name: "remove hash prefix",
			dir: fs.NewDir(t, "test",
				fs.WithFile("note1.md", "---\ntags: [\"#golang\", golang]\n---\nContent"),
			),
			filter: noIgnore,
			want: map[string]int{
				"golang": 2,
			},
			wantTotal: 2,
		},
		{
			name: "with filter",
			dir: fs.NewDir(t, "test",
				fs.WithFile("note1.md", "---\ntags: [golang, daily]\n---\nContent"),
			),
			filter: func(s string) bool {
				return s == "daily"
			},
			want: map[string]int{
				"golang": 1,
			},
			wantTotal: 1,
		},
		{
			name: "skip files with errors",
			dir: fs.NewDir(t, "test",
				fs.WithFile("invalid.md", "---\ntags: [invalid: yaml\n---\nContent"),
			),
			filter:    noIgnore, // shouldn't need this but include for completeness
			want:      map[string]int{},
			wantTotal: 0,
		},
		{
			name: "ignore all tags",
			dir: fs.NewDir(t, "test",
				fs.WithFile("note1.md", "---\ntags: [daily, personal]\n---\nContent"),
			),
			filter: func(string) bool {
				return true
			},
			want:      map[string]int{},
			wantTotal: 0,
		},
		{
			name:      "empty noteSet",
			dir:       fs.NewDir(t, "test"),
			filter:    noIgnore, // shouldn't need this but include for completeness
			want:      map[string]int{},
			wantTotal: 0,
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
			result := collectTags(ns, tt.filter)

			// Verify results
			r.Equal(tt.want, result.Tags)
			r.Equal(tt.wantTotal, result.Total)
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
				Hash:  12345678901234567890,
				Total: 8,
			},
			wantJSON: "{\n\t\"tags\": {\n\t\t\"cobra\": 3,\n\t\t\"golang\": 5\n\t},\n\t\"hash\": 12345678901234567890,\n\t\"total\": 8\n}\n",
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

func Test_tagCounts_fPrint_limit(t *testing.T) {
	// Common test data sorted by count: rust(150), golang(100), python(50)
	common := tagCounts{
		Tags: map[string]int{
			"golang": 100,
			"rust":   150,
			"python": 50,
		},
		Total: 300,
	}

	testCases := []struct {
		name     string
		tc       tagCounts
		limit    int
		expected string
	}{
		{
			name:     "positive limit",
			tc:       common,
			limit:    2,
			expected: "rust\ngolang\n",
		},
		{
			name:     "zero limit shows all",
			tc:       common,
			limit:    0,
			expected: "rust\ngolang\npython\n",
		},
		{
			name:     "negative limit shows all",
			tc:       common,
			limit:    -1,
			expected: "rust\ngolang\npython\n",
		},
		{
			name:     "limit exceeds available tags",
			tc:       common,
			limit:    10,
			expected: "rust\ngolang\npython\n",
		},
		{
			name:     "limit equals available tags",
			tc:       common,
			limit:    3,
			expected: "rust\ngolang\npython\n",
		},
		{
			name: "empty tagCounts",
			tc: tagCounts{
				Tags:  map[string]int{},
				Total: 0,
			},
			limit:    5,
			expected: "",
		},
	}

	r := require.New(t)

	for _, tt := range testCases {
		t.Run(tt.name, func(*testing.T) {
			var buf strings.Builder
			opts := rootOptions{
				limit:       tt.limit,
				displayMode: name, // Use name mode for simplicity
			}
			tt.tc.fPrint(&buf, opts)

			r.Equal(tt.expected, buf.String())
		})
	}
}

func Test_tagCounts_fPrint_displayMode(t *testing.T) {
	tc := tagCounts{
		Tags: map[string]int{
			"rust":   150,
			"golang": 100,
			"python": 50,
		},
		Total: 300,
	}

	testCases := []struct {
		name        string
		displayMode displayMode
		expected    string
	}{
		{
			name:        "name mode",
			displayMode: name,
			expected:    "rust\ngolang\npython\n",
		},
		{
			name:        "count mode",
			displayMode: count,
			expected:    "150  rust\n100  golang\n50   python\n",
		},
		{
			name:        "relative mode",
			displayMode: relative,
			expected:    "50.000  rust\n33.333  golang\n16.667  python\n",
		},
	}

	r := require.New(t)

	for _, tt := range testCases {
		t.Run(tt.name, func(*testing.T) {
			var buf strings.Builder
			opts := rootOptions{
				limit:       -1, // Show all for display mode testing
				displayMode: tt.displayMode,
			}
			tc.fPrint(&buf, opts)

			r.Equal(tt.expected, buf.String())
		})
	}
}
