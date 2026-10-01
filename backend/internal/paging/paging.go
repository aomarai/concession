// Package paging normalizes page/per_page parameters shared by list endpoints.
package paging

import "github.com/aomarai/concession/internal/svcerr"

const (
	DefaultPerPage = 20
	MaxPerPage     = 100
	// MaxPage bounds the page number so (page-1)*perPage cannot overflow into a
	// negative OFFSET, which the database would reject with a 500.
	MaxPage = 1_000_000
)

// Normalize applies defaults (0 means "unset") and limits. A per_page above
// MaxPerPage is clamped; other out-of-range values are validation errors.
func Normalize(page, perPage int) (int, int, error) {
	if page == 0 {
		page = 1
	}
	if perPage == 0 {
		perPage = DefaultPerPage
	}
	if page < 1 || perPage < 1 {
		return 0, 0, svcerr.Invalid("page and per_page must be positive")
	}
	if page > MaxPage {
		return 0, 0, svcerr.Invalid("page is too large")
	}
	return page, min(perPage, MaxPerPage), nil
}

// Offset returns the row offset for a normalized page.
func Offset(page, perPage int) int { return (page - 1) * perPage }
