package messages

import (
	"errors"
	"regexp"
	"strings"
)

type CategoryInput struct {
	Slug, DisplayName, Definition string
	SortOrder                     int
}

var categorySlugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9_-]{0,62}[a-z0-9])?$`)

func NormalizeCategoryInput(in CategoryInput) (CategoryInput, error) {
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.Definition = strings.TrimSpace(in.Definition)
	if !categorySlugPattern.MatchString(in.Slug) {
		return in, errors.New("slug must be 1-64 chars: lowercase letters, digits, '_' or '-'")
	}
	if in.DisplayName == "" {
		return in, errors.New("display_name is required")
	}
	if len(in.Definition) > 280 {
		return in, errors.New("definition is too long (max 280 chars)")
	}
	return in, nil
}
