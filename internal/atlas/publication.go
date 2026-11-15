package atlas

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var photoName = regexp.MustCompile(`^[a-f0-9]{48}\.jpg$`)

func (s *Server) photo(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !photoName.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, filepath.Join(s.Maps.Dir, "runtime", "uploads", name))
}

func (s *Server) removePhotos(paths []string) {
	for _, path := range paths {
		name := strings.TrimPrefix(path, "/uploads/")
		if photoName.MatchString(name) {
			_ = os.Remove(filepath.Join(s.Maps.Dir, "runtime", "uploads", name))
		}
	}
}

// Decode then encode only pixels: original names, EXIF/GPS and other metadata never survive.
func (s *Server) savePhotos(files []*multipart.FileHeader) (paths []string, err error) {
	defer func() {
		if err != nil {
			s.removePhotos(paths)
		}
	}()
	directory := filepath.Join(s.Maps.Dir, "runtime", "uploads")
	if err = os.MkdirAll(directory, 0700); err != nil {
		return nil, errors.New("照片存储暂不可用")
	}
	for _, header := range files {
		if header.Size <= 0 || header.Size > 5<<20 {
			return paths, errors.New("每张照片需小于等于 5 MB")
		}
		file, e := header.Open()
		if e != nil {
			return paths, errors.New("无法读取照片")
		}
		config, format, e := image.DecodeConfig(file)
		if e != nil || (format != "jpeg" && format != "png") || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 12000000 {
			file.Close()
			return paths, errors.New("照片仅支持 JPEG / PNG，且最多 1200 万像素")
		}
		if _, e = file.Seek(0, io.SeekStart); e != nil {
			file.Close()
			return paths, errors.New("无法读取照片")
		}
		pixels, _, e := image.Decode(file)
		file.Close()
		if e != nil {
			return paths, errors.New("照片损坏，请重新选择")
		}
		token, e := randomToken(24)
		if e != nil {
			return paths, errors.New("无法保存照片")
		}
		name := token + ".jpg"
		output, e := os.OpenFile(filepath.Join(directory, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return paths, errors.New("照片存储暂不可用")
		}
		paths = append(paths, "/uploads/"+name)
		e = jpeg.Encode(output, pixels, &jpeg.Options{Quality: 85})
		closeErr := output.Close()
		if e != nil || closeErr != nil {
			return paths, errors.New("无法保存照片")
		}
	}
	return paths, nil
}

func (s *Server) locationInDistrict(ctx context.Context, district Region, point [2]float64) (bool, error) {
	if math.IsNaN(point[0]) || math.IsNaN(point[1]) || math.IsInf(point[0], 0) || math.IsInf(point[1], 0) || point[0] < -180 || point[0] > 180 || point[1] < -90 || point[1] > 90 {
		return false, nil
	}
	data, err := s.Maps.Read(ctx, district.Parent)
	if err != nil {
		return false, err
	}
	var collection struct {
		Features []struct {
			Properties struct {
				Adcode json.Number `json:"adcode"`
			} `json:"properties"`
			Geometry struct {
				Type        string          `json:"type"`
				Coordinates json.RawMessage `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err = json.Unmarshal(data, &collection); err != nil {
		return false, err
	}
	for _, f := range collection.Features {
		if string(f.Properties.Adcode) != district.Code {
			continue
		}
		var polygons [][][][2]float64
		switch f.Geometry.Type {
		case "Polygon":
			var rings [][][2]float64
			err = json.Unmarshal(f.Geometry.Coordinates, &rings)
			polygons = append(polygons, rings)
		case "MultiPolygon":
			err = json.Unmarshal(f.Geometry.Coordinates, &polygons)
		default:
			return false, errors.New("unsupported district geometry")
		}
		if err != nil {
			return false, err
		}
		for _, polygon := range polygons {
			if len(polygon) == 0 || !inRing(point, polygon[0]) {
				continue
			}
			inHole := false
			for _, hole := range polygon[1:] {
				if inRing(point, hole) {
					inHole = true
					break
				}
			}
			if !inHole {
				return true, nil
			}
		}
		return false, nil
	}
	return false, errors.New("district boundary missing")
}

func inRing(p [2]float64, ring [][2]float64) bool {
	inside := false
	if len(ring) < 3 {
		return false
	}
	for i, j := 0, len(ring)-1; i < len(ring); j, i = i, i+1 {
		a, b := ring[j], ring[i]
		cross := (p[0]-a[0])*(b[1]-a[1]) - (p[1]-a[1])*(b[0]-a[0])
		if math.Abs(cross) < 1e-10 && p[0] >= math.Min(a[0], b[0])-1e-10 && p[0] <= math.Max(a[0], b[0])+1e-10 && p[1] >= math.Min(a[1], b[1])-1e-10 && p[1] <= math.Max(a[1], b[1])+1e-10 {
			return true
		}
		if (a[1] > p[1]) != (b[1] > p[1]) && p[0] < (b[0]-a[0])*(p[1]-a[1])/(b[1]-a[1])+a[0] {
			inside = !inside
		}
	}
	return inside
}
