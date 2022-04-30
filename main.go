package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/benthosdev/benthos/v4/public/bloblang"
	concourse "github.com/cludden/concourse-go-sdk"
	"github.com/hashicorp/go-multierror"
)

func main() {
	concourse.Main(&Resource{})
}

// =============================================================================

type (
	// GetParams describes the available parameters for a get step
	GetParams struct {
		Files map[string]string `json:"files"`
	}

	// PutParams describes the available parameters for a put step
	PutParams struct {
		Mapping string `json:"mapping"`
	}

	// Source describes resource configuration
	Source struct{}

	// Version holds arbitrary key value data that can be passed across
	// jobs within a pipeline
	Version struct {
		Data map[string]interface{}
	}
)

// Validate put parameters
func (p *PutParams) Validate(context.Context) (err error) {
	if p == nil {
		return fmt.Errorf("'params' are required")
	}
	if len(p.Mapping) == 0 {
		return fmt.Errorf("'mapping' is required")
	}
	return nil
}

func (s *Source) buildDocument() map[string]interface{} {
	return map[string]interface{}{
		"build_id":            os.Getenv("BUILD_ID"),
		"build_name":          os.Getenv("BUILD_NAME"),
		"build_job":           os.Getenv("BUILD_JOB_NAME"),
		"build_pipeline":      os.Getenv("BUILD_PIPELINE_NAME"),
		"build_instance_vars": os.Getenv("BUILD_PIPELINE_INSTANCE_VARS"),
		"build_team":          os.Getenv("BUILD_TEAM_NAME"),
		"build_created_by":    os.Getenv("BUILD_CREATED_BY"),
		"build_url":           fmt.Sprintf("%s/builds/%s", os.Getenv("ATC_EXTERNAL_URL"), os.Getenv("BUILD_ID")),
	}
}

func (v *Version) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.Data)
}

func (v *Version) UnmarshalJSON(b []byte) error {
	v.Data = make(map[string]interface{})
	return json.Unmarshal(b, &v.Data)
}

// =============================================================================

// Resource implements a keyval concourse resource
type Resource struct{}

// Check is a required resource method, but is a no-op for this resource
func (r *Resource) Check(ctx context.Context, s *Source, v *Version) (versions []Version, err error) {
	if v != nil {
		versions = append(versions, *v)
	}
	return
}

// In writes an incoming version the filesystem, allowing downstream steams to utilize
// arbitary data from an earlier put step
func (r *Resource) In(ctx context.Context, s *Source, v *Version, dir string, p *GetParams) (*Version, []concourse.Metadata, error) {
	version, err := os.Create(path.Join(dir, "version.json"))
	if err != nil {
		return nil, nil, fmt.Errorf("error creating version.json: %v", err)
	}

	enc := json.NewEncoder(version)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v.Data); err != nil {
		return nil, nil, fmt.Errorf("error writing version.json: %v", err)
	}

	if p != nil && len(p.Files) > 0 {
		buildDoc := s.buildDocument()
		for k, v := range v.Data {
			buildDoc[k] = v
		}
		for f, m := range p.Files {
			e, err := bloblang.Parse(m)
			if err != nil {
				return nil, nil, fmt.Errorf("error parsing '%s' file mapping: %v", f, err)
			}

			raw, err := e.Query(buildDoc)
			if err != nil {
				return nil, nil, fmt.Errorf("error executing '%s' file mapping: %v", f, err)
			}

			var b []byte
			switch v := raw.(type) {
			case string:
				b = []byte(v)
			case []byte:
				b = v
			default:
				return nil, nil, fmt.Errorf("invalid type returned by '%s' file mapping: %v", f, v)
			}

			if err := ioutil.WriteFile(path.Join(dir, f), b, 0777); err != nil {
				return nil, nil, fmt.Errorf("error writing '%s' file: %v", f, err)
			}
		}
	}

	return v, nil, nil
}

// Out generates a new version that contains arbitray key value pairs, where both keys
// and values are string data
func (r *Resource) Out(ctx context.Context, s *Source, dir string, p *PutParams) (*Version, []concourse.Metadata, error) {
	e, err := bloblang.Parse(p.Mapping)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing 'mapping': %v", err)
	}

	raw, err := e.Query(s.buildDocument())
	if err != nil {
		return nil, nil, fmt.Errorf("error executing version mapping: %v", err)
	}

	data, ok := raw.(map[string]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("version mapping returned invalid result, expected map, got: %T", raw)
	}

	errs := multierror.Append(nil)
	for k, v := range data {
		if _, ok := v.(string); !ok {
			err = multierror.Append(err, fmt.Errorf("invalid version key '%s', expected string value, got: %T", k, v))
		}
	}
	if errs.Len() > 0 {
		return nil, nil, fmt.Errorf("version mapping returned invalid result: %s", errs.Error())
	}
	return &Version{Data: data}, nil, nil
}
