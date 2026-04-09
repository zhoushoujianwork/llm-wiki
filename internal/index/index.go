package index

// Index maintains a mapping from concepts/entities to wiki pages
type Index struct {
	// Maps concept name → list of page paths
	entries map[string][]string
}

// New creates a new index
func New() *Index {
	return &Index{
		entries: make(map[string][]string),
	}
}

// Add adds an entry to the index
func (i *Index) Add(concept, pagePath string) {
	i.entries[concept] = append(i.entries[concept], pagePath)
}

// Get returns page paths for a concept
func (i *Index) Get(concept string) []string {
	return i.entries[concept]
}

// AllConcepts returns all indexed concepts
func (i *Index) AllConcepts() []string {
	var concepts []string
	for k := range i.entries {
		concepts = append(concepts, k)
	}
	return concepts
}
