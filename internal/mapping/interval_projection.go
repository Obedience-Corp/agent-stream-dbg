package mapping

import (
	"fmt"
	"github.com/tidwall/gjson"
	"math"
)

const maxProjectedRows = 512
const maxProjectionBytes = 1 << 20
const maxProjectionItems = 256

// IntervalArraySpec projects measured array boundaries into generic interval
// evidence. Paths/units/stages remain dialect data, including nested gap rows.
type IntervalArraySpec struct {
	Path     string                 `yaml:"path"`
	Anchor   ValueForm              `yaml:"anchor,omitempty"`
	Fields   map[string]ValueForm   `yaml:"fields,omitempty"`
	Spans    []IntervalBoundarySpec `yaml:"spans,omitempty"`
	Children []IntervalArraySpec    `yaml:"children,omitempty"`
}
type IntervalBoundarySpec struct {
	Stage  string               `yaml:"stage"`
	Start  string               `yaml:"start"`
	End    string               `yaml:"end"`
	Scale  float64              `yaml:"scale"`
	Fields map[string]ValueForm `yaml:"fields,omitempty"`
}

func validateIntervalArrays(specs []IntervalArraySpec, depth int) error {
	if len(specs) > 8 || depth > 4 {
		return fmt.Errorf("interval projection exceeds bounded spec depth/count")
	}
	for _, spec := range specs {
		if spec.Path == "" || len(spec.Spans) > 8 || len(spec.Fields) > 16 {
			return fmt.Errorf("interval projection requires path and at most8 spans/16 fields")
		}
		for _, span := range spec.Spans {
			if span.Stage == "" || len(span.Stage) > 128 || span.Scale <= 0 || math.IsInf(span.Scale, 0) || math.IsNaN(span.Scale) || len(span.Fields) > 16 {
				return fmt.Errorf("invalid interval projection stage, scale or fields")
			}
		}
		if err := validateIntervalArrays(spec.Children, depth+1); err != nil {
			return err
		}
	}
	return nil
}
func projectionScalar(value any) (any, bool) {
	switch v := value.(type) {
	case nil, bool:
		return v, true
	case string:
		return v, len(v) <= 256
	case float64:
		return v, !math.IsNaN(v) && !math.IsInf(v, 0)
	case int:
		return float64(v), true
	default:
		return nil, false
	}
}
func projectedFields(forms map[string]ValueForm, data []byte, inherited map[string]any) map[string]any {
	out := make(map[string]any, len(inherited)+len(forms))
	for k, v := range inherited {
		out[k] = v
	}
	for k, form := range forms {
		if len(k) > 64 {
			continue
		}
		if value, ok := form.BoundedScalar(data); ok {
			if value, ok := projectionScalar(value); ok {
				out[k] = value
			}
		}
	}
	return out
}
func projectionNumber(value any) (float64, bool) {
	n, ok := value.(float64)
	return n, ok && !math.IsNaN(n) && !math.IsInf(n, 0)
}
func projectIntervalArrays(specs []IntervalArraySpec, root []byte) ([]any, bool) {
	rows := make([]any, 0)
	used := 0
	limited := false
	var visit func([]IntervalArraySpec, []byte, map[string]any)
	visit = func(specs []IntervalArraySpec, parent []byte, inherited map[string]any) {
		for _, spec := range specs {
			if limited {
				return
			}
			array := gjson.GetBytes(parent, spec.Path)
			if !array.IsArray() {
				if array.Exists() && array.Type != gjson.Null {
					limited = true
				}
				continue
			}
			anchor, anchored := 0.0, false
			if value, ok := spec.Anchor.BoundedScalar(root); ok {
				anchor, anchored = projectionNumber(value)
			}
			count := 0
			array.ForEach(func(_, item gjson.Result) bool {
				if count >= maxProjectionItems || len(rows) >= maxProjectedRows {
					limited = true
					return false
				}
				count++
				if !item.IsObject() || len(item.Raw) > 64<<10 {
					limited = true
					return false
				}
				raw := []byte(item.Raw)
				fields := projectedFields(spec.Fields, raw, inherited)
				for _, span := range spec.Spans {
					row := projectedFields(span.Fields, raw, fields)
					row["stage"] = span.Stage
					row["evidence"] = "final"
					row["relative"] = !anchored
					row["source_item_index"] = float64(count - 1)
					for key, path := range map[string]string{"start_ms": span.Start, "end_ms": span.End} {
						var value any
						result := gjson.GetBytes(raw, path)
						if result.Type == gjson.Number {
							n := result.Float() * span.Scale
							if anchored {
								n += anchor
							}
							if !math.IsNaN(n) && !math.IsInf(n, 0) {
								value = n
							} else {
								row["invalid_timing"] = true
							}
						}
						if result.Exists() && result.Type != gjson.Null && result.Type != gjson.Number {
							row["invalid_timing"] = true
						}
						row[key] = value
					}
					estimate := 512 + len(row)*192
					for _, value := range row {
						if text, ok := value.(string); ok {
							estimate += len(text)
						}
					}
					if len(rows) >= maxProjectedRows || used+estimate > maxProjectionBytes {
						limited = true
						return false
					}
					rows = append(rows, row)
					used += estimate
				}
				visit(spec.Children, raw, fields)
				return !limited
			})
		}
	}
	visit(specs, root, nil)
	return rows, limited
}
