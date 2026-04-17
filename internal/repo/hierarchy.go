package repo

import "bruv/internal/model"

// CategoryFlat is one category along with the Brand/Stream/Project
// chain it sits inside. Flat walks are cheaper than nested walks
// when several call sites (PinPicker, healTagColors, chat system
// prompts) all need "every category with its full context" — each
// caller doing its own brand → stream → project → category walk
// would repeat the same filesystem reads many times.
type CategoryFlat struct {
	Brand    model.Brand
	Stream   model.Stream
	Project  model.Project
	Category model.Category
}

// ListAllCategoriesFlat walks the whole brand/stream/project/category
// hierarchy once and returns every category along with its parent
// chain. Ordering: the natural traversal order (brands in list order,
// streams within each brand in list order, etc.). Errors at each
// level are swallowed — partial data is more useful than no data
// when rendering a flat list of pin targets, and any individual
// read failure already surfaces via the corresponding CRUD method.
func (r *Repository) ListAllCategoriesFlat() ([]CategoryFlat, error) {
	brands, err := r.ListBrands()
	if err != nil {
		return nil, err
	}
	// Pre-allocate for a typical repo size; slice grows if needed.
	out := make([]CategoryFlat, 0, 128)
	for _, b := range brands {
		streams, _ := r.ListStreams(b.Slug)
		for _, s := range streams {
			projects, _ := r.ListProjects(b.Slug, s.Slug)
			for _, p := range projects {
				cats, _ := r.ListCategories(b.Slug, s.Slug, p.Slug)
				for _, c := range cats {
					out = append(out, CategoryFlat{
						Brand:    b,
						Stream:   s,
						Project:  p,
						Category: c,
					})
				}
			}
		}
	}
	return out, nil
}
