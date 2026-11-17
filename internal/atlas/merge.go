package atlas

import (
	"errors"
	"net/url"
	"strings"
)

const maxMergedDistricts = 16

func districtCodes(q url.Values) ([]string, error) {
	values, present := q["districts"]
	if !present {
		return nil, nil
	}
	if len(values) != 1 || len(values[0]) > maxMergedDistricts*7-1 {
		return nil, errors.New("合区参数无效，最多选择 16 个区县")
	}
	codes := strings.Split(values[0], ",")
	if len(codes) < 2 || len(codes) > maxMergedDistricts {
		return nil, errors.New("合区需选择 2–16 个区县")
	}
	seen := map[string]bool{}
	for _, code := range codes {
		if !validCode.MatchString(code) || seen[code] {
			return nil, errors.New("合区代码无效或重复")
		}
		seen[code] = true
	}
	return codes, nil
}

func (s *Server) validateDistrictMerge(q url.Values) error {
	codes, err := districtCodes(q)
	if err != nil {
		return err
	}
	parent := ""
	for _, code := range codes {
		region, ok := s.Maps.Get(code)
		if !ok || region.Level != "district" || region.Parent == "" {
			return errors.New("合区仅支持有效区县")
		}
		if parent != "" && parent != region.Parent {
			return errors.New("请选择同一城市下的区县")
		}
		parent = region.Parent
	}
	return nil
}
