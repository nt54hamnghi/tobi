package gitignore

import (
	"testing"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/stretchr/testify/require"
	"gotest.tools/v3/fs"
)

func Test_readIgnoreFile(t *testing.T) {
	testCases := []struct {
		name          string
		dir           *fs.Dir
		relIgnoreFile string
		patternCount  int
		items         []string
		want          []bool
	}{
		{
			name: "single",
			dir: fs.NewDir(t, "test",
				fs.WithFile(".gitignore", "*.log"),
			),
			relIgnoreFile: ".gitignore",
			patternCount:  1,
			items:         []string{"debug.log", "main.go"},
			want:          []bool{true, false},
		},
		{
			name: "nested single",
			dir: fs.NewDir(t, "test",
				fs.WithDir("level1",
					fs.WithFile(".gitignore", "*.log\n"),
				),
			),
			relIgnoreFile: "level1/.gitignore",
			patternCount:  1,
			items:         []string{"debug.log", "main.go", "level1/debug.log"},
			want:          []bool{false, false, true},
		},
		{
			name: "multiple",
			dir: fs.NewDir(t, "test",
				fs.WithFile(".gitignore", "*.log\n*.tmp"),
			),
			relIgnoreFile: ".gitignore",
			patternCount:  2,
			items:         []string{"debug.log", "main.go", "data.tmp"},
			want:          []bool{true, false, true},
		},
		{
			name: "multiple with empty line and comment",
			dir: fs.NewDir(t, "test",
				fs.WithFile(".gitignore", "*.log\n\n*.tmp\n# comment"),
			),
			relIgnoreFile: ".gitignore",
			patternCount:  2,
			items:         []string{"debug.log", "main.go", "data.tmp"},
			want:          []bool{true, false, true},
		},
	}

	r := require.New(t)

	for _, tt := range testCases {
		defer tt.dir.Remove()

		t.Run(tt.name, func(_ *testing.T) {
			path, err := NewAbsolutePath(tt.dir.Join(tt.relIgnoreFile))
			r.NoError(err)
			ps, err := readIgnoreFile(path)

			r.NoError(err)
			r.Len(ps, tt.patternCount)

			m := gitignore.NewMatcher(ps)
			for i, f := range tt.items {
				path := splitPath(tt.dir.Join(f))
				r.Equal(tt.want[i], m.Match(path, false))
			}
		})
	}
}

func TestReadPatterns(t *testing.T) {
	testCases := []struct {
		name         string
		dir          *fs.Dir
		patternCount int
		items        []string
		want         []bool
	}{
		{
			name: "no .gitignore files",
			dir: fs.NewDir(t, "test",
				fs.WithDir("level1",
					fs.WithFile("note2.md", "content"),
				),
				fs.WithFile("note.md", "content"),
			),
			patternCount: 0,
			items:        []string{"note.md", "level1/note2.md"},
			want:         []bool{false, false},
		},
		{
			name: "root .gitignore only",
			dir: fs.NewDir(t, "test",
				fs.WithFiles(map[string]string{
					".gitignore": "*.log",
					"note.md":    "content",
					"debug.log":  "logs",
				}),
			),
			patternCount: 1,
			items:        []string{"debug.log", "note.md"},
			want:         []bool{true, false},
		},
		{
			name: "root .tobiignore only",
			dir: fs.NewDir(t, "test",
				fs.WithFiles(map[string]string{
					".tobiignore": "*.log",
					"note.md":     "content",
					"debug.log":   "logs",
				}),
			),
			patternCount: 1,
			items:        []string{"debug.log", "note.md"},
			want:         []bool{true, false},
		},
		{
			name: "root .gitignore and .tobiignore",
			dir: fs.NewDir(t, "test",
				fs.WithDir("level1",
					fs.WithFile("note2.md", "content"),
				),
				fs.WithFiles(map[string]string{
					".gitignore":  "level1/",
					".tobiignore": "*.log",
					"note.md":     "content",
					"debug.log":   "logs",
				}),
			),
			patternCount: 2,
			items:        []string{"debug.log", "note.md", "level1/note2.md"},
			want:         []bool{true, false, true},
		},
		{
			name: "nested gitignore files",
			dir: fs.NewDir(t, "test",
				fs.WithDir("level1",
					fs.WithFiles(map[string]string{
						".gitignore": "*.tmp",
						"data.tmp":   "temp",
						"note2.md":   "content",
					}),
				),
				fs.WithFiles(map[string]string{
					".gitignore": "*.log",
					"note.md":    "content",
					"debug.log":  "logs",
				}),
			),
			patternCount: 2,
			items:        []string{"debug.log", "note.md", "level1/data.tmp", "level1/note2.md"},
			want:         []bool{true, false, true, false},
		},
		{
			name: "git info exclude",
			dir: fs.NewDir(t, "test",
				fs.WithDir(".git",
					fs.WithDir("info",
						fs.WithFile("exclude", "*.log"),
					),
				),
				fs.WithFiles(map[string]string{
					"note.md":   "content",
					"debug.log": "logs",
				}),
			),
			patternCount: 1,
			items:        []string{"debug.log", "note.md"},
			want:         []bool{true, false},
		},
		{
			name: "git info exclude with gitignore",
			dir: fs.NewDir(t, "test",
				fs.WithDir(".git",
					fs.WithDir("info",
						fs.WithFile("exclude", "*.log"),
					),
				),
				fs.WithFiles(map[string]string{
					".gitignore": "*.tmp",
					"note.md":    "content",
					"debug.log":  "logs",
					"data.tmp":   "temp",
				}),
			),
			patternCount: 2,
			items:        []string{"debug.log", "data.tmp", "note.md"},
			want:         []bool{true, true, false},
		},
		{
			name: "skips git directory",
			dir: fs.NewDir(t, "test",
				fs.WithFile("note.md", "content"),
				fs.WithDir(".git",
					fs.WithFile(".gitignore", "should-not-be-processed"),
					fs.WithDir("objects",
						fs.WithFile(".gitignore", "should-not-be-processed"),
					),
				),
			),
			patternCount: 0, // No patterns because .git/.gitignore files are skipped
			items:        []string{"note.md"},
			want:         []bool{false},
		},
	}

	r := require.New(t)

	for _, tt := range testCases {
		defer tt.dir.Remove()

		t.Run(tt.name, func(_ *testing.T) {
			// Execute
			root, err := NewAbsolutePath(tt.dir.Path())
			r.NoError(err)
			ps, err := ReadPatterns(root)

			// Assert
			r.NoError(err)
			r.Len(ps, tt.patternCount)

			// Test pattern matching
			m := gitignore.NewMatcher(ps)
			for i, f := range tt.items {
				path := splitPath(tt.dir.Join(f))
				matched := m.Match(path, false)
				r.Equal(tt.want[i], matched,
					"Path %q should match=%v but got match=%v", f, tt.want[i], matched)
			}
		})
	}
}

func TestReadPatterns_ErrorHandling(t *testing.T) {
	testCases := []struct {
		name        string
		path        string
		expectedErr string
	}{
		{
			name:        "non-existent directory",
			path:        "/nonexistent/directory",
			expectedErr: "no such file or directory",
		},
	}

	r := require.New(t)

	for _, tt := range testCases {
		t.Run(tt.name, func(_ *testing.T) {
			// Execute
			root, err := NewAbsolutePath(tt.path)
			r.NoError(err)
			ps, err := ReadPatterns(root)

			// Assert
			r.Error(err)
			r.Nil(ps)
			r.Contains(err.Error(), tt.expectedErr)
		})
	}
}
