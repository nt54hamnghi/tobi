package tagextract

import (
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/sourcegraph/conc/pool"
)

func Extract(s string) ([]string, error) {
	marker := "---\n"

	// add a newline handle notes with only frontmatter and the closing marker doesn't end with a newline.
	// this ensures SplitN returns 3 parts instead of 2, so frontmatter gets parsed correctly
	parts := strings.SplitN(s+"\n", marker, 3)

	if len(parts) != 3 {
		return fromBody(s)
	}
	if parts[0] != "" {
		return fromBody(s)
	}

	p := pool.NewWithResults[[]string]().WithErrors().WithMaxGoroutines(2)
	p.Go(func() ([]string, error) {
		return fromFrontmatter(parts[1])
	})
	p.Go(func() ([]string, error) {
		return fromBody(parts[2])
	})

	res, err := p.Wait()
	if err != nil {
		return nil, err
	}

	var tags []string
	for _, r := range res {
		tags = append(tags, r...)
	}

	return tags, nil
}

var (
	frontmatterTagRegex = regexp.MustCompile(`^#?([a-zA-Z0-9_/-]+)$`)
	inlineTagRegex      = regexp.MustCompile(`(?:^|\s)#([A-Za-z0-9_/-]+)`)
	allNumericRegex     = regexp.MustCompile(`^[0-9]+$`)
)

func fromFrontmatter(s string) ([]string, error) {
	var fm struct {
		Tags []string `yaml:"tags"`
	}

	if err := yaml.Unmarshal([]byte(s), &fm); err != nil {
		return nil, err
	}

	tags := make([]string, 0, len(fm.Tags))
	for _, tag := range fm.Tags {
		matches := frontmatterTagRegex.FindStringSubmatch(tag)
		if len(matches) != 2 {
			// log.Printf("invalid tag format in frontmatter: %s", tag)
			continue
		}
		tag = matches[1]
		if allNumericRegex.MatchString(tag) {
			// log.Printf("fromFrontmatter: tags containing only numbers are not allowed: %s", tag)
			continue
		}
		tags = append(tags, tag)
	}

	return tags, nil
}

func fromBody(s string) ([]string, error) {
	matches := inlineTagRegex.FindAllStringSubmatch(s, -1)

	var tags []string
	for _, m := range matches {
		// match[0] is the full match
		subMatches := m[1:]
		for _, sm := range subMatches {
			if allNumericRegex.MatchString(sm) {
				// log.Printf("fromBody: tags containing only numbers are not allowed: %s", sm)
				continue
			}
			tags = append(tags, sm)
		}
	}

	return tags, nil
}
