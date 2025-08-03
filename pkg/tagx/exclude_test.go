package tagx

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gotest.tools/v3/fs"

	set "github.com/deckarep/golang-set/v2"
)

func TestLoadPatterns(t *testing.T) {
	testCases := []struct {
		name        string
		fileContent string
		nonExistent bool
		want        []string
	}{
		{
			name:        "single line",
			fileContent: "golang",
			want:        []string{"golang"},
		},
		{
			name:        "multiple lines",
			fileContent: "golang/*\ncobra",
			want:        []string{"cobra", "golang/*"},
		},
		{
			name:        "skip duplicate lines",
			fileContent: "golang\ngolang\ncobra",
			want:        []string{"cobra", "golang"},
		},
		{
			name:        "skip empty lines",
			fileContent: "golang\n\n\ncobra",
			want:        []string{"cobra", "golang"},
		},
		{
			name:        "skip comments",
			fileContent: "#golang\n\n\ncobra",
			want:        []string{"cobra"},
		},
		{
			name:        "empty file",
			fileContent: "",
			want:        []string{},
		},
		{
			name:        "non-existent file",
			nonExistent: true,
			want:        []string{},
		},
	}

	r := require.New(t)

	for _, tt := range testCases {
		var dir *fs.Dir

		if tt.nonExistent {
			dir = fs.NewDir(t, "test")
		} else {
			dir = fs.NewDir(t, "test",
				fs.WithFile(".tobi.exclude", tt.fileContent),
			)
		}

		defer dir.Remove()

		t.Run(tt.name, func(_ *testing.T) {
			root := dir.Join(".tobi.exclude")

			actual, err := readExcludePatterns(root)
			r.NoError(err)

			r.Equal(tt.want, set.Sorted(actual))
		})
	}
}

func TestTagGlobs(t *testing.T) {
	testCases := []struct {
		name        string
		fileContent string
		wantGlobs   int
		wantMatch   map[string]bool
		nonExistent bool
	}{
		{
			name:        "single pattern",
			fileContent: "golang",
			wantGlobs:   1,
			wantMatch: map[string]bool{
				"golang": true,
				"python": false,
			},
		},
		{
			name:        "multiple patterns",
			fileContent: "golang\ngolang/*\n",
			wantGlobs:   2,
			wantMatch: map[string]bool{
				"golang":       true,
				"golang/cobra": true,
				"python":       false,
			},
		},
		{
			name:        "with comments",
			fileContent: "#comment\ngolang\n\n#another comment\ncobra\n\n",
			wantGlobs:   2,
			wantMatch: map[string]bool{
				"golang":  true,
				"cobra":   true,
				"comment": false, // comments are ignored
			},
		},
		{
			name:        "empty file",
			fileContent: "",
			wantGlobs:   0,
			wantMatch: map[string]bool{
				"any-tag": false, // no patterns, nothing matches
			},
		},
		{
			name:        "only comments",
			fileContent: "#comment\n#another",
			wantGlobs:   0,
			wantMatch: map[string]bool{
				"any-tag": false, // no actual patterns, nothing matches
			},
		},
		{
			name:        "non-existent file",
			nonExistent: true,
			wantGlobs:   0,
			wantMatch: map[string]bool{
				"any-tag": false, // no patterns, nothing matches
			},
		},
	}

	r := require.New(t)

	for _, tt := range testCases {
		var dir *fs.Dir

		if tt.nonExistent {
			dir = fs.NewDir(t, "test")
		} else {
			dir = fs.NewDir(t, "test",
				fs.WithFile(".tobi.exclude", tt.fileContent),
			)
		}
		defer dir.Remove()

		t.Run(tt.name, func(_ *testing.T) {
			excludePath := dir.Join(".tobi.exclude")
			tg, err := NewTagGlobs(excludePath)

			r.NoError(err)
			r.Len(tg.Globs, tt.wantGlobs)

			for tag, want := range tt.wantMatch {
				r.Equal(want, tg.Match(tag))
			}
		})
	}
}

func TestTagGlobs_ErrorCases(t *testing.T) {
	testCases := []struct {
		name        string
		fileContent string
	}{
		{
			name:        "invalid glob pattern with unclosed bracket",
			fileContent: "golang\n[invalid\ncobra",
		},
		// FIXME: gobwas/glob doesn’t reject unclosed "{"
		// {
		// 	name:        "invalid glob pattern with unmatched brace",
		// 	fileContent: "valid\n{unclosed\nother",
		// },
	}

	r := require.New(t)

	for _, tt := range testCases {
		dir := fs.NewDir(t, "test",
			fs.WithFile(".tobi.exclude", tt.fileContent),
		)
		defer dir.Remove()

		t.Run(tt.name, func(_ *testing.T) {
			excludePath := dir.Join(".tobi.exclude")
			_, err := NewTagGlobs(excludePath)
			r.Error(err)
		})
	}
}
