package tagextract

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_fromFrontmatter(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name:  "valid",
			input: "tags:\n  - golang\n  - cobra",
			want:  []string{"golang", "cobra"},
		},
		{
			name:  "with underscores and dashes",
			input: "tags:\n  - cli-dev\n  - cobra_cli",
			want:  []string{"cli-dev", "cobra_cli"},
		},
		{
			name:  "with forward slashes",
			input: "tags:\n  - golang/cobra\n  - golang/cobra/Command",
			want:  []string{"golang/cobra", "golang/cobra/Command"},
		},
		{
			name:  "with hash prefixes",
			input: "tags:\n  - \"#golang\"\n  - \"#cobra\"\n  - \"#golang\"",
			want:  []string{"golang", "cobra", "golang"},
		},
		{
			name:  "skip tags with only numbers",
			input: "tags:\n  - 123\n  - golang\n",
			want:  []string{"golang"},
		},
		{
			name:  "skip tags with invalid format",
			input: "tags:\n  - \"##cobra\"\n  - golang\n",
			want:  []string{"golang"},
		},
		{
			name:  "empty tags array",
			input: "tags: []",
			want:  []string{},
		},
		{
			name:  "with other fields",
			input: "title: My Note\ntags:\n  - golang\n  - cobra\nauthor: test",
			want:  []string{"golang", "cobra"},
		},
		{
			name:  "no tags field",
			input: "title: My Note",
			want:  []string{},
		},
		{
			name:  "empty string input",
			input: "",
			want:  []string{},
		},
		{
			name:    "invalid YAML syntax",
			input:   "tags:\n  - golang\n  - cobra\n invalid: [",
			wantErr: true,
		},
		{
			name:    "tags field with wrong type (string instead of array)",
			input:   "tags: \"golang,cobra,cli\"",
			wantErr: true,
		},
	}

	r := require.New(t)

	for _, tt := range testCases {
		t.Run(tt.name, func(_ *testing.T) {
			result, err := fromFrontmatter(tt.input)

			if tt.wantErr {
				r.Error(err)
				r.Nil(result)
				return
			}

			r.NoError(err)
			r.Equal(tt.want, result)
		})
	}
}

func Test_fromBody(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "single inline tag",
			input: "This is a note about #golang programming.",
			want:  []string{"golang"},
		},
		{
			name:  "multiple inline tags",
			input: "Working with #golang and #cobra for #cli development.",
			want:  []string{"golang", "cobra", "cli"},
		},
		{
			name:  "consecutive tags",
			input: "Working with #golang #cobra #cli development.",
			want:  []string{"golang", "cobra", "cli"},
		},
		{
			name:  "with underscores and dashes",
			input: "Learning #cli-dev and #cobra_cli development.",
			want:  []string{"cli-dev", "cobra_cli"},
		},
		{
			name:  "with forward slashes",
			input: "Studying #golang/cobra and #golang/cobra/Command topics.",
			want:  []string{"golang/cobra", "golang/cobra/Command"},
		},
		{
			name:  "skip tags with only numbers",
			input: "Year #2024 was great for #12-factor-app development.",
			want:  []string{"12-factor-app"},
		},
		{
			name:  "no tags in text",
			input: "This is just regular text without any tags.",
			want:  nil,
		},
		{
			name:  "skip tags without leading whitespace",
			input: "Text#golang and#cobra are not matched.",
			want:  nil,
		},
		{
			name:  "tags at start and end of lines",
			input: "#golang is awesome\n#cli tools are useful",
			want:  []string{"golang", "cli"},
		},
		{
			name:  "tags in markdown context",
			input: "## Header\n\nSome text about #golang and #cobra tools.\n\n- List item with #cli tag\n",
			want:  []string{"golang", "cobra", "cli"},
		},
	}

	r := require.New(t)

	for _, tt := range testCases {
		t.Run(tt.name, func(_ *testing.T) {
			result, err := fromBody(tt.input)

			r.NoError(err)
			r.Equal(tt.want, result)
		})
	}
}

func Test_extract(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "frontmatter and body tags",
			input: "---\ntags: [golang, cobra]\n---\nThis is about #cli development.",
			want:  []string{"golang", "cobra", "cli"},
		},
		{
			name:  "frontmatter only",
			input: "---\ntags: [golang, cobra]\n---",
			want:  []string{"golang", "cobra"},
		},
		{
			name:  "frontmatter only with body content but no tags",
			input: "---\ntags: [golang]\n---\nJust some regular text without tags.",
			want:  []string{"golang"},
		},
		{
			name:  "body only",
			input: `This is about #golang and #cobra development.`,
			want:  []string{"golang", "cobra"},
		},
		{
			// treat as body
			name:  "invalid frontmatter - missing closing marker",
			input: "---\ntags: [golang]\nThis should be treated as body #cli content.",
			want:  []string{"cli"},
		},
		{
			// treat as body
			name:  "invalid frontmatter - content before opening marker",
			input: "Some text\n---\ntags: [golang]\nBody with #cobra tag.",
			want:  []string{"cobra"},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "frontmatter with no tags field",
			input: "---\ntitle: My Note\nauthor: test\n---\nContent with #golang tag.",
			want:  []string{"golang"},
		},
	}
	r := require.New(t)

	for _, tt := range testCases {
		t.Run(tt.name, func(_ *testing.T) {
			result, err := Extract(tt.input)

			r.NoError(err)
			slices.Sort(result)
			slices.Sort(tt.want)
			r.Equal(tt.want, result)
		})
	}
}
