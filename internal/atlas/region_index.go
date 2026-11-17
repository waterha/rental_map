package atlas

import (
	_ "embed"
	"encoding/json"
)

// This bundled name/center index enables district deep links without a remote
// district-search request. Detailed boundaries continue to load on demand.
// Source and refresh instructions: docs/place-search.md.
//
//go:embed region-index.json
var regionIndex []byte

func (m *Maps) addRegionIndex() error {
	var regions []Region
	if err := json.Unmarshal(regionIndex, &regions); err != nil {
		return err
	}
	for _, region := range regions {
		m.regions[region.Code] = region
	}
	return nil
}
