package cmd

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

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

func extractTagsFromYAML(data []byte) ([]string, error) {
	var fm struct {
		Tags []string `yaml:"tags"`
	}

	if err := yaml.Unmarshal(data, &fm); err != nil {
		return nil, err
	}

	return fm.Tags, nil
}
