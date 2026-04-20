package domain

type Category struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	ParentID string `json:"parentId,omitempty"`
}
