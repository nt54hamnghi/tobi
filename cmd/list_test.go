package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractFrontMatter(t *testing.T) {
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
