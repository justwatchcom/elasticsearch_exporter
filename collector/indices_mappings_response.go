package collector

// IndicesMappingsResponse is a representation of elasticsearch mappings for each index
type IndicesMappingsResponse map[string]IndexMapping

// IndexMapping defines the struct of the tree for the mappings of each index
type IndexMapping struct {
	Mappings IndexMappings `json:"mappings"`
}

// IndexMappings defines all index mappings
type IndexMappings struct {
	Properties IndexMappingProperties `json:"properties"`
}

// IndexMappingProperties defines all the properties of the current mapping
type IndexMappingProperties map[string]*IndexMappingProperty

// IndexMappingFields defines all the fields of the current mapping
type IndexMappingFields map[string]*IndexMappingField

// IndexMappingProperty defines a single property of the current index properties
type IndexMappingProperty struct {
	Type       *string                `json:"type"`
	Properties IndexMappingProperties `json:"properties"`
	Fields     IndexMappingFields     `json:"fields"`
}

// IndexMappingField defines a single property of the current index field
type IndexMappingField struct {
	Type       *string                `json:"type"`
	Properties IndexMappingProperties `json:"properties"`
	Fields     IndexMappingFields     `json:"fields"`
}
